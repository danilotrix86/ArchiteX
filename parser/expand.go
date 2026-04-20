package parser

import (
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"architex/models"
)

// ---------------------------------------------------------------------------
// Phase 7 (v1.2) -- Parser depth: count / for_each / dynamic expansion.
//
// Rationale: the v1.0/v1.1 parser warned-and-skipped any resource block that
// used count, for_each, or a dynamic nested block. Real-world Terraform uses
// these constructs heavily; skipping them produced silent under-coverage on
// most production repos.
//
// Expansion is intentionally conservative:
//   * We only expand when the construct's input is a deterministic literal
//     (a number, a literal collection, or `length([literal_list])`).
//   * We do NOT evaluate variable / data-source / cross-resource expressions.
//     Those continue to warn as `unsupported_construct` exactly as before so
//     the engine never guesses at unresolved expressions (master.md §3.1
//     "deterministic first" -- no probabilistic logic anywhere).
//   * Expanded resources keep the original Type/Name; only the ID gets an
//     instance suffix (`name[0]`, `name["key"]`). This keeps abstract type
//     mapping and edge inference unchanged.
// ---------------------------------------------------------------------------

// expandResource turns one HCL resource block into one OR MORE RawResources,
// expanding count / for_each into per-instance entries when the input is a
// literal that we can evaluate without an eval context.
//
// Returns nil resources + an unsupported_construct warning when the count/
// for_each input is unresolvable -- mirroring the v1.0/v1.1 behavior so
// downstream consumers see the same warning category and confidence
// deduction they did before.
//
// Phase 7: ctx carries directory-scoped data-block resolutions (e.g. for
// `data.aws_iam_policy.<name>.arn`); pass nil if none are available.
func expandResource(block *hclsyntax.Block, ctx *parseContext) ([]models.RawResource, []models.Warning) {
	if len(block.Labels) != 2 {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("resource block with unexpected label count: %v", block.Labels),
		}}
	}

	resType := block.Labels[0]
	resName := block.Labels[1]
	resID := resType + "." + resName

	if !models.SupportedResources[resType] {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedResource,
			Message:  fmt.Sprintf("unsupported resource type %q (%s)", resType, resID),
		}}
	}

	countAttr, hasCount := block.Body.Attributes["count"]
	forEachAttr, hasForEach := block.Body.Attributes["for_each"]

	if hasCount && hasForEach {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message: fmt.Sprintf("%s declares both count and for_each (invalid Terraform; skipping)",
				resID),
		}}
	}

	switch {
	case hasCount:
		return expandCount(block, resType, resName, resID, countAttr, ctx)
	case hasForEach:
		return expandForEach(block, resType, resName, resID, forEachAttr, ctx)
	default:
		res, warns := buildSingleResource(block, resType, resName, resID, ctx)
		if res == nil {
			return nil, warns
		}
		return []models.RawResource{*res}, warns
	}
}

// buildSingleResource is the v1.0/v1.1 path: one HCL block -> one resource.
// Materializes any literal `dynamic` nested blocks first so downstream
// extraction behaves as if the author had written the static form.
func buildSingleResource(block *hclsyntax.Block, resType, resName, resID string, ctx *parseContext) (*models.RawResource, []models.Warning) {
	body, dynWarnings := materializeDynamicBlocks(block.Body, resID)

	attrs := extractAttributes(body, ctx)
	refs := extractReferences(body)

	return &models.RawResource{
		Type:       resType,
		Name:       resName,
		ID:         resID,
		Attributes: attrs,
		References: refs,
	}, dynWarnings
}

// expandCount handles `count = <literal int>` and `count = length([...])`.
// Anything else falls back to the existing warn-and-skip behavior so we
// never guess at unresolvable expressions.
func expandCount(block *hclsyntax.Block, resType, resName, resID string, countAttr *hclsyntax.Attribute, ctx *parseContext) ([]models.RawResource, []models.Warning) {
	n, ok := evalCount(countAttr)
	if !ok {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("%s uses count with a non-literal expression (skipping)", resID),
		}}
	}
	if n <= 0 {
		// count = 0 is valid Terraform and means "no instances". We honor
		// it: produce no resources, no warning.
		return nil, nil
	}

	// Materialize dynamic blocks once; the materialized body is cheap to
	// re-extract per instance and keeps every instance identical (we do
	// not currently substitute count.index inside attribute literals).
	body, dynWarnings := materializeDynamicBlocks(block.Body, resID)

	out := make([]models.RawResource, 0, n)
	for i := 0; i < n; i++ {
		instanceID := fmt.Sprintf("%s[%d]", resID, i)
		out = append(out, models.RawResource{
			Type:       resType,
			Name:       resName,
			ID:         instanceID,
			Attributes: extractAttributes(body, ctx),
			References: extractReferences(body),
		})
	}
	return out, dynWarnings
}

