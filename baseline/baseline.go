// Package baseline persists a deterministic snapshot of an architecture
// graph's "shape" -- the set of provider resource types, the set of
// abstract types, and the set of edge-type pairs ever observed -- so that
// subsequent analyses can flag the FIRST time a previously-unseen kind of
// thing appears in a PR.
//
// The baseline is a plain, human-auditable JSON file (default path
// `.architex/baseline.json`) committed alongside the Terraform source. It
// is intentionally lean:
//
//   - No raw HCL, no resource names, no attribute values.
//   - Only sets of *kinds* (`provider_types`, `abstract_types`,
//     `edge_pairs`).
//
// This keeps Phase 7 PR5 inside the runner-local trust model (master.md §6
// / §11): the baseline can be checked into a public repo without leaking
// architectural detail beyond what `models.SupportedResources` and
// `models.AbstractionMap` already publish.
//
// A nil *Baseline is the well-defined "no baseline exists yet" state and
// disables all `first_time_*` rules. This is the bit-identical-to-v1.1
// fallback when no baseline file is present in the head directory.
package baseline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"architex/models"
)

// SchemaVersion is the on-disk schema version for baseline.json. Bump when
// the Baseline shape changes in a way that older readers would misinterpret.
const SchemaVersion = "1"

// FileName is the canonical relative path of the baseline file inside a
// Terraform directory. Callers may override.
const FileName = ".architex/baseline.json"

