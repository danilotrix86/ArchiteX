# ArchiteX GitHub Action

Run ArchiteX inside GitHub Actions to post a deterministic risk report as a sticky PR comment, upload an audit bundle as a workflow artifact, and (optionally) fail the check when a high-risk topology change is proposed.

The Action is a composite Action that:

1. Checks out the PR base and head into separate working directories.
2. Builds the `architex` Go binary from the action source.
3. Runs `architex report <base> <head> --out ./.architex` -- the same command you can run locally.
4. Calls `architex comment <bundle> --repo <owner/name> --pr <num> --mode <mode>` to upsert the Markdown summary as a sticky PR comment.
5. Uploads the timestamped audit bundle (`diagram.mmd`, `summary.md`, `score.json`, `egress.json`, `manifest.json`) as a workflow artifact.

Raw `.tf` source never leaves the runner. The only outbound network call is the GitHub PR-comment upsert, performed by the dedicated `architex comment` subcommand. See [`../docs/egress-schema.json`](egress-schema.json) for the only payload shape ever permitted to leave the runner in any future SaaS-mode integration -- this Action does not use it.

## Quickstart -- visibility only (recommended first rollout)

Drop this into `.github/workflows/architex.yml` in your Terraform repo:

```yaml
name: ArchiteX
on:
  pull_request:
permissions:
  contents: read
  pull-requests: write
jobs:
  architex:
    runs-on: ubuntu-latest
    steps:
      - uses: danilotrix86/ArchiteX@v1.3.0
        with:
          terraform-dir: infra
```

That's it. Every PR touching `infra/*.tf` will get a sticky ArchiteX comment. Nothing fails the check.