// expandForEach handles `for_each = <literal map / list / set / tuple>`.
// Map / object keys are taken from the literal; list / set / tuple keys are
// the element values converted to strings (matching Terraform's behavior on
// `toset(["a", "b"])`).
func expandForEach(block *hclsyntax.Block, resType, resName, resID string, forEachAttr *hclsyntax.Attribute, ctx *parseContext) ([]models.RawResource, []models.Warning) {
	keys, ok := evalForEachKeys(forEachAttr)
	if !ok {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("%s uses for_each with a non-literal expression (skipping)", resID),
		}}
	}
	if len(keys) == 0 {
		return nil, nil
	}

	body, dynWarnings := materializeDynamicBlocks(block.Body, resID)

	// Sort keys so expansion is deterministic across runs.
	sort.Strings(keys)

	out := make([]models.RawResource, 0, len(keys))
	for _, k := range keys {
		instanceID := fmt.Sprintf("%s[%q]", resID, k)
		out = append(out, models.RawResource{
			Type:       resType,
			Name:       resName,
			ID:         instanceID,
			Attributes: extractAttributes(body, ctx),
			References: extractReferences(body),
		})
	}
	return out, dynWarnings
}

// evalCount tries to extract a non-negative integer from the count expression.
// Supports literal numbers and the canonical `length([literal_list])` form
// that Terraform tutorials use. Returns ok=false for anything we can't
// evaluate without an eval context.
func evalCount(attr *hclsyntax.Attribute) (int, bool) {
	val, diags := attr.Expr.Value(nil)
	if !diags.HasErrors() && val.IsKnown() && !val.IsNull() && val.Type() == cty.Number {
		bf := val.AsBigFloat()
		f, _ := bf.Float64()
		if f < 0 {
			return 0, false
		}
		return int(f), true
	}

	// Special-case `length([literal_list])` because Terraform users frequently
	// write `count = length(var.x)` (unresolvable -> skip) AND
	// `count = length(["a", "b"])` (a literal we want to honor).
	if call, ok := attr.Expr.(*hclsyntax.FunctionCallExpr); ok && call.Name == "length" && len(call.Args) == 1 {
		argVal, argDiags := call.Args[0].Value(nil)
		if !argDiags.HasErrors() && argVal.IsKnown() && !argVal.IsNull() {
			ty := argVal.Type()
			if ty.IsListType() || ty.IsTupleType() || ty.IsSetType() || ty.IsMapType() || ty.IsObjectType() {
				return argVal.LengthInt(), true
			}
		}
	}

	return 0, false
}

// evalForEachKeys extracts deterministic instance keys from a literal
// for_each input. Maps/objects contribute their keys directly; lists/sets/
// tuples contribute their element values (mirroring Terraform's `toset`
// semantics, which is the canonical idiom).
func evalForEachKeys(attr *hclsyntax.Attribute) ([]string, bool) {
	// Direct evaluation handles literal maps, objects, lists, tuples, and
	// the toset({...}) / toset([...]) wrappers because their argument is a
	// literal and the eval succeeds with no eval context.
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || !val.IsKnown() || val.IsNull() {
		// toset(literal) is normally evaluable above. As a defense, if a
		// user wrapped a literal in `toset(...)` and direct eval failed
		// for any reason, peel one function call layer.
		if call, ok := attr.Expr.(*hclsyntax.FunctionCallExpr); ok && (call.Name == "toset" || call.Name == "tomap") && len(call.Args) == 1 {
			innerVal, innerDiags := call.Args[0].Value(nil)
			if innerDiags.HasErrors() || !innerVal.IsKnown() || innerVal.IsNull() {
				return nil, false
			}
			val = innerVal
		} else {
			return nil, false
		}
	}

	ty := val.Type()
	switch {
	case ty.IsMapType() || ty.IsObjectType():
		var keys []string
		for it := val.ElementIterator(); it.Next(); {
			k, _ := it.Element()
			if k.Type() != cty.String {
				return nil, false
			}
			keys = append(keys, k.AsString())
		}
		return keys, true
	case ty.IsListType() || ty.IsSetType() || ty.IsTupleType():
		var keys []string
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			if v.Type() != cty.String {
				// Numeric / non-string element types do not produce safe
				// instance keys -- skip rather than coerce.
				return nil, false
			}
			keys = append(keys, v.AsString())
		}
		return keys, true
	default:
		return nil, false
	}
}

