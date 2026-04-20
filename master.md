# ArchiteX

**The Architecture Firewall for CI/CD.**
Catch risky infrastructure design before it reaches production.

---

## TL;DR

ArchiteX is a free, open-source GitHub Action that reads the Terraform changes in a Pull Request and posts a deterministic architectural review back as a PR comment. Every PR gets:

- a numeric **risk score** with explainable reasons,
- a **plain-English summary** of what changed,
- a **focused delta diagram** showing only the changed resources and one layer of dependencies,
- and an optional **CI gate** that can fail the build on critical violations.

Everything runs on the user's own GitHub Actions runner. Raw Terraform code never leaves the customer environment. There is no SaaS backend, no telemetry, and no paid tier.

ArchiteX is positioned between code-level scanners (SAST, IaC linters) and runtime cloud security (CSPM, CNAPP). It is the missing **architectural** control layer between code review and deployment.

---

## Table of Contents

1. [The Problem](#1-the-problem)
2. [What ArchiteX Does](#2-what-architex-does)
3. [Product Principles](#3-product-principles)
4. [How It Works](#4-how-it-works)
5. [The PR Experience](#5-the-pr-experience)
6. [Trust Model and Data Sanitization](#6-trust-model-and-data-sanitization)
7. [Rollout Strategy](#7-rollout-strategy)
8. [Scope and Roadmap](#8-scope-and-roadmap)
9. [Distribution and Sustainability](#9-distribution-and-sustainability)
10. [How It Compares](#10-how-it-compares)
11. [Try It](#11-try-it)

---

## 1. The Problem

### 1.1 Infrastructure review has become cognitively unmanageable

Modern Terraform PRs routinely contain hundreds or thousands of changed lines, nested modules, dynamic blocks, and indirect dependency shifts across critical resources. Even experienced reviewers struggle to reliably answer questions like:

- Did this change expose a previously private asset?
- Did this introduce a new dependency on a production database?
- Did this alter trust boundaries?
- Did this increase blast radius?

Critical architectural issues regularly slip through review simply because they are hard to *see* in a text diff.

### 1.2 Documentation drifts the moment it ships

Architecture diagrams maintained in Lucidchart, Miro, or slide decks become stale almost immediately after a merge. The result: security teams lack accurate topology visibility, auditors reconstruct architecture manually, onboarding slows, and incident responders work from out-of-date pictures.

### 1.3 Existing visualization tools fail during review

Most infrastructure visualization tools try to map the entire environment at once. Reviewers don't need the whole cloud account — they need a focused answer to **what changed, what it touches, and why it matters**. That is the core UX insight behind ArchiteX.

### 1.4 Adjacent security tools stop too low in the stack

| Tool category | What it answers | What it misses |
|---|---|---|
| SAST | Code-level security flaws | Architecture |
| SCA | Dependency vulnerabilities | Architecture |
| IaC linters | Static misconfigurations on individual resources | Topology and relationships |
| CSPM / CNAPP | Runtime cloud posture | Pre-deployment review |
| Policy engines | Explicit rules at plan/apply time | Semantic delta context |

None of them answer: **is this infrastructure change architecturally safe?** That is the gap ArchiteX is built for.

### 1.5 Many teams cannot adopt tools that exfiltrate code

Regulated organizations and security-conscious teams often cannot grant a third-party SaaS access to their private infrastructure repos. ArchiteX is designed specifically to remove that blocker by running entirely on the customer's own runner.

---

## 2. What ArchiteX Does

ArchiteX is a deterministic architecture intelligence and governance layer for Infrastructure-as-Code, embedded in CI, that detects, explains, and optionally blocks risky infrastructure topology changes before deployment.

It is **not**:

- just a diagram generator,
- just another IaC linter,
- just a policy engine.

It is the missing architectural control between code review and deployment.

### 2.1 Two audiences, one surface

| Persona | What they get from ArchiteX |
|---|---|
| **Developers and platform engineers** | Faster PR review, fewer hidden topology surprises, plain-English summaries, focused delta diagrams, low-noise findings. |
| **Security, compliance, and engineering leadership** | Deterministic architectural guardrails, explainable risk scoring, audit trail artifacts, optional CI blocking, organization-wide consistency. |

Both groups consume the same PR comment. They just value different parts of it.

---

## 3. Product Principles

ArchiteX is built on six non-negotiable principles. They are the reason the project exists in the form it does.

### 3.1 Deterministic first

Every output is deterministic and auditable. Structural understanding, policy evaluation, risk scoring, summary text, review-focus bullets, and the Mermaid diagram are all produced by explicit, version-controlled rules and templates — never by inference, learned models, or probabilistic reasoning. The same input produces byte-identical output across runs and across machines.

This is a **non-negotiable property**, not a default that can be relaxed. ArchiteX does not call out to any external model or service, does not embed any inference component, and does not include a "smart mode" that bypasses the rule engine. If a future feature ever needs more flexibility than the rule engine provides, the answer is to extend the rule engine — not to add probabilistic logic.

### 3.2 Zero raw code exfiltration

Raw IaC code never leaves the customer environment. Parsing and graph construction happen on the customer's own CI runner. Only tightly sanitized metadata may leave the runner, and only when the user explicitly opts into an external presentation service. See [§6 Trust Model](#6-trust-model-and-data-sanitization).

### 3.3 Blast radius over full mapping

The default review surface is the **localized** architectural context: the changed resources, one layer of dependencies, and the directly affected trust boundaries. This avoids the "spaghetti map" problem that plagues whole-environment visualizers.

### 3.4 Day-1 value without custom setup

ArchiteX ships with built-in opinionated rules, default scoring, useful summaries, and readable visuals. A team should get value within minutes of installing the action — no custom configuration required. Optional `.architex.yml` overrides and inline `# architex:ignore=` directives are available when a team is ready to tune the defaults, but no team is ever required to author a single line of configuration to get the full Day-1 experience.

### 3.5 Developer utility before enforcement

Developers must find the tool useful before they accept it as a gate. ArchiteX wins trust first through clarity, speed, and low-noise findings. Only then does it become enforcement infrastructure. See [§7 Rollout Strategy](#7-rollout-strategy).

### 3.6 Explainability is mandatory

Every meaningful output is explainable: why the score is high, why a specific policy failed, what changed structurally, and exactly what data left the runner. No black boxes.

---

## 4. How It Works

ArchiteX uses a strict four-stage pipeline. Each stage has a narrow, well-defined responsibility.

### Stage 1 — Fact-Finder (deterministic structural parsing)

Runs entirely on the customer's CI runner. Builds a machine-readable infrastructure graph from the Terraform change set.

- Parses Terraform configuration.
- Resolves local module relationships.
- Extracts resources, references, and dependencies.
- Identifies key security-relevant attributes (encryption, exposure, trust boundaries).
- Normalizes everything into a canonical graph format with nodes, edges, and metadata.

**Constraint:** the Fact-Finder relies on deterministic parsing only. No inference, no probabilistic resolution of unresolved expressions — anything that cannot be resolved from a literal is warn-and-skipped so the engine never invents resources.

### Stage 2 — Delta Engine (semantic architectural diffing)

Compares the base graph and the head graph. Its job is **structural meaning**, not line diffing.

- Detects added, removed, and modified resources.
- Detects modified dependencies and trust-boundary changes.
- Detects exposure changes.
- Isolates a bounded delta subgraph for reviewer context.

The output is a **semantic architectural delta** that drives every downstream stage.

### Stage 3 — Risk and Policy Engine (deterministic governance layer)

The product's core moat. Fully deterministic, auditable, version-controlled, explainable, and tunable.

- Evaluates the delta against built-in rules.
- Assigns weighted severity.
- Calculates a numeric score (0–10) with a severity band (low / medium / high) and a CI status (pass / warn / fail).
- Generates explainable reason codes for every triggered rule.

**Hard rule:** risk scores are never generated by inference. Every weight, threshold, and severity band is an explicit constant in [`risk/`](risk/) that anyone can read, audit, and override via [`.architex.yml`](#configuration).

Example output:

```text
Risk Score: HIGH (8.3 / 10)
  Exposure       : 4.0  New public ingress path introduced
  Criticality    : 2.5  Newly connected resource tagged as production data tier
  Change Scope   : 1.8  Cross-boundary dependency chain expanded across critical services
```

Built-in rules cover patterns like:

- No direct public path to a private data tier.
- No unrestricted ingress on sensitive ports.
- Encryption required on persistent data resources.
- No new internet-facing resource in protected environments without an approval tag.
- No broadening of trust boundary between application and data layers without explicit review.

As of v1.2, an optional `.architex.yml` lets users adjust rule weights, disable specific defaults, override warn/fail thresholds, ignore paths, and suppress individual findings (with optional expiry). Inline `# architex:ignore=<rule_id> reason="..."` directives in `.tf` files cover the per-resource case. Adding entirely new rule definitions and environment-specific sensitivity remain future work.

### Stage 4 — Interpreter (deterministic presentation layer)

The Interpreter takes the already-deterministic Delta + RiskResult and produces the human-facing artifacts: a Mermaid delta diagram, a plain-English summary, review-focus bullets, the Markdown PR comment, and the self-contained `report.html` page in the audit bundle.

This stage is also fully template-based. Summary sentences, focus bullets, and diagram layout are all derived from the structured input via explicit functions in [`interpreter/`](interpreter/) — there is no model, no prompt, and no network call anywhere in the rendering pipeline.

The `Interpreter` Go interface exists as a clean seam for alternative deterministic renderers (different locales, alternative output formats, custom summary templates), not as a hook for inference. Any implementation must remain deterministic-equivalent: same input → same shape of output, byte-identical across runs.

---

## 5. The PR Experience

When a developer opens a Pull Request that touches Terraform, ArchiteX posts a single sticky comment back to the PR. The comment contains five sections.

### 5.1 Risk score

A visible severity banner with explicit reasons:

> **Risk Score: HIGH (8.3 / 10)**
>
> - Public ingress introduced
> - New production-tier dependency added
> - Blast radius expanded across trust boundary

### 5.2 Plain-English summary

A short, factual narrative derived from the deterministic delta. Example:

> A new database resource was introduced and connected to an existing application tier inside the production VPC. The change increases data persistence dependencies and adds a new potential exposure path that should be reviewed carefully.

### 5.3 Suggested review focus

A short checklist of the most important things for a human reviewer to actually verify. Example:

- Verify security group rules on the new database path.
- Confirm encryption and backup settings.
- Review whether the new network path should be reachable from the public application tier.
- Confirm this dependency is intended for production.

### 5.4 Delta diagram

A focused Mermaid diagram of only the changed resources plus one layer of dependencies. Visual conventions:

- **Green** = added
- **Red** = removed
- **Neutral** = unchanged supporting context
- Trust boundaries are explicitly drawn

### 5.5 Policy result

Triggered policies, with severity. If the action is configured in `mode: blocking`, a critical violation also fails the GitHub status check.

### 5.6 Audit trail (optional)

Every run also produces a versioned bundle that can be uploaded as a workflow artifact (or committed to a `/docs` path):

- Diagram snapshot
- Score breakdown
- Triggered rules
- Reviewer-facing summary
- Timestamped architectural diff record

This turns architecture documentation into an automatic byproduct of normal engineering work.

---

## 6. Trust Model and Data Sanitization

Because zero exfiltration is a core design goal, ArchiteX defines exactly what may and may not leave the runner.

### 6.1 What stays local — always

- Raw Terraform source files
- Variable files
- Secrets
- Plan files and state files
- Provider credentials
- Resource names that the user marks sensitive
- Comments and surrounding code context

### 6.2 What may leave the runner — only on opt-in

When the user explicitly enables an external presentation service, only a sanitized delta payload is transmitted. That payload may include:

- Abstract node identifiers
- Abstract resource types
- Abstract edge relationships
- Added / removed / modified markers
- Severity reason codes
- Generalized environment labels
- Bounded metadata necessary for summary generation

In the default GitHub Action configuration, **nothing** leaves the runner — the comment is posted from within the runner itself using the user's `GITHUB_TOKEN`.

### 6.3 Sanitization controls

ArchiteX supports name redaction, hashing or tokenization of identifiers, environment aliasing, configurable metadata suppression, and allowlist-based egress fields.

### 6.4 Auditable egress specification

ArchiteX publishes a machine-readable sanitization schema and egress specification, so any organization can verify exactly what bytes leave the runner before approving rollout.

---

## 7. Rollout Strategy

Blocking PRs too early creates rejection. ArchiteX is designed to roll out in three stages.

### Phase 1 — Visibility

PR comments only. No blocking. The goal is trust building and noise tuning. Engineers see ArchiteX show up in their PRs and learn what it catches.

### Phase 2 — Advisory governance

Warnings, soft policy thresholds, evidence collection. Platform and security teams start using the audit bundles as input to broader governance work.

### Phase 3 — Enforced governance

CI blocking for critical violations. Policy-based or opt-in activation. Custom rules and exception workflow.

This sequence preserves developer trust while enabling an eventual control posture.

---

## 8. Scope and Roadmap

### 8.1 Initial supported scope

| Dimension | Scope |
|---|---|
| Source platform | GitHub |
| Distribution | GitHub Action + (planned) GitHub App |
| Execution | Customer's CI runner — local-first |
| IaC language | Terraform |
| Cloud target | AWS |
| Review surface | Pull Requests |

### 8.2 Explicitly out of scope (for now)

ArchiteX deliberately does not try to:

- support all IaC languages on day one,
- map entire cloud estates,
- replace `terraform plan`,
- replace runtime cloud security tools,
- create a general architecture knowledge graph,
- solve every compliance framework out of the box,
- provide interactive full-scale architecture exploration.

The current scope is tightly focused: catch risky Terraform AWS architectural changes in GitHub Pull Requests and make them instantly understandable.

### 8.3 Future roadmap

Once the core experience is solid, expansion proceeds carefully along four axes:

- **Ecosystem:** GitLab, Bitbucket, generic API ingestion.
- **IaC:** Helm / Kubernetes, Pulumi, AWS CDK.
- **Visualization:** richer layout, interactive graph exploration, layered views, zoomable DAG.
- **Intelligence:** temporal anomaly detection, drift trend reporting, organization-wide architectural posture scoring, approval workflows and exceptions.

The historical/temporal layer is intentionally a long-term direction: over time, ArchiteX learns the architectural patterns of a specific repository and surfaces anomalies (first-time exposure events, unusual dependency patterns, sudden trust-boundary expansion, drift from established norms). That repository-specific history is the strongest long-term value.

---

## 9. Distribution and Sustainability

ArchiteX is a **free, open-source project**. There is no paid tier, no commercial product, no SaaS layer, and no license keys. The full source lives in a public GitHub repository, and the GitHub Action is consumed by reference (`uses: <owner>/ArchiteX@<version>`) at zero cost to the user.

### 9.1 Why free

- **Trust is the product.** Every paywall, license server, or sales call directly reduces the surface on which ArchiteX can prove its value. A free tool that earns trust by quietly working aligns better with the project's mission than a paid tool that earns revenue by extracting value from a small audience.
- **The architecture is naturally cost-free to operate.** Parsing, graph construction, scoring, and rendering all happen on the user's own GitHub Actions runner. The maintainer has no servers, no per-user costs, and therefore no business pressure to charge anyone.
- **The space is already crowded with paid commercial offerings.** The most useful niche ArchiteX can occupy is the deterministic, source-auditable, on-runner alternative that any team can adopt without a procurement conversation.

### 9.2 Possible future sustainability paths (none planned, all optional)

- **GitHub Sponsors / OpenCollective donations.** If the project sees broad adoption, a sponsor button may be added to the repository. Donations would fund continued maintenance — never feature gating. No donor receives privileged features.
- **Paid support engagements.** A user with a complex setup may contract the maintainer directly for installation help, custom rule authoring, or training. That is a contract relationship, not a product. The tool itself remains free.
- **Acquisition or hand-off.** If a larger organization wants to commercialize ArchiteX, the codebase or maintainership may be transferred. That is a binary decision made once, not an ongoing business model.

### 9.3 What ArchiteX explicitly does not do

- No paid tiers, freemium gating, "pro" features, or usage limits in the open-source distribution.
- No telemetry collected by default. The egress schema (§6.2) exists so any future opt-in telemetry would be implemented in the same auditable, sanitized way as the rest of the trust model — but no such endpoint exists today and none is planned.
- No advertising, lead capture, or marketing in the tool's output. PR comments contain only the analysis and a non-interactive footer marker.

---

## 10. How It Compares

### vs. HashiCorp Sentinel / OPA

Policy engines enforce explicit rules. ArchiteX adds reviewer-first architectural visualization, semantic delta context inside the PR, and built-in opinionated defaults that work without custom rule authoring.

### vs. Snyk IaC, Bridgecrew, Wiz Code, and similar IaC scanners

Scanners find resource-level misconfigurations well. ArchiteX focuses on **topology and architectural relationships** — blast radius, dependency reasoning, trust-boundary changes — which resource-level scanners are not designed to catch.

### vs. Runtime CSPM / CNAPP (Wiz, Lacework, Prisma Cloud, etc.)

Those tools inspect deployed environments. ArchiteX prevents risky architecture from being merged in the first place.

### vs. Diagram tools (Lucidchart, Miro, Cloudcraft)

Diagram tools document. ArchiteX governs.

### Project moat (over time)

The parser is not the moat. The moat is the combination of:

- **Local-runner architecture** — a trust accelerator: any team can adopt ArchiteX without sending source code to a third party, removing the single largest objection most security-conscious organizations have to modern developer tools.
- **Semantic delta engine** — understanding architectural change, not just code change.
- **Deterministic risk model** — auditable, explainable, version-controlled.
- **Opinionated default rule library** — immediate value without lengthy configuration.
- **Repository-specific historical intelligence** — the long-term value that improves with time-in-repo.

---

## 11. Try It

The simplest way to see ArchiteX in action is to add the GitHub Action to a Terraform repository and open a PR. Full installation, configuration, and trust-model documentation lives in [`docs/github-action.md`](docs/github-action.md).

### 11.1 Contributing and feedback

ArchiteX is maintained in the open. The single best way to influence the roadmap is to:

1. **Open a GitHub Issue** describing your use case, the Terraform pattern that didn't parse correctly, the rule you wished existed, or the false positive that hurt your team's trust in the tool.
2. **Open a Discussion** for broader design questions and proposals.
3. **Open a Pull Request** with tests if you have a fix or a small additive feature.

The maintainer's primary job, post-launch, is to read every Issue and Discussion that comes in. Real user feedback is the only legitimate source of roadmap direction.

### 11.2 License

ArchiteX is released under the [MIT License](LICENSE). You may use, modify, and distribute it freely, including in commercial settings, with no warranty.
