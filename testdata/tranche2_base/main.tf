// Phase 7 PR4 / v1.2 baseline. Each resource is the safe form; the matching
// head fixture introduces the unsafe variant for every tranche-2 rule:
//   - cloudfront_no_waf       (CF distro added without web_acl_id)
//   - ebs_volume_unencrypted  (EBS volume added with encrypted=false)
//   - messaging_topic_public  (SNS topic policy added with Principal="*")
//   - nacl_allow_all_ingress  (NACL rule added with cidr=0.0.0.0/0)

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
