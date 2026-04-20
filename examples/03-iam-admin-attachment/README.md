# 03 — `AdministratorAccess` attached to a role

A PR adds an `aws_iam_role_policy_attachment` whose `policy_arn` is the AWS-managed `AdministratorAccess` policy. ArchiteX recognizes the literal ARN and flags it.

## Expected output

- `iam_admin_policy_attached` (3.5)

**Total: 3.5 / 10 — `medium` / `warn`**

The same rule fires for `IAMFullAccess`. The literal ARN is matched against an allowlist; variable-driven `policy_arn = var.x` does **not** fire (deterministic-first principle).

## Run

```bash
./architex report ./examples/03-iam-admin-attachment/base ./examples/03-iam-admin-attachment/head --out ./.architex/
```
