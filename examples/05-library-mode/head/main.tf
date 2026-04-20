###############################################################################
# v1.3 PR2 -- library-mode head fixture.
#
# Adds a third gated resource (an EKS cluster with a public endpoint) AND
# changes one EXISTING resource's class. The risk engine MUST NOT score
# any of these (they are conditional phantoms); the selftest asserts:
#   - the diagram includes the new conditional phantom;
#   - the report risk score stays low because eks_public_endpoint must
#     refuse to fire on a phantom.
###############################################################################

variable "create" {
  type    = bool
  default = true
}

variable "subnets" {
  type    = list(string)
  default = []
}

variable "create_cluster" {
  type    = bool
  default = true
}

resource "aws_s3_bucket" "data" {
  count  = var.create ? 1 : 0
  bucket = "lm-data"
}

resource "aws_db_subnet_group" "shared" {
  count       = length(var.subnets) > 0 ? 1 : 0
  name        = "lm-shared"
  subnet_ids  = var.subnets
  description = "lm"
}

resource "aws_eks_cluster" "control" {
  count    = var.create_cluster ? 1 : 0
  name     = "lm-control"
  role_arn = "arn:aws:iam::000000000000:role/lm"

  vpc_config {
    subnet_ids              = []
    endpoint_public_access  = true
  }
}
