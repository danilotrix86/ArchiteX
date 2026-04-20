# 01 — Public ALB introduced

A developer adds an internet-facing ALB and opens a security group from `10.0.0.0/16` to `0.0.0.0/0`. This is the canonical "public exposure" scenario.

## Expected output

- `public_exposure_introduced` (4.0) — security group flipped from private to public
- `new_entry_point` (3.0) — load balancer is a new entry point
- `potential_data_exposure` (2.0) — exposure landed alongside an access-control change

**Total: 9.0 / 10 — `high` / `fail`**

## Run

```bash
./architex report ./examples/01-public-alb/base ./examples/01-public-alb/head --out ./.architex/
```
