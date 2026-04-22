package exposure

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// LambdaPublicURL is the Phase 6 "lambda_public_url_introduced" rule.
//
// Triggers for each ADDED aws_lambda_function_url. This rule layers on
// top of the existing new_entry_point rule (3.0) -- the two together
// produce a distinctly higher signal than a generic new entry_point
// because Lambda URLs bypass API Gateway, WAF, and most observability
// surface by default, and frequently ship with authorization_type =
// "NONE".
var LambdaPublicURL api.Rule = lambdaPublicURLRule{}

type lambdaPublicURLRule struct{}

func (lambdaPublicURLRule) ID() string { return "lambda_public_url_introduced" }

func (lambdaPublicURLRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_lambda_function_url" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "lambda_public_url_introduced",
			Message:    fmt.Sprintf("Public Lambda function URL %s was introduced; verify auth type and WAF coverage.", n.ID),
			Impact:     "exposure",
			Weight:     3.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (lambdaPublicURLRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Inspect the Lambda function URL %s for authorization_type, IAM auth, and (ideally) WAF coverage; public Function URLs bypass API Gateway entirely.",
		reason.ResourceID,
	)
}
