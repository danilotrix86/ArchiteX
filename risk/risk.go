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
	"time"

	"architex/baseline"
	"architex/config"
	"architex/delta"
	"architex/risk/api"
	rulesbaseline "architex/risk/rules/baseline"
)

// RiskResult is the output of a risk evaluation against a Delta.
type RiskResult struct {
	Score    float64      `json:"score"`
	Severity string       `json:"severity"` // low, medium, high
	Status   string       `json:"status"`   // pass, warn, fail
	Reasons  []RiskReason `json:"reasons"`

	// Suppressed are findings that would have fired but were silenced by
	// the active config (.architex.yml or inline `# architex:ignore=...`).
	// They are excluded from Score / Reasons but rendered in the audit
	// bundle so suppressions stay auditable. Phase 7 (v1.2 PR3).
	Suppressed []SuppressedFinding `json:"suppressed,omitempty"`
}

// RiskReason describes a single triggered rule and its contribution to
// the score. Aliased from architex/risk/api so rule subpackages can
// construct values of this type without creating an import cycle through
// architex/risk. Existing call sites (interpreter, internal/cli, tests)
// continue to write `risk.RiskReason` and see no behavior change.
type RiskReason = api.RiskReason

// SuppressedFinding records a (rule, resource) pair that was matched and
// silenced by an active suppression. Carries enough metadata for the audit
// bundle and PR comment footer to be useful but never the underlying
// risk Message (which may name internal resources).
type SuppressedFinding struct {
	RuleID     string `json:"rule_id"`
	ResourceID string `json:"resource_id"`
	Reason     string `json:"reason"`
	Source     string `json:"source"`            // e.g. "config:.architex.yml" or "inline:main.tf:42"
	Expired    bool   `json:"expired,omitempty"` // true => the suppression has lapsed; reviewers should refresh or remove it
}

// Evaluate runs all risk rules against a Delta and returns a deterministic
// RiskResult using v1.0/v1.1 default thresholds and weights. This is the
// zero-config path; it stays bit-identical to v1.1 behavior.
func Evaluate(d delta.Delta) RiskResult {
	return EvaluateWithBaseline(d, nil, nil, time.Time{})
}

// EvaluateWith is the configurable variant of Evaluate. A nil cfg or zero
// `now` reproduces the default behavior. When cfg is set, rule weights are
// overridden, disabled rules are dropped, suppressions are applied to
// per-resource findings, and severity/status thresholds come from the
// config. `now` is used to evaluate suppression expiries (use time.Time{}
// to never expire -- useful in tests).
//
// Phase 7 PR5: this variant disables baseline anomaly rules. Use
// EvaluateWithBaseline to opt into them.
func EvaluateWith(d delta.Delta, cfg *config.Config, now time.Time) RiskResult {
	return EvaluateWithBaseline(d, cfg, nil, now)
}