// Baseline is a deterministic snapshot of the kinds of things that have
// ever appeared in an architecture graph. All slice fields are sorted and
// deduplicated by the constructors; consumers may rely on that ordering.
type Baseline struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`

	// ProviderTypes is the set of Terraform resource types ever seen
	// (e.g. "aws_s3_bucket"). Always sorted, always deduplicated.
	ProviderTypes []string `json:"provider_types"`

	// AbstractTypes is the set of architectural abstract types ever seen
	// (e.g. "entry_point"). Always sorted, always deduplicated.
	AbstractTypes []string `json:"abstract_types"`

	// EdgePairs is the set of "<sourceProviderType>|<targetProviderType>"
	// pairs ever seen. The pair is providerType-based (not abstract) so a
	// "first_time_edge_pair" finding maps to a real Terraform-level
	// relationship the user can read in their graph. Always sorted,
	// always deduplicated.
	EdgePairs []string `json:"edge_pairs"`
}

// FromGraph builds a Baseline from a single graph. Used by the
// `architex baseline` subcommand to snapshot the current head state.
//
// Nodes whose ProviderType is empty (defensive -- should not happen in
// well-formed graphs) are skipped; such a node would not be matchable
// against future deltas anyway.
func FromGraph(g models.Graph, now time.Time) *Baseline {
	b := &Baseline{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   now.UTC(),
	}

	provider := make(map[string]struct{}, len(g.Nodes))
	abstract := make(map[string]struct{}, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.ProviderType != "" {
			provider[n.ProviderType] = struct{}{}
		}
		if n.Type != "" {
			abstract[n.Type] = struct{}{}
		}
	}

	// EdgePairs need provider-type lookup per node ID.
	providerByID := make(map[string]string, len(g.Nodes))
	for _, n := range g.Nodes {
		providerByID[n.ID] = n.ProviderType
	}
	pairs := make(map[string]struct{}, len(g.Edges))
	for _, e := range g.Edges {
		from, ok1 := providerByID[e.From]
		to, ok2 := providerByID[e.To]
		if !ok1 || !ok2 || from == "" || to == "" {
			continue
		}
		pairs[from+"|"+to] = struct{}{}
	}

	b.ProviderTypes = sortedKeys(provider)
	b.AbstractTypes = sortedKeys(abstract)
	b.EdgePairs = sortedKeys(pairs)
	return b
}

// Merge returns the union of two baselines. The schema version is taken
// from `into` if non-empty, else from `add`. The GeneratedAt timestamp is
// taken from `into` (the merge is conceptually "extend the existing
// baseline with newly-seen kinds"). A nil receiver or argument is treated
// as the empty baseline.
func Merge(into, add *Baseline) *Baseline {
	if into == nil && add == nil {
		return nil
	}
	out := &Baseline{}
	if into != nil {
		out.SchemaVersion = into.SchemaVersion
		out.GeneratedAt = into.GeneratedAt
	}
	if out.SchemaVersion == "" && add != nil {
		out.SchemaVersion = add.SchemaVersion
	}
	if out.SchemaVersion == "" {
		out.SchemaVersion = SchemaVersion
	}

	out.ProviderTypes = mergeSortedDedup(getProviders(into), getProviders(add))
	out.AbstractTypes = mergeSortedDedup(getAbstracts(into), getAbstracts(add))
	out.EdgePairs = mergeSortedDedup(getEdges(into), getEdges(add))
	return out
}

// Load reads a baseline file from disk. If the file does not exist,
// (nil, nil) is returned -- the caller MUST treat that as "no baseline,
// disable first_time_* rules" so a repo that has never run
// `architex baseline` behaves identically to v1.1.
//
// Schema-version mismatches are downgraded to a returned error rather
// than a silent acceptance: the rules engine will then fall back to
// "no baseline" via the loadBaseline error path in main.go, which is the
// same conservative behavior as a missing file.
func Load(path string) (*Baseline, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("baseline read: %w", err)
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("baseline parse %s: %w", path, err)
	}
	if b.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("baseline %s: unsupported schema_version %q (expected %q)",
			path, b.SchemaVersion, SchemaVersion)
	}

	// Defensively normalize: callers (and the rules engine) rely on the
	// invariants documented on Baseline. Older or hand-edited files may
	// not satisfy them.
	b.ProviderTypes = sortDedup(b.ProviderTypes)
	b.AbstractTypes = sortDedup(b.AbstractTypes)
	b.EdgePairs = sortDedup(b.EdgePairs)
	return &b, nil
}

// Save writes the baseline atomically: a temp file in the same directory
// is renamed over the destination, so a crashed run never leaves a
// half-written baseline that would silently disable rules on the next
// PR. Parent directories are created with 0o755.
func Save(path string, b *Baseline) error {
	if b == nil {
		return errors.New("baseline.Save: nil baseline")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("baseline mkdir: %w", err)
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("baseline marshal: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".architex-baseline-*.tmp")
	if err != nil {
		return fmt.Errorf("baseline tempfile: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("baseline write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("baseline sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("baseline close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("baseline rename: %w", err)
	}
	return nil
}

// Has* helpers are used by the risk engine to decide whether a node /
// edge pair is "first time". A nil receiver returns false (i.e. nothing
// is known), which intentionally suppresses every first_time_* rule.
// Empty inputs always return true so we never raise spurious findings on
// malformed data.

// HasProviderType reports whether a Terraform resource type was already
// recorded in the baseline.
func (b *Baseline) HasProviderType(t string) bool {
	if b == nil || t == "" {
		return b != nil // nil baseline => no info; empty type => benign
	}
	_, ok := slices.BinarySearch(b.ProviderTypes, t)
	return ok
}

// HasAbstractType reports whether an abstract architectural type was
// already recorded in the baseline.
func (b *Baseline) HasAbstractType(t string) bool {
	if b == nil || t == "" {
		return b != nil
	}
	_, ok := slices.BinarySearch(b.AbstractTypes, t)
	return ok
}

// HasEdgePair reports whether a (sourceProviderType, targetProviderType)
// edge was already recorded in the baseline.
func (b *Baseline) HasEdgePair(from, to string) bool {
	if b == nil {
		return false
	}
	if from == "" || to == "" {
		return true
	}
	_, ok := slices.BinarySearch(b.EdgePairs, from+"|"+to)
	return ok
}

// internal helpers ------------------------------------------------------

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortDedup(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	cp := make([]string, len(in))
	copy(cp, in)
	sort.Strings(cp)
	out := cp[:0]
	for i, s := range cp {
		if i > 0 && s == cp[i-1] {
			continue
		}
		// Defensively trim accidental whitespace from hand-edited files.
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func mergeSortedDedup(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	merged := make([]string, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return sortDedup(merged)
}

func getProviders(b *Baseline) []string {
	if b == nil {
		return nil
	}
	return b.ProviderTypes
}
func getAbstracts(b *Baseline) []string {
	if b == nil {
		return nil
	}
	return b.AbstractTypes
}
func getEdges(b *Baseline) []string {
	if b == nil {
		return nil
	}
	return b.EdgePairs
}
