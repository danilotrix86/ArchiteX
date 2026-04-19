// Package risk evaluates architectural risk from a delta between two graphs.
// It runs deterministic, rule-based checks and produces a scored, explainable
// result with no probabilistic or ML-based logic.
package risk

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"architex/delta"
)

// RiskResult is the output of a risk evaluation against a Delta.
type RiskResult struct {
	Score    float64      `json:"score"`
	Severity string       `json:"severity"` // low, medium, high
	Status   string       `json:"status"`   // pass, warn, fail
	Reasons  []RiskReason `json:"reasons"`
}

// RiskReason describes a single triggered rule and its contribution to the score.
type RiskReason struct {
	RuleID  string  `json:"rule_id"`
	Message string  `json:"message"`
	Impact  string  `json:"impact"`
	Weight  float64 `json:"weight"`
}

// Evaluate runs all risk rules against a Delta and returns a deterministic RiskResult.
func Evaluate(d delta.Delta) RiskResult {
	var reasons []RiskReason

	publicReasons := evaluatePublicExposure(d)
	reasons = append(reasons, publicReasons...)
	reasons = append(reasons, evaluateNewData(d)...)
	reasons = append(reasons, evaluateNewEntryPoint(d)...)
	reasons = append(reasons, evaluateDataExposure(d, len(publicReasons) > 0)...)
	reasons = append(reasons, evaluateRemoval(d)...)

	// Phase 6 (v1.1) — AWS Top 10 rules. Order does not affect scoring; the
	// final reason list is sorted by weight desc, rule_id asc by Evaluate.
	reasons = append(reasons, evaluateS3BucketPublicExposure(d)...)
	reasons = append(reasons, evaluateIAMAdminAttached(d)...)
	reasons = append(reasons, evaluateLambdaPublicURL(d)...)

	score := 0.0
	for _, r := range reasons {
		score += r.Weight
	}
	score = math.Min(score, 10.0)
	score = math.Round(score*10) / 10

	severity := severityFromScore(score)
	status := statusFromSeverity(severity)

	slices.SortFunc(reasons, func(a, b RiskReason) int {
		if a.Weight != b.Weight {
			if a.Weight > b.Weight {
				return -1
			}
			return 1
		}
		return strings.Compare(a.RuleID, b.RuleID)
	})

	if reasons == nil {
		reasons = []RiskReason{}
	}

	return RiskResult{
		Score:    score,
		Severity: severity,
		Status:   status,
		Reasons:  reasons,
	}
}

func severityFromScore(score float64) string {
	switch {
	case score >= 7.0:
		return "high"
	case score >= 3.0:
		return "medium"
	default:
		return "low"
	}
}

func statusFromSeverity(severity string) string {
	switch severity {
	case "high":
		return "fail"
	case "medium":
		return "warn"
	default:
		return "pass"
	}
}

// MarshalJSON formats Score with 1 decimal place (9.0 not 9).
func (r RiskResult) MarshalJSON() ([]byte, error) {
	type Alias RiskResult
	return json.Marshal(&struct {
		Score json.RawMessage `json:"score"`
		Alias
	}{
		Score: json.RawMessage(fmt.Sprintf("%.1f", r.Score)),
		Alias: Alias(r),
	})
}

// MarshalJSON formats Weight with 1 decimal place (4.0 not 4).
func (r RiskReason) MarshalJSON() ([]byte, error) {
	type Alias RiskReason
	return json.Marshal(&struct {
		Weight json.RawMessage `json:"weight"`
		Alias
	}{
		Weight: json.RawMessage(fmt.Sprintf("%.1f", r.Weight)),
		Alias:  Alias(r),
	})
}
