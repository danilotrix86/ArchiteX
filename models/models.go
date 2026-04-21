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
//
// The v1.0 set was the canonical 3-tier VPC scope (VPC/subnet/SG/SG-rule/
// EC2/ALB/RDS). v1.1 ("AWS Top 10", Phase 6) adds 10 more types covering
// S3, IAM, Lambda, API Gateway v2, and the Internet Gateway -- the
// resources most commonly seen in real AWS Terraform PRs. v1.2 ("Coverage
// tranche 2", Phase 7 PR4) adds CloudFront, Route53, KMS, SNS, SQS, NAT
// Gateway, NACL, Secrets Manager, EBS, and ECS.
var SupportedResources = map[string]bool{
	// v1.0 -- canonical 3-tier VPC scope
	"aws_vpc":                 true,
	"aws_subnet":              true,
	"aws_security_group":      true,
	"aws_security_group_rule": true,
	"aws_instance":            true,
	"aws_lb":                  true,
	"aws_db_instance":         true,

	// v1.1 -- AWS Top 10 expansion (Phase 6)
	"aws_s3_bucket":                     true,
	"aws_s3_bucket_public_access_block": true,
	"aws_s3_bucket_policy":              true,
	"aws_iam_role":                      true,
	"aws_iam_policy":                    true,
	"aws_iam_role_policy_attachment":    true,
	"aws_lambda_function":               true,
	"aws_lambda_function_url":           true,
	"aws_apigatewayv2_api":              true,
	"aws_internet_gateway":              true,

	// v1.2 -- Coverage tranche 2 (Phase 7 PR4)
	"aws_cloudfront_distribution": true,
	"aws_route53_zone":            true,
	"aws_route53_record":          true,
	"aws_kms_key":                 true,
	"aws_kms_alias":               true,
	"aws_sns_topic":               true,
	"aws_sns_topic_policy":        true,
	"aws_sqs_queue":               true,
	"aws_sqs_queue_policy":        true,
	"aws_nat_gateway":             true,
	"aws_network_acl":             true,
	"aws_network_acl_rule":        true,
	"aws_secretsmanager_secret":   true,
	"aws_ebs_volume":              true,
	"aws_ecs_cluster":             true,
	"aws_ecs_service":             true,
	"aws_ecs_task_definition":     true,

	// v1.3 -- Coverage tranche 3 (Phase 8). EKS family covers the #1
	// missing resource cluster from the v1.2 real-world validation
	// sweep (`docs/v1.2-validation-findings.md`); RDS subnet/parameter/
	// option groups close the persistent-data gap; EC2 launch templates
	// + autoscaling group + autoscaling policy cover the modern compute
	// substrate (any EKS managed-node group / non-trivial EC2 fleet
	// uses these).
	"aws_eks_cluster":                  true,
	"aws_eks_node_group":               true,
	"aws_eks_addon":                    true,
	"aws_eks_fargate_profile":          true,
	"aws_eks_identity_provider_config": true,
	"aws_db_subnet_group":              true,
	"aws_db_parameter_group":           true,
	"aws_db_option_group":              true,
	"aws_launch_template":              true,
	"aws_autoscaling_group":            true,
	"aws_autoscaling_policy":           true,

	// v1.4 -- Azure tranche-0 (Phase 9). First-class Azure (azurerm)
	// support. Mirrors the AWS v1.0 canonical 3-tier scope: a virtual
	// network with subnets, a network-security boundary (NSG + rule),
	// public IPs and load balancers as the entry-point edge, Linux /
	// Windows VMs as compute, a storage account as object-storage at
	// rest, and an MSSQL server + database as the data tier. Network
	// interfaces are included so VM topology renders faithfully (a VM
	// without a NIC looks stranded in the diagram).
	//
	// `azurerm_resource_group` is INTENTIONALLY excluded -- it is purely
	// organizational, has no architectural-review value, and including
	// it would clutter every Azure diagram with an inert root node.
	// References to it from other resources simply do not produce edges
	// (same warn-and-skip behavior as `var.*` / `data.*` references).
	"azurerm_virtual_network":         true,
	"azurerm_subnet":                  true,
	"azurerm_public_ip":               true,
	"azurerm_network_security_group":  true,
	"azurerm_network_security_rule":   true,
	"azurerm_network_interface":       true,
	"azurerm_linux_virtual_machine":   true,
	"azurerm_windows_virtual_machine": true,
	"azurerm_lb":                      true,
	"azurerm_storage_account":         true,
	"azurerm_mssql_server":            true,
	"azurerm_mssql_database":          true,
}

