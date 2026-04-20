package risk

import (
	"encoding/json"
	"fmt"
	"strings"

	"architex/delta"
)

// Rule 1 — Public exposure introduced.
// Triggers when a node's "public" attribute changed from false to true.
func evaluatePublicExposure(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, cn := range d.ChangedNodes {
		attr, ok := cn.ChangedAttributes["public"]
		if !ok {
			continue
		}
		// Defensive type assertion: ChangedAttribute fields are `any`, so a
		// future serialization round-trip could produce non-bool values.
		before, beforeOK := attr.Before.(bool)
		after, afterOK := attr.After.(bool)
		if !beforeOK || !afterOK {
			continue
		}
		if !before && after {
			reasons = append(reasons, RiskReason{
				RuleID:     "public_exposure_introduced",
				Message:    fmt.Sprintf("Resource %s became publicly accessible.", cn.ID),
				Impact:     "exposure",
				Weight:     4.0,
				ResourceID: cn.ID,
			})
		}
	}
	return reasons
}

// Rule 2 — New data resource.
// Triggers for each added node with abstract type "data".
func evaluateNewData(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.Type == "data" {
			reasons = append(reasons, RiskReason{
				RuleID:     "new_data_resource",
				Message:    fmt.Sprintf("New data resource %s introduced.", n.ID),
				Impact:     "data",
				Weight:     2.5,
				ResourceID: n.ID,
			})
		}
	}
	return reasons
}

// Rule 3 — New entry point.
// Triggers for each added node with abstract type "entry_point".
func evaluateNewEntryPoint(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.Type == "entry_point" {
			reasons = append(reasons, RiskReason{
				RuleID:     "new_entry_point",
				Message:    fmt.Sprintf("New public entry point %s introduced.", n.ID),
				Impact:     "exposure",
				Weight:     3.0,
				ResourceID: n.ID,
			})
		}
	}
	return reasons
}

// Rule 4 — Potential data exposure.
// Triggers when public exposure was introduced (Rule 1) AND either:
//   - a data node was added, OR
//   - a changed node is security-related (access_control or data abstract type)
func evaluateDataExposure(d delta.Delta, publicExposureTriggered bool) []RiskReason {
	if !publicExposureTriggered {
		return nil
	}

	dataNodeAdded := false
	for _, n := range d.AddedNodes {
		if n.Type == "data" {
			dataNodeAdded = true
			break
		}
	}

	securityRelatedChange := false
	for _, cn := range d.ChangedNodes {
		if cn.Type == "access_control" || cn.Type == "data" {
			securityRelatedChange = true
			break
		}
	}

	if !dataNodeAdded && !securityRelatedChange {
		return nil
	}

	return []RiskReason{{
		RuleID:  "potential_data_exposure",
		Message: "Public exposure introduced in presence of data resources or security-related changes. Review potential data exposure risk.",
		Impact:  "data_exposure",
		Weight:  2.0,
	}}
}

// Rule 5 — Resource removal.
// 0.5 per removed node, capped at 2 reasons (total weight 1.0). The cap
// prevents large teardowns from dominating the score.
const removalMaxReasons = 2

func evaluateRemoval(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.RemovedNodes {
		if len(reasons) >= removalMaxReasons {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "resource_removed",
			Message:    fmt.Sprintf("Resource %s was removed.", n.ID),
			Impact:     "change",
			Weight:     0.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// ---------------------------------------------------------------------------
// Phase 6 (v1.1) — AWS Top 10 risk rules
//
// Each Phase 6 rule keys off a small, deterministic delta-level signal so
// the engine stays free of cross-resource graph traversal. The trade-off
// is documented in CHANGELOG.md: per-resource signals are slightly noisier
// than full graph reasoning but are far easier to audit and override.
// ---------------------------------------------------------------------------

// phase6CapPerRule bounds how many reasons any one Phase 6 rule may emit, so
// a sweeping refactor (e.g. removing public_access_block on 5 buckets in one
// PR) cannot single-handedly saturate the 10.0 score cap. A reviewer who
// sees 2 instances of the same finding will already understand the pattern.
const phase6CapPerRule = 2

// Rule 6 — S3 bucket public exposure (Phase 6, refined Phase 7).
//
// Per-resource signal model:
//   - REMOVING an aws_s3_bucket_public_access_block weakens the bucket's
//     deny-by-default posture and is the single most common cause of S3
//     leaks observed in postmortems.
//   - ADDING an aws_s3_bucket_policy may grant public principals; without
//     full IAM evaluation we conservatively flag the addition for review.
//
// We do not infer which bucket is affected by walking edges — the message
// names the access-control resource itself, which the reviewer can map to
// its bucket trivially in the diagram.
//
// Phase 7 (v1.2 PR2): when the parser resolved the bucket policy JSON
// literal (because `policy = jsonencode({...})` is evaluable without an
// eval context), this rule now inspects `Statement[].Effect`. If EVERY
// statement is "Deny" the policy cannot grant public access and the
// finding is suppressed. This eliminates the documented v1.1 false
// positive on strict-deny policies. Variable-driven policies still land
// here as nil and continue to fire conservatively (no guessing).
func evaluateS3BucketPublicExposure(d delta.Delta) []RiskReason {
	var reasons []RiskReason

	emit := func(ruleMessage, sourceID string) {
		if len(reasons) >= phase6CapPerRule {
			return
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "s3_bucket_public_exposure",
			Message:    fmt.Sprintf(ruleMessage, sourceID),
			Impact:     "exposure",
			Weight:     4.0,
			ResourceID: sourceID,
		})
	}

	for _, n := range d.RemovedNodes {
		if n.ProviderType == "aws_s3_bucket_public_access_block" {
			emit("S3 public access block %s was removed; bucket may become publicly accessible.", n.ID)
		}
	}
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_s3_bucket_policy" {
			continue
		}
		if isDenyOnlyBucketPolicy(n.Attributes) {
			// Phase 7 PR2: deny-only policies cannot introduce public
			// exposure. Skip silently -- the policy is still visible in
			// the diagram and audit bundle.
			continue
		}
		emit("S3 bucket policy %s was added; review for public principal grants.", n.ID)
	}

	return reasons
}

