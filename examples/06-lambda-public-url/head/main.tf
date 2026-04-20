resource "aws_iam_role" "fn" {
  name               = "fn-role"
  assume_role_policy = "{}"
}

resource "aws_lambda_function" "worker" {
  function_name = "background-worker"
  role          = aws_iam_role.fn.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
  filename      = "function.zip"
}

# NEW: public Lambda URL with no auth.
resource "aws_lambda_function_url" "worker_public" {
  function_name      = aws_lambda_function.worker.function_name
  authorization_type = "NONE"
}
