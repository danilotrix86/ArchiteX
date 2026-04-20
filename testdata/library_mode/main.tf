###############################################################################
# Phase 8 PR2 (v1.3) -- library-mode fixture.
#
# This fixture intentionally mirrors the canonical "module-author repo" shape:
# every resource is wrapped in a `count = var.<flag> ? <int> : 0` (or
# `length(var.X) > 0 ? 1 : 0`) gate. Without library mode the v1.2 parser
# warned-and-skipped every block and produced an empty graph. With library
# mode enabled the parser materializes ONE phantom per gate, marked
# Attributes["conditional"] = true, so the diagram and PR comment are
# meaningful while risk rules still refuse to score conditional nodes.
###############################################################################

variable "create_bucket" {
  type    = bool
  default = true
}

variable "create_role" {
  type    = bool
  default = true
}

variable "subnets" {
  type    = list(string)
  default = []
}

# var.X ? 1 : 0  -- the canonical gate.
resource "aws_s3_bucket" "data" {
  count  = var.create_bucket ? 1 : 0
  bucket = "library-mode-bucket"
}

# var.X ? 1 : 0  -- with a public-access toggle so we can also verify that
# the existing s3_bucket_public_exposure rule does NOT fire on phantoms.
resource "aws_s3_bucket_public_access_block" "data" {
  count                   = var.create_bucket ? 1 : 0
  bucket                  = "library-mode-bucket"
  block_public_acls       = false
  block_public_policy     = false
  ignore_public_acls      = false
  restrict_public_buckets = false
}

# local.X ? 1 : 0  -- locals are a supported variant.
locals {
  enable_role = true
}

resource "aws_iam_role" "task" {
  count              = local.enable_role ? 1 : 0
  name               = "library-mode-role"
  assume_role_policy = "{}"
}

# length(var.X) > 0 ? 1 : 0  -- the second canonical shape.
resource "aws_db_subnet_group" "shared" {
  count       = length(var.subnets) > 0 ? 1 : 0
  name        = "library-mode-shared"
  subnet_ids  = var.subnets
  description = "library-mode conditional"
}
