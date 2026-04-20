resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "private_a" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.10.0/24"
}

resource "aws_subnet" "private_b" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.11.0/24"
}

resource "aws_iam_role" "cluster" {
  name               = "eks-cluster-role"
  assume_role_policy = "{}"
}

# NEW: EKS cluster with a wide-open API endpoint and no logging.
resource "aws_eks_cluster" "control" {
  name     = "platform-control"
  role_arn = aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids             = [aws_subnet.private_a.id, aws_subnet.private_b.id]
    endpoint_public_access = true
  }
}
