// Phase 7 PR4 / v1.2 head state. Adds resources that trigger every new
// tranche-2 rule (cloudfront_no_waf, ebs_volume_unencrypted,
// messaging_topic_public, nacl_allow_all_ingress) PLUS the existing
// new_entry_point rule on CF, with no other changes from the baseline.

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "main" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}

resource "aws_kms_key" "main" {
  description = "tranche2-baseline-key"
}

resource "aws_secretsmanager_secret" "db" {
  name       = "tranche2-db-creds"
  kms_key_id = aws_kms_key.main.id
}

resource "aws_ecs_cluster" "main" {
  name = "tranche2-cluster"
}

// --- Newly introduced resources ---

resource "aws_cloudfront_distribution" "edge" {
  enabled             = true
  default_root_object = "index.html"
  // intentionally NO web_acl_id -> cloudfront_no_waf fires
}

resource "aws_ebs_volume" "data" {
  availability_zone = "us-east-1a"
  size              = 50
  encrypted         = false
}

resource "aws_sns_topic" "alerts" {
  name = "tranche2-alerts"
}

resource "aws_sns_topic_policy" "alerts" {
  arn    = aws_sns_topic.alerts.arn
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect    = "Allow"
        Principal = "*"
        Action    = "sns:Publish"
        Resource  = "*"
      },
    ]
  })
}

resource "aws_network_acl" "open" {
  vpc_id     = aws_vpc.main.id
  subnet_ids = [aws_subnet.main.id]
}

resource "aws_network_acl_rule" "open_inbound" {
  network_acl_id = aws_network_acl.open.id
  rule_number    = 100
  egress         = false
  protocol       = "-1"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
}