// AbstractionMap maps AWS provider types to generic architecture types.
//
// Abstract types in v1.0: compute, data, entry_point, network, access_control.
// Phase 6 introduces two new abstract types so the new resources still slot
// into a small, opinionated vocabulary instead of inflating to one type per
// provider:
//
//   - storage  -- S3 buckets (object storage at rest)
//   - identity -- IAM roles / policies / attachments (principals + permissions)
var AbstractionMap = map[string]string{
	// v1.0
	"aws_instance":            "compute",
	"aws_db_instance":         "data",
	"aws_lb":                  "entry_point",
	"aws_vpc":                 "network",
	"aws_subnet":              "network",
	"aws_security_group":      "access_control",
	"aws_security_group_rule": "access_control",

	// v1.1 -- Phase 6
	"aws_s3_bucket":                     "storage",
	"aws_s3_bucket_public_access_block": "access_control",
	"aws_s3_bucket_policy":              "access_control",
	"aws_iam_role":                      "identity",
	"aws_iam_policy":                    "identity",
	"aws_iam_role_policy_attachment":    "identity",
	"aws_lambda_function":               "compute",
	"aws_lambda_function_url":           "entry_point",
	"aws_apigatewayv2_api":              "entry_point",
	"aws_internet_gateway":              "network",

	// v1.2 -- Phase 7 PR4 (Coverage tranche 2). No new abstract types are
	// introduced -- everything slots into the seven established roles. KMS
	// keys/aliases are "identity" because their key policies act like IAM
	// policies (subject access control). SNS/SQS topics/queues are "data"
	// because they hold messages at rest. SNS/SQS *policies*, NACLs, and
	// NACL rules are "access_control" because they govern WHO may touch
	// the underlying resource. Secrets Manager is "data" (sensitive
	// payload at rest, sibling to a DB). EBS volumes are "storage". ECS
	// resources are "compute".
	"aws_cloudfront_distribution": "entry_point",
	"aws_route53_zone":            "network",
	"aws_route53_record":          "network",
	"aws_kms_key":                 "identity",
	"aws_kms_alias":               "identity",
	"aws_sns_topic":               "data",
	"aws_sns_topic_policy":        "access_control",
	"aws_sqs_queue":               "data",
	"aws_sqs_queue_policy":        "access_control",
	"aws_nat_gateway":             "network",
	"aws_network_acl":             "access_control",
	"aws_network_acl_rule":        "access_control",
	"aws_secretsmanager_secret":   "data",
	"aws_ebs_volume":              "storage",
	"aws_ecs_cluster":             "compute",
	"aws_ecs_service":             "compute",
	"aws_ecs_task_definition":     "compute",

	// v1.3 -- Phase 8 (Coverage tranche 3). No new abstract types. EKS
	// clusters / node groups / fargate profiles are "compute" (their
	// purpose is to run workloads). EKS addons are "compute" too --
	// they ship as cluster-attached compute units (CoreDNS, kube-proxy,
	// vpc-cni). EKS identity provider configs are "identity" because
	// they govern who may assume cluster RBAC roles. RDS group
	// resources split by what they govern: subnet groups are "network"
	// (where the DB lives), parameter and option groups are
	// "access_control" (they govern engine knobs and connection
	// behavior, including TLS / auth options). Launch templates and
	// ASGs are "compute" siblings of aws_instance / aws_ecs_service.
	"aws_eks_cluster":                  "compute",
	"aws_eks_node_group":               "compute",
	"aws_eks_addon":                    "compute",
	"aws_eks_fargate_profile":          "compute",
	"aws_eks_identity_provider_config": "identity",
	"aws_db_subnet_group":              "network",
	"aws_db_parameter_group":           "access_control",
	"aws_db_option_group":              "access_control",
	"aws_launch_template":              "compute",
	"aws_autoscaling_group":            "compute",
	"aws_autoscaling_policy":           "compute",

	// v1.4 -- Phase 9 (Azure tranche-0). No new abstract types are
	// introduced -- every Azure resource slots into one of the seven
	// established roles. azurerm_lb and azurerm_public_ip are
	// "entry_point" / "network" respectively; even though both can
	// front public traffic, the LB is the architectural ingress and
	// the public IP is the address-plane attachment that may flip
	// from private to public. azurerm_network_interface is "network"
	// because it is the wiring layer between VMs, subnets, and NSGs.
	// azurerm_mssql_server is "data" alongside the database itself --
	// the server is the public-network-access boundary, the database
	// is the per-tenant data resource. azurerm_storage_account is
	// "storage" (object storage at rest, sibling of S3).
	"azurerm_virtual_network":         "network",
	"azurerm_subnet":                  "network",
	"azurerm_public_ip":               "network",
	"azurerm_network_security_group":  "access_control",
	"azurerm_network_security_rule":   "access_control",
	"azurerm_network_interface":       "network",
	"azurerm_linux_virtual_machine":   "compute",
	"azurerm_windows_virtual_machine": "compute",
	"azurerm_lb":                      "entry_point",
	"azurerm_storage_account":         "storage",
	"azurerm_mssql_server":            "data",
	"azurerm_mssql_database":          "data",
}
