package interpreter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ToolVersion is the ArchiteX build identifier embedded in audit manifests.
// Bump when the audit bundle layout changes. v1.2 (this release) added
// `report.html` to the bundle and is therefore a layout bump.
const ToolVersion = "0.5.0"

// AuditOptions configures a single WriteAudit call. Clock is injected for
// deterministic tests; production callers leave it nil.
type AuditOptions struct {
	OutDir   string
	BaseDir  string
	HeadDir  string
	Clock    func() time.Time
	HashSalt string
}

// AuditBundle describes the on-disk layout produced by WriteAudit.
type AuditBundle struct {
	Path     string            `json:"path"`
	Files    map[string]string `json:"files"` // filename -> sha256 hex
	Manifest Manifest          `json:"manifest"`
}

// Manifest is the JSON-serialized provenance record written next to the
// audit artifacts. Filename digests let downstream auditors verify nothing
// in the bundle was tampered with after generation.
type Manifest struct {
	Timestamp   string            `json:"timestamp"`
	BaseDir     string            `json:"base_dir"`
	HeadDir     string            `json:"head_dir"`
	ToolVersion string            `json:"tool_version"`
	SchemaVer   string            `json:"egress_schema_version"`
	Score       float64           `json:"score"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"`
	Files       map[string]string `json:"files"`
}

// WriteAudit persists the Report (and a sanitized egress preview) to a
// timestamped subdirectory under opts.OutDir. The directory name is
// `<YYYYMMDD-HHMMSS>-<8 hex chars>` where the hex prefix is a hash of the
// timestamp + base/head dirs, ensuring concurrent writes from sibling jobs
// don't collide.
func WriteAudit(rep Report, opts AuditOptions) (AuditBundle, error) {
	if opts.OutDir == "" {
		return AuditBundle{}, fmt.Errorf("audit: OutDir is required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC()

	stampSec := now.Format("20060102-150405")
	short := shortDigest(stampSec, opts.BaseDir, opts.HeadDir)
	dirName := fmt.Sprintf("%s-%s", stampSec, short)
	bundleDir := filepath.Join(opts.OutDir, dirName)

	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return AuditBundle{}, fmt.Errorf("audit: mkdir: %w", err)
	}

	egress := Sanitize(rep, SanitizationPolicy{HashSalt: opts.HashSalt})

	files := map[string][]byte{}
	files["diagram.mmd"] = []byte(rep.Diagram)
	files["summary.md"] = []byte(FormatMarkdown(rep))

	scoreJSON, err := json.MarshalIndent(rep.Risk, "", "  ")
	if err != nil {
		return AuditBundle{}, fmt.Errorf("audit: marshal score: %w", err)
	}
	files["score.json"] = append(scoreJSON, '\n')

	egressJSON, err := json.MarshalIndent(egress, "", "  ")
	if err != nil {
		return AuditBundle{}, fmt.Errorf("audit: marshal egress: %w", err)
	}
	files["egress.json"] = append(egressJSON, '\n')

	// Compute non-HTML checksums first so the manifest passed to FormatHTML
	// already contains the verifiable file list. The HTML page references
	// those checksums (so a reviewer reading report.html alone can verify
	// the sibling artifacts) -- but report.html is intentionally NOT in
	// the manifest.Files map (a manifest cannot contain a checksum of a
	// page that itself displays the checksums; that would be a chicken/egg).
	checksums := make(map[string]string, len(files)+1)
	for name, data := range files {
		path := filepath.Join(bundleDir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return AuditBundle{}, fmt.Errorf("audit: write %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		checksums[name] = hex.EncodeToString(sum[:])
	}

	manifest := Manifest{
		Timestamp:   now.Format(time.RFC3339),
		BaseDir:     opts.BaseDir,
		HeadDir:     opts.HeadDir,
		ToolVersion: ToolVersion,
		SchemaVer:   SchemaVersion,
		Score:       rep.Risk.Score,
		Severity:    rep.Risk.Severity,
		Status:      rep.Risk.Status,
		Files:       checksums,
	}

	// Phase 7 PR6: self-contained report.html (no JS, no external assets,
	// no network at render time). Written AFTER the manifest is built so
	// the rendered table can show every other artifact's SHA-256.
	htmlBytes := []byte(FormatHTML(rep, manifest))
	if err := os.WriteFile(filepath.Join(bundleDir, "report.html"), htmlBytes, 0o644); err != nil {
		return AuditBundle{}, fmt.Errorf("audit: write report.html: %w", err)
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return AuditBundle{}, fmt.Errorf("audit: marshal manifest: %w", err)
	}
	manifestJSON = append(manifestJSON, '\n')
	if err := os.WriteFile(filepath.Join(bundleDir, "manifest.json"), manifestJSON, 0o644); err != nil {
		return AuditBundle{}, fmt.Errorf("audit: write manifest: %w", err)
	}

	return AuditBundle{
		Path:     bundleDir,
		Files:    checksums,
		Manifest: manifest,
	}, nil
}

// shortDigest produces the 8-char hex suffix used in audit directory names.
// Inputs are joined with "|" so concurrent runs against different
// base/head pairs at the same wall-clock second still produce unique names.
func shortDigest(parts ...string) string {
	keys := make([]string, len(parts))
	copy(keys, parts)
	sort.Strings(keys)
	h := sha256.Sum256([]byte(joinPipe(keys)))
	return hex.EncodeToString(h[:4])
}

func joinPipe(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "|"
		}
		out += p
	}
	return out
}
