# Changelog

All notable changes to ArchiteX are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2026-04-19

The "AWS Top 10" release. Adds 10 new resource types, two new abstract types,
and three new risk rules covering S3 public exposure, IAM admin attachment,
and Lambda public function URLs. Backward compatible with v1.0: the canonical
`testdata/base` -> `testdata/head` fixture still scores 9.0/10 (locked by
`TestEvaluate_HighRiskFixture_NoRegression`).

### Added

- 10 new supported resource types:
  - **Storage:** `aws_s3_bucket`, `aws_s3_bucket_public_access_block`,
    `aws_s3_bucket_policy`
  - **Identity:** `aws_iam_role`, `aws_iam_policy`,
    `aws_iam_role_policy_attachment`
  - **Compute:** `aws_lambda_function`
  - **Entry:** `aws_lambda_function_url`, `aws_apigatewayv2_api`
  - **Network:** `aws_internet_gateway`
- Two new abstract types: `storage` (S3 buckets) and `identity` (IAM
  principals + permissions). Both get explicit ranks in the Mermaid diagram
  budget tiebreaker so node truncation stays deterministic.
- 7 new explicit edge relationships (`applies_to`, `attached_to`, `part_of`)
  between Phase 6 resources, e.g. `aws_iam_role_policy_attachment ->
  aws_iam_role`, `aws_lambda_function -> aws_iam_role`,
  `aws_internet_gateway -> aws_vpc`.
- Three new risk rules:
  - **`s3_bucket_public_exposure`** (4.0): triggers when an
    `aws_s3_bucket_public_access_block` is removed OR an
    `aws_s3_bucket_policy` is added. Per-resource signal model -- we do not
    walk the graph to attribute each signal to a specific bucket.
  - **`iam_admin_policy_attached`** (3.5): triggers when an
    `aws_iam_role_policy_attachment` is added with a literal `policy_arn`
    ending in `:policy/AdministratorAccess` or `:policy/IAMFullAccess`.
    Variable-driven ARNs are intentionally NOT matched.
  - **`lambda_public_url_introduced`** (3.0): triggers per added
    `aws_lambda_function_url`. Layers on top of the existing
    `new_entry_point` rule so a Lambda URL alone scores 6.0 (medium) and
    immediately upgrades to high when paired with admin/IAM signals.
- Each new rule emits at most 2 reasons per evaluation (`phase6CapPerRule`)
  so a sweeping refactor cannot single-handedly saturate the 10.0 cap.
- `aws_iam_role_policy_attachment` graph nodes now carry a literal
  `policy_arn` attribute when the parser captured one, so the
  `iam_admin_policy_attached` rule can inspect it without re-parsing.
- New testdata fixtures:
  - `testdata/top10_resources/` -- one Terraform module exercising every
    new resource type (parser contract test).
  - `testdata/top10_base/` + `testdata/top10_head/` -- end-to-end fixture
    that triggers all three new rules from a single base->head pair.

### Known limitations and caveats

- **`s3_bucket_public_exposure` false-positive caveat:** the rule fires on
  any `aws_s3_bucket_policy` addition, including policies that explicitly
  DENY public access. Reviewers can dismiss in the PR thread; cross-resource
  policy evaluation is planned for v1.2.
- `iam_admin_policy_attached` matches only literal policy ARNs. Attachments
  whose `policy_arn` is a variable reference (`var.admin_arn`) or comes
  from a `data.aws_iam_policy.x.arn` lookup will not match. This is a
  deliberate choice: the engine never guesses at unresolved expressions.
- Lambda URLs with `authorization_type = "AWS_IAM"` still trigger
  `lambda_public_url_introduced`. The URL itself is the entry point; the
  message asks the reviewer to verify the auth type.

[1.1.0]: https://github.com/danilotrix86/ArchiteX/releases/tag/v1.1.0

## [1.0.0] - 2026-04-19

Initial public release.

### Added

- HCL parsing of Terraform `.tf` files with cross-resource reference detection
  and confidence scoring.
- Architecture graph construction for 7 supported AWS resource types
  (`aws_vpc`, `aws_subnet`, `aws_security_group`, `aws_security_group_rule`,
  `aws_instance`, `aws_lb`, `aws_db_instance`).
- Semantic delta engine: graph-to-graph comparison producing added, removed,
  and changed nodes/edges with deterministic ordering.
- Deterministic risk engine with 5 built-in rules (`public_exposure_introduced`,
  `new_data_resource`, `new_entry_point`, `potential_data_exposure`,
  `resource_removed`) and a 0--10 severity score.
- Stage 4 interpreter: Mermaid delta diagram, plain-English summary,
  review-focus bullets, five-section Markdown PR comment, `EgressPayload`
  sanitization with salted SHA-256 ID hashing, and timestamped audit bundle
  with SHA-256 manifest checksums.
- Composite GitHub Action (`action.yml`) with sticky PR comment posting,
  audit-bundle artifact upload, advisory and blocking modes.
- Large-delta hardening: deterministic 45,000-byte Mermaid budget cap and
  240,000-byte comment-body safety net with visible truncation markers.
- 68+ unit tests across 6 packages.
- Published egress schema (`docs/egress-schema.json`, JSON Schema draft-07)
  with build-time parity test.

### Supported resource types

| Terraform type | Abstract type |
|---|---|
| `aws_vpc` | `network` |
| `aws_subnet` | `network` |
| `aws_security_group` | `access_control` |
| `aws_security_group_rule` | `access_control` |
| `aws_instance` | `compute` |
| `aws_lb` | `entry_point` |
| `aws_db_instance` | `data` |

### Known limitations

- AWS Terraform only; 7 resource types (see above).
- `module`, `for_each`, `count`, and `dynamic` blocks are warned and skipped.
- No user-configurable rules or thresholds (opinionated defaults only).
- Multi-provider, GitLab/Bitbucket, and non-Terraform IaC are out of scope.

[1.0.0]: https://github.com/danilotrix86/ArchiteX/releases/tag/v1.0.0
