# Changelog

All notable changes to ArchiteX are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - 2026-04-20

The "Launch" release (Phase 8). Closes the highest-leverage coverage gap
from the v1.2 real-world validation sweep (EKS, Auto Scaling, RDS
auxiliary groups), introduces a new parser mode purpose-built for
module-author repos, ships an examples gallery wired into CI, an
auto-deployed GitHub Pages site with a live sample report, prebuilt
binaries for every release, and a complete launch kit.

Backward compatible with v1.2: a repo with no `parser.mode: library` in
its `.architex.yml` and no Tranche-3 resources MUST produce bit-identical
output to a v1.2 run. The default `parser.mode` is `consumer` (the v1.2
behavior).

### Added (PR1 — coverage tranche 3)

- Eleven new AWS resource types (45 total):
  - **EKS family** — `aws_eks_cluster`, `aws_eks_node_group`,
    `aws_eks_addon`, `aws_eks_fargate_profile`,
    `aws_eks_identity_provider_config`. Closes the #1 missing resource
    cluster from the v1.2 real-world validation sweep
    (`docs/v1.2-validation-findings.md`).
  - **EC2 Auto Scaling family** — `aws_launch_template`,
    `aws_autoscaling_group`, `aws_autoscaling_policy`. Covers the modern
    EC2 substrate (any non-trivial fleet uses these).
  - **RDS auxiliary groups** — `aws_db_subnet_group`,
    `aws_db_parameter_group`, `aws_db_option_group`. Closes the
    persistent-data gap.
- Three new risk rules (18 total):
  - `eks_public_endpoint` (weight 3.5) — fires when an added
    `aws_eks_cluster` has `vpc_config.endpoint_public_access = true` AND
    no `vpc_config.endpoint_public_access_cidrs` allow-list. Equal weight
    to `iam_admin_policy_attached` — a public EKS API surface without a
    CIDR allow-list is a textbook ransomware foothold.
  - `eks_no_logging` (weight 1.5) — fires when an added
    `aws_eks_cluster` has no `enabled_cluster_log_types`. Hygiene
    finding; stacks with `eks_public_endpoint` to 5.0 (medium tier).
  - `asg_unrestricted_scaling` (weight 1.0) — fires when an added
    `aws_autoscaling_group` has literal `max_size > 100` AND no
    `min_size` floor. Runaway-cost / stampede primitive.
- New edge types: `part_of` (EKS node groups → cluster),
  `attached_to` / `deployed_in` / `applies_to` for the EKS, ASG, and
  RDS-aux family relationships.
- Tranche-3 selftest fixtures (`testdata/tranche3_base`,
  `testdata/tranche3_head`) wired into the GitHub Actions workflow as
  regression checks.

### Added (PR2 — library-mode parsing)

- New `parser.mode: library` setting in `.architex.yml`. Default
  remains `consumer` (the v1.2 behavior). Library mode is built for
  module-author repos where every resource is wrapped in
  `count = var.create ? 1 : 0`.
- The parser now recognizes the canonical "create-or-not" gate shapes:
  - `count = var.<name> ? <int> : <int>`
  - `count = local.<name> ? <int> : <int>`
  - `count = length(var.<name>) > 0 ? <int> : <int>`
- For each gate it materializes ONE phantom resource marked
  `Attributes["conditional"] = true`. Anything else (raw
  `count = var.replicas`, resource-driven gates, etc.) continues to
  warn-and-skip — the engine never invents resources from values it
  cannot resolve.
- Risk rules MUST refuse to score conditional phantoms. The
  `isConditionalNode` helper in `risk/rules_v13.go` is the single source
  of truth; all per-resource rules in `risk/rules.go`,
  `risk/rules_v12.go`, and `risk/rules_v13.go` consult it before firing.
- Mermaid diagrams render conditional phantoms with a leading `?`
  (e.g. `+ ? compute: aws_eks_cluster.control`) so reviewers can never
  confuse a phantom with a definite resource.
- New `testdata/library_mode_*` fixtures and selftest assertions:
  the diagram MUST contain the phantom and the score.json MUST NOT
  contain the rule that would have fired on a definite resource.

### Added (PR3 — marketing-grade README)

