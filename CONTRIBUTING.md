# Contributing to ArchiteX

Thank you for considering a contribution. ArchiteX is a free, open-source
project with no paid tier and no commercial roadmap -- every contribution
benefits every user equally.

## Dev setup

**Requirements:** Go 1.26+ and Git.

```bash
git clone https://github.com/danilotrix86/ArchiteX.git
cd ArchiteX
go test ./...                       # ~150 tests across 9 packages
go build -o architex ./cmd/architex # produces the CLI binary
```

For large-delta stress testing, run `scripts/stress-mermaid.ps1` (PowerShell)
to generate synthetic Terraform pairs and verify byte-cap regressions.

## What kinds of PRs are welcome

- **Bug fixes** with a failing test that demonstrates the bug.
- **New test cases** for edge cases or Terraform patterns not yet covered.
- **Documentation improvements** to README, docs/, or code comments.
- **New AWS / Azure resource support.** This is the most impactful
  contribution right now. As of the v1.4 readability refactor, adding a
  resource is a one-file edit: add a `Register(...)` line (and any
  `RegisterEdge(...)` lines) to the right provider file under
  [`models/registry/`](models/registry/) -- `aws.go` for `aws_*`,
  `azure.go` for `azurerm_*`. If the resource needs custom attribute
  promotion (e.g. passing a literal `policy_arn` through to the graph
  node), write a small promoter function in the same file and pass it
  as `Promoter:`. Add at least one test in
  [`parser/parser_test.go`](parser/parser_test.go) or
  [`graph/graph_test.go`](graph/graph_test.go) and the byte-identical
  golden tests in [`internal/golden/`](internal/golden/) will guard the
  rest.
- **New built-in risk rule.** Also a one-file edit: create
  `risk/rules/<domain>/<rule_id>.go` with a struct that implements the
  three-method `risk/api.Rule` interface (`ID()`, `Evaluate()`,
  `ReviewFocus()`) and register it in
  [`risk/rules/rules.go`](risk/rules/rules.go) at the position you want
  it evaluated. The interpreter looks rules up by ID via the registry
  -- no edit to `interpreter/summary.go` is needed.
- **New cloud provider** (e.g. GCP): drop a `models/registry/gcp.go`
  alongside `aws.go` / `azure.go`, register the types and edges, and
  add the rules under `risk/rules/<domain>/` next to their AWS / Azure
  counterparts (rules are organized by what they detect, not by which
  provider they fire on). No other package needs to change.

## What needs discussion first

Open an Issue or Discussion before sending a PR for:

- Anything that **adds a network call** outside the `github/` package.
  The analysis pipeline (`parser`, `graph`, `delta`, `risk`, `interpreter`)
  must never import networking packages. This is the structural enforcement of
  the zero-exfiltration guarantee.
- Anything that introduces **non-deterministic behavior** (randomness,
  timestamp-dependent ordering, model inference, network-dependent
  outputs). The whole pipeline is required to be byte-identical across
  runs given the same input.
- New built-in risk rules. The rule library is opinionated and small on
  purpose; before adding one, open a Discussion describing the
  Terraform pattern, the abstract signal, and why it can't be expressed
  as a `.architex.yml` override or an inline suppression on an existing
  rule.
- Changes to the **egress schema** (`docs/egress-schema.json`). The schema is
  a procurement-facing contract; changes must be backward-compatible.
- Changes to the **`.architex.yml` schema** or the `.architex/baseline.json`
  format. Both are user-facing contracts that ship in customer repos.

## Decision principles

These come from [master.md](master.md) and are non-negotiable:

1. **Deterministic first.** All structural understanding, policy evaluation,
   and risk scoring must be deterministic and auditable.
2. **Zero raw code exfiltration.** Raw IaC source never leaves the runner.
3. **Blast radius over full mapping.** Show the delta, not the whole account.
4. **Day-1 value without custom setup.** The tool must be useful within
   minutes of installation, with no configuration required.
5. **Developer utility before enforcement.** Earn trust through clarity and
   low noise before becoming a gate.
6. **Explainability is mandatory.** Every score, every triggered rule, and
   every egress decision must be explainable.

## Test conventions

- **Empty slices, never nil.** JSON output must produce `[]`, not `null`.
  Initialize with `make([]T, 0)`.
- **Deterministic ordering.** All output slices are sorted (nodes by ID,
  edges by from/to/type, reasons by weight descending then rule ID ascending).
- **Stderr format.** Warnings use `[architex] WARN [<category>]: <message>`.
- **No test pollution.** Tests must not write to the working directory or
  depend on network access.

## Commit messages

Use imperative mood, one sentence. Examples:

- `Add aws_s3_bucket support to parser and graph`
- `Fix false positive on security group without cidr_blocks`
- `Expand delta test coverage for attribute removal`

## Where to ask questions

- **GitHub Issues** -- bugs, coverage gaps, false positives/negatives.
- **GitHub Discussions** -- design proposals, roadmap questions, use cases.
