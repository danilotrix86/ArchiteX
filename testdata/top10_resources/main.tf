// Phase 6 / v1.1 fixture -- exercises every new resource type in isolation.
// Used by parser_test.go to verify each type parses, abstracts, and emits
// the expected derived attributes with zero unsupported-resource warnings.

resource "aws_s3_bucket" "logs" {
  bucket = "phase6-logs"
}

resource "aws_s3_bucket_public_access_block" "logs" {
  bucket                  = aws_s3_bucket.logs.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_policy" "logs" {
  bucket = aws_s3_bucket.logs.id
  policy = "{}"
}

resource "aws_iam_role" "lambda_exec" {
  name               = "phase6-lambda-exec"
  assume_role_policy = "{}"
}

resource "aws_iam_policy" "read_only" {
  name   = "phase6-read-only"
  policy = "{}"
}

resource "aws_iam_role_policy_attachment" "lambda_read_only" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = aws_iam_policy.read_only.arn
}

resource "aws_lambda_function" "worker" {
  function_name = "phase6-worker"
  role          = aws_iam_role.lambda_exec.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
}

resource "aws_lambda_function_url" "worker" {
  function_name      = aws_lambda_function.worker.function_name
  authorization_type = "NONE"
}

resource "aws_apigatewayv2_api" "http" {
  name          = "phase6-http"
  protocol_type = "HTTP"
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
}

// Required so aws_internet_gateway has a target node to point its `part_of`
// edge at. The VPC itself is already covered by v1.0 tests, so we keep it
// minimal here.
resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
