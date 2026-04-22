package risk

import "architex/risk/api"

// Rule is re-exported from architex/risk/api so callers that already
// import "architex/risk" can refer to risk.Rule without learning about
// the api leaf package. The leaf split exists ONLY to break a hypothetical
// import cycle (rule subpackages need the Rule interface; the risk
// orchestrator needs the rule registry); end users should treat
// risk.Rule and api.Rule as the same type.
type Rule = api.Rule

// Register installs a rule into the global registry. See architex/risk/api
// for the full contract; this is a thin re-export so rule subpackages can
// optionally import architex/risk instead of the deeper api package when
// they have other risk-package dependencies.
var Register = api.Register

// registered returns the current rule registry in registration order.
// The orchestrator (EvaluateWithBaseline) walks this list to evaluate
// every migrated rule. Rules that have NOT yet been migrated to the
// registry-based pattern are still hand-called from EvaluateWithBaseline
// during the staged refactor (see PR2-PR4 in the readability plan).
func registered() []api.Rule {
	return api.Registered()
}

// RuleByID returns the registered rule with the given ID, or nil when no
// rule with that ID is registered. interpreter.focusForRule uses this to
// dispatch review-focus rendering through the rule's own ReviewFocus
// method, eliminating the duplicated copy that used to live in
// interpreter/summary.go.
func RuleByID(id string) Rule { return api.RuleByID(id) }