// materializeDynamicBlocks expands `dynamic "label" { for_each = <literal>;
// content { ... } }` into N concrete nested blocks, preserving the order the
// author wrote them. Dynamic blocks whose for_each is unresolvable produce a
// warning and are dropped (same as v1.0/v1.1, but per-block now -- the
// surrounding resource is kept).
//
// Returns a NEW *hclsyntax.Body whose Blocks slice is the flattened result.
// Attributes are not modified. The returned body is safe to feed into
// extractAttributes / extractReferences.
func materializeDynamicBlocks(body *hclsyntax.Body, resID string) (*hclsyntax.Body, []models.Warning) {
	hasDynamic := false
	for _, nested := range body.Blocks {
		if nested.Type == "dynamic" {
			hasDynamic = true
			break
		}
	}
	if !hasDynamic {
		return body, nil
	}

	var warnings []models.Warning
	out := *body
	out.Blocks = make([]*hclsyntax.Block, 0, len(body.Blocks))

	for _, nested := range body.Blocks {
		if nested.Type != "dynamic" {
			out.Blocks = append(out.Blocks, nested)
			continue
		}

		expanded, warns := expandDynamicBlock(nested, resID)
		warnings = append(warnings, warns...)
		out.Blocks = append(out.Blocks, expanded...)
	}

	return &out, warnings
}

// expandDynamicBlock turns ONE `dynamic "label" { ... }` into 0..N concrete
// `label { ... }` blocks. Returns an unsupported_construct warning when the
// for_each is unresolvable; the dynamic block is dropped in that case but
// the surrounding resource is preserved.
func expandDynamicBlock(dyn *hclsyntax.Block, resID string) ([]*hclsyntax.Block, []models.Warning) {
	if len(dyn.Labels) != 1 {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("%s has dynamic block with %d labels (expected 1, skipping)", resID, len(dyn.Labels)),
		}}
	}
	label := dyn.Labels[0]

	forEachAttr, ok := dyn.Body.Attributes["for_each"]
	if !ok {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("%s dynamic %q missing for_each (skipping)", resID, label),
		}}
	}

	keys, ok := evalForEachKeys(forEachAttr)
	if !ok {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("%s dynamic %q uses non-literal for_each (skipping)", resID, label),
		}}
	}

	var content *hclsyntax.Block
	for _, nested := range dyn.Body.Blocks {
		if nested.Type == "content" {
			content = nested
			break
		}
	}
	if content == nil {
		return nil, []models.Warning{{
			Category: models.WarnUnsupportedConstruct,
			Message:  fmt.Sprintf("%s dynamic %q missing content block (skipping)", resID, label),
		}}
	}

	out := make([]*hclsyntax.Block, 0, len(keys))
	for range keys {
		// Each instance reuses the SAME content body. We do not rewrite
		// `each.value` references inside; the literal-only attribute
		// extractor already records nil for unresolvable expressions, so
		// dynamic-driven attributes that reference each.value land in
		// downstream consumers as nil exactly as if they were variable-
		// driven. References inside the content block (e.g. to other
		// resources) ARE extracted -- and that is the high-value path
		// (security_group_rules referencing security groups, etc.).
		instance := &hclsyntax.Block{
			Type:        label,
			Labels:      nil,
			Body:        content.Body,
			TypeRange:   content.TypeRange,
			LabelRanges: nil,
			OpenBraceRange: content.OpenBraceRange,
			CloseBraceRange: content.CloseBraceRange,
		}
		out = append(out, instance)
	}
	return out, nil
}
