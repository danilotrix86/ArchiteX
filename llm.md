# ArchiteX -- LLM Context File

Use this document to onboard into what has been built so far. Read it fully before making changes.

## What is ArchiteX?

A DevSecOps CLI tool that parses Terraform (`.tf`) files, builds an architecture graph, computes semantic deltas between graph versions, and evaluates architectural risk. It runs locally or in CI. It is NOT a full Terraform interpreter -- it handles a narrow, curated subset of constructs.

## Current Phase: Phase 4 (COMPLETE)

Phase 4 delivers Stage 4 of the pipeline -- the deterministic Interpreter Layer. Given a `delta.Delta` and a `risk.RiskResult`, it produces a Mermaid delta diagram, a plain-English summary, reviewer-focus bullets, a Markdown PR comment, a sanitized egress payload (with a published JSON Schema), and an on-disk audit bundle. There is no LLM, no network, and no GitHub integration in this phase -- those are Phase 5/6. The `Interpreter` interface is the seam where an LLM provider can be slotted in later without touching the diagram, sanitizer, or formatter.

### Phase 1 (COMPLETE)

- HCL parsing of `.tf` files in a directory
- Resource extraction for 7 supported AWS types
- Cross-resource reference detection
- Edge inference with typed relationships
- Derived attributes (public/private)
- Confidence scoring with warnings
- JSON output to stdout, warnings to stderr

### Phase 2 (COMPLETE)

- Graph-to-graph comparison via `delta.Compare(base, head)`
- Node diff by ID: added, removed, changed
- Edge diff by composite key (`from|to|type`): added, removed
- Attribute diff via `reflect.DeepEqual` with before/after tracking
- Deterministic output ordering (nodes by ID, edges by from/to/type, changed nodes by ID)
- Empty slices in JSON (never `null`)
- `HumanSummary()` helper with correct pluralization
- 9 unit tests covering all delta cases

### Phase 3 (COMPLETE)

- Rule-based risk evaluation via `risk.Evaluate(d delta.Delta)`
- 5 deterministic rules: public exposure, new data resource, new entry point, potential data exposure, resource removal
- Numeric score (0--10) with severity (low/medium/high) and status (pass/warn/fail)
- Explainable output: each triggered rule produces a RiskReason with rule ID, message, impact label, and weight
- Score = sum of triggered rule weights, capped at 10.0, rounded to 1 decimal
- Reasons sorted by weight descending, rule ID ascending for ties
- 7 unit tests covering all rules, edge cases, and score capping

### Phase 4 (COMPLETE)

- Deterministic Interpreter Layer via `interpreter.Render(d, r, interp) Report`
- Mermaid `flowchart LR` renderer for the delta sub-graph (added/removed/changed/context nodes, dashed arrows for removed edges)
- Plain-English summary + review-focus bullets templated from `RiskReason` + `delta.Delta` (no free-form text parsing)
- Markdown PR-comment formatter producing the five sections from `master.md` §10.1
- `EgressPayload` + `Sanitize()` with name hashing (salted SHA-256, 32-bit prefix), abstract-type-only metadata, rule IDs only (no message text), no attribute values
- Published `docs/egress-schema.json` (JSON Schema draft-07) -- a parity test fails the build if `EgressPayload` and the schema diverge
- `Interpreter` interface (`Summary`, `ReviewFocus`) -- `DeterministicInterpreter` is the default; the diagram is intentionally NOT routed through this interface (trust surface)
- Audit artifact writer (`WriteAudit`) producing a timestamped bundle: `diagram.mmd`, `summary.md`, `score.json`, `egress.json`, `manifest.json` with SHA-256 checksums
- Two new CLI subcommands: `architex report` (Markdown to stdout, optional `--out` for audit bundle) and `architex sanitize` (egress JSON to stdout)
- 29 unit tests across 5 files covering renderer determinism, summary/focus templating, Markdown shape, no-leak sanitization, schema parity, audit checksums, and the Interpreter seam

## Tech Stack

- Language: Go 1.26
- Module name: `architex`
- Key dependency: `github.com/hashicorp/hcl/v2` (v2.24.0) for HCL parsing
- Secondary dep: `github.com/zclconf/go-cty` (v1.18.0) for cty-to-Go value conversion

## Project Structure

