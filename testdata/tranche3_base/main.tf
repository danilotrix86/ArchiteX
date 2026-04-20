// Phase 8 / v1.3 baseline. Each resource is the safe form; the matching
// head fixture introduces the unsafe variant for every tranche-3 rule:
//   - eks_public_endpoint    (EKS cluster added with public endpoint, no CIDR)
//   - eks_no_logging         (EKS cluster added with no log types)
//   - asg_unrestricted_scaling (ASG added with max_size > 100 and min_size = 0)

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
