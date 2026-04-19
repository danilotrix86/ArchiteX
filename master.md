# Master Product Specification ArchiteX

Category: DevSecOps / CI/CD Infrastructure Governance

Positioning: The Architecture Firewall for CI/CD

Tagline: Prevent risky infrastructure design before it reaches production

## 1. Executive Summary

ArchiteX is an Infrastructure Intelligence and Governance Layer that runs inside the software delivery pipeline and acts as an automated Architectural Review Gate for Infrastructure-as-Code. Its purpose is simple: Detect, explain, and block risky infrastructure topology changes before they are merged. As infrastructure complexity continues to grow, human review is no longer sufficient. Senior engineers are expected to mentally parse thousands of lines of Terraform diffs, infer topological changes, validate security assumptions, and preserve documentation accuracy — all within the time constraints of modern CI/CD. This does not scale. ArchiteX solves this by analyzing Terraform pull requests directly inside GitHub and producing four outputs: A deterministic Risk Score Policy violations with clear explanations A plain-English summary of the architectural change A localized visual delta diagram showing only what changed Unlike documentation tools, ArchiteX is not a passive observer. It is an active pre-deployment architecture control. Unlike cloud-based AI tooling, ArchiteX performs parsing and structural analysis locally on the customer’s CI runner, ensuring that raw infrastructure code never leaves the customer environment. Unlike static IaC scanners, ArchiteX reasons about topology, relationships, blast radius, and architectural intent, not just isolated misconfigurations. ArchiteX is designed to become the missing control layer between: code scanning tools policy engines cloud posture tools In short: SAST scans code. CSPM scans deployed cloud. ArchiteX scans architecture before deployment.

## 2. The Problem

### 2.1 The Review Capacity Crisis

Infrastructure review has become cognitively unmanageable. In modern engineering teams, a Pull Request may include: hundreds or thousands of lines of Terraform changes nested modules dynamic blocks indirect networking changes subtle dependency shifts across critical resources Even highly experienced reviewers struggle to reliably answer questions like: Did this change expose a previously private asset? Did this introduce a new dependency on a production database? Did this alter trust boundaries? Did this increase blast radius? Is the resulting architecture still compliant? The result is that critical architectural issues often pass through review simply because they are hard to see in text.

### 2.2 Documentation Drift Creates Invisible Risk

Architecture documentation is usually maintained manually through tools like Lucidchart, Miro, or slide decks. These artifacts become outdated almost immediately after a merge. This creates serious problems: security teams lack accurate topology visibility auditors reconstruct architecture manually onboarding slows down incident responders work from stale diagrams platform teams lose confidence in their own system map Documentation drift is not just a nuisance. In many environments, it becomes a governance failure.

### 2.3 Existing Visualization Tools Fail During Review

Most infrastructure visualization tools attempt to map the entire environment at once. That creates: dense unreadable graphs too many edges poor review usability low developer trust Developers reviewing a PR do not need the whole cloud account. They need: What changed, what it touches, and why it matters. This is the core UX insight behind ArchiteX.

### 2.4 Existing Security Tools Stop Too Low in the Stack

Current tools solve adjacent problems: SAST: code-level security flaws SCA: dependency vulnerabilities IaC scanners: static misconfigurations CSPM/CNAPP: runtime cloud posture Policy engines: rule enforcement at plan/apply time But none of them fully answer this question: Is this infrastructure change architecturally safe? ArchiteX occupies this missing layer.

### 2.5 Security and Compliance Buyers Reject Code Exfiltration

Many organizations, especially in regulated sectors, cannot adopt tools that require: syncing private repos to a third-party SaaS uploading infrastructure code externally granting full cloud account access without strict controls This is a major blocker for many modern AI-assisted developer tools. ArchiteX is designed specifically to remove that blocker.

## 3. Product Thesis

