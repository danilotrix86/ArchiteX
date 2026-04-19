// Phase 6 / v1.1 head state.
//
// Diff vs top10_base:
//   - aws_s3_bucket_public_access_block.logs       -> REMOVED (triggers s3_bucket_public_exposure)
//   - aws_iam_role_policy_attachment.app_admin     -> ADDED with AdministratorAccess (triggers iam_admin_policy_attached)
//   - aws_lambda_function_url.worker               -> ADDED (triggers lambda_public_url_introduced AND new_entry_point)

resource "aws_s3_bucket" "logs" {
  bucket = "top10-logs"
}

// Public access block has been REMOVED -- this is the trigger for the
// s3_bucket_public_exposure rule under Phase 6's per-resource-signal model.

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

// NEW: full administrator access attached to the role used by the lambda.
// The string "AdministratorAccess" in policy_arn is what the
// iam_admin_policy_attached rule keys off (deterministic suffix match).
resource "aws_iam_role_policy_attachment" "app_admin" {
  role       = aws_iam_role.app.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

resource "aws_lambda_function" "worker" {
  function_name = "top10-worker"
  role          = aws_iam_role.app.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
}

// NEW: a public function URL. authorization_type=NONE makes it
// internet-callable; the rule fires regardless of the auth type because the
// URL itself is the new entry point.
resource "aws_lambda_function_url" "worker" {
  function_name      = aws_lambda_function.worker.function_name
  authorization_type = "NONE"
}
