// Package models defines the shared domain types for the architecture graph,
// including nodes, edges, confidence scoring, and the supported resource registry.
package models

import "architex/models/registry"

// Graph is the top-level output structure serialized to JSON.
type Graph struct {
	Nodes      []Node     `json:"nodes"`
	Edges      []Edge     `json:"edges"`
	Confidence Confidence `json:"confidence"`
}

type Node struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	ProviderType string         `json:"provider_type"`
	Attributes   map[string]any `json:"attributes"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type Confidence struct {
	Score    float64   `json:"score"`
	Warnings []Warning `json:"warnings"`
}

// Warning categories. Each category corresponds to a deterministic confidence
// deduction in graph.computeConfidence. Categories are the contract between
// parser and graph -- format strings of Message are NOT.
const (
	WarnUnsupportedResource  = "unsupported_resource"  // resource type not in SupportedResources
	WarnUnsupportedConstruct = "unsupported_construct" // for_each, count, dynamic, module, unknown block
	WarnParseError           = "parse_error"           // a .tf file failed to parse
	WarnInfo                 = "info"                  // informational, no confidence impact
)

// Warning is a typed diagnostic produced by the parser and consumed by the
// graph builder for confidence scoring.
type Warning struct {
	Category string `json:"category"`
	Message  string `json:"message"`
}

// RawResource holds everything extracted from a single HCL resource block
// before graph construction. This is the parser's output.
type RawResource struct {
	Type       string
	Name       string
	ID         string         // "type.name"
	Attributes map[string]any // scalar values we could evaluate; nil for unresolvable expressions
	References []Reference
}

type Reference struct {
	SourceAttr string // attribute where the reference was found
	TargetID   string // e.g. "aws_security_group.web"
}

// SupportedResources defines the Terraform resource types we handle.
//
// As of the v1.4 readability refactor (PR5), this map is built at
// package init() from architex/models/registry. The registry owns the
// per-provider entries (one file per provider) -- this map remains
// the public-facing union for backward compatibility with every
// existing consumer (parser/expand.go, parser/extract.go,
// baseline/baseline.go, etc.).
//
// Historical notes (preserved for context): the v1.0 set was the
// canonical 3-tier VPC scope (VPC/subnet/SG/SG-rule/EC2/ALB/RDS).
// v1.1 ("AWS Top 10", Phase 6) added 10 more types covering S3, IAM,
// Lambda, API Gateway v2, and the Internet Gateway. v1.2 ("Coverage
// tranche 2", Phase 7 PR4) added CloudFront, Route53, KMS, SNS, SQS,
// NAT Gateway, NACL, Secrets Manager, EBS, and ECS. v1.3 (Phase 8)
// added EKS, RDS auxiliary groups, and EC2 launch template / ASG
// family. v1.4 (Phase 9) added Azure (azurerm) tranche-0.
var SupportedResources = registry.SupportedTypes()

// AbstractionMap maps every supported provider type to a generic
// architecture role. Built at package init() from
// architex/models/registry the same way SupportedResources is.
//
// The seven abstract roles in use (v1.0 -> v1.4) are: compute, data,
// entry_point, network, access_control, storage (added Phase 6 for S3
// / EBS), and identity (added Phase 6 for IAM and reused Phase 8 for
// EKS identity provider configs / KMS keys-as-policy-subjects).
//
// Adding a new provider type / role goes in the registry's per-
// provider file -- never here.
var AbstractionMap = registry.AbstractTypes()
