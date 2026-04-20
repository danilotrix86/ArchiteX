# 02 — EKS cluster with open API endpoint and no logging

A team adds a brand-new EKS cluster. The control-plane endpoint is public (`endpoint_public_access = true`) with **no CIDR allow-list**, and `enabled_cluster_log_types` is omitted.

## Expected output

- `eks_public_endpoint` (3.5) — public API surface with no IP scoping
- `eks_no_logging` (1.5) — control-plane activity will not be auditable

**Combined: 5.0 / 10 — `medium` / `warn`** (other rules can compound depending on your baseline).

## Run

```bash
./architex report ./examples/02-eks-public-endpoint/base ./examples/02-eks-public-endpoint/head --out ./.architex/
```