ArchiteX is built on one core thesis: Infrastructure architecture should be treated as a governable control surface, not just as code to be linted or diagrams to be drawn. This means ArchiteX is not fundamentally a documentation product. It is a security and governance product with strong developer experience. That distinction matters because it determines: who buys it how it is priced how it is trusted whether it becomes mandatory or optional If ArchiteX only generates diagrams and summaries, it remains a DevEx tool. If ArchiteX can deterministically identify risky topology changes, explain them, and enforce policies in CI, it becomes a real enterprise control. That is the category ArchiteX is built for.

## 4. Product Principles

ArchiteX is designed around six non-negotiable principles.

### 4.1 Deterministic First

All structural understanding, policy evaluation, and risk scoring must be deterministic and auditable. The LLM is never allowed to: decide risk classify compliance infer topology modify facts The LLM is limited to presentation only.

### 4.2 Zero Raw Code Exfiltration

Raw IaC code must never leave the customer environment. All parsing and graph construction happens locally on the customer runner. Only tightly sanitized metadata may leave the runner, and only when external presentation services are used.

### 4.3 Blast Radius Over Full Mapping

The default review mode is localized architectural context: changed resources one layer of dependencies directly affected trust boundaries This avoids the “spaghetti map” problem.

### 4.4 Day-1 Value Without Custom Setup

Customers should get useful results immediately. ArchiteX must ship with: built-in rules default scoring logic useful summaries readable visuals Custom policy authoring is an enterprise feature, not an MVP requirement.

### 4.5 Developer Utility Before Enforcement

Developers must find the tool useful before they accept it as a gate. ArchiteX should first win trust through: clarity speed useful review assistance low-noise findings Then it can become enforcement infrastructure.

### 4.6 Explainability Is Mandatory

Every meaningful output must be explainable: why the score is high why a policy failed what changed structurally what data left the runner No black boxes.

## 5. Product Scope

Initial Target Scope Platform: GitHub Format: Native GitHub App + GitHub Action Execution: Local CI Runner / GitHub Actions Runner IaC Scope: Terraform Cloud Scope: AWS Review Surface: Pull Requests Explicit Non-Goals for MVP The MVP will not attempt to: support all IaC languages map entire cloud estates replace Terraform plan replace runtime cloud security tools create a general architecture knowledge graph solve all compliance frameworks out of the box provide interactive full-scale architecture exploration The MVP is tightly scoped to one job: Catch risky Terraform AWS architecture changes in GitHub Pull Requests and make them instantly understandable.

## 6. Core User Personas

ArchiteX serves two different personas with one product surface, but they are buying different value.

### 6.1 Developer / Platform Engineer Persona

What they want: faster PR review easier Terraform understanding less cognitive load fewer hidden topology surprises better visibility into blast radius confidence before merge What they love: “Explain my change” visual deltas clear plain-English summaries useful review focus suggestions low noise This is the adoption persona.

### 6.2 Security / Compliance / Engineering Leadership Persona

What they want: deterministic governance enforceable architectural guardrails documented change history compliance evidence lower review risk fewer dangerous merges What they buy: policy engine CI blocking audit trail custom rules organization-wide governance This is the budget persona.

## 7. Category Positioning

ArchiteX is best understood as: The Architecture Firewall for CI/CD It sits between: code-level security tools and runtime cloud security platforms ArchiteX vs Diagram Tools Diagram tools help people communicate after the fact. ArchiteX prevents unsafe infrastructure changes before they happen. ArchiteX vs IaC Scanners IaC scanners identify resource-level misconfigurations. ArchiteX evaluates architectural relationships, dependency changes, and topology risk. ArchiteX vs Policy Engines Policy engines enforce explicit rules. ArchiteX adds: topology-aware semantic diffing default architecture intelligence visual review context historical change understanding ArchiteX vs CSPM/CNAPP CSPM tools inspect deployed environments. ArchiteX governs proposed infrastructure before deployment.

## 8. Product Architecture

ArchiteX uses a strict four-stage pipeline.

### Stage 1: Fact-Finder Deterministic Structural Parsing

