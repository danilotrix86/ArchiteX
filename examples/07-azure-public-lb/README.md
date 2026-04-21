# 07 — Azure public load balancer + open NSG

A developer adds an internet-facing Azure load balancer (with a public IP) and opens the network-security group from `*` (any) inbound. This is the canonical Azure "public exposure" scenario — the cross-provider equivalent of [example 01 (public ALB)](../01-public-alb/).

## Expected output

- `nsg_allow_all_ingress` (3.5) — NSG rule allows inbound from `*`
- `new_entry_point` (3.0) — load balancer is a new entry point

**Total: 6.5 / 10 — `medium` / `warn`**

The PR comment also renders the v1.4 provider banner above the risk header:

> _Detected providers: azurerm — N resources analyzed._

confirming auto-detection ran on this Azure-only diff.

## Run

```bash
./architex report ./examples/07-azure-public-lb/base ./examples/07-azure-public-lb/head --out ./.architex/
```

## Why this matters

Public Azure load balancers + permissive NSG rules are the most common shape of accidental Azure exposure in PR review. ArchiteX surfaces both signals deterministically — same input, same output, every time — so reviewers can sign off in seconds.
