// Package graph constructs an architecture graph from parsed Terraform resources,
// including typed nodes, inferred edges, derived attributes, and confidence scoring.
package graph

import (
	"fmt"

	"architex/models"
	"architex/models/registry"
)

// All resource-type metadata (supported types, abstract roles, edge
// labels, attribute promoters) lives in architex/models/registry.
// Adding or changing a resource is a one-file edit there; this file
// owns only the graph-construction algorithm.

// Build constructs a Graph from parsed resources and accumulated warnings.
func Build(resources []models.RawResource, warnings []models.Warning) models.Graph {
	resourceIndex := make(map[string]*models.RawResource, len(resources))
	for i := range resources {
		resourceIndex[resources[i].ID] = &resources[i]
	}

	nodes := buildNodes(resources)
	edges := buildEdges(resources, resourceIndex)
	confidence := computeConfidence(warnings)

	return models.Graph{
		Nodes:      nodes,
		Edges:      edges,
		Confidence: confidence,
	}
}

func buildNodes(resources []models.RawResource) []models.Node {
	nodes := make([]models.Node, 0, len(resources))

	for _, res := range resources {
		abstractType, ok := models.AbstractionMap[res.Type]
		if !ok {
			abstractType = "unknown"
		}

		attrs := deriveAttributes(res)

		nodes = append(nodes, models.Node{
			ID:           res.ID,
			Type:         abstractType,
			ProviderType: res.Type,
			Attributes:   attrs,
		})
	}

	return nodes
}

// deriveAttributes promotes a parser-emitted RawResource's attributes
// onto a graph node. As of the v1.4 readability refactor (PR5) the
// per-resource promotion logic lives in
// architex/models/registry/<provider>.go; this function is only
// responsible for (a) dispatching to the right promoter and (b)
// stamping the cross-cutting `conditional` flag, which the parser
// sets on library-mode phantoms and which the rule layer / Mermaid
// renderer both consume.
func deriveAttributes(res models.RawResource) map[string]any {
	attrs := registry.AttrPromoterFor(res.Type)(res.Attributes)
	if v, ok := res.Attributes["conditional"]; ok {
		if b, ok := v.(bool); ok && b {
			attrs["conditional"] = true
		}
	}
	return attrs
}

func buildEdges(resources []models.RawResource, index map[string]*models.RawResource) []models.Edge {
	edges := make([]models.Edge, 0)
	seen := make(map[string]bool)

	for _, res := range resources {
		for _, ref := range res.References {
			target, exists := index[ref.TargetID]
			if !exists {
				continue
			}

			edgeType := inferEdgeType(res.Type, target.Type)
			dedupKey := fmt.Sprintf("%s|%s|%s", res.ID, ref.TargetID, edgeType)

			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true

			edges = append(edges, models.Edge{
				From: res.ID,
				To:   ref.TargetID,
				Type: edgeType,
			})
		}
	}

	return edges
}

// inferEdgeType returns the architectural label for a (source, target)
// resource-type pair. As of the v1.4 readability refactor (PR5) the
// per-pair table lives in architex/models/registry; pairs absent from
// the registry fall through to the generic "references" label, which
// matches the pre-refactor edgeTypeMap default.
func inferEdgeType(sourceType, targetType string) string {
	if label := registry.EdgeLabelFor(sourceType, targetType); label != "" {
		return label
	}
	return "references"
}

// confidenceDeduction maps a warning category to its score impact.
// Categories not in this map have no effect on confidence (e.g. WarnInfo).
var confidenceDeduction = map[string]float64{
	models.WarnUnsupportedResource:  0.1,
	models.WarnUnsupportedConstruct: 0.05,
	models.WarnParseError:           0.15,
}

func computeConfidence(warnings []models.Warning) models.Confidence {
	score := 1.0

	for _, w := range warnings {
		if d, ok := confidenceDeduction[w.Category]; ok {
			score -= d
		}
	}

	if score < 0 {
		score = 0
	}

	if warnings == nil {
		warnings = []models.Warning{}
	}

	return models.Confidence{
		Score:    score,
		Warnings: warnings,
	}
}