The Fact-Finder runs entirely on the customer’s CI runner. Its job is to build a machine-readable infrastructure graph from the Terraform change set. Responsibilities parse Terraform configuration resolve local module relationships extract resources, references, and dependencies identify key security-relevant attributes normalize structure into canonical graph format Output A deterministic structured graph representation, for example: nodes edges resource metadata trust boundary tags exposure flags encryption attributes environment classification when inferable from deterministic rules Design Principle The Fact-Finder must rely on deterministic parsing and controlled evaluation only. It must not rely on LLMs. MVP Constraint The MVP will support a bounded subset of real-world Terraform patterns first, then expand. Priority will go to the patterns most common in production AWS repositories. 

### Stage 2: Delta Engine Semantic Architectural Diffing

The Delta Engine compares: the base graph the head graph Its purpose is not line diffing. Its purpose is structural meaning. Responsibilities detect added resources detect removed resources detect modified dependencies detect trust-boundary changes detect exposure changes isolate local blast radius produce a bounded delta subgraph for reviewer context Core Output A semantic architectural delta, not just a code diff. This is the foundation for: diagrams scoring summaries review focus suggestions 

### Stage 3: Risk & Policy Engine Deterministic Governance Layer

This is the core product moat. The Risk & Policy Engine must be fully: deterministic auditable version-controlled explainable tunable Hard Rule Risk scores are never generated by an LLM. Responsibilities evaluate delta graph against built-in rules apply policy severity levels calculate weighted risk score generate explainable reason codes determine PR pass/warn/fail status Risk Score Model Each PR receives a numeric score and severity band. Example: Risk Score: HIGH (8.3 / 10) Breakdown Exposure: 4.0 New public ingress path introduced Criticality: 2.5 Newly connected resource tagged as production data tier Change Scope: 1.8 Cross-boundary dependency chain expanded across critical services The scoring function must be documented and deterministic. Tunability Enterprise customers may: adjust severity thresholds disable specific default rules add custom rules define environment-specific sensitivity Day-1 Value ArchiteX ships with built-in opinionated architecture rules inspired by: AWS security best practices common IaC failure patterns network segmentation principles common audit concerns Example Built-In Rules No direct public path to private data tier No unrestricted ingress on sensitive ports Encryption required on persistent data resources No production database introduced without explicit access controls No new internet-facing resource in protected environment without approval tag No broadening of trust boundary between app and data layers without review 

### Stage 4: Interpreter Layer Constrained Presentation Layer

This is the only stage where an LLM may be used. Allowed Responsibilities convert sanitized delta metadata into Mermaid layout generate plain-English summary suggest reviewer focus areas improve readability of factual output Forbidden Responsibilities infer topology assign severity decide policy outcome invent relationships override deterministic output Safe Output Principle All LLM-generated content must be derived from deterministic structured inputs. If the LLM is unavailable, ArchiteX must still produce: score policy results machine-generated diagram fallback factual review output The LLM improves presentation. It is not part of the trust model.

## 9. Data Sanitization Model

Because “zero exfiltration” is a core buying reason, ArchiteX must define exactly what leaves the runner.

### 9.1 What Stays Local

The following never leave the customer CI environment: raw Terraform source files variable files secrets plan files state files provider credentials full resource names if configured as sensitive comments and surrounding code context

### 9.2 What May Leave the Runner

Only a sanitized delta payload may be transmitted, and only if the customer enables external presentation services. This payload may include: abstract node identifiers abstract resource types abstract edge relationships added / removed / modified markers severity reason codes generalized environment labels bounded metadata necessary for summary generation

### 9.3 Sanitization Rules

Sanitization must support: name redaction hashing or tokenization of identifiers environment aliasing configurable metadata suppression allowlist-based egress fields

### 9.4 Enterprise Requirement

ArchiteX must publish a machine-readable sanitization schema and egress specification showing exactly what bytes may leave the runner. This is essential for enterprise trust and procurement.

## 10. User Experience



### 10.1 Pull Request Payload

