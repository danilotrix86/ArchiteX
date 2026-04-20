# Security Policy

ArchiteX is a static analysis tool that runs on its operator's own CI infrastructure. It has no SaaS component, no telemetry, and no account system, so there is nothing to compromise on the project's side. The threat model is limited to the binary you run and the GitHub Action that invokes it. The two failure modes that matter to us are:

1. **A vulnerability in the parser, graph builder, risk engine, or interpreter** that lets a crafted Terraform input crash the tool, exfiltrate the runner's environment, or produce silently incorrect risk output that hides a real misconfiguration.
2. **A vulnerability in the GitHub Action wrapper** (`action.yml`, the comment poster, the audit-bundle uploader) that lets a hostile PR escalate beyond the documented `pull-requests: write / contents: read` scope or read secrets it should not.

## Reporting a vulnerability

Please **do not open a public GitHub issue** for security reports. Use one of the following channels:

- **Preferred:** [GitHub private security advisories](https://github.com/danilotrix86/ArchiteX/security/advisories/new). This routes the report directly to the maintainer with no public surface and supports private fix collaboration + coordinated disclosure.
- Alternatively, open an empty public issue titled `security: please contact me` and the maintainer will reach out via the email on the GitHub profile to continue the conversation in private.

Please include:

- ArchiteX version (`./architex version`) or commit SHA
- A minimal Terraform input or workflow snippet that reproduces the issue
- The observed behavior vs. what you expected
- Any proof-of-concept artifacts (logs, generated graph JSON, etc.)

## Response expectations

ArchiteX is currently maintained by a single person on a best-effort basis. We aim to:

- Acknowledge a report within **5 business days**.
- Decide whether the report is in scope and share an initial severity assessment within **10 business days**.
- For in-scope, exploitable issues, ship a fix in the **next minor release** (`v1.x.0`) or sooner via a patch release if the impact warrants it. The release notes will credit the reporter unless they ask to remain anonymous.

Reports about deterministic-but-suboptimal risk scoring, missing resource-type coverage, or false positives / negatives are not security issues. Please file those as normal `coverage`, `false-negative`, or `false-positive` issues using the templates in `.github/ISSUE_TEMPLATE/`.

## Supported versions

Only the latest minor release line receives security fixes. As of `v1.3.x`, that is `v1.3`. Older `v1.x` releases are end-of-life on a fix-only basis -- if you are pinned to one and hit a security issue, expect to be advised to upgrade to the latest `v1.x` tag.