// EvaluateWithBaseline is the full Phase 7 entry point. It runs the v1.0
// + v1.1 + Phase 7 PR4 rule set AND, when `base` is non-nil, the
// `first_time_*` baseline anomaly rules from PR5. A nil baseline is the
// well-defined "no baseline known" state and the anomaly rules are
// silently skipped -- repos that never run `architex baseline` keep
// bit-identical behavior to a PR3-config-only run.
//
// This is the canonical place where the rule order is fixed. The final
// reason list is sorted by weight desc + rule_id asc later, so the
// physical addition order here only matters for tie-breaking inside
// applyConfig (which preserves its input order for equal weights).
func EvaluateWithBaseline(d delta.Delta, cfg *config.Config, base *baseline.Baseline, now time.Time) RiskResult {
	var reasons []RiskReason

	// Migrated rules: every rule registered via architex/risk/api.Register()
	// runs here as a uniform loop. Today this covers the v1.0 rules
	// (PR2: public_exposure_introduced, new_data_resource, new_entry_point,
	// potential_data_exposure, resource_removed); PR3-PR4 will move the
	// remaining hand-called groups into the same loop. Order does not
	// affect output -- applyConfig preserves input order for equal
	// weights and the final list is sorted by (weight desc, ruleID asc).
	for _, rule := range registered() {
		reasons = append(reasons, rule.Evaluate(d)...)
	}

	// Phase 7 PR5 (v1.2) — Baseline anomaly rules. Called explicitly
	// (not via the registry) because they need *baseline.Baseline as a
	// second input. Skipped entirely when no baseline exists; this
	// preserves the v1.1 zero-config invariant.
	reasons = append(reasons, rulesbaseline.EvaluateFirstTimeResourceType(d, base)...)
	reasons = append(reasons, rulesbaseline.EvaluateFirstTimeAbstractType(d, base)...)
	reasons = append(reasons, rulesbaseline.EvaluateFirstTimeEdgePair(d, base)...)

	// Phase 7 (v1.2 PR3): apply config -- weight overrides, disabled
	// rules, and suppressions -- before scoring. When cfg is nil this is
	// a no-op and the result is bit-identical to v1.1.
	reasons, suppressed := applyConfig(reasons, cfg, now)

	score := 0.0
	for _, r := range reasons {
		score += r.Weight
	}
	score = math.Min(score, 10.0)
	score = math.Round(score*10) / 10

	severity := severityFromScoreCfg(score, cfg)
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
		Score:      score,
		Severity:   severity,
		Status:     status,
		Reasons:    reasons,
		Suppressed: suppressed,
	}
}

// applyConfig walks the candidate reasons and:
//   - drops reasons whose RuleID is `enabled: false` in the config.
//   - replaces each reason's Weight with the configured override (when set).
//   - matches per-resource (RuleID, ResourceID) pairs against suppressions
//     and moves matches to a separate Suppressed slice.
//
// Cross-resource reasons (ResourceID == "") are NEVER suppressed by
// (rule, resource) tuples -- that would silently disable a whole category
// of finding. Use `enabled: false` for that.
func applyConfig(reasons []RiskReason, cfg *config.Config, now time.Time) ([]RiskReason, []SuppressedFinding) {
	if cfg == nil {
		return reasons, nil
	}
	if now.IsZero() {
		// Treat the absence of a clock as "never expired" -- callers that
		// genuinely want expiry must pass time.Now().
		now = time.Time{}
	}

	out := reasons[:0]
	var suppressed []SuppressedFinding
	for _, r := range reasons {
		if !cfg.RuleEnabled(r.RuleID) {
			continue
		}
		r.Weight = cfg.RuleWeight(r.RuleID, r.Weight)

		if r.ResourceID != "" {
			if sup, expired, ok := cfg.MatchSuppression(r.RuleID, r.ResourceID, now); ok {
				suppressed = append(suppressed, SuppressedFinding{
					RuleID:     r.RuleID,
					ResourceID: r.ResourceID,
					Reason:     sup.Reason,
					Source:     sup.Source,
					Expired:    expired,
				})
				continue
			}
		}
		out = append(out, r)
	}

	slices.SortFunc(suppressed, func(a, b SuppressedFinding) int {
		if c := strings.Compare(a.RuleID, b.RuleID); c != 0 {
			return c
		}
		return strings.Compare(a.ResourceID, b.ResourceID)
	})
	return out, suppressed
}

func severityFromScore(score float64) string {
	return severityFromScoreCfg(score, nil)
}

func severityFromScoreCfg(score float64, cfg *config.Config) string {
	failT := config.DefaultThresholdFail
	warnT := config.DefaultThresholdWarn
	if cfg != nil {
		failT = cfg.FailThreshold()
		warnT = cfg.WarnThreshold()
	}
	switch {
	case score >= failT:
		return "high"
	case score >= warnT:
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

// RiskReason's MarshalJSON (1-decimal weight formatting) lives on the
// underlying type in architex/risk/api. We cannot redeclare it here
// because RiskReason is a Go type alias, not a distinct type.