```
go.mod
go.sum
main.go                  # CLI entry point: graph | diff | score | report | sanitize
models/models.go         # Core shared types (Graph, Node, Edge, Confidence, Warning, RawResource)
parser/parser.go         # HCL file/dir parsing, resource + reference extraction
parser/extract.go        # Attribute eval, nested-block walk, reference traversal helpers
graph/graph.go           # Graph construction (nodes, edges, derived attrs, confidence)
delta/delta.go           # Delta Engine: Compare(), HumanSummary(), delta types
risk/risk.go             # Risk Engine: Evaluate(), scoring, severity/status, JSON formatting
risk/rules.go            # 5 rule implementations
interpreter/interpreter.go  # Stage 4: Report struct, Interpreter interface, Render()
interpreter/mermaid.go      # Deterministic Mermaid flowchart renderer
interpreter/summary.go      # DeterministicInterpreter (Summary + ReviewFocus templates)
interpreter/markdown.go     # Five-section PR-comment formatter
interpreter/sanitize.go     # EgressPayload + Sanitize() (name hashing, allowlisted fields)
interpreter/audit.go        # WriteAudit() -- timestamped bundle with manifest checksums
docs/egress-schema.json     # Published JSON Schema for EgressPayload (parity-tested)
testdata/
  main.tf                # Example Terraform exercising all 7 resource types
  base/ + head/          # Scenario: SG opened + LB added (high risk)
  db_added_base/head/    # Scenario: DB added only (low risk)
  removed_base/head/     # Scenario: instance removed (low risk)
```

## Usage

The CLI exposes five subcommands. Machine-readable artifacts (JSON, Markdown) go to stdout. Warnings and human-readable summaries go to stderr, prefixed with `[architex]`. Flags can appear before OR after positional arguments.

```bash
# Build the graph for a single Terraform directory.
go run . graph ./testdata/

# Print the semantic delta between two graphs.
go run . diff ./testdata/base ./testdata/head

# Evaluate risk for the delta between two graphs.
go run . score ./testdata/base ./testdata/head

# Render the full PR-ready Markdown payload (banner + summary + focus + diagram + policy).
go run . report ./testdata/base ./testdata/head

# Same as above, plus persist a timestamped audit bundle to ./.architex/.
go run . report ./testdata/base ./testdata/head --out ./.architex/

# Print the sanitized egress payload only (the bytes that would leave a runner).
go run . sanitize ./testdata/base ./testdata/head --salt my-run-salt

# Or build once and reuse:
go build -o architex . && ./architex score ./base ./head
```

Parser warnings on stderr have the form `[architex] WARN [<category>]: <message>`, where `<category>` is one of `unsupported_resource`, `unsupported_construct`, `parse_error`, `info`.

## Data Pipeline

```
.tf files (base)                    .tf files (head)
  |                                   |
  v                                   v
parser.ParseDir(baseDir)            parser.ParseDir(headDir)
  |                                   |
  v                                   v
[]RawResource + warnings            []RawResource + warnings
  |                                   |
  v                                   v
graph.Build(resources, warnings)    graph.Build(resources, warnings)
  |                                   |
  v                                   v
models.Graph (base)                 models.Graph (head)
  |                                   |
  +-----------------------------------+
  |
  v
delta.Compare(base, head)
  |-- nodes: added / removed / changed (by ID)
  |-- edges: added / removed (by from|to|type key)
  |-- attributes: before/after diff (reflect.DeepEqual)
  |-- deterministic sort on all output slices
  |
  v
delta.Delta
  |
  v
risk.Evaluate(delta)
  |-- 5 rules evaluated against delta
  |-- score = sum(weights), capped at 10.0
  |-- severity + status derived from score
  |
  v
risk.RiskResult  -->  JSON
```

The CLI in `main.go` exposes the full pipeline through five subcommands:

- `graph <dir>` runs the left side only (parse -> Build).
- `diff <base> <head>` runs both sides through `delta.Compare`.
- `score <base> <head>` runs the entire pipeline through `risk.Evaluate`.
- `report <base> <head> [--out <dir>] [--salt <s>]` runs the entire pipeline through `interpreter.Render` and prints the Markdown PR comment. With `--out`, also persists an audit bundle.
- `sanitize <base> <head> [--salt <s>]` runs the entire pipeline through `interpreter.Sanitize` and prints the egress payload JSON.

After Stage 3 the pipeline forks into Stage 4:

```
risk.RiskResult + delta.Delta
  |
  v
interpreter.Render(d, r, interp) -> Report
  |-- Diagram     (interpreter.RenderMermaid -- always deterministic)
  |-- Summary     (interp.Summary, default = DeterministicInterpreter)
  |-- ReviewFocus (interp.ReviewFocus, default = DeterministicInterpreter)
  |
  +--> interpreter.FormatMarkdown(rep) -> PR comment string
  +--> interpreter.Sanitize(rep, policy) -> EgressPayload (the only bytes allowed to leave)
  +--> interpreter.WriteAudit(rep, opts) -> on-disk bundle with manifest checksums
```

## Core Types (models/models.go)

