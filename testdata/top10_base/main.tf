// Phase 6 / v1.1 baseline state.
//
// The baseline contains:
//   - A locked-down S3 bucket with a public_access_block (will be REMOVED in head).
//   - An IAM role with a benign read-only attachment (will gain AdministratorAccess in head).
//   - A Lambda function with NO function URL (URL will be ADDED in head).
//
// Together, the head's diff must trigger all three new Phase 6 risk rules:
//   - s3_bucket_public_exposure   (PAB removed)
//   - iam_admin_policy_attached   (AdministratorAccess attached)
//   - lambda_public_url_introduced (function URL added)

resource "aws_s3_bucket" "logs" {
  bucket = "top10-logs"
}

resource "aws_s3_bucket_public_access_block" "logs" {
  bucket                  = aws_s3_bucket.logs.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_iam_role" "app" {
  name               = "top10-app"
  assume_role_policy = "{}"
}

resource "aws_iam_policy" "read_only" {
  name   = "top10-read-only"
  policy = "{}"
}

resource "aws_iam_role_policy_attachment" "app_read_only" {
  role       = aws_iam_role.app.name
  policy_arn = aws_iam_policy.read_only.arn
}

resource "aws_lambda_function" "worker" {
  function_name = "top10-worker"
  role          = aws_iam_role.app.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
}