// isDenyOnlyBucketPolicy returns true iff the resource's attributes carry a
// resolved `policy` JSON string AND that policy contains a non-empty
// Statement list whose every entry has Effect == "Deny" (case-insensitive).
//
// Anything that fails to parse, lacks a `policy` literal, has zero
// statements, or contains any non-Deny effect returns false -- which makes
// the rule fire conservatively. This preserves design decision 14
// ("never guess at unresolved expressions").
func isDenyOnlyBucketPolicy(attrs map[string]any) bool {
	raw, ok := attrs["policy"].(string)
	if !ok || raw == "" {
		return false
	}
	var doc struct {
		Statement []struct {
			Effect string `json:"Effect"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return false
	}
	if len(doc.Statement) == 0 {
		return false
	}
	for _, st := range doc.Statement {
		if !strings.EqualFold(st.Effect, "Deny") {
			return false
		}
	}
	return true
}

// adminPolicyARNSuffixes lists the AWS-managed policy ARNs whose attachment
// to an IAM role represents an immediate, high-blast-radius privilege grant.
// We match on suffix because the literal string "arn:aws:iam::aws:policy/X"
// is the canonical form a Terraform author writes; variable-driven ARNs are
// captured by the parser as nil and intentionally do not match (we do not
// guess at unresolved expressions).
var adminPolicyARNSuffixes = []string{
	":policy/AdministratorAccess",
	":policy/IAMFullAccess",
}

// Rule 7 — IAM admin/dangerous policy attached (Phase 6).
//
// Triggers when an aws_iam_role_policy_attachment is ADDED whose policy_arn
// literal ends in a known wildcard-admin AWS managed policy. Weight 3.5
// reflects the privilege-escalation impact: an attacker who later compromises
// any principal assuming this role has root-equivalent access.
func evaluateIAMAdminAttached(d delta.Delta) []RiskReason {
	var reasons []RiskReason

	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_iam_role_policy_attachment" {
			continue
		}
		raw, ok := n.Attributes["policy_arn"]
		if !ok {
			continue
		}
		arn, ok := raw.(string)
		if !ok || arn == "" {
			continue
		}
		matchedSuffix := ""
		for _, suffix := range adminPolicyARNSuffixes {
			if strings.HasSuffix(arn, suffix) {
				matchedSuffix = strings.TrimPrefix(suffix, ":policy/")
				break
			}
		}
		if matchedSuffix == "" {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "iam_admin_policy_attached",
			Message:    fmt.Sprintf("IAM attachment %s grants %s; review for privilege escalation.", n.ID, matchedSuffix),
			Impact:     "identity",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}

	return reasons
}

// Rule 8 — Lambda public function URL introduced (Phase 6).
//
// Triggers for each ADDED aws_lambda_function_url. This rule layers on top
// of the existing new_entry_point rule (3.0) — the two together produce a
// distinctly higher signal than a generic new entry_point because Lambda
// URLs bypass API Gateway, WAF, and most observability surface by default,
// and frequently ship with authorization_type = "NONE".
func evaluateLambdaPublicURL(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_lambda_function_url" {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "lambda_public_url_introduced",
			Message:    fmt.Sprintf("Public Lambda function URL %s was introduced; verify auth type and WAF coverage.", n.ID),
			Impact:     "exposure",
			Weight:     3.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}