```go
type Graph struct {
    Nodes      []Node     `json:"nodes"`
    Edges      []Edge     `json:"edges"`
    Confidence Confidence `json:"confidence"`
}

type Node struct {
    ID           string         `json:"id"`            // "aws_instance.web"
    Type         string         `json:"type"`           // abstract: "compute"
    ProviderType string         `json:"provider_type"`  // "aws_instance"
    Attributes   map[string]any `json:"attributes"`     // {"public": true}
}

type Edge struct {
    From string `json:"from"`
    To   string `json:"to"`
    Type string `json:"type"`  // "attached_to", "deployed_in", "part_of", etc.
}

type Confidence struct {
    Score    float64   `json:"score"`    // 1.0 = perfect, reduced by warnings
    Warnings []Warning `json:"warnings"` // typed; never null in JSON
}

// Warning categories (constants in models/models.go):
//   WarnUnsupportedResource  = "unsupported_resource"
//   WarnUnsupportedConstruct = "unsupported_construct"
//   WarnParseError           = "parse_error"
//   WarnInfo                 = "info"
type Warning struct {
    Category string `json:"category"`
    Message  string `json:"message"`
}

type RawResource struct {
    Type       string
    Name       string
    ID         string                  // "type.name"
    Attributes map[string]any          // literal values; nil for unresolvable
    References []Reference
}

type Reference struct {
    SourceAttr string  // attribute name where ref was found
    TargetID   string  // e.g. "aws_security_group.web"
}
```

## Delta Types (delta/delta.go)

```go
type Delta struct {
    AddedNodes   []models.Node `json:"added_nodes"`
    RemovedNodes []models.Node `json:"removed_nodes"`
    AddedEdges   []models.Edge `json:"added_edges"`
    RemovedEdges []models.Edge `json:"removed_edges"`
    ChangedNodes []ChangedNode `json:"changed_nodes"`
    Summary      DeltaSummary  `json:"summary"`
}

type ChangedNode struct {
    ID                string                      `json:"id"`
    Type              string                      `json:"type"`          // abstract type, e.g. "access_control"
    ProviderType      string                      `json:"provider_type"` // e.g. "aws_security_group"
    ChangedAttributes map[string]ChangedAttribute `json:"changed_attributes"`
}

type ChangedAttribute struct {
    Before any `json:"before"`
    After  any `json:"after"`
}

type DeltaSummary struct {
    AddedNodes   int `json:"added_nodes"`
    RemovedNodes int `json:"removed_nodes"`
    AddedEdges   int `json:"added_edges"`
    RemovedEdges int `json:"removed_edges"`
    ChangedNodes int `json:"changed_nodes"`
}
```

## Delta Comparison Rules (delta/delta.go)

### Node comparison (by `node.ID`)
- In head but not base -> `added_nodes`
- In base but not head -> `removed_nodes`
- In both -> compare `Attributes` with `reflect.DeepEqual`; if any key differs -> `changed_nodes`
- Changed nodes carry the head's `Type` and `ProviderType` so downstream consumers (e.g. risk) don't need to re-parse the ID
- Attributes added or removed between versions are tracked (before/after = nil for missing side)

### Edge comparison (by composite key `from|to|type`)
- In head but not base -> `added_edges`
- In base but not head -> `removed_edges`

### Deterministic ordering
- Nodes sorted by ID
- Edges sorted by From, then To, then Type
- Changed nodes sorted by ID

### JSON conventions
- Empty slices serialize as `[]`, never `null`
- Summary counts are computed automatically from slice lengths

### Public API
- `delta.Compare(base, head models.Graph) delta.Delta`
- `delta.HumanSummary(d delta.Delta) string` -- e.g. `"1 node added, 2 edges removed, 1 node changed"`

## Risk Types (risk/risk.go)

```go
type RiskResult struct {
    Score    float64      `json:"score"`
    Severity string       `json:"severity"` // low, medium, high
    Status   string       `json:"status"`   // pass, warn, fail
    Reasons  []RiskReason `json:"reasons"`
}

type RiskReason struct {
    RuleID  string  `json:"rule_id"`
    Message string  `json:"message"`
    Impact  string  `json:"impact"`  // exposure, data, data_exposure, change
    Weight  float64 `json:"weight"`
}
```

### Public API

- `risk.Evaluate(d delta.Delta) risk.RiskResult`

### Risk Rules

| Rule ID                       | Trigger                                                        | Weight | Impact          |
|-------------------------------|----------------------------------------------------------------|--------|-----------------|
| `public_exposure_introduced`  | ChangedNode `public` went false -> true                        | 4.0    | `exposure`      |
| `new_data_resource`           | AddedNode with Type `"data"`                                   | 2.5    | `data`          |
| `new_entry_point`             | AddedNode with Type `"entry_point"`                            | 3.0    | `exposure`      |
| `potential_data_exposure`     | Rule 1 triggered AND (data node added OR security-related change) | 2.0 | `data_exposure` |
| `resource_removed`            | RemovedNode (0.5 each, capped at 1.0 total)                   | 0.5    | `change`        |