When a developer opens a PR containing Terraform changes, ArchiteX posts a structured PR comment. Section 1: Risk Score A visible severity banner, for example: Risk Score: HIGH (8.3 / 10) With clear reasons: Public ingress introduced New production-tier dependency added Blast radius expanded across trust boundary Section 2: Plain-English Summary Example: A new database resource was introduced and connected to an existing application tier inside the production VPC. The change increases data persistence dependencies and adds a new potential exposure path that should be reviewed carefully. Section 3: Suggested Review Focus Example: Verify security group rules on the new database path Confirm encryption and backup settings Review whether the new network path should be reachable from the public application tier Confirm this dependency is intended for production Section 4: Delta Diagram A focused diagram of only the changed resources plus one dependency layer. Visual conventions: green = added red = removed neutral = unchanged supporting context trust boundaries clearly indicated Section 5: Policy Result Example: 2 policy warnings 1 critical policy violation If blocking is enabled, the PR status check fails.

### 10.2 Audit Trail

ArchiteX may optionally commit generated artifacts to a versioned /docs path or store them in an audit log system. These artifacts include: diagram snapshot score breakdown triggered rules reviewer-facing summary timestamped architectural diff record This turns architecture documentation into an automatic byproduct of engineering work.

### 10.3 Developer Utility Features

To become loved, not merely tolerated, ArchiteX must provide immediate value to engineers. Examples: Explain My Change What Did I Break? Why Is This High Risk? Show Only What Changed What Should I Review First? The experience should feel like an architectural co-reviewer, not a noisy compliance bot.

## 11. Rollout Strategy

Blocking too early creates rejection. ArchiteX should roll out in stages. Phase 1: Visibility PR comments only no blocking trust building noise tuning Phase 2: Advisory Governance warnings soft policy thresholds team dashboards evidence collection Phase 3: Enforced Governance CI blocking for critical violations opt-in or policy-based activation custom rules and exception workflow This rollout preserves developer trust while enabling eventual control posture.

## 12. Pricing and Packaging

Because ArchiteX serves two personas, packaging must reflect both adoption and enforcement. Team Tier For developer-led adoption Indicative price: $99–149/month per team Includes: PR delta diagrams plain-English summaries basic deterministic risk scoring built-in rules limited history Purpose: fast adoption bottom-up usage prove immediate value Growth Tier For scaling platform teams Indicative price: $299–499/month Includes: full risk breakdown trend and history view soft policy enforcement basic reporting multi-repo coverage Purpose: move from utility to governance Enterprise Tier For security, compliance, and engineering leadership Indicative price: $1,000+/month, usage and org dependent Includes: CI blocking custom policy rules compliance audit logs SSO / RBAC sanitization controls organization-wide governance executive and audit-ready reporting Purpose: move into mandatory security budget

## 13. Go-To-Market Strategy

ArchiteX requires a dual-motion GTM strategy.

### 13.1 Bottom-Up Motion

The PR Payload is the viral mechanism. Flow: one engineer installs ArchiteX PR comment appears reviewers see value immediately adoption spreads repo to repo This mirrors successful developer tools that spread via workflow visibility.

### 13.2 Top-Down Motion

Enterprise monetization comes from: security compliance platform leadership To cross the gap, ArchiteX needs a champion toolkit. Champion Toolkit Every internal champion should be able to show: number of risky changes caught estimated review time saved examples of production-impacting findings audit trail artifacts generated mapping to compliance controls or governance requirements This is how developer love becomes executive approval.

### 13.3 Core GTM Message

For developers: “Understand Terraform changes instantly.” For platform teams: “Reduce review load and hidden blast radius.” For CISOs and compliance leaders: “Enforce architectural controls before deployment and generate auditable evidence automatically.”

## 14. Competitive Position

Against HashiCorp Sentinel / OPA They provide policy enforcement, but not strong reviewer-first architecture visualization and semantic delta context in PRs. ArchiteX wins on: earlier feedback better review ergonomics built-in default value architecture-first context Against Snyk IaC Snyk finds resource-level issues well, but ArchiteX focuses on topology and architectural relationships. ArchiteX wins on: architectural delta awareness blast radius visibility dependency reasoning trust boundary changes Against Runtime CSPM / CNAPP Those tools inspect deployed environments. ArchiteX wins by preventing risky architecture from ever being merged. Against Diagram Tools They document. ArchiteX governs.

## 15. Moat

The parser alone is not the moat. The moat is the combination of:

