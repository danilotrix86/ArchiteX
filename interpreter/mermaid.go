package interpreter

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"architex/delta"
	"architex/models"
)

// nodeStatus tags how a node should be rendered in the diagram.
type nodeStatus int

const (
	statusContext nodeStatus = iota
	statusAdded
	statusRemoved
	statusChanged
)

func (s nodeStatus) class() string {
	switch s {
	case statusAdded:
		return "added"
	case statusRemoved:
		return "removed"
	case statusChanged:
		return "changed"
	default:
		return "context"
	}
}

func (s nodeStatus) marker() string {
	switch s {
	case statusAdded:
		return "+ "
	case statusRemoved:
		return "- "
	case statusChanged:
		return "~ "
	default:
		return ""
	}
}

// rendered carries the per-node info needed for stable Mermaid emission.
type rendered struct {
	ID           string // sanitized identifier
	OriginalID   string // raw "type.name" for label and sort key
	AbstractType string
	Status       nodeStatus
}

// MermaidBudget is the default soft byte budget for a rendered Mermaid block.
//
// Mermaid-js refuses to render diagrams whose source exceeds its `maxTextSize`
// configuration (default 50,000 chars). Above that threshold GitHub displays
// "Maximum text size in diagram exceeded" instead of the diagram. We default
// to 45,000 so we stay clear of that cliff with a 5,000-char safety margin.
//
// Callers can override by calling RenderMermaidBudgeted directly.
const MermaidBudget = 45_000

// RenderMermaid produces a deterministic Mermaid `flowchart LR` representation
// of the delta. Output is byte-identical for identical input.
//
// Status is conveyed by both:
//   - a leading marker on the label ("+ ", "- ", "~ ") so the meaning is
//     visible in any rendering theme, and
//   - a `classDef` with stroke styling so themed environments (like GitHub PRs)
//     can color the borders without relying on background fills.
//
// Edges directly added or removed by the delta are rendered. Their endpoints
// always appear in the diagram, even when one endpoint is unchanged context.
//
// This function does NOT enforce any size budget -- for large deltas the
// output may exceed mermaid-js's renderer limit. Use RenderMermaidBudgeted
// when posting to a renderer with a known cap (e.g. GitHub PR comments).
func RenderMermaid(d delta.Delta) string {
	nodes := collectNodes(d)
	edges := collectEdges(d)
	return renderFromSets(nodes, edges, truncationNotice{})
}

// RenderMermaidBudgeted is like RenderMermaid but guarantees the rendered
// block stays at or below maxBytes. If the full delta would exceed the
// budget, lower-priority nodes are dropped and a placeholder node is appended
// announcing the truncation; the placeholder is part of the trust surface so
// readers can always tell when content was hidden.
//
// Priority order (highest first):
//  1. status: changed > added > removed > context
//  2. abstract type impact: entry_point > data > compute > network > access_control
//  3. original ID alphabetically (stable tiebreaker)
//
// Edges are kept only when both endpoints are in the kept-node set.
//
// Pass MermaidBudget for the default 45,000-byte threshold.
func RenderMermaidBudgeted(d delta.Delta, maxBytes int) string {
	nodes := collectNodes(d)
	edges := collectEdges(d)

	full := renderFromSets(nodes, edges, truncationNotice{})
	if maxBytes <= 0 || len(full) <= maxBytes {
		return full
	}

	keptNodes, keptEdges, hiddenNodes, hiddenEdges := budgetFit(nodes, edges, maxBytes)
	notice := truncationNotice{
		active:      true,
		hiddenNodes: hiddenNodes,
		hiddenEdges: hiddenEdges,
		totalNodes:  len(nodes),
		totalEdges:  len(edges),
	}
	return renderFromSets(keptNodes, keptEdges, notice)
}

// truncationNotice is rendered as a placeholder node at the end of the
// diagram when budgetFit dropped content. Zero-value = no notice.
type truncationNotice struct {
	active                   bool
	hiddenNodes, hiddenEdges int
	totalNodes, totalEdges   int
}