"Security-related change" = any ChangedNode whose abstract `Type` is `access_control` or `data` (read directly from the ChangedNode, no re-derivation from the ID).

Rule 1 (`public_exposure_introduced`) requires both `Before` and `After` to type-assert as `bool`. If either side is a non-bool (e.g. from a JSON round-trip that produced a string), the rule does not fire -- it does not panic and does not silently match. Rule 5 (`resource_removed`) emits at most `removalMaxReasons` (= 2) reasons at 0.5 each, capping total weight at 1.0.

### Severity / Status Mapping

| Score Range | Severity | Status |
|-------------|----------|--------|
| 0.0 -- 2.9  | low      | pass   |
| 3.0 -- 6.9  | medium   | warn   |
| 7.0 -- 10.0 | high     | fail   |

### Output Determinism

- Score = sum of all triggered rule weights, capped at 10.0, rounded to 1 decimal place
- Reasons sorted by weight descending; ties broken by rule ID ascending
- Empty Reasons slice serializes as `[]`, never `null`

## Interpreter Types (interpreter/interpreter.go)

```go
type Report struct {
    Delta       delta.Delta     `json:"delta"`
    Risk        risk.RiskResult `json:"risk"`
    Diagram     string          `json:"diagram"`      // Mermaid source
    Summary     string          `json:"summary"`      // plain English
    ReviewFocus []string        `json:"review_focus"` // bullets
}

type Interpreter interface {
    Summary(d delta.Delta, r risk.RiskResult) string
    ReviewFocus(d delta.Delta, r risk.RiskResult) []string
}

type DeterministicInterpreter struct{} // default; template-based, no I/O
```

### Public API

- `interpreter.Render(d delta.Delta, r risk.RiskResult, interp Interpreter) Report`
- `interpreter.RenderMermaid(d delta.Delta) string`
- `interpreter.FormatMarkdown(rep Report) string`
- `interpreter.Sanitize(rep Report, policy SanitizationPolicy) EgressPayload`
- `interpreter.WriteAudit(rep Report, opts AuditOptions) (AuditBundle, error)`
- `interpreter.SchemaVersion` constant -- bump when `EgressPayload` shape changes
- `interpreter.ToolVersion` constant -- bump when audit bundle layout changes

### Mermaid Renderer

- Output is always a `flowchart LR` with four `classDef`s: `added`, `removed`, `changed`, `context`
- `classDef`s use stroke-only styling so themed environments (GitHub PRs, dark mode) render correctly
- Status is also encoded in the node label as a leading marker: `+ ` (added), `- ` (removed), `~ ` (changed), no marker (context)
- Removed edges use the dashed arrow `-.->` to distinguish them from added edges (`-->`)
- Node IDs are sanitized: any character outside `[a-zA-Z0-9_]` is replaced with `_`
- Edge endpoints not present in `AddedNodes`/`RemovedNodes`/`ChangedNodes` are emitted as `context` nodes (one-layer dependency surface)
- Empty delta produces a single placeholder node `empty["no architectural changes"]:::context`
- Sort order: nodes by original ID; edges by `from`, `to`, `type`, with added before removed for ties

### Markdown Formatter (mirrors master.md §10.1)

Five sections in a fixed order, every PR:

1. **Risk Level banner** -- `## [OK|WARN|FAIL] Risk Level: <SEVERITY> (X.X/10 &mdash; higher means more risk)` plus status/severity line. The "higher means more risk" qualifier is intentional UX -- without it, non-technical reviewers misread "9.0/10" as a school-grade-style high mark instead of a danger signal.
2. **Plain-English Summary** -- single paragraph from `DeterministicInterpreter.Summary`
3. **Suggested Review Focus** -- ordered bullets from `DeterministicInterpreter.ReviewFocus`, sorted by rule weight descending
4. **Delta Diagram** -- the Mermaid source wrapped in a fenced ` ```mermaid ` block
5. **Policy Result** -- one bullet per `RiskReason` (impact label, rule ID, weight, message); `"No policy violations triggered."` when empty

A trailing horizontal rule + `_Generated by ArchiteX (deterministic mode)._` footer makes the comment identifiable for future updates.

## Egress Schema (the §9.4 contract)

`EgressPayload` is the only shape allowed to leave a customer runner. Every field maps 1:1 to `docs/egress-schema.json` (JSON Schema draft-07). The `TestEgressPayload_SchemaParity` test fails if either side adds a field without the other.

| Field                  | Type     | Notes                                                       |
|------------------------|----------|-------------------------------------------------------------|
| `schema_version`       | string   | semver of the egress schema (`SchemaVersion` constant)      |
| `score`                | number   | 0--10, mirrored from `RiskResult.Score`                     |
| `severity`             | enum     | `low`/`medium`/`high`                                       |
| `status`               | enum     | `pass`/`warn`/`fail`                                        |
| `reason_codes`         | string[] | rule IDs only -- free-form `RiskReason.Message` is excluded |
| `added_nodes`          | object[] | `{id, type}` -- ID is salted SHA-256 prefix `n_xxxxxxxx`    |
| `removed_nodes`        | object[] | same shape as added                                          |
| `changed_nodes`        | object[] | `{id, type, changed_attribute_keys}` -- KEYS only, no values |
| `added_edges`          | object[] | `{from, to, type}` -- endpoints hashed                       |
| `removed_edges`        | object[] | same shape                                                   |
| `summary`              | object   | mirror of `delta.DeltaSummary` (counts only)                 |

`SanitizationPolicy` exposes a single knob (`HashSalt`). Empty salt = stable IDs across runs (good for trend diffing). Per-run salt = opaque IDs (good for one-off submissions).

## Audit Bundle Layout

`WriteAudit` produces a directory named `<YYYYMMDD-HHMMSS>-<8 hex>` under the configured `OutDir`. The 8-hex suffix is `sha256(timestamp|baseDir|headDir)[:4]` so concurrent jobs at the same wall-clock second don't collide.

```
.architex/
  20260419-143022-a1b2c3d4/
    diagram.mmd       # Mermaid source
    summary.md        # the PR comment
    score.json        # full risk.RiskResult
    egress.json       # what would leave the runner
    manifest.json     # timestamps, dirs, tool/schema versions, file checksums
