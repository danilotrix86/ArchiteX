// Phase 8 / v1.3 head state. Adds resources that trigger every new
// tranche-3 rule (eks_public_endpoint, eks_no_logging,
// asg_unrestricted_scaling) PLUS the existing new_entry_point semantics
// on the EKS cluster (its endpoint goes public).

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

resource "aws_iam_role" "cluster" {
  name = "tranche3-base-cluster"
}

// --- Newly introduced resources ---

resource "aws_eks_cluster" "open" {
  name     = "tranche3-open"
  role_arn = aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids             = [aws_subnet.main.id]
    endpoint_public_access = true
    // intentionally NO endpoint_public_access_cidrs -> eks_public_endpoint fires
    // intentionally NO enabled_cluster_log_types     -> eks_no_logging fires
  }
}

resource "aws_launch_template" "app" {
  name          = "tranche3-app"
  instance_type = "t3.micro"
  vpc_security_group_ids = [aws_security_group.nodes.id]
}

resource "aws_autoscaling_group" "runaway" {
  name                = "tranche3-runaway"
  max_size            = 250
  min_size            = 0
  desired_capacity    = 1
  vpc_zone_identifier = [aws_subnet.main.id]

  launch_template {
    id = aws_launch_template.app.id
  }
}
