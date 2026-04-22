package exposure

import (
	"encoding/json"
	"fmt"
	"strings"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// S3BucketPublic is the Phase 6 (refined Phase 7) "s3_bucket_public_exposure"
// rule.
//
// Per-resource signal model:
//
//   - REMOVING an aws_s3_bucket_public_access_block weakens the bucket's
//     deny-by-default posture and is the single most common cause of S3
//     leaks observed in postmortems.
//   - ADDING an aws_s3_bucket_policy may grant public principals; without
//     full IAM evaluation we conservatively flag the addition for review.
//
// We do not infer which bucket is affected by walking edges -- the
// message names the access-control resource itself, which the reviewer
// can map to its bucket trivially in the diagram.
//
// Phase 7 (v1.2 PR2): when the parser resolved the bucket policy JSON
// literal (because `policy = jsonencode({...})` is evaluable without an
// eval context), this rule inspects `Statement[].Effect`. If EVERY
// statement is "Deny" the policy cannot grant public access and the
// finding is suppressed. This eliminates the documented v1.1 false
// positive on strict-deny policies. Variable-driven policies still land
// here as nil and continue to fire conservatively (no guessing).
var S3BucketPublic api.Rule = s3BucketPublicRule{}

type s3BucketPublicRule struct{}

func (s3BucketPublicRule) ID() string { return "s3_bucket_public_exposure" }

func (s3BucketPublicRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason

	emit := func(ruleMessage, sourceID string) {
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			return
		}
		reasons = append(reasons, api.RiskReason{
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
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		if isDenyOnlyBucketPolicy(n.Attributes) {
			// Deny-only policies cannot introduce public exposure.
			// Skip silently -- the policy is still visible in the
			// diagram and audit bundle.
			continue
		}
		emit("S3 bucket policy %s was added; review for public principal grants.", n.ID)
	}

	return reasons
}

func (s3BucketPublicRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Re-verify the bucket exposure on %s -- removal of a public-access block or addition of a bucket policy can grant public read; confirm Statement[].Principal is scoped.",
		reason.ResourceID,
	)
}

// isDenyOnlyBucketPolicy returns true iff the resource's attributes
// carry a resolved `policy` JSON string AND that policy contains a
// non-empty Statement list whose every entry has Effect == "Deny"
// (case-insensitive).
//
// Anything that fails to parse, lacks a `policy` literal, has zero
// statements, or contains any non-Deny effect returns false -- which
// makes the rule fire conservatively. This preserves design decision 14
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
