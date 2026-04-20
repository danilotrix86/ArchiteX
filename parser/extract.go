package parser

import (
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"architex/models"
)

// minimalEvalContext registers the small set of HCL stdlib functions whose
// inputs are commonly literal strings/objects/lists in the wild. Keeping the
// set tight is intentional: every function we register here is a function
// whose evaluation contract we accept (no side effects, no IO, deterministic).
//
// Today only `jsonencode` is registered, so policies written as
// `policy = jsonencode({Statement = [...]})` resolve to a literal JSON
// string the S3 rule can inspect for `Effect = "Deny"` (Phase 7 PR2).
//
// Variables are NEVER added to this context. An expression that references
// var.*, local.*, or another resource still evaluates to an error and the
// attribute lands as nil -- preserving design decision 14: the engine
// never guesses at unresolved expressions.
var minimalEvalContext = &hcl.EvalContext{
	Functions: map[string]function.Function{
		"jsonencode": stdlib.JSONEncodeFunc,
	},
}

// extractAttributes tries to evaluate each attribute to a literal value.
// Expressions that depend on variables will fail -- that's expected and fine.
// Also walks nested blocks (ingress, egress, etc.) to surface their attributes.
//
// Phase 7: a non-nil ctx unlocks two extra resolutions:
//   - `policy_arn` on aws_iam_role_policy_attachment can be resolved when it
//     points at a data block we pre-scanned (ctx.dataPolicyARNs).
//   - jsonencode(literal) is evaluable via minimalEvalContext.
func extractAttributes(body *hclsyntax.Body, ctx *parseContext) map[string]any {
	attrs := make(map[string]any)

	for name, attr := range body.Attributes {
		val, diags := attr.Expr.Value(minimalEvalContext)
		if diags.HasErrors() {
			// Phase 7: targeted data-source resolution before giving up.
			// Only the explicit allowlist in resolveDataPolicyARN matches.
			if name == "policy_arn" {
				if resolved := resolveDataPolicyARN(attr.Expr, ctx); resolved != "" {
					attrs[name] = resolved
					continue
				}
			}
			attrs[name] = nil
			continue
		}
		attrs[name] = ctyToGo(val)
	}

	// Walk nested blocks (e.g. ingress/egress on security groups).
	// Merge their attributes into the parent using the block type as prefix context.
	// For cidr_blocks specifically, promote to top level so derived-attribute logic finds it.
	for _, nested := range body.Blocks {
		for name, attr := range nested.Body.Attributes {
			key := name
			if name != "cidr_blocks" {
				key = nested.Type + "." + name
			}

			val, diags := attr.Expr.Value(minimalEvalContext)
			if diags.HasErrors() {
				// Mirror the top-level path: record the key with nil so
				// downstream consumers know the attribute existed but was
				// unresolvable (e.g. depended on a variable).
				if _, ok := attrs[key]; !ok {
					attrs[key] = nil
				}
				continue
			}
			goVal := ctyToGo(val)

			if name == "cidr_blocks" {
				if existing, ok := attrs["cidr_blocks"]; ok && existing != nil {
					attrs["cidr_blocks"] = mergeSlices(existing, goVal)
				} else {
					attrs["cidr_blocks"] = goVal
				}
			} else {
				attrs[key] = goVal
			}
		}
	}

	return attrs
}

func mergeSlices(a, b any) []any {
	var result []any
	if sa, ok := a.([]any); ok {
		result = append(result, sa...)
	}
	if sb, ok := b.([]any); ok {
		result = append(result, sb...)
	}
	return result
}

// extractReferences walks all attribute expressions (including nested blocks)
// and collects references to other supported resources.
// Filters out var.*, local.*, data.*, module.*.
func extractReferences(body *hclsyntax.Body) []models.Reference {
	var refs []models.Reference
	seen := make(map[string]bool)

	collectFromAttrs := func(attrs hclsyntax.Attributes, prefix string) {
		for name, attr := range attrs {
			attrName := prefix + name
			for _, traversal := range attr.Expr.Variables() {
				targetID := traversalToResourceID(traversal)
				if targetID == "" {
					continue
				}
				key := attrName + "->" + targetID
				if seen[key] {
					continue
				}
				seen[key] = true
				refs = append(refs, models.Reference{
					SourceAttr: attrName,
					TargetID:   targetID,
				})
			}
		}
	}

	collectFromAttrs(body.Attributes, "")

	for _, nested := range body.Blocks {
		collectFromAttrs(nested.Body.Attributes, nested.Type+".")
	}

	return refs
}

// traversalToResourceID converts a traversal like [aws_security_group, web, id]
// into "aws_security_group.web" -- but only if the first segment is a supported
// resource type. Returns "" for var.*, local.*, data.*, module.*, etc.
func traversalToResourceID(traversal hcl.Traversal) string {
	if len(traversal) < 2 {
		return ""
	}

	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return ""
	}

	if !models.SupportedResources[root.Name] {
		return ""
	}

	second, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		return ""
	}

	return root.Name + "." + second.Name
}

// ctyToGo converts a cty.Value to a native Go type for JSON serialization.
func ctyToGo(val cty.Value) any {
	if !val.IsKnown() || val.IsNull() {
		return nil
	}

	ty := val.Type()
	switch {
	case ty == cty.String:
		return val.AsString()
	case ty == cty.Number:
		bf := val.AsBigFloat()
		f, _ := bf.Float64()
		return f
	case ty == cty.Bool:
		return val.True()
	case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
		var items []any
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			items = append(items, ctyToGo(v))
		}
		return items
	case ty.IsMapType() || ty.IsObjectType():
		m := make(map[string]any)
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			m[k.AsString()] = ctyToGo(v)
		}
		return m
	default:
		log.Printf("unhandled cty type: %s", ty.FriendlyName())
		return nil
	}
}
