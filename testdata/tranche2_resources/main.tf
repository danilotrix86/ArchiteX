// Phase 7 PR4 / v1.2 fixture -- exercises every tranche-2 resource type in
// isolation. Used by parser_test.go to verify each type parses, abstracts,
// and emits the expected derived attributes with zero unsupported-resource
// warnings.

resource "aws_cloudfront_distribution" "web" {
  enabled             = true
  default_root_object = "index.html"
  // Intentionally NO web_acl_id -- so the cloudfront_no_waf rule fires when
  // this fixture is used as the head side of a delta.
}

resource "aws_route53_zone" "main" {
  name = "example.com"
}

resource "aws_route53_record" "www" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "www.example.com"
  type    = "A"
  ttl     = 300
  records = ["192.0.2.1"]
}

resource "aws_kms_key" "main" {
  description = "tranche2-kms"
}

resource "aws_kms_alias" "main" {
  name          = "alias/tranche2-kms"
  target_key_id = aws_kms_key.main.id
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

resource "aws_sqs_queue" "jobs" {
  name = "tranche2-jobs"
}

resource "aws_sqs_queue_policy" "jobs" {
  queue_url = aws_sqs_queue.jobs.id
  policy    = jsonencode({
    Statement = [
      {
        Effect    = "Allow"
        Principal = { AWS = "arn:aws:iam::123456789012:role/app" }
        Action    = "sqs:SendMessage"
      },
    ]
  })
}

resource "aws_nat_gateway" "main" {
  allocation_id = "eipalloc-deadbeef"
  subnet_id     = aws_subnet.main.id
}

resource "aws_network_acl" "main" {
  vpc_id     = aws_vpc.main.id
  subnet_ids = [aws_subnet.main.id]
}

resource "aws_network_acl_rule" "open_inbound" {
  network_acl_id = aws_network_acl.main.id
  rule_number    = 100
  egress         = false
  protocol       = "-1"
  rule_action    = "allow"
  cidr_block     = "0.0.0.0/0"
}

resource "aws_secretsmanager_secret" "db" {
  name = "tranche2-db-creds"
}

resource "aws_ebs_volume" "data" {
  availability_zone = "us-east-1a"
  size              = 100
  encrypted         = false
}

resource "aws_ecs_cluster" "main" {
  name = "tranche2-cluster"
}

resource "aws_ecs_task_definition" "app" {
  family                = "tranche2-app"
  container_definitions = "[]"
}

resource "aws_ecs_service" "app" {
  name            = "tranche2-app"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = 1
}

// Anchors for cross-references above.
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "main" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
