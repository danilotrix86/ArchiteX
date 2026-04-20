// Phase 8 / v1.3 fixture -- exercises every tranche-3 resource type in
// isolation. Used by parser_test.go to verify each type parses, abstracts,
// and emits the expected derived attributes with zero unsupported-resource
// warnings.

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "main" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}

resource "aws_security_group" "nodes" {
  vpc_id = aws_vpc.main.id
}

// --- EKS family ---

resource "aws_iam_role" "cluster" {
  name = "tranche3-eks-cluster"
}

resource "aws_iam_role" "nodes" {
  name = "tranche3-eks-nodes"
}

resource "aws_eks_cluster" "main" {
  name     = "tranche3-cluster"
  role_arn = aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids              = [aws_subnet.main.id]
    endpoint_public_access  = true
    // Intentionally NO endpoint_public_access_cidrs and NO
    // enabled_cluster_log_types -- so eks_public_endpoint and
    // eks_no_logging fire when this fixture is used as the head side
    // of a delta.
  }
}

resource "aws_eks_node_group" "default" {
  cluster_name    = aws_eks_cluster.main.name
  node_group_name = "default"
  node_role_arn   = aws_iam_role.nodes.arn
  subnet_ids      = [aws_subnet.main.id]

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
  }
}

resource "aws_eks_addon" "vpc_cni" {
  cluster_name = aws_eks_cluster.main.name
  addon_name   = "vpc-cni"
}

resource "aws_eks_fargate_profile" "default" {
  cluster_name           = aws_eks_cluster.main.name
  fargate_profile_name   = "default"
  pod_execution_role_arn = aws_iam_role.nodes.arn
  subnet_ids             = [aws_subnet.main.id]
}

resource "aws_eks_identity_provider_config" "oidc" {
  cluster_name = aws_eks_cluster.main.name

  oidc {
    client_id                     = "sts.amazonaws.com"
    identity_provider_config_name = "oidc"
    issuer_url                    = "https://token.actions.githubusercontent.com"
  }
}

// --- RDS auxiliary groups ---

resource "aws_db_subnet_group" "main" {
  name       = "tranche3-db-subnet"
  subnet_ids = [aws_subnet.main.id]
}

resource "aws_db_parameter_group" "main" {
  name   = "tranche3-db-params"
  family = "postgres15"
}

resource "aws_db_option_group" "main" {
  name                 = "tranche3-db-options"
  engine_name          = "postgres"
  major_engine_version = "15"
}

// --- EC2 ASG family ---

resource "aws_launch_template" "app" {
  name          = "tranche3-app"
  instance_type = "t3.micro"
  vpc_security_group_ids = [aws_security_group.nodes.id]
}

resource "aws_autoscaling_group" "app" {
  name             = "tranche3-app-asg"
  max_size         = 200
  min_size         = 0
  desired_capacity = 1
  vpc_zone_identifier = [aws_subnet.main.id]

  launch_template {
    id = aws_launch_template.app.id
  }
}

resource "aws_autoscaling_policy" "scale_out" {
  name                   = "scale-out"
  autoscaling_group_name = aws_autoscaling_group.app.name
  adjustment_type        = "ChangeInCapacity"
  scaling_adjustment     = 1
  cooldown               = 60
}