- Full README rebuild with hero section, comparison table vs.
  tfsec / Checkov / Terraform Cloud, "how it works" Mermaid diagram,
  v1.3 highlights, and a polished examples-gallery section.
- Suggested review focus messages now exist for every rule
  (`interpreter/summary.go`); the previous behavior of falling back to
  "No risk rules triggered" when only post-v1.0 rules fired is fixed.

### Added (PR4 — examples gallery)

- New `examples/` directory with six reviewer-grade scenarios:
  1. `01-public-alb` — canonical exposure, score 9.0 / high.
  2. `02-eks-public-endpoint` — EKS open API + no logging, score 5.0.
  3. `03-iam-admin-attachment` — `AdministratorAccess` attached, 3.5.
  4. `04-cloudfront-no-waf` — CloudFront with no WAF, 5.5.
  5. `05-library-mode` — module-author phantoms; 0.0, no rules fire.
  6. `06-lambda-public-url` — public Lambda URL, 6.0.
- Each example carries a `README.md` describing the scenario and
  expected output, plus a full `base/` and `head/` snapshot.
- The selftest workflow runs `architex report` against every example
  and asserts both the exact score and the must-fire / must-not-fire
  rule sets — anything that silently changes rendered output fails CI.

### Added (PR5 — GitHub Pages site)

- New zero-build landing page at `docs/site/index.html`. Self-contained
  HTML/CSS, no JavaScript, no remote fonts.
- New `.github/workflows/pages.yml` deploys the site to GitHub Pages on
  every push to `main`, embedding a freshly-rendered sample
  `report.html` from `examples/01-public-alb` so visitors can see what
  the report looks like without installing anything.

### Added (PR6 — prebuilt binaries via goreleaser)

- New `.goreleaser.yaml` cross-compiles the CLI for
  linux/darwin/windows on amd64+arm64 (skipping windows-arm64 which is
  not a primary target).
- Release workflow updated to invoke goreleaser. Each tagged release
  now ships archives + `checksums.txt` (SHA-256) attached to the
  GitHub release page.
- New `--version` / `version` subcommand on the CLI. The
  `version`, `commit`, and `date` ldflags are populated by goreleaser at
  build time; `dev` / `none` / `unknown` for `go build` callers.

### Added (PR7 — announcement post)

- New `docs/v1.3-announcement.md` — long-form release announcement that
  doubles as the seed for the GitHub Discussions launch post and any
  external write-up.

### Changed

- `interpreter/summary.go`: every rule that ships in v1.0 / v1.1 / v1.2 /
  v1.3 now has a dedicated review-focus message. The previous behavior
  silently fell back to the generic "No risk rules triggered" message
  for any rule that wasn't on the v1.0 hand-curated list.
- All per-resource risk rules in `risk/rules.go`, `risk/rules_v12.go`,
  and `risk/rules_v13.go` now consult `isConditionalNode` and silently
  refuse to score conditional phantoms. This is the load-bearing
  contract that makes library mode safe.

### Coverage

- 45 AWS resource types (was 34).
- 18 deterministic risk rules (was 15).
- 6 reviewer-grade examples (was 0; previously testdata was the only
  source of truth).

## [1.2.0] - 2026-04-20

The "Depth & Configurability" release (Phase 7). Expands what the parser
can resolve from literal Terraform constructs, doubles AWS resource
coverage to 34 types, adds 7 new risk rules (4 from PR4 + 3 baseline
anomaly rules from PR5), gives users a first-class way to customize and
silence findings without forking the binary, ships a self-contained
`report.html` in every audit bundle, and bumps the audit bundle layout
to ToolVersion `0.5.0`.

Backward compatible with v1.1: a repo with no `.architex.yml`, no inline
`# architex:ignore=` directives, and no `.architex/baseline.json` MUST
produce bit-identical output to a v1.1 run (locked by
`TestEvaluateWith_NilConfig_BehavesAsV11`,
`TestEvaluateWithBaseline_NilBaseline_NoFirstTimeReasons`, and the
existing high-risk fixture regression tests).

### Added (PR1 — parser depth)

