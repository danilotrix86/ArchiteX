package interpreter

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"architex/risk"
)

// FormatHTML renders a self-contained, single-file HTML report from a
// Report plus the optional Manifest produced by WriteAudit. The output is
// designed to satisfy the Phase 7 PR6 trust constraints:
//
//   - NO external resources. No CDN scripts, no remote fonts, no
//     `<img src>` to anything other than inline data: URIs. A reviewer
//     can open the file in an air-gapped browser and see everything.
//   - NO JavaScript. We render the Mermaid diagram as a plain `<pre>`
//     block plus an OPTIONAL link to the public Mermaid Live editor
//     (the user's manual click is the only thing that touches the
//     network -- the file itself does not).
//   - Deterministic output. Two runs against the same Report produce the
//     same bytes (assuming the same Manifest is passed). The Manifest
//     is what carries the wall-clock timestamp, so a nil Manifest still
//     yields a deterministic result keyed only on the Report.
//
// Pass a zero Manifest{} when calling outside of WriteAudit.
func FormatHTML(rep Report, m Manifest) string {
	data := htmlData{
		Score:        rep.Risk.Score,
		Severity:     rep.Risk.Severity,
		Status:       rep.Risk.Status,
		SeverityRank: severityRank(rep.Risk.Severity),
		Summary:      rep.Summary,
		ReviewFocus:  rep.ReviewFocus,
		Reasons:      rep.Risk.Reasons,
		Suppressed:   rep.Risk.Suppressed,
		Diagram:      rep.Diagram,
		MermaidLink:  mermaidLiveURL(rep.Diagram),
		Manifest:     m,
		ManifestRows: manifestRows(m),
		Generated:    chooseTimestamp(m),
		ToolVersion:  ToolVersion,
		Counts: deltaCounts{
			AddedNodes:   len(rep.Delta.AddedNodes),
			RemovedNodes: len(rep.Delta.RemovedNodes),
			ChangedNodes: len(rep.Delta.ChangedNodes),
			AddedEdges:   len(rep.Delta.AddedEdges),
			RemovedEdges: len(rep.Delta.RemovedEdges),
		},
	}

	var buf bytes.Buffer
	if err := htmlTemplate.Execute(&buf, data); err != nil {
		// Template execution failures here would be a programmer error
		// (the template is a constant). Surface a tiny inline message
		// rather than panicking so an audit bundle is never empty.
		return fmt.Sprintf("<!DOCTYPE html><pre>ArchiteX HTML render error: %s</pre>", template.HTMLEscapeString(err.Error()))
	}
	return buf.String()
}

// htmlData is the view model bound to htmlTemplate.
type htmlData struct {
	Score        float64
	Severity     string
	Status       string
	SeverityRank string
	Summary      string
	ReviewFocus  []string
	Reasons      []risk.RiskReason
	Suppressed   []risk.SuppressedFinding
	Diagram      string
	MermaidLink  string
	Manifest     Manifest
	ManifestRows []manifestRow
	Generated    string
	ToolVersion  string
	Counts       deltaCounts
}

type deltaCounts struct {
	AddedNodes, RemovedNodes, ChangedNodes int
	AddedEdges, RemovedEdges               int
}

type manifestRow struct {
	Name string
	SHA  string
}