// renderFromSets is the shared rendering core. It is the only place that
// emits Mermaid syntax; both RenderMermaid and RenderMermaidBudgeted go
// through here, guaranteeing byte-identical output for identical inputs.
func renderFromSets(nodes []rendered, edges []renderedEdge, notice truncationNotice) string {
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	b.WriteString("    classDef added stroke:#28a745,stroke-width:2px\n")
	b.WriteString("    classDef removed stroke:#dc3545,stroke-width:2px,stroke-dasharray: 5 5\n")
	b.WriteString("    classDef changed stroke:#d39e00,stroke-width:2px\n")
	b.WriteString("    classDef context stroke:#6c757d,stroke-width:1px\n")
	b.WriteString("    classDef truncated stroke:#6f42c1,stroke-width:2px,stroke-dasharray: 2 2\n")

	if len(nodes) == 0 && len(edges) == 0 && !notice.active {
		b.WriteString("    empty[\"no architectural changes\"]:::context\n")
		return b.String()
	}

	b.WriteString("\n")
	for _, n := range nodes {
		fmt.Fprintf(&b, "    %s[\"%s%s: %s\"]:::%s\n",
			n.ID,
			n.Status.marker(),
			n.AbstractType,
			n.OriginalID,
			n.Status.class(),
		)
	}

	if len(edges) > 0 {
		b.WriteString("\n")
		for _, e := range edges {
			arrow := "-->"
			if e.removed {
				arrow = "-.->"
			}
			fmt.Fprintf(&b, "    %s %s|%s| %s\n",
				sanitizeID(e.from), arrow, e.label, sanitizeID(e.to))
		}
	}

	if notice.active {
		b.WriteString("\n")
		fmt.Fprintf(&b,
			"    _architex_truncated[\"... and %d more node(s), %d more edge(s) hidden -- full diagram in audit bundle\"]:::truncated\n",
			notice.hiddenNodes, notice.hiddenEdges,
		)
	}

	return b.String()
}

// typePriority returns a sort key for abstract types: lower = higher priority
// when keeping nodes within a budget. The order reflects PR-review impact:
// public-facing entry points and data stores matter most, infrastructure
// scaffolding least.
//
// Phase 6 added two new abstract types ("storage", "identity"). They are
// inserted into the existing scale rather than appended: storage sits next
// to data (it is data-at-rest), identity sits below compute but above
// pure network/access-control (an over-permissive role can dwarf an
// individual SG-rule change). The "default" arm remains for any future
// abstract type not yet ranked.
func typePriority(abstractType string) int {
	switch abstractType {
	case "entry_point":
		return 0
	case "data":
		return 1
	case "storage":
		return 2
	case "compute":
		return 3
	case "identity":
		return 4
	case "network":
		return 5
	case "access_control":
		return 6
	default:
		return 7
	}
}

// statusPriority returns a sort key for node status: lower = higher priority.
// changed/added/removed all rank above unchanged-context endpoints.
func statusPriority(s nodeStatus) int {
	switch s {
	case statusChanged:
		return 0
	case statusAdded:
		return 1
	case statusRemoved:
		return 2
	default:
		return 3
	}
}

// budgetFit greedily selects the highest-priority nodes (and the subset of
// edges whose endpoints are both kept) such that renderFromSets on the
// returned sets, plus a truncation placeholder, fits within maxBytes.
//
// Returns the kept nodes, kept edges, and counts of hidden nodes/edges for
// the placeholder. Always keeps at least one node so the diagram never
// degenerates to "no architectural changes" when there were in fact changes.
func budgetFit(nodes []rendered, edges []renderedEdge, maxBytes int) ([]rendered, []renderedEdge, int, int) {
	prioritized := make([]rendered, len(nodes))
	copy(prioritized, nodes)
	slices.SortFunc(prioritized, func(a, b rendered) int {
		if d := statusPriority(a.Status) - statusPriority(b.Status); d != 0 {
			return d
		}
		if d := typePriority(a.AbstractType) - typePriority(b.AbstractType); d != 0 {
			return d
		}
		return strings.Compare(a.OriginalID, b.OriginalID)
	})

	// Index edges by endpoints for O(1) lookup during the fit loop.
	type edgeKey struct{ from, to string }
	edgesByEndpoints := make(map[edgeKey][]renderedEdge, len(edges))
	for _, e := range edges {
		k := edgeKey{from: sanitizeID(e.from), to: sanitizeID(e.to)}
		edgesByEndpoints[k] = append(edgesByEndpoints[k], e)
	}

	keptNodeIDs := make(map[string]bool)
	var keptNodes []rendered
	var keptEdges []renderedEdge

	tryFit := func(extra rendered) ([]rendered, []renderedEdge, bool) {
		candidateNodes := append(append([]rendered{}, keptNodes...), extra)
		// Re-sort kept nodes for deterministic output (renderFromSets does
		// not sort itself).
		slices.SortFunc(candidateNodes, func(a, b rendered) int {
			return strings.Compare(a.OriginalID, b.OriginalID)
		})
		ids := make(map[string]bool, len(candidateNodes))
		for _, n := range candidateNodes {
			ids[n.ID] = true
		}
		// Re-derive kept edges: an edge is kept iff both endpoints are in
		// the kept-node set. We deterministically walk the original edges
		// list to preserve their existing sort order.
		var candidateEdges []renderedEdge
		for _, e := range edges {
			if ids[sanitizeID(e.from)] && ids[sanitizeID(e.to)] {
				candidateEdges = append(candidateEdges, e)
			}
		}
		notice := truncationNotice{
			active:      true,
			hiddenNodes: len(nodes) - len(candidateNodes),
			hiddenEdges: len(edges) - len(candidateEdges),
		}
		out := renderFromSets(candidateNodes, candidateEdges, notice)
		return candidateNodes, candidateEdges, len(out) <= maxBytes
	}

	for _, n := range prioritized {
		nextNodes, nextEdges, ok := tryFit(n)
		if !ok {
			break
		}
		keptNodes = nextNodes
		keptEdges = nextEdges
		keptNodeIDs[n.ID] = true
	}

	// Guarantee at least one node so we don't emit an "empty" diagram when
	// changes do exist. Pick the highest-priority node even if it overflows
	// slightly (the placeholder + classDefs add ~400 bytes; the alternative
	// is a misleading diagram).
	if len(keptNodes) == 0 && len(prioritized) > 0 {
		keptNodes = []rendered{prioritized[0]}
	}

	hiddenNodes := len(nodes) - len(keptNodes)
	hiddenEdges := len(edges) - len(keptEdges)
	return keptNodes, keptEdges, hiddenNodes, hiddenEdges
}

