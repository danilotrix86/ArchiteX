package registry

// This file owns every aws_* resource registration: supported types +
// abstract role + edge labels + attribute promoter. Adding a new AWS
// resource = one entry here (not one entry in models.go +
// models.AbstractionMap + graph.deriveAttributes + graph.edgeTypeMap).
//
// Registration order mirrors the pre-refactor map-literal order in
// models/models.go so iteration order (where it leaks into
// SupportedTypes / AbstractTypes maps and downstream callers that
// happen to range over them) stays stable. Same for edges.

func init() {
	registerAWSResources()
	registerAWSEdges()
}

func registerAWSResources() {
	// v1.0 -- canonical 3-tier VPC scope.
	Register(Resource{ProviderType: "aws_vpc", AbstractType: "network"})
	Register(Resource{ProviderType: "aws_subnet", AbstractType: "network"})
	Register(Resource{ProviderType: "aws_security_group", AbstractType: "access_control", Promoter: promoteSecurityGroup})
	Register(Resource{ProviderType: "aws_security_group_rule", AbstractType: "access_control", Promoter: promoteSecurityGroup})
	Register(Resource{ProviderType: "aws_instance", AbstractType: "compute", Promoter: promoteAWSInstance})
	Register(Resource{ProviderType: "aws_lb", AbstractType: "entry_point", Promoter: promotePublicTrue})
	Register(Resource{ProviderType: "aws_db_instance", AbstractType: "data", Promoter: promotePublicFalse})

	// v1.1 -- AWS Top 10 (Phase 6).
	Register(Resource{ProviderType: "aws_s3_bucket", AbstractType: "storage"})
	Register(Resource{ProviderType: "aws_s3_bucket_public_access_block", AbstractType: "access_control"})
	Register(Resource{ProviderType: "aws_s3_bucket_policy", AbstractType: "access_control", Promoter: promotePolicyJSON})
	Register(Resource{ProviderType: "aws_iam_role", AbstractType: "identity"})
	Register(Resource{ProviderType: "aws_iam_policy", AbstractType: "identity"})
	Register(Resource{ProviderType: "aws_iam_role_policy_attachment", AbstractType: "identity", Promoter: promoteIAMRolePolicyAttachment})
	Register(Resource{ProviderType: "aws_lambda_function", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_lambda_function_url", AbstractType: "entry_point", Promoter: promotePublicTrue})
	Register(Resource{ProviderType: "aws_apigatewayv2_api", AbstractType: "entry_point", Promoter: promotePublicTrue})
	Register(Resource{ProviderType: "aws_internet_gateway", AbstractType: "network", Promoter: promotePublicTrue})

	// v1.2 -- Coverage tranche 2 (Phase 7 PR4).
	Register(Resource{ProviderType: "aws_cloudfront_distribution", AbstractType: "entry_point", Promoter: promoteCloudFront})
	Register(Resource{ProviderType: "aws_route53_zone", AbstractType: "network"})
	Register(Resource{ProviderType: "aws_route53_record", AbstractType: "network"})
	Register(Resource{ProviderType: "aws_kms_key", AbstractType: "identity"})
	Register(Resource{ProviderType: "aws_kms_alias", AbstractType: "identity"})
	Register(Resource{ProviderType: "aws_sns_topic", AbstractType: "data"})
	Register(Resource{ProviderType: "aws_sns_topic_policy", AbstractType: "access_control", Promoter: promotePolicyJSON})
	Register(Resource{ProviderType: "aws_sqs_queue", AbstractType: "data"})
	Register(Resource{ProviderType: "aws_sqs_queue_policy", AbstractType: "access_control", Promoter: promotePolicyJSON})
	Register(Resource{ProviderType: "aws_nat_gateway", AbstractType: "network"})
	Register(Resource{ProviderType: "aws_network_acl", AbstractType: "access_control"})
	Register(Resource{ProviderType: "aws_network_acl_rule", AbstractType: "access_control", Promoter: promoteNACLRule})
	Register(Resource{ProviderType: "aws_secretsmanager_secret", AbstractType: "data"})
	Register(Resource{ProviderType: "aws_ebs_volume", AbstractType: "storage", Promoter: promoteEBSVolume})
	Register(Resource{ProviderType: "aws_ecs_cluster", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_ecs_service", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_ecs_task_definition", AbstractType: "compute"})

	// v1.3 -- Coverage tranche 3 (Phase 8). EKS family + RDS auxiliary
	// groups + EC2 launch template / ASG family.
	Register(Resource{ProviderType: "aws_eks_cluster", AbstractType: "compute", Promoter: promoteEKSCluster})
	Register(Resource{ProviderType: "aws_eks_node_group", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_eks_addon", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_eks_fargate_profile", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_eks_identity_provider_config", AbstractType: "identity"})
	Register(Resource{ProviderType: "aws_db_subnet_group", AbstractType: "network"})
	Register(Resource{ProviderType: "aws_db_parameter_group", AbstractType: "access_control"})
	Register(Resource{ProviderType: "aws_db_option_group", AbstractType: "access_control"})
	Register(Resource{ProviderType: "aws_launch_template", AbstractType: "compute"})
	Register(Resource{ProviderType: "aws_autoscaling_group", AbstractType: "compute", Promoter: promoteASG})
	Register(Resource{ProviderType: "aws_autoscaling_policy", AbstractType: "compute"})
}

func registerAWSEdges() {
	// v1.0 -- canonical 3-tier VPC scope.
	RegisterEdge("aws_instance", "aws_security_group", "attached_to")
	RegisterEdge("aws_instance", "aws_subnet", "deployed_in")
	RegisterEdge("aws_subnet", "aws_vpc", "part_of")
	RegisterEdge("aws_security_group_rule", "aws_security_group", "applies_to")
	RegisterEdge("aws_lb", "aws_subnet", "deployed_in")
	RegisterEdge("aws_lb", "aws_security_group", "attached_to")
	RegisterEdge("aws_db_instance", "aws_subnet", "deployed_in")
	RegisterEdge("aws_db_instance", "aws_security_group", "attached_to")
	RegisterEdge("aws_security_group", "aws_vpc", "part_of")

	// v1.1 -- Phase 6 (AWS Top 10).
	RegisterEdge("aws_s3_bucket_public_access_block", "aws_s3_bucket", "applies_to")
	RegisterEdge("aws_s3_bucket_policy", "aws_s3_bucket", "applies_to")
	RegisterEdge("aws_iam_role_policy_attachment", "aws_iam_role", "applies_to")
	RegisterEdge("aws_iam_role_policy_attachment", "aws_iam_policy", "applies_to")
	RegisterEdge("aws_lambda_function", "aws_iam_role", "attached_to")
	RegisterEdge("aws_lambda_function_url", "aws_lambda_function", "applies_to")
	RegisterEdge("aws_internet_gateway", "aws_vpc", "part_of")

	// v1.2 -- Phase 7 PR4 (Coverage tranche 2).
	RegisterEdge("aws_route53_record", "aws_route53_zone", "part_of")
	RegisterEdge("aws_kms_alias", "aws_kms_key", "applies_to")
	RegisterEdge("aws_sns_topic_policy", "aws_sns_topic", "applies_to")
	RegisterEdge("aws_sqs_queue_policy", "aws_sqs_queue", "applies_to")
	RegisterEdge("aws_nat_gateway", "aws_subnet", "deployed_in")
	RegisterEdge("aws_network_acl", "aws_vpc", "part_of")
	RegisterEdge("aws_network_acl", "aws_subnet", "applies_to")
	RegisterEdge("aws_network_acl_rule", "aws_network_acl", "applies_to")
	RegisterEdge("aws_ecs_service", "aws_ecs_cluster", "deployed_in")
	RegisterEdge("aws_ecs_service", "aws_ecs_task_definition", "uses")
	RegisterEdge("aws_ecs_service", "aws_lb", "attached_to")
	RegisterEdge("aws_ecs_task_definition", "aws_iam_role", "attached_to")
	RegisterEdge("aws_cloudfront_distribution", "aws_lb", "attached_to")
	RegisterEdge("aws_cloudfront_distribution", "aws_s3_bucket", "attached_to")

	// v1.3 -- Phase 8 (Coverage tranche 3).
	RegisterEdge("aws_eks_node_group", "aws_eks_cluster", "part_of")
	RegisterEdge("aws_eks_fargate_profile", "aws_eks_cluster", "part_of")
	RegisterEdge("aws_eks_addon", "aws_eks_cluster", "applies_to")
	RegisterEdge("aws_eks_identity_provider_config", "aws_eks_cluster", "applies_to")
	RegisterEdge("aws_eks_cluster", "aws_iam_role", "attached_to")
	RegisterEdge("aws_eks_node_group", "aws_iam_role", "attached_to")
	RegisterEdge("aws_eks_node_group", "aws_subnet", "deployed_in")
	RegisterEdge("aws_eks_fargate_profile", "aws_iam_role", "attached_to")
	RegisterEdge("aws_eks_fargate_profile", "aws_subnet", "deployed_in")
	RegisterEdge("aws_db_instance", "aws_db_subnet_group", "deployed_in")
	RegisterEdge("aws_db_instance", "aws_db_parameter_group", "applies_to")
	RegisterEdge("aws_db_instance", "aws_db_option_group", "applies_to")
	RegisterEdge("aws_autoscaling_group", "aws_launch_template", "uses")
	RegisterEdge("aws_autoscaling_group", "aws_subnet", "deployed_in")
	RegisterEdge("aws_autoscaling_group", "aws_security_group", "attached_to")
	RegisterEdge("aws_autoscaling_policy", "aws_autoscaling_group", "applies_to")
	RegisterEdge("aws_launch_template", "aws_security_group", "attached_to")
}

// promotePublicTrue is the canonical "if it exists, it is public"
// promoter shared by aws_lb, aws_lambda_function_url,
// aws_apigatewayv2_api, and aws_internet_gateway. The new_entry_point
// rule reads `public=true` to fire on add.
func promotePublicTrue(_ map[string]any) map[string]any {
	return map[string]any{"public": true}
}

// promotePublicFalse is the canonical "this resource has no
// public-by-itself surface" promoter shared by aws_db_instance and
// the policy attachments below (which then layer their own
// passthroughs on top).
func promotePublicFalse(_ map[string]any) map[string]any {
	return map[string]any{"public": false}
}

// promoteSecurityGroup computes `public` from the cidr_blocks
// attribute -- shared between aws_security_group and
// aws_security_group_rule (an SG rule's CIDR is what actually opens
// the world; the SG itself just collects rules).
func promoteSecurityGroup(attrs map[string]any) map[string]any {
	return map[string]any{"public": hasCIDRAllTraffic(attrs)}
}

// promoteAWSInstance reads associate_public_ip_address. A literal
// `true` makes the instance public; anything else (literal `false`,
// missing, var-driven) is private, mirroring AWS provider defaults.
func promoteAWSInstance(attrs map[string]any) map[string]any {
	pub := false
	if v, ok := attrs["associate_public_ip_address"]; ok {
		if b, ok := v.(bool); ok {
			pub = b
		}
	}
	return map[string]any{"public": pub}
}

// promoteCloudFront marks every distro public (see promotePublicTrue
// for the rule contract) and additionally passes `web_acl_id` through
// when literal so the cloudfront_no_waf rule can read it without
// re-parsing.
func promoteCloudFront(attrs map[string]any) map[string]any {
	out := map[string]any{"public": true}
	if v, ok := attrs["web_acl_id"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out["web_acl_id"] = s
		}
	}
	return out
}

// promoteIAMRolePolicyAttachment passes `policy_arn` through when
// captured as a literal string, so the iam_admin_policy_attached rule
// can inspect it without re-parsing. Variable-driven ARNs land as nil
// and are intentionally NOT promoted -- we never guess at unresolved
// expressions.
func promoteIAMRolePolicyAttachment(attrs map[string]any) map[string]any {
	out := map[string]any{"public": false}
	if v, ok := attrs["policy_arn"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out["policy_arn"] = s
		}
	}
	return out
}

// promotePolicyJSON passes the resolved `policy` JSON literal through
// to the graph node so the s3_bucket_public_exposure /
// messaging_topic_public rules can inspect Statement[].Effect. The
// parser resolves `policy = jsonencode({...})` when the inner value is
// a literal. Variable-driven policies land as nil and are
// intentionally NOT promoted -- the rule then conservatively fires.
func promotePolicyJSON(attrs map[string]any) map[string]any {
	out := map[string]any{"public": false}
	if v, ok := attrs["policy"]; ok {
		if s, ok := v.(string); ok && s != "" {
			out["policy"] = s
		}
	}
	return out
}

// promoteEBSVolume passes `encrypted` through when it was a literal
// bool. A missing attribute lands as nil; the ebs_unencrypted rule
// treats unresolved as "trust the user did the right thing" (no false
// positive on var.encrypted-style indirection). Only an explicit
// `encrypted = false` fires.
func promoteEBSVolume(attrs map[string]any) map[string]any {
	out := map[string]any{"public": false}
	if v, ok := attrs["encrypted"]; ok {
		if b, ok := v.(bool); ok {
			out["encrypted"] = b
		}
	}
	return out
}

// promoteNACLRule passes the trio (cidr_block, rule_action, egress)
// through only when literal so nacl_allow_all_ingress has a clean
// view of whether the rule opens the world inbound.
func promoteNACLRule(attrs map[string]any) map[string]any {
	out := map[string]any{"public": false}
	for _, k := range []string{"cidr_block", "rule_action"} {
		if v, ok := attrs[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				out[k] = s
			}
		}
	}
	if v, ok := attrs["egress"]; ok {
		if b, ok := v.(bool); ok {
			out["egress"] = b
		}
	}
	return out
}

// promoteEKSCluster reads vpc_config.endpoint_public_access (the
// parser promotes nested-block attrs as `<blockType>.<attrName>`) and
// also passes through endpoint_public_access_cidrs and
// enabled_cluster_log_types when they are literal lists with
// elements. Variable-driven values land as missing and the rule
// treats them as such.
func promoteEKSCluster(attrs map[string]any) map[string]any {
	out := map[string]any{}
	pub := false
	if v, ok := attrs["vpc_config.endpoint_public_access"]; ok {
		if b, ok := v.(bool); ok {
			out["endpoint_public_access"] = b
			pub = b
		}
	}
	out["public"] = pub
	if v, ok := attrs["vpc_config.endpoint_public_access_cidrs"]; ok {
		if lst, ok := v.([]any); ok && len(lst) > 0 {
			out["endpoint_public_access_cidrs"] = lst
		}
	}
	if v, ok := attrs["enabled_cluster_log_types"]; ok {
		if lst, ok := v.([]any); ok && len(lst) > 0 {
			out["enabled_cluster_log_types"] = lst
		}
	}
	return out
}

// promoteASG passes max_size / min_size through when they were
// literal numbers so asg_unrestricted_scaling can read them without
// re-parsing. Variable-driven values land as nil and the rule stays
// silent (project rule: never guess at unresolved expressions).
func promoteASG(attrs map[string]any) map[string]any {
	out := map[string]any{"public": false}
	for _, k := range []string{"max_size", "min_size"} {
		if v, ok := attrs[k]; ok {
			if f, ok := v.(float64); ok {
				out[k] = f
			}
		}
	}
	return out
}

// hasCIDRAllTraffic checks if any cidr_blocks attribute contains
// "0.0.0.0/0". Lifted verbatim from graph/graph.go so the SG
// promoters keep their pre-refactor behavior bit-for-bit.
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
