package interpreter

import (
	"strings"
	"testing"

	"architex/risk"
)

// TestFormatHTML_SelfContained_NoExternalResources is the trust-critical
// test: the audit-bundle HTML report must not load anything from the
// network at render time. No CDN scripts, no remote fonts, no remote
// stylesheets, no remote `<img src>`. The only allowed outbound URL is
// the optional Mermaid Live link, which is a USER-initiated click in
// `<a href>`, not an automatic request the browser fires on page load.
func TestFormatHTML_SelfContained_NoExternalResources(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	out := FormatHTML(rep, Manifest{})

	// No `<script>` tags at all -- the page is fully no-JS.
	if strings.Contains(strings.ToLower(out), "<script") {
		t.Errorf("HTML report must not contain any <script> tags")
	}

	// No `<link rel="stylesheet">` to anywhere; CSS is inlined.
	if strings.Contains(strings.ToLower(out), `<link rel="stylesheet"`) {
		t.Errorf("HTML report must not load external stylesheets")
	}

	// No `<img>` -- we don't render any images. Future additions must use
	// inline data: URIs only.
	if strings.Contains(strings.ToLower(out), "<img ") {
		t.Errorf("HTML report must not contain <img> tags")
	}

	// No iframe / embed / object. Defense in depth.
	for _, tag := range []string{"<iframe", "<embed", "<object"} {
		if strings.Contains(strings.ToLower(out), tag) {
			t.Errorf("HTML report must not contain %s", tag)
		}
	}

	// The only allowed outbound URLs are explicit `<a href="https://...">`
	// links the user must click. Walk the output and confirm every http/https
	// URL is inside an href attribute, never a src/url()/etc.
	lower := strings.ToLower(out)
	for _, scheme := range []string{"http://", "https://"} {
		idx := 0
		for {
			pos := strings.Index(lower[idx:], scheme)
			if pos < 0 {
				break
			}
			abs := idx + pos
			window := lower[max0(abs-30):abs]
			if !strings.Contains(window, `href="`) && !strings.Contains(window, "href='") {
				t.Errorf("found %s URL not inside an href= attribute (context: %q)",
					scheme, lower[max0(abs-20):min(abs+40, len(lower))])
			}
			idx = abs + len(scheme)
		}
	}
}

func max0(i int) int {
	if i < 0 {
		return 0
	}
	return i
}

func TestFormatHTML_RendersSeverityAndScore(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	out := FormatHTML(rep, Manifest{Timestamp: "2026-04-19T14:30:22Z"})

	if !strings.Contains(out, "Severity: HIGH") {
		t.Errorf("expected severity badge, got body=%q", first200(out))
	}
	if !strings.Contains(out, "Status: FAIL") {
		t.Errorf("expected status badge")
	}
	if !strings.Contains(out, "Score: 9.0/10") {
		t.Errorf("expected score badge")
	}
	if !strings.Contains(out, "2026-04-19T14:30:22Z") {
		t.Errorf("expected manifest timestamp in header")
	}
	if !strings.Contains(out, "ArchiteX "+ToolVersion) {
		t.Errorf("expected tool version in header")
	}
}

func TestFormatHTML_RendersReasonsTable(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	out := FormatHTML(rep, Manifest{})

	for _, want := range []string{
		"public_exposure_introduced",
		"new_entry_point",
		"potential_data_exposure",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected reason %q in HTML report", want)
		}
	}
}

func TestFormatHTML_NoSuppressionsSectionWhenEmpty(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	rep.Risk.Suppressed = nil
	out := FormatHTML(rep, Manifest{})

	if strings.Contains(out, "Suppressed Findings") {
		t.Errorf("Suppressed Findings section must be omitted when empty")
	}
}

func TestFormatHTML_RendersSuppressionsSectionWithExpiredFlag(t *testing.T) {
	rep := Render(emptyDelta(), runRisk(emptyDelta()), nil)
	rep.Risk.Suppressed = []risk.SuppressedFinding{
		{
			RuleID:     "iam_admin_policy_attached",
			ResourceID: "aws_iam_role_policy_attachment.legacy",
			Reason:     "Legacy admin role; tracked in JIRA-123",
			Source:     "config:.architex.yml",
			Expired:    true,
		},
		{
			RuleID:     "ebs_volume_unencrypted",
			ResourceID: "aws_ebs_volume.scratch",
			Reason:     "Ephemeral scratch disk",
			Source:     "inline:main.tf:42",
			Expired:    false,
		},
	}
	out := FormatHTML(rep, Manifest{})

	if !strings.Contains(out, "Suppressed Findings") {
		t.Errorf("expected Suppressed Findings section to render")
	}
	if !strings.Contains(out, "iam_admin_policy_attached") {
		t.Errorf("expected suppressed rule id")
	}
	if !strings.Contains(out, "EXPIRED") {
		t.Errorf("expected EXPIRED flag for the lapsed suppression")
	}
	if !strings.Contains(out, "config:.architex.yml") {
		t.Errorf("expected source label for config-driven suppression")
	}
	if !strings.Contains(out, "inline:main.tf:42") {
		t.Errorf("expected source label for inline-driven suppression")
	}
}

func TestFormatHTML_RendersManifestChecksums(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	m := Manifest{
		Timestamp: "2026-04-19T14:30:22Z",
		Files: map[string]string{
			"diagram.mmd": "aaaa",
			"summary.md":  "bbbb",
			"score.json":  "cccc",
			"egress.json": "dddd",
		},
	}
	out := FormatHTML(rep, m)

	if !strings.Contains(out, "Audit Manifest") {
		t.Errorf("expected Audit Manifest section when manifest has files")
	}
	for name, sha := range m.Files {
		if !strings.Contains(out, name) {
			t.Errorf("expected manifest file %q in HTML report", name)
		}
		if !strings.Contains(out, sha) {
			t.Errorf("expected SHA %q for %q in HTML report", sha, name)
		}
	}
}

func TestFormatHTML_Deterministic_SameInputSameOutput(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	m := Manifest{
		Timestamp:   "2026-04-19T14:30:22Z",
		ToolVersion: ToolVersion,
		Files: map[string]string{
			"a.json": "1111",
			"b.json": "2222",
			"c.json": "3333",
		},
	}
	a := FormatHTML(rep, m)
	b := FormatHTML(rep, m)
	if a != b {
		t.Fatalf("FormatHTML must be deterministic for the same Report+Manifest")
	}
}

func TestFormatHTML_MermaidLink_OmittedWhenDiagramTooLarge(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	rep.Diagram = strings.Repeat("X", 16*1024)
	out := FormatHTML(rep, Manifest{})

	if strings.Contains(out, "Open in Mermaid Live") {
		t.Errorf("Mermaid Live link should be omitted for >8 KiB diagrams")
	}
	if !strings.Contains(out, "Diagram is too large") {
		t.Errorf("expected fallback explanation when link is omitted")
	}
}

func TestFormatHTML_HandlesEmptyReportSafely(t *testing.T) {
	rep := Render(emptyDelta(), runRisk(emptyDelta()), nil)
	out := FormatHTML(rep, Manifest{})

	if !strings.Contains(out, "<!DOCTYPE html>") {
		t.Errorf("output must start with <!DOCTYPE html>, got %q", first200(out))
	}
	if !strings.Contains(out, "No risk reasons fired") {
		t.Errorf("expected empty-reasons message in HTML output")
	}
}

func first200(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
