// Package models defines the shared domain types for the architecture graph,
// including nodes, edges, confidence scoring, and the supported resource registry.
package models

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
var SupportedResources = map[string]bool{
	"aws_vpc":                 true,
	"aws_subnet":              true,
	"aws_security_group":      true,
	"aws_security_group_rule": true,
	"aws_instance":            true,
	"aws_lb":                  true,
	"aws_db_instance":         true,
}

// AbstractionMap maps AWS provider types to generic architecture types.
var AbstractionMap = map[string]string{
	"aws_instance":            "compute",
	"aws_db_instance":         "data",
	"aws_lb":                  "entry_point",
	"aws_vpc":                 "network",
	"aws_subnet":              "network",
	"aws_security_group":      "access_control",
	"aws_security_group_rule": "access_control",
}