// collectNodes builds the deterministic list of nodes to emit. Status
// precedence (when a node would qualify for multiple): changed > added >
// removed > context. In practice the delta engine never produces conflicts
// (a node cannot be both added and changed), so precedence is defensive.
func collectNodes(d delta.Delta) []rendered {
	byID := make(map[string]*rendered)

	upsert := func(id, abstractType string, status nodeStatus) {
		if existing, ok := byID[id]; ok {
			if status > existing.Status {
				existing.Status = status
			}
			if abstractType != "" && existing.AbstractType == "context" {
				existing.AbstractType = abstractType
			}
			return
		}
		t := abstractType
		if t == "" {
			t = "context"
		}
		byID[id] = &rendered{
			ID:           sanitizeID(id),
			OriginalID:   id,
			AbstractType: t,
			Status:       status,
		}
	}

	for _, n := range d.AddedNodes {
		upsert(n.ID, n.Type, statusAdded)
	}
	for _, n := range d.RemovedNodes {
		upsert(n.ID, n.Type, statusRemoved)
	}
	for _, cn := range d.ChangedNodes {
		upsert(cn.ID, cn.Type, statusChanged)
	}
	for _, e := range d.AddedEdges {
		upsert(e.From, abstractFromID(e.From), statusContext)
		upsert(e.To, abstractFromID(e.To), statusContext)
	}
	for _, e := range d.RemovedEdges {
		upsert(e.From, abstractFromID(e.From), statusContext)
		upsert(e.To, abstractFromID(e.To), statusContext)
	}

	out := make([]rendered, 0, len(byID))
	for _, n := range byID {
		out = append(out, *n)
	}
	slices.SortFunc(out, func(a, b rendered) int {
		return strings.Compare(a.OriginalID, b.OriginalID)
	})
	return out
}

type renderedEdge struct {
	from, to, label string
	removed         bool
}

func collectEdges(d delta.Delta) []renderedEdge {
	out := make([]renderedEdge, 0, len(d.AddedEdges)+len(d.RemovedEdges))
	for _, e := range d.AddedEdges {
		out = append(out, renderedEdge{from: e.From, to: e.To, label: e.Type, removed: false})
	}
	for _, e := range d.RemovedEdges {
		out = append(out, renderedEdge{from: e.From, to: e.To, label: e.Type, removed: true})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].from != out[j].from {
			return out[i].from < out[j].from
		}
		if out[i].to != out[j].to {
			return out[i].to < out[j].to
		}
		if out[i].label != out[j].label {
			return out[i].label < out[j].label
		}
		// added before removed for stable ordering
		return !out[i].removed && out[j].removed
	})
	return out
}

// sanitizeID converts a Terraform-style ID like "aws_security_group.web" into
// a Mermaid-safe identifier ("aws_security_group_web"). Any character outside
// [a-zA-Z0-9_] is replaced with `_`.
func sanitizeID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// abstractFromID looks up the abstract type for a Terraform-style node ID by
// splitting on the first "." and consulting the abstraction map. Used only
// for edge endpoints that are not present in the added/removed/changed sets
// (i.e. unchanged context nodes).
func abstractFromID(id string) string {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) == 0 {
		return ""
	}
	if t, ok := models.AbstractionMap[parts[0]]; ok {
		return t
	}
	return ""
}