- Local `module` blocks (`source = "./..."` / `"../..."` / absolute) are
  now expanded recursively. Resources from a sub-module are namespaced
  `module.<name>.<original_id>` so they participate in the graph as
  first-class nodes alongside top-level resources. Remote module sources
  (registry, `git::`, `https://`, ...) intentionally stay warn-and-skip
  to preserve the runner-local trust model.
- `count = <int>` and `count = length([...])` now expand into N
  independent `RawResource`s, suffixed `[0]`, `[1]`, ... `count = 0`
  produces zero resources (not a warning).
- `for_each = { ... }` and `for_each = toset([...])` with literal keys now
  expand into one `RawResource` per key, suffixed `["<key>"]`. Variable-
  driven `for_each` still warns and skips so we never invent resources.
- `dynamic "block" { for_each = [...] }` blocks with literal iterators now
  materialize the inner block per iteration before attribute extraction.
- `maxModuleDepth = 8` guards against accidental module cycles.

### Added (PR2 — fix v1.1 caveats)

- The parser now resolves `policy = jsonencode({ ... })` for
  `aws_s3_bucket_policy` to a literal JSON string and forwards it to the
  graph. The `s3_bucket_public_exposure` rule parses that string and is
  suppressed when **every** statement has `Effect = "Deny"` -- eliminating
  the v1.1 false-positive on strict bucket-lockdown policies.
- The parser pre-scans each directory for `data "aws_iam_policy" "name" {
  arn = "<literal>" }` blocks and uses them to resolve
  `policy_arn = data.aws_iam_policy.name.arn` references at extraction
  time. The `iam_admin_policy_attached` rule now fires on this idiom too.

### Added (PR3 — config + suppressions)

- New optional `.architex.yml` repository config, loaded from the head
  directory (the side asserting intent). Supported sections:
  - `rules.<id>.weight` / `.enabled` -- per-rule overrides.
  - `thresholds.warn` / `.fail` -- numeric severity cutoffs.
  - `ignore.paths` -- `**/*.tf` glob patterns; matching files are skipped
    by the parser before they enter the graph (applied to both base AND
    head dirs for consistent diffs).
  - `suppressions[]` -- `(rule, resource, reason, [expires])` tuples.
    `resource` may end in `*` for prefix wildcards. `expires` accepts
    RFC3339 or `YYYY-MM-DD`; expired entries still drop the rule but are
    flagged in the audit footer so reviewers can refresh or remove them.
- Inline `# architex:ignore=<rule_id> reason="<text>"` and
  `// architex:ignore=...` directives on the line(s) immediately
  preceding a `resource "type" "name" {` block synthesize equivalent
  suppressions. Multiple stacked directives all attach to the next
  resource. A non-comment, non-resource line clears the pending stack so
  directives never accidentally drift to a later block.
- New `risk.RiskReason.ResourceID` field, populated by every per-resource
  rule, so suppressions can match `(rule_id, resource_id)` precisely.
  Cross-resource rules (e.g. `potential_data_exposure`) leave it empty
  and are intentionally NOT suppressible by tuple -- use
  `rules.<id>.enabled: false` to silence those.
- `risk.RiskResult.Suppressed` carries the silenced findings (with their
  reason and source -- `config:.architex.yml` or
  `inline:<file>:<line>`) so they remain auditable.
- The PR comment now includes a **Suppressed Findings** section above the
  sticky footer when any are present, with each entry showing the rule,
  resource, reason, source, and an `(EXPIRED)` flag where applicable.
  Repos with no suppressions render exactly as in v1.1.
- The egress payload gains a single new field, `suppressed_count` (an
  integer, never the rule IDs or resource IDs). The published JSON
  schema in `docs/egress-schema.json` is updated to match;
  `TestEgressPayload_SchemaParity` enforces it.
- New `risk.EvaluateWith(d, cfg, now)` is the configurable entry point;
  `risk.Evaluate(d)` is now a thin wrapper that calls
  `EvaluateWith(d, nil, time.Time{})` and remains bit-identical to v1.1.
- New `parser.ParseDirWith(dir, cfg)` is the configurable parser entry;
  `parser.ParseDir(dir)` keeps the v1.0/v1.1 zero-config signature.

### Added (PR4 — AWS coverage tranche 2 + 4 new risk rules)

