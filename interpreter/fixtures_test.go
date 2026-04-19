package interpreter

import (
	"architex/delta"
	"architex/models"
	"architex/risk"
)

// emptyDelta returns the zero-value Delta as produced by delta.Compare for two
// identical graphs (all slices empty, summary all zero).
func emptyDelta() delta.Delta {
	return delta.Delta{
		AddedNodes:   []models.Node{},
		RemovedNodes: []models.Node{},
		AddedEdges:   []models.Edge{},
		RemovedEdges: []models.Edge{},
		ChangedNodes: []delta.ChangedNode{},
		Summary:      delta.DeltaSummary{},
	}
}

// highRiskDelta replicates the testdata/base -> testdata/head scenario:
// the SG is opened to the internet (changed: public false->true) AND a public
// load balancer is added with edges to the SG and the public subnet.
//
// Expected risk: 9.0 / high / fail
//
//	+4.0 public_exposure_introduced
//	+3.0 new_entry_point
//	+2.0 potential_data_exposure
func highRiskDelta() delta.Delta {
	return delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_lb.web",
				Type:         "entry_point",
				ProviderType: "aws_lb",
				Attributes:   map[string]any{"public": true},
			},
		},
		RemovedNodes: []models.Node{},
		AddedEdges: []models.Edge{
			{From: "aws_lb.web", To: "aws_security_group.web", Type: "attached_to"},
			{From: "aws_lb.web", To: "aws_subnet.public", Type: "deployed_in"},
		},
		RemovedEdges: []models.Edge{},
		ChangedNodes: []delta.ChangedNode{
			{
				ID:           "aws_security_group.web",
				Type:         "access_control",
				ProviderType: "aws_security_group",
				ChangedAttributes: map[string]delta.ChangedAttribute{
					"public": {Before: false, After: true},
				},
			},
		},
		Summary: delta.DeltaSummary{
			AddedNodes:   1,
			RemovedNodes: 0,
			AddedEdges:   2,
			RemovedEdges: 0,
			ChangedNodes: 1,
		},
	}
}

// dbAddedDelta represents the testdata/db_added_* scenario:
// a new aws_db_instance is added (no exposure changes).
//
// Expected risk: 2.5 / low / pass (only new_data_resource).
func dbAddedDelta() delta.Delta {
	return delta.Delta{
		AddedNodes: []models.Node{
			{
				ID:           "aws_db_instance.main",
				Type:         "data",
				ProviderType: "aws_db_instance",
				Attributes:   map[string]any{"public": false},
			},
		},
		RemovedNodes: []models.Node{},
		AddedEdges:   []models.Edge{},
		RemovedEdges: []models.Edge{},
		ChangedNodes: []delta.ChangedNode{},
		Summary: delta.DeltaSummary{
			AddedNodes: 1,
		},
	}
}

// removalDelta represents the testdata/removed_* scenario: an instance was removed.
//
// Expected risk: 0.5 / low / pass (resource_removed).
func removalDelta() delta.Delta {
	return delta.Delta{
		AddedNodes: []models.Node{},
		RemovedNodes: []models.Node{
			{
				ID:           "aws_instance.web",
				Type:         "compute",
				ProviderType: "aws_instance",
				Attributes:   map[string]any{"public": true},
			},
		},
		AddedEdges:   []models.Edge{},
		RemovedEdges: []models.Edge{},
		ChangedNodes: []delta.ChangedNode{},
		Summary: delta.DeltaSummary{
			RemovedNodes: 1,
		},
	}
}

// runRisk evaluates risk for the given delta and returns the result. Wrapping
// the call keeps test bodies focused on assertions, not setup.
func runRisk(d delta.Delta) risk.RiskResult {
	return risk.Evaluate(d)
}
