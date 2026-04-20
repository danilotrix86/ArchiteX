resource "aws_iam_role" "ops" {
  name               = "ops"
  assume_role_policy = "{}"
}

# NEW: attaches AdministratorAccess to the ops role.
resource "aws_iam_role_policy_attachment" "ops_admin" {
  role       = aws_iam_role.ops.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}