- 17 additional first-class AWS resource types, mapped to existing abstract
  types so the Mermaid diagram, edge ranking, and confidence math need no
  schema migration:
  - **Entry:** `aws_cloudfront_distribution`
  - **Network:** `aws_route53_zone`, `aws_route53_record`, `aws_nat_gateway`
  - **Identity:** `aws_kms_key`, `aws_kms_alias`
  - **Data:** `aws_sns_topic`, `aws_sqs_queue`, `aws_secretsmanager_secret`
  - **Access control:** `aws_sns_topic_policy`, `aws_sqs_queue_policy`,
    `aws_network_acl`, `aws_network_acl_rule`
  - **Storage:** `aws_ebs_volume`
  - **Compute:** `aws_ecs_cluster`, `aws_ecs_task_definition`,
    `aws_ecs_service`
- 10 new explicit edge relationships between tranche-2 resources, e.g.
  `aws_route53_record -> aws_route53_zone` (`part_of`),
  `aws_kms_alias -> aws_kms_key` (`applies_to`),
  `aws_sns_topic_policy -> aws_sns_topic` (`applies_to`),
  `aws_nat_gateway -> aws_subnet` (`deployed_in`),
  `aws_network_acl -> aws_vpc` (`part_of`),
  `aws_ecs_service -> aws_ecs_cluster` (`deployed_in`),
  `aws_ecs_service -> aws_ecs_task_definition` (`uses`).
- The graph now passes through additional literal attributes that the new
  rules need to fire deterministically without re-parsing HCL:
  `web_acl_id` for CloudFront, `encrypted` for EBS, `policy` for both
  SNS/SQS topic policies, and `cidr_block` / `egress` / `rule_action` for
  NACL rules. Variable-driven values stay `nil` so rules conservatively
  do NOT fire on unresolved expressions.
- Four new deterministic risk rules:
  - **`cloudfront_no_waf`** (2.5): triggers per added
    `aws_cloudfront_distribution` with no literal `web_acl_id`.
    Variable-driven `web_acl_id` is intentionally treated as absent so a
    distribution that ships with no statically-provable WAF is flagged.
  - **`ebs_volume_unencrypted`** (3.0): triggers per added `aws_ebs_volume`
    with an explicit literal `encrypted = false`. Unset / variable-driven
    `encrypted` does NOT fire (account default may be encryption-on).
  - **`messaging_topic_public`** (3.5): triggers per added
    `aws_sns_topic_policy` / `aws_sqs_queue_policy` whose resolved
    `policy` JSON contains an `Effect = "Allow"` statement with
    `Principal = "*"` (or `AWS: "*"`, including list form). Unresolvable
    or non-JSON policies do not fire.
  - **`nacl_allow_all_ingress`** (3.5): triggers per added
    `aws_network_acl_rule` with `cidr_block = "0.0.0.0/0"`,
    `egress = false`, AND `rule_action = "allow"`. All three must be
    literal; missing any one leaves the rule silent.
- Every new rule respects PR3's config + suppression machinery
  (`rules.<id>.enabled`, `rules.<id>.weight`, per-resource suppressions,
  inline `# architex:ignore=` directives) and emits at most 2 reasons per
  evaluation so a single sweeping refactor cannot saturate the 10.0 cap.
- New testdata fixtures:
  - `testdata/tranche2_resources/` -- one Terraform module exercising
    every new resource type, used as the parser-coverage contract test
    (no `unsupported_resource` warnings, confidence 1.0).
  - `testdata/tranche2_base/` + `testdata/tranche2_head/` -- end-to-end
    fixture that triggers all four new rules from a single base->head
    pair (plus the existing `new_entry_point` on the new CloudFront).

### Added (PR5 — baseline anomaly rules + `architex baseline` subcommand)

- New `baseline` package persists a deterministic snapshot of an
  architecture graph's "shape" -- the sets of provider resource types,
  abstract types, and (sourceProviderType, targetProviderType) edge pairs
  ever observed. The on-disk representation is a small, human-auditable
  JSON file (`schema_version: "1"`, default path `.architex/baseline.json`)
  that contains NO raw HCL, NO resource names, and NO attribute values --
  only the *kinds* of things already present, so it can be checked into a
  public repo without leaking architectural detail beyond what
  `models.SupportedResources` and `models.AbstractionMap` already publish.
