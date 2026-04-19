package interpreter

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"architex/models"
	"architex/risk"
)

// EgressPayload is the ONLY shape that may leave the customer runner. Its
// JSON keys are mirrored in docs/egress-schema.json; a test enforces that any
// new field is added to both places. See master.md §9.
type EgressPayload struct {
	SchemaVersion string                 `json:"schema_version"`
	Score         float64                `json:"score"`
	Severity      string                 `json:"severity"`
	Status        string                 `json:"status"`
	ReasonCodes   []string               `json:"reason_codes"`
	AddedNodes    []SanitizedNode        `json:"added_nodes"`
	RemovedNodes  []SanitizedNode        `json:"removed_nodes"`
	ChangedNodes  []SanitizedChangedNode `json:"changed_nodes"`
	AddedEdges    []SanitizedEdge        `json:"added_edges"`
	RemovedEdges  []SanitizedEdge        `json:"removed_edges"`
	Summary       SanitizedSummary       `json:"summary"`
}

// SanitizedNode never carries the original Terraform name. The ID is the
// hash of the original ID under the run's salt; the abstract type comes from
// the AbstractionMap allowlist only. ProviderType is intentionally omitted.
type SanitizedNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// SanitizedChangedNode reports which attribute KEYS changed but never their
// values. Values are out of scope for egress.
type SanitizedChangedNode struct {
	ID                   string   `json:"id"`
	Type                 string   `json:"type"`
	ChangedAttributeKeys []string `json:"changed_attribute_keys"`
}

// SanitizedEdge mirrors models.Edge but with hashed endpoints.
type SanitizedEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// SanitizedSummary mirrors delta.DeltaSummary. Counts are not sensitive.
type SanitizedSummary struct {
	AddedNodes   int `json:"added_nodes"`
	RemovedNodes int `json:"removed_nodes"`
	AddedEdges   int `json:"added_edges"`
	RemovedEdges int `json:"removed_edges"`
	ChangedNodes int `json:"changed_nodes"`
}

// SanitizationPolicy controls per-run sanitization behavior. Defaults are
// conservative: full hashing, no environment leakage. Keeping this struct
// small and explicit is a procurement requirement -- security teams will
// audit every knob.
type SanitizationPolicy struct {
	// HashSalt is mixed into every node ID hash. Empty salt produces stable
	// IDs across runs (useful for diffing across PRs); a unique salt per run
	// produces opaque IDs (useful for one-off submissions).
	HashSalt string
}

// DefaultSanitizationPolicy returns a zero-knob policy: empty salt (stable
// hashing). Callers that need per-run opacity should construct a policy
// with a fresh HashSalt.
func DefaultSanitizationPolicy() SanitizationPolicy {
	return SanitizationPolicy{}
}

// Sanitize converts a Report into an EgressPayload using the given policy.
// The output contains no Terraform names, no attribute values, no risk
// messages -- only allowlisted, abstract metadata.
func Sanitize(rep Report, policy SanitizationPolicy) EgressPayload {
	hash := func(id string) string {
		return hashID(id, policy.HashSalt)
	}

	addedNodes := make([]SanitizedNode, 0, len(rep.Delta.AddedNodes))
	for _, n := range rep.Delta.AddedNodes {
		addedNodes = append(addedNodes, SanitizedNode{ID: hash(n.ID), Type: n.Type})
	}
	sort.SliceStable(addedNodes, func(i, j int) bool { return addedNodes[i].ID < addedNodes[j].ID })

	removedNodes := make([]SanitizedNode, 0, len(rep.Delta.RemovedNodes))
	for _, n := range rep.Delta.RemovedNodes {
		removedNodes = append(removedNodes, SanitizedNode{ID: hash(n.ID), Type: n.Type})
	}
	sort.SliceStable(removedNodes, func(i, j int) bool { return removedNodes[i].ID < removedNodes[j].ID })

	changedNodes := make([]SanitizedChangedNode, 0, len(rep.Delta.ChangedNodes))
	for _, cn := range rep.Delta.ChangedNodes {
		keys := make([]string, 0, len(cn.ChangedAttributes))
		for k := range cn.ChangedAttributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		changedNodes = append(changedNodes, SanitizedChangedNode{
			ID:                   hash(cn.ID),
			Type:                 cn.Type,
			ChangedAttributeKeys: keys,
		})
	}
	sort.SliceStable(changedNodes, func(i, j int) bool { return changedNodes[i].ID < changedNodes[j].ID })

	addedEdges := sanitizeEdges(rep.Delta.AddedEdges, hash)
	removedEdges := sanitizeEdges(rep.Delta.RemovedEdges, hash)

	codes := reasonCodes(rep.Risk)

	return EgressPayload{
		SchemaVersion: SchemaVersion,
		Score:         rep.Risk.Score,
		Severity:      rep.Risk.Severity,
		Status:        rep.Risk.Status,
		ReasonCodes:   codes,
		AddedNodes:    addedNodes,
		RemovedNodes:  removedNodes,
		ChangedNodes:  changedNodes,
		AddedEdges:    addedEdges,
		RemovedEdges:  removedEdges,
		Summary: SanitizedSummary{
			AddedNodes:   rep.Delta.Summary.AddedNodes,
			RemovedNodes: rep.Delta.Summary.RemovedNodes,
			AddedEdges:   rep.Delta.Summary.AddedEdges,
			RemovedEdges: rep.Delta.Summary.RemovedEdges,
			ChangedNodes: rep.Delta.Summary.ChangedNodes,
		},
	}
}

func sanitizeEdges(edges []models.Edge, hash func(string) string) []SanitizedEdge {
	out := make([]SanitizedEdge, 0, len(edges))
	for _, e := range edges {
		out = append(out, SanitizedEdge{
			From: hash(e.From),
			To:   hash(e.To),
			Type: e.Type,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Type < out[j].Type
	})
	return out
}

// hashID returns a stable, salted, truncated SHA-256 of the original ID.
// 8 hex characters (32 bits) is enough collision resistance for delta-sized
// payloads while keeping IDs short and human-comparable.
func hashID(id, salt string) string {
	h := sha256.Sum256([]byte(salt + "|" + id))
	return "n_" + hex.EncodeToString(h[:4])
}

// reasonCodes returns the deduplicated, sorted list of triggered rule IDs.
// Free-form Message text from RiskReason is intentionally NOT included --
// rule IDs are stable identifiers; messages may contain resource names.
func reasonCodes(r risk.RiskResult) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(r.Reasons))
	for _, reason := range r.Reasons {
		if seen[reason.RuleID] {
			continue
		}
		seen[reason.RuleID] = true
		out = append(out, reason.RuleID)
	}
	sort.Strings(out)
	return out
}
