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
      - uses: <owner>/architex@v1
        with:
          terraform-dir: infra
```

That's it. Every PR touching `infra/*.tf` will get a sticky ArchiteX comment. Nothing fails the check.

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

## Rollout pattern (matches `master.md` §11)

ArchiteX is meant to be adopted in three phases. The Action makes each one a one-line change.

### Phase 1 -- Visibility

```yaml
- uses: <owner>/architex@v1
  with:
    terraform-dir: infra
    mode: advisory     # default; never fails the check
```

Use this until the team trusts the findings.

### Phase 2 -- Advisory governance

Same Action, plus a separate required check on a different signal (e.g. `risk.Status == "warn"` triggers a Slack ping via the artifact). Comment is informational; check stays green.

### Phase 3 -- Enforced governance

```yaml
- uses: <owner>/architex@v1
  with:
    terraform-dir: infra
    mode: blocking     # exits non-zero when risk.Status == "fail"
```

Add the job as a required status check in your branch protection rules. PRs whose risk evaluates to `fail` cannot be merged.

> `warn` is intentionally non-blocking even in `blocking` mode. `master.md` §11 reserves enforcement for the `fail` tier so warnings remain warnings.

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

## Limitations (Phase 5 scope)

- AWS Terraform resources only -- the supported subset is documented in `llm.md` ("Supported Resources").
- `module`, `for_each`, `count`, and `dynamic` blocks emit warnings and are skipped (see "Unsupported Constructs" in `llm.md`).
- The diagram is one-layer (changed nodes plus direct edge endpoints); deeper dependency expansion is on the roadmap.
- Multi-provider, GitLab, Bitbucket, and non-Terraform IaC are out of scope for the MVP.
