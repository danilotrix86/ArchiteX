package risk

import (
	"encoding/json"
	"fmt"
	"strings"

	"architex/delta"
)

// ---------------------------------------------------------------------------
// Phase 7 PR4 (v1.2) — Coverage tranche 2 risk rules.
//
// Same per-resource signal philosophy as the Phase 6 rules: each rule reads
// a small, deterministic property of an added node. No graph traversal, no
// guessing at unresolved expressions. Each rule is capped at
// phase6CapPerRule (2) reasons per evaluation so a sweeping refactor cannot
// single-handedly saturate the 10.0 score cap.
// ---------------------------------------------------------------------------

// Rule 9 — CloudFront distribution without WAF.
//
// Triggers for each ADDED aws_cloudfront_distribution that does NOT have a
// literal `web_acl_id` attribute. CF distros are internet-facing edge
// caches; without a WAF they expose the origin to every L7 attack pattern
// AWS WAF would otherwise mitigate.
//
// Variable-driven `web_acl_id = var.waf_id` lands here as missing (the
// graph layer only promotes literals) and the rule fires conservatively.
// This is consistent with master.md design decision 14 ("never guess at
// unresolved expressions"). A reviewer who sees this finding on a
// var-driven attachment can suppress it in `.architex.yml` (Phase 7 PR3).
//
// Weight 2.5 -- below the existing entry_point rule (3.0) and the AWS
// Top-10 group (3.0-4.0). It is signal, not blocker, on its own; it
// stacks with new_entry_point so a brand-new public CF distro without
// WAF lands at 5.5 (medium).
func evaluateCloudFrontNoWAF(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_cloudfront_distribution" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		if v, ok := n.Attributes["web_acl_id"]; ok {
			if s, ok := v.(string); ok && s != "" {
				continue
			}
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "cloudfront_no_waf",
			Message:    fmt.Sprintf("CloudFront distribution %s was introduced without a WAF (web_acl_id); add AWS WAF to mitigate L7 attacks at the edge.", n.ID),
			Impact:     "exposure",
			Weight:     2.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 10 — EBS volume unencrypted.
//
// Triggers for each ADDED aws_ebs_volume whose `encrypted` attribute is
// the LITERAL boolean false. A missing attribute is intentionally NOT a
// match: many providers default to encryption-by-default at the account
// level, and the parser cannot read account state. This means an explicit
// `encrypted = false` (the only way to OPT OUT in Terraform) is the only
// thing that fires. Variable-driven `encrypted = var.encrypted` lands as
// missing and is silent.
//
// Weight 3.0 -- on par with new_entry_point. Unencrypted volumes at rest
// are a regulatory hard-no in many compliance regimes (PCI, HIPAA, SOC2),
// and a single misconfigured EBS attached to a legacy EC2 silently
// creates audit findings months later.
func evaluateEBSUnencrypted(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_ebs_volume" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		v, ok := n.Attributes["encrypted"]
		if !ok {
			continue
		}
		b, ok := v.(bool)
		if !ok || b {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "ebs_volume_unencrypted",
			Message:    fmt.Sprintf("EBS volume %s was introduced with encrypted=false; data at rest will be unencrypted.", n.ID),
			Impact:     "data",
			Weight:     3.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// Rule 11 — Public messaging topic/queue policy.
//
// Triggers when an aws_sns_topic_policy OR aws_sqs_queue_policy is ADDED
// whose resolved `policy` JSON contains a Statement with Effect=Allow AND
// a Principal that is the literal "*" (or {"AWS": "*"} or a list
// containing "*"). The graph layer passes the JSON literal through
// (Phase 7 PR2 mechanism, extended in PR4 to messaging policies).
//
// Variable-driven policies land as nil and the rule does NOT fire (no
// guessing). Unresolvable JSON strings (e.g. heredoc with interpolation)
// also don't fire because we never see a parseable string.
//
// Weight 3.5 -- equal to iam_admin_policy_attached. A public SNS or SQS
// is a notorious data-leak primitive: anyone with the ARN can subscribe
// to the topic / poll the queue.
func evaluateMessagingTopicPublic(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_sns_topic_policy" && n.ProviderType != "aws_sqs_queue_policy" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		raw, ok := n.Attributes["policy"].(string)
		if !ok || raw == "" {
			continue
		}
		if !policyAllowsAnyPrincipal(raw) {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		kind := "SNS topic"
		if n.ProviderType == "aws_sqs_queue_policy" {
			kind = "SQS queue"
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "messaging_topic_public",
			Message:    fmt.Sprintf("%s policy %s grants Allow to Principal=\"*\"; the topic/queue is publicly accessible.", kind, n.ID),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

// policyAllowsAnyPrincipal parses an AWS resource-policy JSON string and
// returns true if ANY statement has Effect=Allow AND a Principal that
// matches "*" (the wildcard literal, an {"AWS":"*"} object, or a list
// containing one of those forms). Anything that fails to parse returns
// false -- we never guess at strings we can't read.
func policyAllowsAnyPrincipal(raw string) bool {
	var doc struct {
		Statement []struct {
			Effect    string          `json:"Effect"`
			Principal json.RawMessage `json:"Principal"`
		} `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return false
	}
	for _, st := range doc.Statement {
		if !strings.EqualFold(st.Effect, "Allow") {
			continue
		}
		if principalIsWildcard(st.Principal) {
			return true
		}
	}
	return false
}

// principalIsWildcard tests whether a Principal field encodes the public
// wildcard. Accepted forms:
//
//	"Principal": "*"
//	"Principal": ["*"]
//	"Principal": {"AWS": "*"}
//	"Principal": {"AWS": ["*"]}
//	"Principal": {"AWS": ["arn", "*", "arn"]}     (any element matches)
func principalIsWildcard(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString == "*"
	}

	var asList []string
	if err := json.Unmarshal(raw, &asList); err == nil {
		for _, p := range asList {
			if p == "*" {
				return true
			}
		}
		return false
	}

	var asObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObj); err != nil {
		return false
	}
	for _, v := range asObj {
		var s string
		if err := json.Unmarshal(v, &s); err == nil && s == "*" {
			return true
		}
		var l []string
		if err := json.Unmarshal(v, &l); err == nil {
			for _, p := range l {
				if p == "*" {
					return true
				}
			}
		}
	}
	return false
}

// Rule 12 — NACL rule allows world-inbound.
//
// Triggers when an aws_network_acl_rule is ADDED with the trio
// (cidr_block = 0.0.0.0/0, egress = false, rule_action = "allow"). NACLs
// are ordered, so even if a later rule denies traffic, an Allow with
// 0.0.0.0/0 sitting at a low rule_number will be evaluated first. Any
// ONE such rule effectively opens the subnet. Reviewers should justify
// it explicitly (e.g. "yes, this NACL fronts a public ALB").
//
// Weight 3.5 -- equal to iam_admin_policy_attached. NACLs sit a layer
// below SG and are easy to misconfigure (rule_number ordering); a
// permissive Allow at a low number quietly defeats the SG above it.
//
// Variable-driven attributes land as missing and the rule does not fire.
// Egress rules (`egress = true`) are treated as an outbound concern and
// are out of scope -- they do not match.
func evaluateNACLAllowAllIngress(d delta.Delta) []RiskReason {
	var reasons []RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_network_acl_rule" {
			continue
		}
		if isConditionalNode(n.Attributes) {
			continue
		}
		cidr, _ := n.Attributes["cidr_block"].(string)
		if cidr != "0.0.0.0/0" {
			continue
		}
		action, _ := n.Attributes["rule_action"].(string)
		if !strings.EqualFold(action, "allow") {
			continue
		}
		if egress, ok := n.Attributes["egress"].(bool); ok && egress {
			continue
		}
		if len(reasons) >= phase6CapPerRule {
			break
		}
		reasons = append(reasons, RiskReason{
			RuleID:     "nacl_allow_all_ingress",
			Message:    fmt.Sprintf("Network ACL rule %s allows inbound traffic from 0.0.0.0/0; the subnet is open at the network layer.", n.ID),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}
