// Package config loads and validates the optional `.architex.yml` repository
// configuration. The file is purely additive: every field has a documented
// default that reproduces v1.0/v1.1 behavior exactly. Removing the file (or
// never adding one) MUST yield bit-identical output to a v1.1 run.
//
// Phase 7 (v1.2) introduces:
//   - rules.<id>.weight / .enabled  -- per-rule overrides and on/off toggles.
//   - thresholds.warn / .fail       -- severity/status cutoffs.
//   - ignore.paths                  -- glob patterns; matching .tf files are
//     skipped during parsing (file path is matched relative to the directory
//     passed to ParseDir, using filepath.Match per segment).
//   - suppressions[]                -- (rule, resource, reason, expires)
//     entries that drop matching risk reasons. Inline `# architex:ignore=...`
//     comments in .tf files produce additional, equivalent suppressions
//     synthesized by config.LoadInlineSuppressions.
//
// Suppressions are NEVER silent: the PR comment renders a footer counting
// active and expired ones, and the egress payload exposes only the count
// (never the rule IDs or resource names).
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// FileName is the default config file name resolved relative to the
// repository root (typically the working directory of the architex CLI).
const FileName = ".architex.yml"

// DefaultThresholdWarn / DefaultThresholdFail mirror v1.0/v1.1 behavior:
// score >= 7.0 -> high/fail, >= 3.0 -> medium/warn, otherwise low/pass.
// Configs may move these but cannot remove them.
const (
	DefaultThresholdWarn = 3.0
	DefaultThresholdFail = 7.0
)

// Config is the parsed `.architex.yml` document plus any inline suppressions
// merged in from .tf files. A nil *Config means "use defaults" everywhere.
type Config struct {
	Rules        map[string]RuleConfig `yaml:"rules,omitempty"`
	Thresholds   Thresholds            `yaml:"thresholds,omitempty"`
	Ignore       Ignore                `yaml:"ignore,omitempty"`
	Suppressions []Suppression         `yaml:"suppressions,omitempty"`
}

// RuleConfig overrides per-rule defaults. Either field may be unset
// (pointer-nil) and is then ignored.
type RuleConfig struct {
	Weight  *float64 `yaml:"weight,omitempty"`
	Enabled *bool    `yaml:"enabled,omitempty"`
}

// Thresholds overrides the severity/status cutoffs. Pointer-nil means "use
// the default constant".
type Thresholds struct {
	Warn *float64 `yaml:"warn,omitempty"`
	Fail *float64 `yaml:"fail,omitempty"`
}

// Ignore specifies paths the parser should skip. Globs are matched against
// the file path relative to the directory passed to ParseDir; both `**`
// recursive and `*` single-segment wildcards are supported.
type Ignore struct {
	Paths []string `yaml:"paths,omitempty"`
}

// Suppression silences a (rule, resource) pair. Resource may be the literal
// resource ID (`aws_s3_bucket.public_assets`) or a glob ending in `*`
// (`aws_s3_bucket.legacy_*`). Reason is required and surfaced in the audit
// bundle. Expires (RFC3339 date or YYYY-MM-DD) optional; expired
// suppressions emit a `warn`-level reason so they cannot rot silently.
type Suppression struct {
	Rule     string `yaml:"rule"`
	Resource string `yaml:"resource"`
	Reason   string `yaml:"reason"`
	Expires  string `yaml:"expires,omitempty"`

	// Source labels where the suppression came from -- the YAML config or
	// an inline `# architex:ignore=` comment. Used by the interpreter to
	// produce an auditable footer; never serialized in egress.
	Source string `yaml:"-" json:"-"`
}

// Load reads `.architex.yml` from path. Returns (nil, nil) when the file
// does not exist, so callers can default to v1.1 behavior with zero
// special-casing. Returns an error only when the file exists but is
// malformed -- never on absence.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	for i := range cfg.Suppressions {
		if cfg.Suppressions[i].Source == "" {
			cfg.Suppressions[i].Source = "config:" + filepath.Base(path)
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	return &cfg, nil
}

// validate enforces documented invariants without imposing opinionated
// defaults. Anything we cannot validate locally (e.g. unknown rule IDs)
// is intentionally permissive -- new rule IDs land all the time and we do
// not want a config from v1.2 to break a v1.3 upgrade.
func (c *Config) validate() error {
	if c.Thresholds.Warn != nil && *c.Thresholds.Warn < 0 {
		return fmt.Errorf("thresholds.warn must be >= 0")
	}
	if c.Thresholds.Fail != nil && *c.Thresholds.Fail < 0 {
		return fmt.Errorf("thresholds.fail must be >= 0")
	}
	if c.Thresholds.Warn != nil && c.Thresholds.Fail != nil &&
		*c.Thresholds.Warn > *c.Thresholds.Fail {
		return fmt.Errorf("thresholds.warn (%.2f) must be <= thresholds.fail (%.2f)",
			*c.Thresholds.Warn, *c.Thresholds.Fail)
	}
	for i, s := range c.Suppressions {
		if s.Rule == "" || s.Resource == "" {
			return fmt.Errorf("suppressions[%d]: both rule and resource are required", i)
		}
		if s.Reason == "" {
			return fmt.Errorf("suppressions[%d] (%s/%s): reason is required",
				i, s.Rule, s.Resource)
		}
		if s.Expires != "" {
			if _, err := parseExpiry(s.Expires); err != nil {
				return fmt.Errorf("suppressions[%d] (%s/%s): invalid expires %q: %w",
					i, s.Rule, s.Resource, s.Expires, err)
			}
		}
	}
	return nil
}

