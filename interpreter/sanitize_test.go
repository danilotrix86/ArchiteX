package interpreter

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSanitize_NoTerraformNamesLeak(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	payload := Sanitize(rep, DefaultSanitizationPolicy())

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(raw)

	leaks := []string{
		"aws_lb.web",
		"aws_security_group.web",
		"aws_subnet.public",
		"aws_lb_web", // sanitized form should also not appear
		"became publicly accessible",
		"public entry point",
	}
	for _, l := range leaks {
		if strings.Contains(out, l) {
			t.Errorf("egress payload leaks %q:\n%s", l, out)
		}
	}
}

func TestSanitize_HashIsStableAcrossCallsWithSameSalt(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	a := Sanitize(rep, SanitizationPolicy{HashSalt: "test"})
	b := Sanitize(rep, SanitizationPolicy{HashSalt: "test"})
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("Sanitize is non-deterministic with the same salt:\n%+v\nvs\n%+v", a, b)
	}
}

func TestSanitize_DifferentSaltProducesDifferentHashes(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	a := Sanitize(rep, SanitizationPolicy{HashSalt: "salt1"})
	b := Sanitize(rep, SanitizationPolicy{HashSalt: "salt2"})
	if len(a.AddedNodes) == 0 || len(b.AddedNodes) == 0 {
		t.Fatalf("expected at least one added node")
	}
	if a.AddedNodes[0].ID == b.AddedNodes[0].ID {
		t.Fatalf("expected different salt to produce different hash, got %q == %q",
			a.AddedNodes[0].ID, b.AddedNodes[0].ID)
	}
}

func TestSanitize_ReasonCodesContainOnlyRuleIDs(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	payload := Sanitize(rep, DefaultSanitizationPolicy())

	allowed := map[string]bool{
		"public_exposure_introduced": true,
		"new_data_resource":          true,
		"new_entry_point":            true,
		"potential_data_exposure":    true,
		"resource_removed":           true,
	}
	for _, code := range payload.ReasonCodes {
		if !allowed[code] {
			t.Errorf("unexpected reason code %q in egress payload", code)
		}
	}
	// Codes must be sorted.
	sorted := append([]string{}, payload.ReasonCodes...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(sorted, payload.ReasonCodes) {
		t.Errorf("reason codes not sorted: %v", payload.ReasonCodes)
	}
}

func TestSanitize_ChangedAttributesContainKeysOnly(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	payload := Sanitize(rep, DefaultSanitizationPolicy())

	if len(payload.ChangedNodes) != 1 {
		t.Fatalf("expected 1 changed node, got %d", len(payload.ChangedNodes))
	}
	keys := payload.ChangedNodes[0].ChangedAttributeKeys
	if len(keys) != 1 || keys[0] != "public" {
		t.Errorf("expected ChangedAttributeKeys=[public], got %v", keys)
	}

	// Make sure the marshaled JSON does NOT contain the boolean values
	// before/after that the source ChangedAttribute carried.
	raw, _ := json.Marshal(payload)
	for _, forbidden := range []string{`"before"`, `"after"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("egress payload leaked %s:\n%s", forbidden, raw)
		}
	}
}

// TestEgressPayload_SchemaParity is the procurement guarantee from
// master.md §9.4: any field present in the EgressPayload JSON must be declared
// in docs/egress-schema.json, and vice versa. Adding a field to one without
// the other will fail this test.
func TestEgressPayload_SchemaParity(t *testing.T) {
	rep := Render(highRiskDelta(), runRisk(highRiskDelta()), nil)
	payload := Sanitize(rep, DefaultSanitizationPolicy())
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	payloadKeys := keysOf(asMap)

	schemaBytes, err := os.ReadFile("../docs/egress-schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	schemaKeys := keysOf(schema.Properties)
	requiredKeys := append([]string{}, schema.Required...)
	sort.Strings(requiredKeys)

	if !reflect.DeepEqual(payloadKeys, schemaKeys) {
		t.Errorf("EgressPayload top-level keys differ from schema:\npayload: %v\nschema:  %v",
			payloadKeys, schemaKeys)
	}
	if !reflect.DeepEqual(payloadKeys, requiredKeys) {
		t.Errorf("EgressPayload keys differ from schema required[]:\npayload: %v\nrequired: %v",
			payloadKeys, requiredKeys)
	}
}

func keysOf[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
