package interpreter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestWriteAudit_ProducesExpectedFileSet(t *testing.T) {
	tmp := t.TempDir()
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)

	bundle, err := WriteAudit(rep, AuditOptions{
		OutDir:  tmp,
		BaseDir: "/in/base",
		HeadDir: "/in/head",
		Clock:   func() time.Time { return time.Date(2026, 4, 19, 14, 30, 22, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("WriteAudit: %v", err)
	}

	wantFiles := []string{"diagram.mmd", "summary.md", "score.json", "egress.json", "report.html", "manifest.json"}
	for _, name := range wantFiles {
		path := filepath.Join(bundle.Path, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s in bundle, got: %v", name, err)
		}
	}
}

func TestWriteAudit_DirectoryNameIsDeterministic(t *testing.T) {
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()

	clock := func() time.Time { return time.Date(2026, 4, 19, 14, 30, 22, 0, time.UTC) }
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)

	a, err := WriteAudit(rep, AuditOptions{
		OutDir: tmp1, BaseDir: "/in/base", HeadDir: "/in/head", Clock: clock,
	})
	if err != nil {
		t.Fatalf("WriteAudit a: %v", err)
	}
	b, err := WriteAudit(rep, AuditOptions{
		OutDir: tmp2, BaseDir: "/in/base", HeadDir: "/in/head", Clock: clock,
	})
	if err != nil {
		t.Fatalf("WriteAudit b: %v", err)
	}

	if filepath.Base(a.Path) != filepath.Base(b.Path) {
		t.Errorf("expected deterministic dir names, got %s vs %s",
			filepath.Base(a.Path), filepath.Base(b.Path))
	}
}

func TestWriteAudit_ManifestChecksumsMatchFiles(t *testing.T) {
	tmp := t.TempDir()
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	bundle, err := WriteAudit(rep, AuditOptions{
		OutDir:  tmp,
		BaseDir: "/in/base",
		HeadDir: "/in/head",
		Clock:   func() time.Time { return time.Date(2026, 4, 19, 14, 30, 22, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("WriteAudit: %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(bundle.Path, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}

	for name, wantSum := range m.Files {
		data, err := os.ReadFile(filepath.Join(bundle.Path, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != wantSum {
			t.Errorf("checksum mismatch for %s\nmanifest: %s\nactual:   %s",
				name, wantSum, hex.EncodeToString(got[:]))
		}
	}

	// Manifest must list exactly the four content files (manifest.json itself
	// is not self-referenced -- a manifest cannot contain its own checksum).
	wantNames := []string{"diagram.mmd", "egress.json", "score.json", "summary.md"}
	got := make([]string, 0, len(m.Files))
	for k := range m.Files {
		got = append(got, k)
	}
	sort.Strings(got)
	if !equalSlices(got, wantNames) {
		t.Errorf("manifest file list mismatch: got %v, want %v", got, wantNames)
	}

	if m.Score != rep.Risk.Score {
		t.Errorf("manifest score %.1f != report score %.1f", m.Score, rep.Risk.Score)
	}
	if m.SchemaVer != SchemaVersion {
		t.Errorf("manifest schema version %s != %s", m.SchemaVer, SchemaVersion)
	}
}

func TestWriteAudit_RequiresOutDir(t *testing.T) {
	rep := Render(emptyDelta(), runRisk(emptyDelta()), nil)
	_, err := WriteAudit(rep, AuditOptions{})
	if err == nil {
		t.Fatalf("expected error when OutDir is empty")
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