- New `architex baseline <dir> [--out <path>] [--merge]` subcommand
  generates (or, with `--merge`, unions into) a baseline file from the
  current graph of `<dir>`. Atomic write (temp file + rename) so a
  crashed run never leaves a half-written baseline that would silently
  disable rules on the next PR.
- Three new deterministic anomaly rules, all silenced when no baseline
  is present (the v1.1 zero-config behavior is bit-identical):
  - **`first_time_resource_type`** (1.0): triggers on each added node
    whose `provider_type` (e.g. `aws_kms_key`) has never appeared in the
    baseline. Deduped per type and capped at 2 reasons per evaluation,
    so a multi-instance module rollout cannot saturate the score.
  - **`first_time_abstract_type`** (1.5): triggers on each added node
    whose abstract type (e.g. `entry_point`, `identity`) has never
    appeared in the baseline. Same dedup + 2-cap. The strongest signal
    of the three -- a brand-new architectural category usually marks a
    real inflection point.
  - **`first_time_edge_pair`** (0.5): triggers per added edge whose
    `(sourceProviderType, targetProviderType)` pair is unknown to the
    baseline. Both endpoints must be resolvable from the delta's added
    or changed nodes -- unresolvable endpoints conservatively skip
    (never guess).
- New `risk.EvaluateWithBaseline(d, cfg, base, now)` is the canonical
  entry point that runs the full v1.0 + v1.1 + PR4 + PR5 rule set.
  `risk.EvaluateWith(d, cfg, now)` is now a thin wrapper that passes a
  `nil` baseline; `risk.Evaluate(d)` continues to pass `nil, nil`. All
  three preserve their pre-PR5 contracts.
- Suppressions and rule overrides from PR3 work unchanged on the new
  rules: `rules.first_time_*.enabled: false` silences a category, and
  `(rule, resource)` suppressions work because every reason populates
  `ResourceID`. Suppressed novel findings still surface in the audit
  footer so reviewers see what was filtered.
- `score`, `report`, and `sanitize` subcommands now auto-load
  `<head-dir>/.architex/baseline.json` when present and forward it to
  `EvaluateWithBaseline`. A malformed baseline degrades to "no baseline"
  with a stderr warning -- a typo in the file can never brick CI.

### Added (PR6 — self-contained `report.html` in audit bundle)

- New `report.html` artifact written to every audit bundle alongside
  `diagram.mmd`, `summary.md`, `score.json`, `egress.json`, and
  `manifest.json`. The page is a single self-contained file with NO
  JavaScript, NO external CDN scripts, NO remote stylesheets, and NO
  remote fonts/images -- just inlined CSS using system fonts. A
  reviewer can open it in an air-gapped browser and see the entire
  report (severity badges, score, summary, review focus, reasons table,
  suppressed findings, delta counts, and Mermaid source).
- Optional Mermaid Live Editor deep link: when the diagram is under
  8 KiB, the page renders a single `<a href>` to
  `https://mermaid.live/edit#base64:<envelope>`. The link is the ONLY
  outbound URL in the file and only fires on a user click -- the page
  itself never makes a network request at render time. This is gated
  by a regex test (`TestFormatHTML_SelfContained_NoExternalResources`)
  that fails the build if a `<script>`, `<link rel="stylesheet">`,
  `<img>`, `<iframe>`, `<embed>`, `<object>`, or any non-`href`-bound
  http(s) URL is ever introduced.
- New exported `interpreter.FormatHTML(rep, manifest)` function so
  third-party tooling can render the same page outside of the audit
  bundle pipeline (pass `Manifest{}` for a single-shot render).
- The HTML page surfaces the `Manifest.Files` SHA-256 table, so a
  reviewer reading `report.html` alone can verify the sibling artifacts
  in the bundle have not been tampered with after generation. The HTML
  itself is intentionally NOT in `Manifest.Files` -- a manifest cannot
  contain a checksum of a page that itself displays the checksums.
- Output is fully deterministic: the same `Report` + `Manifest` produce
  byte-identical bytes, locked by
  `TestFormatHTML_Deterministic_SameInputSameOutput`.

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