### 15.1 Local-Runner Architecture

A short-term enterprise wedge and procurement accelerator.

### 15.2 Semantic Delta Engine

Understanding architectural change, not just code change.

### 15.3 Deterministic Risk Model

An auditable, explainable architecture scoring framework.

### 15.4 Opinionated Default Rule Library

Immediate customer value without lengthy custom configuration.

### 15.5 Historical and Temporal Intelligence

Over time, ArchiteX will learn architectural patterns within a repository and highlight anomalies such as: first-time exposure events unusual dependency patterns sudden trust-boundary expansion drift from established repository norms This historical layer is the strongest long-term moat because it improves with repository-specific history.

## 16. Future Roadmap

After the MVP proves value in Terraform + AWS + GitHub, expansion can proceed carefully. Ecosystem Expansion GitLab Bitbucket API ingestion IaC Expansion Helm / Kubernetes Pulumi AWS CDK Visualization Expansion proprietary layout engine interactive graph exploration layered views zoomable DAG interface Intelligence Expansion temporal anomaly detection drift trend reporting architecture review analytics approval workflows and exceptions organization-wide architectural posture scoring

## 17. MVP Definition

The MVP must prove one thing: Can ArchiteX accurately and usefully detect risky Terraform AWS architectural changes in PRs, with low enough noise that developers keep it enabled? MVP Success Criteria parses real Terraform repositories with acceptable accuracy produces useful delta diagrams generates trustworthy deterministic scores catches issues developers care about creates low enough noise to preserve trust gives platform/security teams enough evidence to explore rollout MVP Deliverables Terraform structural parser for bounded AWS scope Base vs head graph diff engine Built-in deterministic ruleset Explainable risk scoring model GitHub PR comment integration Diagram generation Sanitized egress schema Basic audit artifact generation

## 18. Single Biggest Execution Risk

The biggest early risk is not GTM. It is accuracy. If ArchiteX cannot correctly parse and interpret real-world Terraform, everything downstream breaks: wrong diagrams wrong scores false positives developer distrust enterprise rejection The most difficult cases include: modules dynamic blocks for_each count conditional expressions external data-driven config patterns indirect dependencies Because of this, MVP scope discipline is critical. The product should not pretend to solve all Terraform immediately. It should instead: support a carefully chosen production-relevant subset first be explicit about unsupported patterns degrade safely make uncertainty visible when confidence is limited Trust is more important than breadth.

## 19. Strategic Decision: Deterministic vs Probabilistic

This must be an explicit architectural decision. Decision ArchiteX will use deterministic logic for all trust-sensitive outputs. That includes: graph construction semantic diffing policy evaluation risk scoring CI pass/fail decisions LLM Usage LLMs may be used only for: natural-language summaries review suggestions diagram layout refinement This is not optional. It is essential to: enterprise trust security credibility auditability procurement success If ArchiteX blurs this line, it stops being a true control and becomes just another AI dev tool.

## 20. Why Companies Will Buy It

Companies will buy ArchiteX because it helps them do something they currently do poorly, manually, and inconsistently: review risky infra changes keep architecture documentation current reduce audit pain impose architecture guardrails without slowing every team down create a trustworthy pre-deployment control layer ArchiteX converts architecture review from: tribal knowledge stale diagrams stressed senior reviewers vague security assumptions into: deterministic controls visible blast radius versioned evidence faster safe review

## 21. Why Developers Will Love It

Developers will love ArchiteX if it does three things well: shows only the relevant change explains it clearly stays accurate and low-noise If it helps them answer: what changed why it matters what to review whether they broke something then it becomes a real workflow improvement, not just another gate. That is how ArchiteX becomes both: a product companies buy and a tool developers actually keep turned on

## 22. Final Product Definition

ArchiteX is: A deterministic architecture intelligence and governance layer for Infrastructure-as-Code, embedded in CI/CD, that detects, explains, and optionally blocks risky infrastructure topology changes before deployment. It is not: just a diagram generator just an LLM wrapper just another IaC linter just a policy engine It is the missing architecture control layer between code review and deployment.
