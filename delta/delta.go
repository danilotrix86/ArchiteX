// Package delta computes a deterministic semantic diff between two architecture
// graphs, tracking added/removed/changed nodes and edges.
package delta

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"architex/models"
)

// Delta types -----------------------------------------------------------------

// Delta captures all differences between a base graph and a head graph.
type Delta struct {
	AddedNodes   []models.Node `json:"added_nodes"`
	RemovedNodes []models.Node `json:"removed_nodes"`
	AddedEdges   []models.Edge `json:"added_edges"`
	RemovedEdges []models.Edge `json:"removed_edges"`
	ChangedNodes []ChangedNode `json:"changed_nodes"`
	Summary      DeltaSummary  `json:"summary"`
}

// ChangedNode carries enough type metadata for downstream consumers (e.g. the
// risk engine) to classify the node without re-parsing the ID string.
type ChangedNode struct {
	ID                string                      `json:"id"`
	Type              string                      `json:"type"`          // abstract type, e.g. "access_control"
	ProviderType      string                      `json:"provider_type"` // e.g. "aws_security_group"
	ChangedAttributes map[string]ChangedAttribute `json:"changed_attributes"`
}

type ChangedAttribute struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

type DeltaSummary struct {
	AddedNodes   int `json:"added_nodes"`
	RemovedNodes int `json:"removed_nodes"`
	AddedEdges   int `json:"added_edges"`
	RemovedEdges int `json:"removed_edges"`
	ChangedNodes int `json:"changed_nodes"`
}

// Compare produces a deterministic semantic delta between two graphs.
func Compare(base, head models.Graph) Delta {
	baseNodes := nodeMapByID(base.Nodes)
	headNodes := nodeMapByID(head.Nodes)

	added := make([]models.Node, 0)
	removed := make([]models.Node, 0)
	changed := make([]ChangedNode, 0)

	for id, node := range headNodes {
		if _, exists := baseNodes[id]; !exists {
			added = append(added, node)
		}
	}

	for id, node := range baseNodes {
		if _, exists := headNodes[id]; !exists {
			removed = append(removed, node)
		}
	}

	for id, headNode := range headNodes {
		baseNode, exists := baseNodes[id]
		if !exists {
			continue
		}
		diff := diffAttributes(baseNode.Attributes, headNode.Attributes)
		if len(diff) > 0 {
			changed = append(changed, ChangedNode{
				ID:                id,
				Type:              headNode.Type,
				ProviderType:      headNode.ProviderType,
				ChangedAttributes: diff,
			})
		}
	}

	baseEdges := edgeMapByKey(base.Edges)
	headEdges := edgeMapByKey(head.Edges)

	addedEdges := make([]models.Edge, 0)
	removedEdges := make([]models.Edge, 0)

	for key, edge := range headEdges {
		if _, exists := baseEdges[key]; !exists {
			addedEdges = append(addedEdges, edge)
		}
	}

	for key, edge := range baseEdges {
		if _, exists := headEdges[key]; !exists {
			removedEdges = append(removedEdges, edge)
		}
	}

	sortNodes(added)
	sortNodes(removed)
	sortEdges(addedEdges)
	sortEdges(removedEdges)
	sortChangedNodes(changed)

	return Delta{
		AddedNodes:   added,
		RemovedNodes: removed,
		AddedEdges:   addedEdges,
		RemovedEdges: removedEdges,
		ChangedNodes: changed,
		Summary: DeltaSummary{
			AddedNodes:   len(added),
			RemovedNodes: len(removed),
			AddedEdges:   len(addedEdges),
			RemovedEdges: len(removedEdges),
			ChangedNodes: len(changed),
		},
	}
}

// HumanSummary returns a one-line readable description of the delta.
//
//	"1 node added, 2 edges removed, 1 node changed"
//
// Sections with zero count are omitted. Returns "no changes" for an empty delta.
func HumanSummary(d Delta) string {
	var parts []string

	if n := d.Summary.AddedNodes; n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s added", n, plural(n, "node")))
	}
	if n := d.Summary.RemovedNodes; n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s removed", n, plural(n, "node")))
	}
	if n := d.Summary.AddedEdges; n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s added", n, plural(n, "edge")))
	}
	if n := d.Summary.RemovedEdges; n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s removed", n, plural(n, "edge")))
	}
	if n := d.Summary.ChangedNodes; n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s changed", n, plural(n, "node")))
	}

	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// Helpers ---------------------------------------------------------------------

func nodeMapByID(nodes []models.Node) map[string]models.Node {
	m := make(map[string]models.Node, len(nodes))
	for _, n := range nodes {
		m[n.ID] = n
	}
	return m
}

func edgeKey(e models.Edge) string {
	return e.From + "|" + e.To + "|" + e.Type
}

func edgeMapByKey(edges []models.Edge) map[string]models.Edge {
	m := make(map[string]models.Edge, len(edges))
	for _, e := range edges {
		m[edgeKey(e)] = e
	}
	return m
}

// diffAttributes returns only the keys whose values differ between base and head.
// Keys present in one map but not the other are treated as changes (before/after = nil).
func diffAttributes(base, head map[string]any) map[string]ChangedAttribute {
	diff := make(map[string]ChangedAttribute)

	for k, headVal := range head {
		baseVal, exists := base[k]
		if !exists {
			diff[k] = ChangedAttribute{Before: nil, After: headVal}
		} else if !reflect.DeepEqual(baseVal, headVal) {
			diff[k] = ChangedAttribute{Before: baseVal, After: headVal}
		}
	}

	for k, baseVal := range base {
		if _, exists := head[k]; !exists {
			diff[k] = ChangedAttribute{Before: baseVal, After: nil}
		}
	}

	return diff
}

// Sorting helpers for deterministic output ------------------------------------

func sortNodes(nodes []models.Node) {
	slices.SortFunc(nodes, func(a, b models.Node) int {
		return strings.Compare(a.ID, b.ID)
	})
}

func sortEdges(edges []models.Edge) {
	slices.SortFunc(edges, func(a, b models.Edge) int {
		if c := strings.Compare(a.From, b.From); c != 0 {
			return c
		}
		if c := strings.Compare(a.To, b.To); c != 0 {
			return c
		}
		return strings.Compare(a.Type, b.Type)
	})
}

func sortChangedNodes(nodes []ChangedNode) {
	slices.SortFunc(nodes, func(a, b ChangedNode) int {
		return strings.Compare(a.ID, b.ID)
	})
}
