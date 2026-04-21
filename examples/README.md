# ArchiteX examples gallery

Each subdirectory is a **reviewer-grade scenario** — a `base/` snapshot and a `head/` snapshot, with a short `README.md` describing what the change demonstrates and what risk rules ArchiteX should fire.

Every example here doubles as a CI regression check: the selftest workflow runs `architex report base head` against each one and asserts that the expected rule IDs land in `score.json`. If any example silently changes its output, the build fails.

## Run an example locally

```bash
go build -o architex .
./architex report ./examples/01-public-alb/base ./examples/01-public-alb/head --out ./.architex/
```

Open the resulting `.architex/<bundle>/report.html` in any browser (no JavaScript, no CDN).

## The gallery

| # | Scenario | Demonstrates | Expected score |
|---|---|---|---:|
| [01](./01-public-alb/) | Public ALB introduced | `new_entry_point` (3.0) + `public_exposure_introduced` (4.0) + `potential_data_exposure` (2.0) | 9.0 / **high** |
| [02](./02-eks-public-endpoint/) | EKS cluster with open API + no logging | `eks_public_endpoint` (3.5) + `eks_no_logging` (1.5) + `new_entry_point`-class signals | 5.0+ / **medium** |
| [03](./03-iam-admin-attachment/) | `AdministratorAccess` attached to a role | `iam_admin_policy_attached` (3.5) | 3.5 / **medium** |
| [04](./04-cloudfront-no-waf/) | CloudFront added without a WAF | `cloudfront_no_waf` (2.5) + `new_entry_point` (3.0) | 5.5 / **medium** |
| [05](./05-library-mode/) | Module-author repo: `count = var.create ? 1 : 0` | **No rules fire** — phantoms are correctly silent | 0.0 / **low** |
| [06](./06-lambda-public-url/) | Public Lambda URL introduced | `lambda_public_url_introduced` (3.0) + `new_entry_point` (3.0) | 6.0 / **medium** |
| [07](./07-azure-public-lb/) | **Azure** public LB + open NSG (v1.4) | `nsg_allow_all_ingress` (3.5) + `new_entry_point` (3.0) | 6.5 / **medium** |

## What "deterministic" looks like

Run the same command twice. The bundle's `score.json` will be byte-identical. The Mermaid diagram will be byte-identical. Open a PR with the same diff next month: same output. ArchiteX has **no inference, no model calls, no time-of-day variance**.
