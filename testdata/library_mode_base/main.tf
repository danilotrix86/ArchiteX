###############################################################################
# v1.3 PR2 -- library-mode base fixture.
#
# A typical "module-author repo": every resource is gated behind a
# `count = var.create ? 1 : 0`. Without library mode this entire block
# evaporates from the parser. With library mode (configured via the
# co-located .architex.yml) the parser materializes one phantom per gate.
###############################################################################

variable "create" {
  type    = bool
  default = true
}

variable "subnets" {
  type    = list(string)
  default = []
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
