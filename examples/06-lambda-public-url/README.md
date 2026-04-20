# 06 — Public Lambda URL introduced

A team adds a Lambda function and exposes it via a Lambda Function URL. The function is now reachable from the internet without an API Gateway in front.

## Expected output

- `new_entry_point` (3.0) — `aws_lambda_function_url` is an entry point
- `lambda_public_url_introduced` (3.0) — explicit, dedicated rule that layers on top

**Total: 6.0 / 10 — `medium` / `warn`**

## Run

```bash
./architex report ./examples/06-lambda-public-url/base ./examples/06-lambda-public-url/head --out ./.architex/
```
