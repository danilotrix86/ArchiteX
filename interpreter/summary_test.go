package interpreter

import (
	"strings"
	"testing"
)

func TestDeterministicInterpreter_SummaryHighRisk(t *testing.T) {
	got := DeterministicInterpreter{}.Summary(highRiskDelta(), runRisk(highRiskDelta()))

	mustContain := []string{
		"Added 1 entry-point resource.",
		"Modified 1 access-control resource.",
		"Connectivity changed: 2 new dependency edges",
		"publicly accessible",
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("expected summary to contain %q, got:\n%s", want, got)
		}
	}
}

func TestDeterministicInterpreter_SummaryEmptyDelta(t *testing.T) {
	got := DeterministicInterpreter{}.Summary(emptyDelta(), runRisk(emptyDelta()))
	if !strings.Contains(got, "No architectural changes") {
		t.Errorf("empty delta summary should announce no changes, got: %q", got)
	}
}

func TestDeterministicInterpreter_SummaryDeterministic(t *testing.T) {
	d := highRiskDelta()
	r := runRisk(d)
	a := DeterministicInterpreter{}.Summary(d, r)
	b := DeterministicInterpreter{}.Summary(d, r)
	if a != b {
		t.Fatalf("Summary not deterministic across calls:\n%q\nvs\n%q", a, b)
	}
}

func TestDeterministicInterpreter_ReviewFocusOrderedByWeight(t *testing.T) {
	d := highRiskDelta()
	r := runRisk(d)
	focus := DeterministicInterpreter{}.ReviewFocus(d, r)

	// Risk reasons are sorted by weight desc, so the public_exposure_introduced
	// bullet (weight 4.0) must precede the new_entry_point bullet (weight 3.0).
	exposureIdx := indexContaining(focus, "public exposure is intended")
	entryIdx := indexContaining(focus, "TLS, authentication")
	if exposureIdx == -1 || entryIdx == -1 {
		t.Fatalf("expected both exposure and entry-point bullets, got: %#v", focus)
	}
	if exposureIdx > entryIdx {
		t.Errorf("expected exposure bullet before entry-point bullet, got order: %#v", focus)
	}
}

func TestDeterministicInterpreter_ReviewFocusEmptyDelta(t *testing.T) {
	focus := DeterministicInterpreter{}.ReviewFocus(emptyDelta(), runRisk(emptyDelta()))
	if len(focus) != 1 || !strings.Contains(focus[0], "No review focus") {
		t.Errorf("expected single 'no review focus' bullet, got: %#v", focus)
	}
}

func TestDeterministicInterpreter_ReviewFocusForRemoval(t *testing.T) {
	d := removalDelta()
	r := runRisk(d)
	focus := DeterministicInterpreter{}.ReviewFocus(d, r)
	if len(focus) == 0 || !strings.Contains(focus[0], "removal of aws_instance.web is intended") {
		t.Errorf("expected removal focus bullet for aws_instance.web, got: %#v", focus)
	}
}

func TestJoinHumanList(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b, and c"},
		{[]string{"a", "b", "c", "d"}, "a, b, c, and d"},
	}
	for _, c := range cases {
		if got := joinHumanList(c.in); got != c.want {
			t.Errorf("joinHumanList(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func indexContaining(haystack []string, needle string) int {
	for i, s := range haystack {
		if strings.Contains(s, needle) {
			return i
		}
	}
	return -1
}