// WarnThreshold returns the configured warn cutoff or the v1.1 default.
func (c *Config) WarnThreshold() float64 {
	if c == nil || c.Thresholds.Warn == nil {
		return DefaultThresholdWarn
	}
	return *c.Thresholds.Warn
}

// FailThreshold returns the configured fail cutoff or the v1.1 default.
func (c *Config) FailThreshold() float64 {
	if c == nil || c.Thresholds.Fail == nil {
		return DefaultThresholdFail
	}
	return *c.Thresholds.Fail
}

// RuleEnabled reports whether the rule should be evaluated. Defaults to
// true for unknown rule IDs (forward-compatible).
func (c *Config) RuleEnabled(ruleID string) bool {
	if c == nil {
		return true
	}
	rc, ok := c.Rules[ruleID]
	if !ok || rc.Enabled == nil {
		return true
	}
	return *rc.Enabled
}

// RuleWeight returns the configured weight override for a rule, OR the
// fallback weight passed in. Callers (the risk engine) supply the rule's
// built-in default weight as the fallback so the engine remains the single
// source of truth for defaults.
func (c *Config) RuleWeight(ruleID string, fallback float64) float64 {
	if c == nil {
		return fallback
	}
	rc, ok := c.Rules[ruleID]
	if !ok || rc.Weight == nil {
		return fallback
	}
	return *rc.Weight
}

// IsPathIgnored reports whether a relative path matches any ignore pattern.
// The path is normalized to forward slashes before matching so the same
// patterns work on Windows and POSIX.
func (c *Config) IsPathIgnored(rel string) bool {
	if c == nil || len(c.Ignore.Paths) == 0 {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, pat := range c.Ignore.Paths {
		if matched, _ := globMatch(pat, rel); matched {
			return true
		}
	}
	return false
}

// AllSuppressions returns the merged list of suppressions (config + any
// inline ones already merged in via Add).
func (c *Config) AllSuppressions() []Suppression {
	if c == nil {
		return nil
	}
	return c.Suppressions
}

// Add merges additional suppressions (typically synthesized from inline
// `# architex:ignore=...` comments) into the config in-place.
func (c *Config) Add(extra ...Suppression) {
	if c == nil || len(extra) == 0 {
		return
	}
	c.Suppressions = append(c.Suppressions, extra...)
}

// MatchSuppression searches the config for a suppression matching this
// (rule_id, resource_id) pair. Returns the matching suppression and true
// when one applies. The comparison is glob-aware on the Resource field --
// trailing `*` acts as a wildcard.
//
// Expired suppressions are still considered "matching" (so the rule does
// not fire) but the returned `expired` is true so the caller can emit a
// warning telling the team to refresh or remove the entry.
func (c *Config) MatchSuppression(ruleID, resourceID string, now time.Time) (s Suppression, expired bool, ok bool) {
	if c == nil {
		return Suppression{}, false, false
	}
	for _, sup := range c.Suppressions {
		if sup.Rule != ruleID {
			continue
		}
		if !resourceMatches(sup.Resource, resourceID) {
			continue
		}
		if sup.Expires != "" {
			t, err := parseExpiry(sup.Expires)
			if err == nil && now.After(t) {
				return sup, true, true
			}
		}
		return sup, false, true
	}
	return Suppression{}, false, false
}

// resourceMatches supports literal equality and a trailing `*` wildcard.
// Patterns starting with `module.` are matched as-is so module-namespaced
// resource IDs (Phase 7 PR1) work without surprises.
func resourceMatches(pattern, resourceID string) bool {
	if pattern == resourceID {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(resourceID, prefix)
	}
	return false
}

// globMatch supports `*` (single segment) and `**` (any number of segments).
// We use path.Match (forward-slash-only, OS-independent) per segment so the
// same patterns work identically on Windows and POSIX. Hand-rolled because
// the standard library's path.Match does not understand `**`.
func globMatch(pattern, name string) (bool, error) {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) (bool, error) {
	switch {
	case len(pat) == 0:
		return len(name) == 0, nil
	case pat[0] == "**":
		// `**` matches zero or more name segments.
		for i := 0; i <= len(name); i++ {
			ok, err := matchSegments(pat[1:], name[i:])
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	case len(name) == 0:
		return false, nil
	default:
		ok, err := path.Match(pat[0], name[0])
		if err != nil || !ok {
			return false, err
		}
		return matchSegments(pat[1:], name[1:])
	}
}

// parseExpiry accepts RFC3339 or `YYYY-MM-DD` (interpreted as UTC midnight).
func parseExpiry(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		// Treat YYYY-MM-DD as end-of-day UTC so a suppression scheduled to
		// expire "on" the 1st remains valid the entire day.
		return t.Add(24*time.Hour - time.Second).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 or YYYY-MM-DD")
}