func manifestRows(m Manifest) []manifestRow {
	if len(m.Files) == 0 {
		return nil
	}
	rows := make([]manifestRow, 0, len(m.Files))
	for name, sum := range m.Files {
		rows = append(rows, manifestRow{Name: name, SHA: sum})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// chooseTimestamp returns the manifest timestamp when present, falling
// back to "(no manifest)" for callers that render outside of an audit
// bundle. This keeps single-shot HTML renders deterministic without
// needing a clock dependency.
func chooseTimestamp(m Manifest) string {
	if m.Timestamp != "" {
		return m.Timestamp
	}
	return "(no manifest)"
}

func severityRank(s string) string {
	switch s {
	case "high":
		return "sev-high"
	case "medium":
		return "sev-medium"
	default:
		return "sev-low"
	}
}

// mermaidLiveURL returns a public Mermaid Live Editor deep link for the
// given diagram source. The link is a USER-INITIATED outbound action --
// the HTML file itself never fetches anything from this URL. We follow
// the documented "pako"-less base64 fallback (the editor accepts a raw
// base64-encoded JSON envelope under the `#base64:` fragment).
//
// Why include this at all? The whole point of the HTML report is that
// reviewers can examine the bundle offline; but on the rare run where
// Mermaid syntax matters, the link saves a copy/paste round-trip. The
// link only works for diagrams under the editor's URL-length budget; we
// gate it at 8 KiB to keep PR-comment-sized diagrams clickable while
// silently skipping huge ones (the in-page `<pre>` block always works).
func mermaidLiveURL(diagram string) string {
	const max = 8 * 1024
	if diagram == "" || len(diagram) > max {
		return ""
	}
	envelope := fmt.Sprintf(`{"code":%q,"mermaid":{"theme":"default"}}`, diagram)
	encoded := base64.StdEncoding.EncodeToString([]byte(envelope))
	return "https://mermaid.live/edit#base64:" + encoded
}

// htmlTemplate is a single-file, no-JS, no-external-asset HTML report.
// The CSS is intentionally compact and uses system fonts only so the
// file renders identically on macOS, Windows, and Linux without any
// network or font fallback.
var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"upper": strings.ToUpper,
	"oneDecimal": func(f float64) string {
		return fmt.Sprintf("%.1f", f)
	},
	"reasonImpact": func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	},
	"truncate": func(n int, s string) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "…"
	},
}).Parse(htmlSource))

const htmlSource = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ArchiteX Report — {{ upper .Severity }} ({{ oneDecimal .Score }}/10)</title>
<style>
:root {
  --bg: #ffffff;
  --fg: #1f2328;
  --muted: #57606a;
  --border: #d0d7de;
  --card: #f6f8fa;
  --code-bg: #f6f8fa;
  --high-bg: #ffebe9; --high-fg: #82071e; --high-border: #ff8182;
  --medium-bg: #fff8c5; --medium-fg: #6f5400; --medium-border: #d4a72c;
  --low-bg: #dafbe1; --low-fg: #1a7f37; --low-border: #4ac26b;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8b949e;
    --border: #30363d; --card: #161b22; --code-bg: #161b22;
    --high-bg: #67060c; --high-fg: #ff8182; --high-border: #b62324;
    --medium-bg: #4a3000; --medium-fg: #e3b341; --medium-border: #9e6a03;
    --low-bg: #033a16; --low-fg: #56d364; --low-border: #196c2e;
  }
}
* { box-sizing: border-box; }
body {
  font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, sans-serif;
  color: var(--fg); background: var(--bg);
  margin: 0; padding: 24px; max-width: 1080px; margin-inline: auto;
}
h1, h2, h3 { line-height: 1.25; margin-top: 1.6em; margin-bottom: 0.4em; }
h1 { font-size: 1.75rem; margin-top: 0; }
h2 { font-size: 1.25rem; padding-bottom: 0.3em; border-bottom: 1px solid var(--border); }
h3 { font-size: 1.05rem; }
p { margin: 0.5em 0; }
code, pre { font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace; }
pre {
  background: var(--code-bg); border: 1px solid var(--border);
  border-radius: 6px; padding: 12px; overflow: auto; font-size: 12px;
}
a { color: #0969da; text-decoration: none; }
a:hover { text-decoration: underline; }
.badges { display: flex; flex-wrap: wrap; gap: 8px; margin: 12px 0 24px; }
.badge {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 4px 10px; border-radius: 999px;
  border: 1px solid var(--border); background: var(--card);
  font-size: 12px; font-weight: 600;
}
.sev-high   { background: var(--high-bg);   color: var(--high-fg);   border-color: var(--high-border); }
.sev-medium { background: var(--medium-bg); color: var(--medium-fg); border-color: var(--medium-border); }
.sev-low    { background: var(--low-bg);    color: var(--low-fg);    border-color: var(--low-border); }
table { width: 100%; border-collapse: collapse; margin: 8px 0 16px; }
th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid var(--border); vertical-align: top; }
th { font-size: 12px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); }
td.weight { font-variant-numeric: tabular-nums; white-space: nowrap; width: 1%; }
td.code { font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace; font-size: 12.5px; }
ul { margin: 0.5em 0 0.5em 1.5em; padding: 0; }
.muted { color: var(--muted); }
.summary-block { background: var(--card); border: 1px solid var(--border); border-radius: 6px; padding: 12px 16px; }
.diagram-actions { display: flex; gap: 12px; align-items: center; margin: 8px 0; }
footer {
  margin-top: 32px; padding-top: 16px; border-top: 1px solid var(--border);
  font-size: 12px; color: var(--muted);
}
.expired { color: var(--high-fg); font-weight: 600; }
</style>
</head>
<body>