```

`manifest.json` includes SHA-256 hex digests for the four content files. The manifest does NOT contain its own digest (a manifest cannot self-reference). `TestWriteAudit_ManifestChecksumsMatchFiles` round-trips this contract.

## Supported Resources

| Terraform Type              | Abstract Type    |
|-----------------------------|------------------|
| `aws_instance`              | `compute`        |
| `aws_db_instance`           | `data`           |
| `aws_lb`                    | `entry_point`    |
| `aws_vpc`                   | `network`        |
| `aws_subnet`                | `network`        |
| `aws_security_group`        | `access_control` |
| `aws_security_group_rule`   | `access_control` |

Defined in `models.SupportedResources` (allowlist) and `models.AbstractionMap`.

## Edge Type Inference (graph/graph.go)

Edges are typed by looking up `"sourceType|targetType"` in a map:

| Source -> Target                                | Edge Type     |
|-------------------------------------------------|---------------|
| `aws_instance` -> `aws_security_group`          | `attached_to` |
| `aws_instance` -> `aws_subnet`                  | `deployed_in` |
| `aws_subnet` -> `aws_vpc`                       | `part_of`     |
| `aws_security_group` -> `aws_vpc`               | `part_of`     |
| `aws_security_group_rule` -> `aws_security_group` | `applies_to`  |
| `aws_lb` -> `aws_subnet`                        | `deployed_in` |
| `aws_lb` -> `aws_security_group`                | `attached_to` |
| `aws_db_instance` -> `aws_subnet`               | `deployed_in` |
| `aws_db_instance` -> `aws_security_group`       | `attached_to` |
| (anything else)                                 | `references`  |

Edges are deduplicated by `from|to|type` key. Edges are only created when the target resource exists in the parsed set.

## Derived Attribute Rules (graph/graph.go)

- `aws_lb` -> always `public: true`
- `aws_db_instance` -> always `public: false`
- `aws_security_group` / `aws_security_group_rule` -> `public: true` if `cidr_blocks` contains `"0.0.0.0/0"`
- `aws_instance` -> `public: true` if `associate_public_ip_address` is `true`
- Everything else -> `public: false`

## Reference Detection (parser/parser.go)

References are found by calling `expr.Variables()` on every attribute expression (including inside nested blocks like `ingress`/`egress`). Each traversal is checked:

1. Must have at least 2 segments (e.g. `aws_security_group.web`)
2. First segment must be a supported resource type (filters out `var.*`, `local.*`, `data.*`, `module.*`)
3. Result is `"type.name"` (third+ segments like `.id` are dropped)

## Confidence Scoring (graph/graph.go)

Starts at `1.0`. Deductions are looked up by `Warning.Category` in `confidenceDeduction` (no string matching against message text):

| Category                  | Constant                          | Deduction |
|---------------------------|-----------------------------------|-----------|
| `unsupported_resource`    | `models.WarnUnsupportedResource`  | -0.10     |
| `unsupported_construct`   | `models.WarnUnsupportedConstruct` | -0.05     |
| `parse_error`             | `models.WarnParseError`           | -0.15     |
| `info`                    | `models.WarnInfo`                 | 0.00      |

Floor: `0.0`. A nil/empty warnings input always serializes as `[]`, never `null`.

## Unsupported Constructs (parser/parser.go)

These are detected and logged, NOT silently skipped. Each warning carries a category constant from `models` -- this is the contract between parser and graph; warning message text is for humans only.

| Construct                                 | Category                          | Result                  |
|-------------------------------------------|-----------------------------------|-------------------------|
| `for_each` / `count` on a resource        | `unsupported_construct`           | Resource skipped        |
| `dynamic` nested block                    | `unsupported_construct`           | Resource skipped        |
| `module` block                            | `unsupported_construct`           | No resource produced    |
| Unsupported resource type                 | `unsupported_resource`            | Resource skipped        |
| Unknown top-level block type              | `unsupported_construct`           | Skipped                 |
| `.tf` file fails to parse                 | `parse_error`                     | File skipped            |
| Empty supported-resource set in directory | `info`                            | Logged, no score impact |
| `data`/`variable`/`output`/`terraform`/`provider`/`locals` | n/a              | Silently skipped (normal TF) |

## Attribute Extraction (parser/parser.go)

(Note: see `parser/extract.go` for the actual implementation.)

- Each top-level attribute expression is evaluated with `expr.Value(nil)` (no eval context). On evaluation failure the attribute is stored as `nil`.
- Nested-block attributes (e.g. `cidr_blocks` inside `ingress`/`egress`) are walked the same way: on success the value is stored, on failure the key is recorded with `nil` (matches the top-level behavior). This consistency lets downstream code treat "key exists but value is nil" as a uniform "unresolvable" signal.
- `cidr_blocks` specifically is promoted to the top-level attribute map so the derived-attribute logic finds it. Multiple nested `cidr_blocks` (e.g. several `ingress` blocks) are merged with `mergeSlices`.
- All other nested attributes are stored under `<blockType>.<attrName>` (e.g. `ingress.from_port`).
- Values are converted from `cty.Value` to native Go types via `ctyToGo` (handles string, number, bool, list, tuple, set, map, object).

## Example Output

Running against `testdata/main.tf` produces 9 nodes, 11 edges, confidence 1.0:

```json
{
  "nodes": [
    {"id": "aws_vpc.main", "type": "network", "provider_type": "aws_vpc", "attributes": {"public": false}},
    {"id": "aws_subnet.public", "type": "network", "provider_type": "aws_subnet", "attributes": {"public": false}},
    {"id": "aws_subnet.private", "type": "network", "provider_type": "aws_subnet", "attributes": {"public": false}},
    {"id": "aws_security_group.web", "type": "access_control", "provider_type": "aws_security_group", "attributes": {"public": true}},
    {"id": "aws_security_group.db", "type": "access_control", "provider_type": "aws_security_group", "attributes": {"public": false}},
    {"id": "aws_security_group_rule.allow_all_outbound", "type": "access_control", "provider_type": "aws_security_group_rule", "attributes": {"public": true}},
    {"id": "aws_instance.web", "type": "compute", "provider_type": "aws_instance", "attributes": {"public": true}},
    {"id": "aws_lb.web", "type": "entry_point", "provider_type": "aws_lb", "attributes": {"public": true}},
    {"id": "aws_db_instance.main", "type": "data", "provider_type": "aws_db_instance", "attributes": {"public": false}}
  ],
  "edges": [
    {"from": "aws_subnet.public", "to": "aws_vpc.main", "type": "part_of"},
    {"from": "aws_subnet.private", "to": "aws_vpc.main", "type": "part_of"},
    {"from": "aws_security_group.web", "to": "aws_vpc.main", "type": "part_of"},
    {"from": "aws_security_group.db", "to": "aws_vpc.main", "type": "part_of"},
    {"from": "aws_security_group.db", "to": "aws_security_group.web", "type": "references"},
    {"from": "aws_security_group_rule.allow_all_outbound", "to": "aws_security_group.web", "type": "applies_to"},
    {"from": "aws_instance.web", "to": "aws_subnet.public", "type": "deployed_in"},
    {"from": "aws_instance.web", "to": "aws_security_group.web", "type": "attached_to"},
    {"from": "aws_lb.web", "to": "aws_security_group.web", "type": "attached_to"},
    {"from": "aws_lb.web", "to": "aws_subnet.public", "type": "deployed_in"},
    {"from": "aws_db_instance.main", "to": "aws_security_group.db", "type": "attached_to"}
  ],
  "confidence": {"score": 1, "warnings": []}
}
```

## Test Coverage

59 passing tests across 5 packages. Run `go test ./...` to verify.

| Package      | Tests | Files                                                                                      |
|--------------|-------|--------------------------------------------------------------------------------------------|
| parser       | 7     | parser_test.go                                                                             |
| graph        | 5     | graph_test.go                                                                              |
| delta        | 9     | delta_test.go                                                                              |
| risk         | 9     | risk_test.go                                                                               |
| interpreter  | 29    | mermaid_test.go, summary_test.go, markdown_test.go, sanitize_test.go, audit_test.go, interpreter_test.go (+ fixtures_test.go) |

## Design Decisions to Preserve

1. **`hclsyntax.Body` direct traversal** -- we do NOT use `gohcl.DecodeBody` (schema-based). This is intentional: we generically walk blocks and attributes without needing Go struct definitions for every Terraform resource schema.

2. **`expr.Value(nil)` for attribute evaluation** -- no eval context. Most expressions with variable references will fail and return nil. This is expected and by design. We only capture literals.

3. **Reference-only edges** -- edges come exclusively from HCL expression references. We do NOT infer edges from attribute values (e.g., matching CIDR ranges). This keeps the logic simple and reliable.

4. **Unsupported = logged, not silent** -- every unsupported construct produces a warning and hits the confidence score. This is a trust signal for downstream consumers.

5. **Nested block walking** -- `cidr_blocks` inside `ingress`/`egress` blocks is promoted to the parent resource's attribute map. References inside nested blocks are also extracted with a `blockType.` prefix on the source attribute name.

## Design Decisions to Preserve (Phase 2)

6. **Delta types in `delta/` package** -- delta-specific types (`Delta`, `ChangedNode`, `ChangedAttribute`, `DeltaSummary`) live in the `delta` package, keeping `models/` focused on core graph types.

7. **`reflect.DeepEqual` for attribute comparison** -- safe and correct for `map[string]any` values without fragile string coercion.

8. **Empty slices, not nil** -- all delta slices are initialized with `make([]T, 0)` so JSON always produces `[]` instead of `null`. Cleaner for downstream consumers.

9. **CLI is subcommand-based** -- `main.go` exposes `graph`, `diff`, and `score`. There is no separate demo binary; `cmd/riskdemo/` was removed during the hardening pass. New subcommands should follow the same shape: parse args, build graph(s), call into pipeline, write JSON to stdout, write human summary to stderr prefixed with `[architex]`.

## Design Decisions to Preserve (Phase 3)

10. **Risk types in `risk/` package** -- risk-specific types (`RiskResult`, `RiskReason`) and all rule logic live in the `risk` package. The package depends on `delta` and `models` but nothing depends on `risk`.

11. **Delta-only evaluation** -- `risk.Evaluate` receives only a `delta.Delta`, not the full graph. This limits what rules can detect (e.g., pre-existing data nodes not in the delta are invisible) but keeps the API surface minimal. Rule 4 compensates by also triggering on security-related changes in the delta.

12. **Rule 4 broad trigger** -- `potential_data_exposure` fires when public exposure is introduced AND either a data node was added OR a security-related node (access_control/data abstract type) was changed. This deliberately over-flags rather than under-flags, since the delta alone cannot see the full graph.

13. **Removal weight cap** -- Rule 5 caps total removal weight at 1.0 (max 2 reasons at 0.5 each). This prevents a large teardown from dominating the score.

14. **No probabilistic logic** -- all rules are deterministic boolean checks. No ML, no LLM, no heuristics. Same input always produces same output.

15. **Defensive type assertions on `any`-typed delta values** -- `ChangedAttribute.Before`/`After` are `any`. Rules that compare them must type-assert and skip on mismatch rather than relying on `==` against an untyped literal. See `evaluatePublicExposure` for the reference pattern.

16. **ChangedNode carries type metadata** -- `delta.ChangedNode` embeds `Type` and `ProviderType` from the head node so risk (and any future consumer) does not need to re-parse the ID string or look up `models.AbstractionMap`. Keep this contract: never derive node type from an ID outside the delta package.

## Design Decisions to Preserve (Phase 4)

17. **Diagram is part of the trust surface, summary is not.** `interpreter.Render` always calls `RenderMermaid` directly -- it does NOT route through the `Interpreter` interface. A future LLM-backed `Interpreter` may rewrite the prose `Summary` and `ReviewFocus` but cannot affect the diagram. This preserves "diagrams are facts, prose is presentation."

18. **Sanitization keys off RuleID and AbstractionMap, not free-form text.** `reasonCodes` only emits known rule IDs from `RiskReason.RuleID`. `SanitizedNode.Type` only contains values from `models.AbstractionMap` (already constrained by Stage 1). No `strings.Contains` against `RiskReason.Message` or any user data. This is the same anti-pattern the Phase-3 hardening pass eliminated from the parser/graph boundary.

19. **Schema parity is a test, not a hope.** `TestEgressPayload_SchemaParity` parses `docs/egress-schema.json` and asserts that the JSON keys produced by `Sanitize` match the schema's `properties` AND `required` arrays exactly. Adding a field to `EgressPayload` without updating the schema (or vice versa) fails the build. This is the procurement guarantee from `master.md` §9.4.

20. **No attribute values in egress.** `SanitizedChangedNode` carries `ChangedAttributeKeys` (sorted strings) but never `ChangedAttribute.Before`/`After`. `TestSanitize_ChangedAttributesContainKeysOnly` asserts no `"before"` / `"after"` keys appear anywhere in the marshaled payload.

21. **Salted, truncated SHA-256 for ID hashing.** `hashID(id, salt) = "n_" + sha256(salt+"|"+id)[:8 hex]`. 32 bits of collision resistance is sufficient for delta-sized payloads (typically <100 nodes). The `n_` prefix keeps IDs syntactically distinct from any unhashed string.

22. **Mermaid styling uses stroke only, never fill.** `classDef` lines use `stroke` + `stroke-width` (and `stroke-dasharray` for removed). No `fill` or `color` properties, so dark-mode renderers (GitHub PRs, dark-themed dashboards) don't get washed out. Status is also conveyed in the label marker (`+ `, `- `, `~ `) so the meaning survives any rendering.

23. **Audit manifest excludes itself.** `Manifest.Files` lists the four content artifacts only -- a manifest cannot contain its own checksum. `TestWriteAudit_ManifestChecksumsMatchFiles` enforces this set explicitly.

24. **CLI flags work in any position.** `splitFlagsAndPositional` separates `-flag` / `-flag=value` / `-flag value` from positionals before calling `flag.FlagSet.Parse`. This avoids the standard-library footgun where `report ./base ./head --out X` silently treats `--out` as a positional. Apply the same helper to any new subcommand that mixes flags and positionals.

25. **Empty slices, never nil, in `Report` and `EgressPayload`.** `Render` initializes `ReviewFocus` to `[]string{}` if the interpreter returns nil. Sanitize uses `make([]T, 0, len(...))`. JSON consumers (and the parity test) rely on `[]` over `null`.

## Hardening Pass (Pre-Phase-4)

Done before starting Phase 4. No rule semantics or scoring weights changed; this was a structural cleanup.

1. **Typed warnings.** `models.Warning{Category, Message}` replaced `[]string`. `parser.ParseDir` and `graph.Build` now exchange `[]models.Warning`. Confidence deductions are looked up by `Category` in `confidenceDeduction` -- no more `strings.Contains` on free-form messages. Categories are constants in `models/models.go`.

2. **`Confidence.Warnings` always serializes as `[]`.** `computeConfidence` initializes a nil input to `[]models.Warning{}` before returning, matching the empty-slice discipline already used in `delta` and `risk`.

3. **`delta.ChangedNode` carries `Type` + `ProviderType`.** Eliminates the `abstractTypeFromID` leak in `risk/rules.go`. Rule 4 now reads `cn.Type` directly.

4. **Rule 1 defensive type assertion.** `evaluatePublicExposure` now type-asserts `Before`/`After` as `bool` and skips on mismatch. Previously `attr.Before == false` worked but would silently miss if the value were ever a string from a JSON round-trip.

5. **Rule 5 dead code removed.** `evaluateRemoval` previously contained an unreachable `math.Min(0.5, 1.0-total)` due to an early `break`. Replaced with a simple `removalMaxReasons` constant + fixed 0.5 weight. Behavior unchanged.

6. **Nested-block attribute eval consistency.** When a nested-block attribute fails to evaluate (e.g. `from_port = var.port`), the key is now recorded with `nil` -- matching the top-level path. Previously the key was dropped entirely.

7. **CLI consolidation.** `main.go` is now a subcommand dispatcher (`graph` | `diff` | `score`). The standalone `cmd/riskdemo/` binary was removed. Stderr lines are uniformly prefixed with `[architex]`; warnings print as `[architex] WARN [<category>]: <message>`.

8. **Minor cleanup in `graph.buildEdges`.** Avoids the double `index[ref.TargetID]` lookup; initializes the slice with `make([]models.Edge, 0)` so an edgeless graph serializes as `[]`.

## What Is NOT Built Yet

- No GitHub integration (Phase 5: composite Action wrapping `architex report --out`)
- No LLM-powered summaries (Phase 6: `LLMInterpreter` against the existing `Interpreter` interface)
- No multi-layer dependency expansion in the diagram (currently: changed/added/removed nodes plus direct edge endpoints only)
- No module support (warned, skipped)
- No `for_each` / `count` support (warned, resource skipped)
- No `dynamic` block support (warned, resource skipped)
- No variable/local resolution
- No data source handling
- No multi-provider support (AWS only)
- No `Meta` field on Node (noted as future addition for derived metadata separate from `Attributes`)
- No environment-tag inference yet (`SanitizationPolicy` has only `HashSalt`; environment knobs will be added when the parser learns to infer them)
