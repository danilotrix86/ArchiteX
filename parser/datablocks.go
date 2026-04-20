package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// ---------------------------------------------------------------------------
// Phase 7 (v1.2) -- selective data-source resolution.
//
// The v1.0/v1.1 parser silently skipped every `data` block. That made the
// `iam_admin_policy_attached` rule blind to the canonical Terraform idiom:
//
//   data "aws_iam_policy" "admin" {
//     arn = "arn:aws:iam::aws:policy/AdministratorAccess"
//   }
//   resource "aws_iam_role_policy_attachment" "x" {
//     role       = aws_iam_role.app.name
//     policy_arn = data.aws_iam_policy.admin.arn
//   }
//
// We now resolve a narrow allowlist of data-source attributes when their
// declaration is a literal string. Everything else is still skipped (we
// do not guess at unresolved expressions -- design decision 14).
//
// Allowlist:
//   data "aws_iam_policy" "<name>" { arn = "<literal>" }
// produces dataPolicyARNs["data.aws_iam_policy.<name>"] = "<literal>"
//
// Adding a new data source here requires (a) extracting the attribute
// literal and (b) extending the substitution path in extract.go.
// ---------------------------------------------------------------------------

// parseContext carries directory-scoped knowledge that resource extraction
// needs, but that is not part of any single resource block. Today it only
// holds resolved data-policy ARNs; extending it is the right place to add
// future cross-block lookups.
type parseContext struct {
	// dataPolicyARNs maps a "data.aws_iam_policy.<name>" base traversal to
	// the literal ARN string captured from the corresponding data block.
	// Empty when the directory has no resolvable data blocks.
	dataPolicyARNs map[string]string
}

// newParseContext returns a context with no resolved data sources. It is
// safe to use as the default when scanning has not yet happened.
func newParseContext() *parseContext {
	return &parseContext{
		dataPolicyARNs: map[string]string{},
	}
}

// scanDataBlocks walks an hclsyntax body and extracts the allowlisted
// data-source literals. Called once per file before resource expansion so
// every resource in the file sees the full set, regardless of declaration
// order (Terraform itself is order-independent here).
func scanDataBlocks(body *hclsyntax.Body, ctx *parseContext) {
	for _, block := range body.Blocks {
		if block.Type != "data" || len(block.Labels) != 2 {
			continue
		}
		dataType := block.Labels[0]
		dataName := block.Labels[1]

		if dataType != "aws_iam_policy" {
			continue
		}

		arnAttr, ok := block.Body.Attributes["arn"]
		if !ok {
			continue
		}
		val, diags := arnAttr.Expr.Value(nil)
		if diags.HasErrors() || !val.IsKnown() || val.IsNull() || val.Type() != cty.String {
			continue
		}
		key := "data.aws_iam_policy." + dataName
		ctx.dataPolicyARNs[key] = val.AsString()
	}
}

// resolveDataPolicyARN inspects an attribute expression and returns the
// resolved ARN literal when the expression is exactly
// `data.aws_iam_policy.<name>.arn` AND that data block was captured in ctx.
// Returns "" when the expression is anything else.
//
// We deliberately match only the exact 4-segment traversal shape; complex
// expressions (concat, conditionals) are out of scope and continue to land
// as nil in the attribute map.
func resolveDataPolicyARN(expr hclsyntax.Expression, ctx *parseContext) string {
	if ctx == nil || len(ctx.dataPolicyARNs) == 0 {
		return ""
	}
	traversal, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		return ""
	}
	t := traversal.Traversal
	if len(t) != 4 {
		return ""
	}
	root, ok := t[0].(hcl.TraverseRoot)
	if !ok || root.Name != "data" {
		return ""
	}
	dataType, ok := t[1].(hcl.TraverseAttr)
	if !ok || dataType.Name != "aws_iam_policy" {
		return ""
	}
	dataName, ok := t[2].(hcl.TraverseAttr)
	if !ok {
		return ""
	}
	arnAttr, ok := t[3].(hcl.TraverseAttr)
	if !ok || arnAttr.Name != "arn" {
		return ""
	}
	return ctx.dataPolicyARNs["data.aws_iam_policy."+dataName.Name]
}
