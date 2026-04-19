# Changelog

All notable changes to ArchiteX are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