<h1>ArchiteX Report</h1>

<div class="badges">
  <span class="badge {{ .SeverityRank }}">Severity: {{ upper .Severity }}</span>
  <span class="badge {{ .SeverityRank }}">Status: {{ upper .Status }}</span>
  <span class="badge">Score: {{ oneDecimal .Score }}/10</span>
  <span class="badge muted">Tool: ArchiteX {{ .ToolVersion }}</span>
  <span class="badge muted">Generated: {{ .Generated }}</span>
</div>

<h2>Plain-English Summary</h2>
<div class="summary-block">{{ .Summary }}</div>

{{ if .ReviewFocus }}
<h2>Suggested Review Focus</h2>
<ul>
{{ range .ReviewFocus }}<li>{{ . }}</li>
{{ end }}</ul>
{{ end }}

<h2>Risk Reasons</h2>
{{ if .Reasons }}
<table>
<thead><tr><th>Weight</th><th>Rule</th><th>Impact</th><th>Resource</th><th>Message</th></tr></thead>
<tbody>
{{ range .Reasons }}<tr>
  <td class="weight">{{ oneDecimal .Weight }}</td>
  <td class="code">{{ .RuleID }}</td>
  <td>{{ reasonImpact .Impact }}</td>
  <td class="code">{{ if .ResourceID }}{{ .ResourceID }}{{ else }}—{{ end }}</td>
  <td>{{ .Message }}</td>
</tr>
{{ end }}</tbody>
</table>
{{ else }}
<p class="muted">No risk reasons fired. ✓</p>
{{ end }}

{{ if .Suppressed }}
<h2>Suppressed Findings</h2>
<p class="muted">These would have fired but were silenced by an active suppression. They do not contribute to the score but are listed here so reviewers can audit what was filtered.</p>
<table>
<thead><tr><th>Rule</th><th>Resource</th><th>Reason</th><th>Source</th><th></th></tr></thead>
<tbody>
{{ range .Suppressed }}<tr>
  <td class="code">{{ .RuleID }}</td>
  <td class="code">{{ .ResourceID }}</td>
  <td>{{ .Reason }}</td>
  <td class="code">{{ .Source }}</td>
  <td>{{ if .Expired }}<span class="expired">EXPIRED</span>{{ end }}</td>
</tr>
{{ end }}</tbody>
</table>
{{ end }}

<h2>Delta Summary</h2>
<table>
<thead><tr><th>Metric</th><th>Count</th></tr></thead>
<tbody>
<tr><td>Added nodes</td><td class="weight">{{ .Counts.AddedNodes }}</td></tr>
<tr><td>Removed nodes</td><td class="weight">{{ .Counts.RemovedNodes }}</td></tr>
<tr><td>Changed nodes</td><td class="weight">{{ .Counts.ChangedNodes }}</td></tr>
<tr><td>Added edges</td><td class="weight">{{ .Counts.AddedEdges }}</td></tr>
<tr><td>Removed edges</td><td class="weight">{{ .Counts.RemovedEdges }}</td></tr>
</tbody>
</table>

<h2>Delta Diagram (Mermaid source)</h2>
<div class="diagram-actions">
{{ if .MermaidLink }}<a href="{{ .MermaidLink }}" rel="noopener noreferrer" target="_blank">Open in Mermaid Live ↗</a>
<span class="muted">(this is the only network action and only fires when YOU click it)</span>
{{ else }}<span class="muted">Diagram is too large for a one-click Mermaid Live link; copy the source below into the editor.</span>
{{ end }}</div>
<pre>{{ .Diagram }}</pre>

{{ if .ManifestRows }}
<h2>Audit Manifest</h2>
<p class="muted">SHA-256 checksums for each artifact in this bundle. Use them to verify nothing was tampered with after generation.</p>
<table>
<thead><tr><th>File</th><th>SHA-256</th></tr></thead>
<tbody>
{{ range .ManifestRows }}<tr><td class="code">{{ .Name }}</td><td class="code">{{ .SHA }}</td></tr>
{{ end }}</tbody>
</table>
{{ end }}

<footer>
  <p>Generated by ArchiteX (deterministic mode). This file is fully self-contained: no scripts, no external assets, no network requests.</p>
</footer>

</body>
</html>
`
