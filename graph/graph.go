// Package graph constructs an architecture graph from parsed Terraform resources,
// including typed nodes, inferred edges, derived attributes, and confidence scoring.
package graph

import (
	"fmt"

	"architex/models"
)

// edgeTypeMap encodes source+target resource types to a relationship label.
//
// Pairs absent from this map fall through to "references" in inferEdgeType.
// We only declare a pair explicitly when the relationship has a stronger,
// well-understood architectural meaning ("attached_to", "deployed_in",
// "part_of", "applies_to") that helps reviewers read the diagram.
var edgeTypeMap = map[string]string{
	// v1.0 -- canonical 3-tier VPC scope
	"aws_instance|aws_security_group":            "attached_to",
	"aws_instance|aws_subnet":                    "deployed_in",
	"aws_subnet|aws_vpc":                         "part_of",
	"aws_security_group_rule|aws_security_group": "applies_to",
	"aws_lb|aws_subnet":                          "deployed_in",
	"aws_lb|aws_security_group":                  "attached_to",
	"aws_db_instance|aws_subnet":                 "deployed_in",
	"aws_db_instance|aws_security_group":         "attached_to",
	"aws_security_group|aws_vpc":                 "part_of",

	// v1.1 -- Phase 6 (AWS Top 10)
	// S3: access-control resources "apply_to" the bucket they govern.
	"aws_s3_bucket_public_access_block|aws_s3_bucket": "applies_to",
	"aws_s3_bucket_policy|aws_s3_bucket":              "applies_to",
	// IAM: a role-policy attachment binds a role to a policy.
	"aws_iam_role_policy_attachment|aws_iam_role":   "applies_to",
	"aws_iam_role_policy_attachment|aws_iam_policy": "applies_to",
	// Lambda: a function "uses" an execution role; a function URL is
	// "applied_to" its parent function (same direction as SG-rule -> SG).
	"aws_lambda_function|aws_iam_role":            "attached_to",
	"aws_lambda_function_url|aws_lambda_function": "applies_to",
	// Networking: an internet gateway is "part_of" its VPC, mirroring the
	// existing aws_subnet|aws_vpc relationship.
	"aws_internet_gateway|aws_vpc": "part_of",
}

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

func deriveAttributes(res models.RawResource) map[string]any {
	attrs := make(map[string]any)

	switch res.Type {
	case "aws_lb":
		attrs["public"] = true

	case "aws_db_instance":
		attrs["public"] = false

	case "aws_security_group", "aws_security_group_rule":
		attrs["public"] = hasCIDRAllTraffic(res.Attributes)

	case "aws_instance":
		pub := false
		if v, ok := res.Attributes["associate_public_ip_address"]; ok {
			if b, ok := v.(bool); ok {
				pub = b
			}
		}
		attrs["public"] = pub

	// Phase 6: resources that, if present, definitionally introduce
	// internet-facing surface. Marking these public:true at the node level
	// lets the existing `new_entry_point` rule (Phase 3) and reviewer-focus
	// templates (Phase 4) work for them with no rule changes.
	case "aws_lambda_function_url",
		"aws_apigatewayv2_api",
		"aws_internet_gateway":
		attrs["public"] = true

	// Phase 6: IAM role-policy attachment. We pass `policy_arn` through to
	// the graph node when (and only when) it was captured as a literal
	// string by the parser, so the iam_admin_policy_attached risk rule can
	// inspect it without re-parsing. Variable-driven ARNs land here as nil
	// and are intentionally NOT promoted -- we never guess at unresolved
	// expressions (see risk/rules.go).
	case "aws_iam_role_policy_attachment":
		attrs["public"] = false
		if v, ok := res.Attributes["policy_arn"]; ok {
			if s, ok := v.(string); ok && s != "" {
				attrs["policy_arn"] = s
			}
		}

	default:
		// Includes the rest of the Phase 6 resources (aws_s3_bucket,
		// aws_s3_bucket_policy, aws_s3_bucket_public_access_block,
		// aws_iam_*, aws_lambda_function). These are NOT inherently
		// public on their own -- bucket exposure is governed by sibling
		// resources (PAB + policy), and IAM is identity-only. Phase 6's
		// risk rules look at the delta-level shape rather than at a
		// single derived attribute on these resources.
		attrs["public"] = false
	}

	return attrs
}

// hasCIDRAllTraffic checks if any cidr_blocks attribute contains "0.0.0.0/0".
func hasCIDRAllTraffic(attrs map[string]any) bool {
	raw, ok := attrs["cidr_blocks"]
	if !ok {
		return false
	}

	list, ok := raw.([]any)
	if !ok {
		return false
	}

	for _, item := range list {
		if s, ok := item.(string); ok && s == "0.0.0.0/0" {
			return true
		}
	}
	return false
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

func inferEdgeType(sourceType, targetType string) string {
	key := sourceType + "|" + targetType
	if t, ok := edgeTypeMap[key]; ok {
		return t
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
