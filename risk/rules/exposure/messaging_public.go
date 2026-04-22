package exposure

import (
	"encoding/json"
	"fmt"
	"strings"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// MessagingTopicPublic is the Phase 7 PR4 "messaging_topic_public" rule.
//
// Triggers when an aws_sns_topic_policy OR aws_sqs_queue_policy is
// ADDED whose resolved `policy` JSON contains a Statement with
// Effect=Allow AND a Principal that is the literal "*" (or {"AWS": "*"}
// or a list containing "*"). The graph layer passes the JSON literal
// through (Phase 7 PR2 mechanism, extended in PR4 to messaging
// policies).
//
// Variable-driven policies land as nil and the rule does NOT fire (no
// guessing). Unresolvable JSON strings (e.g. heredoc with interpolation)
// also don't fire because we never see a parseable string.
//
// Weight 3.5 -- equal to iam_admin_policy_attached. A public SNS or SQS
// is a notorious data-leak primitive: anyone with the ARN can subscribe
// to the topic / poll the queue.
var MessagingTopicPublic api.Rule = messagingTopicPublicRule{}

type messagingTopicPublicRule struct{}

func (messagingTopicPublicRule) ID() string { return "messaging_topic_public" }

func (messagingTopicPublicRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "aws_sns_topic_policy" && n.ProviderType != "aws_sqs_queue_policy" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		raw, ok := n.Attributes["policy"].(string)
		if !ok || raw == "" {
			continue
		}
		if !policyAllowsAnyPrincipal(raw) {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		kind := "SNS topic"
		if n.ProviderType == "aws_sqs_queue_policy" {
			kind = "SQS queue"
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "messaging_topic_public",
			Message:    fmt.Sprintf("%s policy %s grants Allow to Principal=\"*\"; the topic/queue is publicly accessible.", kind, n.ID),
			Impact:     "exposure",
			Weight:     3.5,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (messagingTopicPublicRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Restrict the topic/queue policy %s -- a Principal=\"*\" Allow lets anyone with the ARN subscribe / poll.",
		reason.ResourceID,
	)
}

// policyAllowsAnyPrincipal parses an AWS resource-policy JSON string
// and returns true if ANY statement has Effect=Allow AND a Principal
// that matches "*" (the wildcard literal, an {"AWS":"*"} object, or a
// list containing one of those forms). Anything that fails to parse
// returns false -- we never guess at strings we can't read.
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

// principalIsWildcard tests whether a Principal field encodes the
// public wildcard. Accepted forms:
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
