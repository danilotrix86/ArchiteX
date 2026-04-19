# Contributing to ArchiteX

Thank you for considering a contribution. ArchiteX is a free, open-source
project with no paid tier and no commercial roadmap -- every contribution
benefits every user equally.

## Dev setup

**Requirements:** Go 1.26+ and Git.

```bash
git clone https://github.com/danilotrix86/ArchiteX.git
cd ArchiteX
go test ./...          # 68+ tests across 6 packages
go build -o architex . # produces the CLI binary
```

For large-delta stress testing, run `scripts/stress-mermaid.ps1` (PowerShell)
to generate synthetic Terraform pairs and verify byte-cap regressions.

## What kinds of PRs are welcome

- **Bug fixes** with a failing test that demonstrates the bug.
- **New test cases** for edge cases or Terraform patterns not yet covered.
- **Documentation improvements** to README, docs/, or code comments.
- **New AWS resource support.** This is the most impactful contribution right
  now. Adding a resource requires entries in:
  1. `models.SupportedResources` and `models.AbstractionMap`
     ([`models/models.go`](models/models.go))
  2. The edge-type lookup table in [`graph/graph.go`](graph/graph.go)
  3. Optionally, a `public` derivation rule in `graph/graph.go`
  4. At least one test in `parser/parser_test.go` or `graph/graph_test.go`

## What needs discussion first

Open an Issue or Discussion before sending a PR for:

- Anything that **adds a network call** outside the `github/` package.
  The analysis pipeline (`parser`, `graph`, `delta`, `risk`, `interpreter`)
  must never import networking packages. This is the structural enforcement of
  the zero-exfiltration guarantee.
- Anything that introduces **non-deterministic behavior** (randomness,
  timestamp-dependent ordering, LLM calls in the trust surface).
- Anything that adds **user-configurable rules, weights, or thresholds**.
  This is planned but the design is not yet settled.
- Changes to the **egress schema** (`docs/egress-schema.json`). The schema is
  a procurement-facing contract; changes must be backward-compatible.

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
