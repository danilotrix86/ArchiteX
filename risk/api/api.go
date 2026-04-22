// Package api is the leaf package the risk-rule registry hangs off. It
// exists ONLY to break the import cycle between the risk package (which
// owns the orchestration code in EvaluateWithBaseline) and the rule
// subpackages under risk/rules/<domain>/ (which own the per-rule logic).
//
// Cycle-avoidance contract:
//
//   - architex/risk      imports architex/risk/api  (one-way)
//   - architex/risk/rules/<domain>/* imports architex/risk/api ONLY
//
// Crucially, architex/risk does NOT import any subpackage of architex/risk/rules.
// Whoever needs the rules registered (the binary, the golden harness, the
// risk-package's own tests) blank-imports `architex/risk/rules` themselves.
// That keeps the dependency arrows one-directional and lets `go build
// ./risk` succeed even if no rule subpackage exists.
//
// Stability: this package is treated as a stable internal API across the
// readability refactor (PR0-PR6). Any change to the Rule interface or the
// RiskReason struct here ripples into every rule subpackage, so changes
// here are deliberate and rare.
package api

import (
	"encoding/json"
	"fmt"

	"architex/delta"
)

// RiskReason describes a single triggered rule and its contribution to the
// score. Lives here (not in the risk package) so rule subpackages can
// construct values of this type without importing architex/risk -- which
// would create a cycle once the risk package needs to call into the rule
// registry.
//
// architex/risk re-exports this type as `risk.RiskReason` via a Go type
// alias so existing call sites and tests continue to work verbatim.
type RiskReason struct {
	RuleID  string  `json:"rule_id"`
	Message string  `json:"message"`
	Impact  string  `json:"impact"`
	Weight  float64 `json:"weight"`

	// ResourceID is the Terraform resource ID this finding is about, when
	// the rule fires per-resource. Empty for cross-resource rules
	// (potential_data_exposure, etc.) -- those cannot be suppressed by
	// (rule, resource) tuple. Phase 7 (v1.2 PR3): populated by all
	// per-resource rules so suppressions can match.
	ResourceID string `json:"resource_id,omitempty"`
}

// MarshalJSON formats Weight with 1 decimal place (4.0 not 4). Identical
// to the pre-refactor implementation in risk/risk.go; preserved here so
// the type's JSON shape is unchanged regardless of which package the
// caller imports it through.
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

// Rule is the contract every risk rule satisfies. The registry stores
// values of this interface keyed by ID().
//
// Method semantics:
//
//   - ID() returns the rule's stable identifier (e.g. "public_exposure_introduced").
//     This is the same string used in score.json reason.rule_id, in
//     .architex.yml suppressions, and in inline `# architex:ignore=` directives.
//     CHANGING an ID is a breaking change for downstream consumers and is a
//     major-version event.
//
//   - Evaluate(d) returns the unsorted, unfiltered list of RiskReasons this
//     rule produces against the given delta. A rule that does not fire
//     returns nil. The orchestrator (EvaluateWithBaseline) is responsible
//     for applying config overrides, suppressions, score capping, and the
//     final sort; rules MUST NOT do any of that themselves.
//
//   - ReviewFocus(reason, d) returns the reviewer-facing instruction string
//     that the interpreter renders in the "Suggested Review Focus" section.
//     Returning "" means "no specific focus from this rule" (the interpreter
//     dedupes on the returned text). Aggregated rules (e.g. "list every new
//     data resource in one bullet") return the same string for every reason
//     they produce; per-resource rules return a unique string per reason.
type Rule interface {
	ID() string
	Evaluate(d delta.Delta) []RiskReason
	ReviewFocus(reason RiskReason, d delta.Delta) string
}

// registered is the global rule registry. Order is registration order
// (init() ordering across files within a package is by filename; across
// packages it is by import dependency order). Because the orchestrator
// sorts the final reason list by (weight desc, ruleID asc), registration
// order does not affect output -- it only affects internal traversal
// order, which is irrelevant to the byte-identical contract.
var registered []Rule

// byID is a fast-lookup index built lazily on first call to RuleByID. It
// is safe for the readability refactor (single-threaded init then
// read-only) but would need a sync.Once if rules ever became dynamically
// (re-)registered at runtime.
var byID map[string]Rule

// Register installs a rule into the global registry. Intended to be
// called from each rule package's init():
//
//	func init() { api.Register(myRule{}) }
//
// Panics on a duplicate ID -- two rules sharing the same RuleID would
// silently double-count in the score. A panic at init time is the right
// failure mode (a unit test exercises every binary path that imports
// risk/rules, so a duplicate ID never reaches a user).
func Register(r Rule) {
	id := r.ID()
	if id == "" {
		panic("risk/api: Register: rule has empty ID")
	}
	for _, existing := range registered {
		if existing.ID() == id {
			panic("risk/api: Register: duplicate rule ID " + id)
		}
	}
	registered = append(registered, r)
	byID = nil // invalidate the index; rebuilt on next RuleByID call
}

// Registered returns a stable copy of the rule registry. Returned slice is
// a defensive copy so callers can iterate without worrying about the
// (read-only) underlying registry shifting beneath them.
func Registered() []Rule {
	out := make([]Rule, len(registered))
	copy(out, registered)
	return out
}

// RuleByID returns the registered rule with the given ID, or nil when no
// rule with that ID is registered. The interpreter uses this to dispatch
// review-focus rendering through the rule's own ReviewFocus method.
func RuleByID(id string) Rule {
	if byID == nil {
		byID = make(map[string]Rule, len(registered))
		for _, r := range registered {
			byID[r.ID()] = r
		}
	}
	return byID[id]
}