Pin to an exact version (`v1.3.0`, `v1.2.0`, `v1.1.0`, ...) -- see the [Versioning](#versioning) section below for why.

## Inputs

| Input              | Default                                              | Notes                                                                                          |
|--------------------|------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `terraform-dir`    | `.`                                                  | Subdirectory (relative to repo root) containing `.tf` files. Use `.` for the repo root.        |
| `base-ref`         | `${{ github.event.pull_request.base.sha }}`         | Git ref to check out as the diff base.                                                         |
| `head-ref`         | `${{ github.event.pull_request.head.sha }}`         | Git ref to check out as the diff head.                                                         |
| `mode`             | `advisory`                                           | `advisory` never fails the check. `blocking` exits non-zero when `risk.Status == "fail"`.      |
| `comment`          | `true`                                               | Set to `false` to skip the PR comment (artifact upload still happens).                         |
| `upload-artifact`  | `true`                                               | Set to `false` to skip the audit-bundle artifact upload.                                       |
| `salt`             | (empty)                                              | Sanitization hashing salt. Empty = stable IDs across runs (good for trend diffing).            |
| `github-token`     | `${{ github.token }}`                                | Token used to post the PR comment. Needs `pull-requests: write`.                               |
| `go-version-file`  | (action's own `go.mod`)                              | Override only if you want to pin the Go toolchain to your repo's `go.mod`.                     |

## Outputs

| Output         | Notes                                                                 |
|----------------|----------------------------------------------------------------------|
| `bundle-path`  | Filesystem path of the audit bundle directory.                        |
| `status`       | `pass` &middot; `warn` &middot; `fail` -- mirror of `risk.Status`.    |
| `score`        | `0.0`-`10.0`, mirror of `risk.Score`.                                 |

## Required permissions

```yaml
permissions:
  contents: read         # checkout the PR head and base
  pull-requests: write   # upsert the sticky comment
```

If you only want the artifact and no PR comment, you can drop `pull-requests: write` and set `comment: false`.

## Rollout pattern (matches [master.md §7](../master.md#7-rollout-strategy))

ArchiteX is meant to be adopted in three phases. The Action makes each one a one-line change.

### Phase 1 -- Visibility

```yaml
- uses: danilotrix86/ArchiteX@v1.3.0
  with:
    terraform-dir: infra
    mode: advisory     # default; never fails the check
```

Use this until the team trusts the findings.

### Phase 2 -- Advisory governance

Same Action, plus a separate required check on a different signal (e.g. `risk.Status == "warn"` triggers a Slack ping via the artifact). Comment is informational; check stays green.

### Phase 3 -- Enforced governance

```yaml
- uses: danilotrix86/ArchiteX@v1.3.0
  with:
    terraform-dir: infra
    mode: blocking     # exits non-zero when risk.Status == "fail"
```

Add the job as a required status check in your branch protection rules. PRs whose risk evaluates to `fail` cannot be merged.

> `warn` is intentionally non-blocking even in `blocking` mode. Enforcement is reserved for the `fail` tier so warnings remain warnings.

## Sticky comment behavior

The Action posts one comment per PR. On every subsequent push, it finds the existing ArchiteX comment (matched by the footer `_Generated by ArchiteX (deterministic mode)._`) and updates it in place. There is no comment spam, no per-commit duplicates, and no state stored anywhere except in the comment itself.

The marker is part of the comment body emitted by `interpreter.FormatMarkdown`; the Action never injects it, which keeps the formatter as the single source of truth for what the PR sees.

## Trust model

- **Analysis runs locally on the runner.** Parsing, graph construction, delta, risk evaluation, and Markdown rendering all happen in the `architex` binary on the GitHub-hosted (or self-hosted) runner. No `.tf` source is transmitted anywhere.
- **The only network call is the PR comment.** It POSTs (or PATCHes) the rendered Markdown to the GitHub REST API using `${{ github.token }}`. That's the same token your other workflow steps already have.
- **The egress schema is unrelated to this Action.** [`docs/egress-schema.json`](egress-schema.json) defines what *would* leave the runner if you ever attached an external presentation service. The Action does not call any such service.

## Local equivalence

Anything the Action does, you can run locally:

```bash
go build -o architex .
./architex report ./base ./head --out ./.architex --salt my-salt
# --> reads .architex/<bundle>/summary.md
GITHUB_TOKEN=ghp_xxx ./architex comment ./.architex/<bundle> \
  --repo my-org/my-repo --pr 42 --mode advisory
```

This makes Action issues debuggable on a developer laptop without needing to push commits to test.

## Limits & large deltas

ArchiteX defends two real limits that affect large PRs (think: bulk migrations, environment forks, or refactors touching 100+ resources):

| Limit | Source | What ArchiteX does |
|---|---|---|
| **mermaid-js `maxTextSize`** = 50,000 chars (default in [mermaid-cli/issues/113](https://github.com/mermaid-js/mermaid-cli/issues/113)). Above this, GitHub renders "Maximum text size in diagram exceeded" instead of your diagram. | Used by GitHub's Markdown renderer for `\`\`\`mermaid` blocks. | The renderer caps the Mermaid block at **45,000 chars** (5 KB safety margin). When the cap engages, lower-priority nodes (and any orphaned edges) are dropped and the diagram gains an explicit `_architex_truncated` placeholder node announcing the hidden counts. Priority order: `changed > added > removed > context`, then `entry_point > data > compute > network > access_control`, then ID alphabetically. |
| **GitHub comment body** = 262,144 bytes (a `mediumblob` in MySQL; see [community/27190](https://github.community/t/maximum-length-for-the-comment-body-in-issues-and-pr/148867)). The frequently-cited "65,536 chars" is the worst-case 4-byte-per-char view of the same number; for ASCII-dominated bodies the byte limit dominates. | Hard rejection on POST. | The `architex comment` subcommand caps the body at **240,000 bytes** (22 KB safety margin). When the cap engages, the diagram block is stripped from the comment and replaced with `_Diagram omitted: rendered comment was N bytes (over the 240,000-byte safety budget). Full diagram is in the ArchiteX audit bundle artifact._`. The sticky-marker footer is preserved, so the next run still updates the same comment in place. If even that is not enough, a hard truncate with a visible marker is the last resort. |

In every case the **full, untruncated** diagram is in the audit bundle artifact (`actions/upload-artifact@v4`), uploaded by the Action regardless of whether the comment was capped. Trust is preserved; only presentation is reduced.

You can verify the caps locally with `scripts/stress-mermaid.ps1` -- it generates synthetic deltas of any N and prints the byte sizes plus a `DiagramCapped` flag.

## Supported resources

ArchiteX v1 handles the canonical 3-tier AWS stack:

| Terraform type | Abstract type |
|---|---|
| `aws_vpc` | `network` |
| `aws_subnet` | `network` |
| `aws_security_group` | `access_control` |
| `aws_security_group_rule` | `access_control` |
| `aws_instance` | `compute` |
| `aws_lb` | `entry_point` |
| `aws_db_instance` | `data` |

Unsupported resource types are logged as warnings (category `unsupported_resource`) and reduce the confidence score; they do not cause failures.

### Unsupported constructs

These Terraform constructs are detected and warned about, not silently skipped:

| Construct | Result |
|---|---|
| `for_each` / `count` on a resource | Resource skipped (warning) |
| `dynamic` nested block | Resource skipped (warning) |
| `module` block | No resource produced (warning) |
| Unknown resource type | Resource skipped (warning) |

## Limitations

- AWS Terraform resources only (45 types as of v1.3 -- see [README § Coverage](../README.md#-coverage)). Broader provider and resource coverage continues each minor release.
- The diagram shows one layer of dependencies (changed nodes plus direct edge endpoints); deeper expansion is on the roadmap.
- Multi-provider, GitLab, Bitbucket, and non-Terraform IaC are out of scope for v1.

## Versioning

**Always pin to an exact, immutable version tag** (`v1.3.0`, `v1.2.0`, `v1.1.0`, ...). Each tag points at a single commit forever, so a copy-pasted workflow keeps producing the same output until you intentionally upgrade.

```yaml
- uses: danilotrix86/ArchiteX@v1.3.0
```

Pinning is recommended because:

1. **Auditability.** A security-review tool that silently changes its own behaviour under your CI is a contradiction. Pinning means the rules you reviewed last week are the rules running today.
2. **Reproducibility.** If a PR's score changes, you know it's because the Terraform changed -- not because ArchiteX changed.
3. **Explicit upgrades.** When you bump `v1.3.0` -> `v1.4.0`, you read the [CHANGELOG](../CHANGELOG.md) and decide whether to take it.

To upgrade, check the [Releases page](https://github.com/danilotrix86/ArchiteX/releases) and bump the tag in your workflow file. Renovate / Dependabot can automate the PRs.
