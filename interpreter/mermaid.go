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
func RenderMermaid(d delta.Delta) string {
	nodes := collectNodes(d)
	edges := collectEdges(d)

	var b strings.Builder
	b.WriteString("flowchart LR\n")
	b.WriteString("    classDef added stroke:#28a745,stroke-width:2px\n")
	b.WriteString("    classDef removed stroke:#dc3545,stroke-width:2px,stroke-dasharray: 5 5\n")
	b.WriteString("    classDef changed stroke:#d39e00,stroke-width:2px\n")
	b.WriteString("    classDef context stroke:#6c757d,stroke-width:1px\n")

	if len(nodes) == 0 && len(edges) == 0 {
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

	return b.String()
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
