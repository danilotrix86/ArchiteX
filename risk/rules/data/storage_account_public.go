package data

import (
	"fmt"

	"architex/delta"
	"architex/risk/api"
	"architex/risk/rules/internal/rulefmt"
)

// StorageAccountPublic is the Phase 9 (v1.4) "storage_account_public"
// rule -- the Azure analog of s3_bucket_public_exposure.
//
// Triggers when an azurerm_storage_account is ADDED with EITHER
// `public_network_access_enabled = true` OR
// `allow_nested_items_to_be_public = true`. Either flag is sufficient
// to expose blob/file/table/queue contents to anonymous internet
// callers (the public-network-access flag opens the data plane;
// allow-nested-items opens per-container anonymous access).
//
// Default-false posture: a missing attribute does NOT fire. azurerm
// provider defaults vary by version, and we never guess at runtime
// state. Variable-driven flags also land as missing and stay silent.
//
// Weight 4.0 -- equal to s3_bucket_public_exposure. A public storage
// account is the Azure equivalent of a public S3 bucket and carries
// the same exfiltration risk. The two literal flags are independent
// ways the same posture is configured; we report the rule once per
// resource (not once per flag) to avoid double-counting.
//
// Lives in package data (not exposure) because the SIGNAL is "data is
// reachable from anywhere"; the AWS-side bucket rule lives in exposure
// because its signal is "an access-control resource changed". The
// asymmetry mirrors how the two providers express the same concept.
var StorageAccountPublic api.Rule = storageAccountPublicRule{}

type storageAccountPublicRule struct{}

func (storageAccountPublicRule) ID() string { return "storage_account_public" }

func (storageAccountPublicRule) Evaluate(d delta.Delta) []api.RiskReason {
	var reasons []api.RiskReason
	for _, n := range d.AddedNodes {
		if n.ProviderType != "azurerm_storage_account" {
			continue
		}
		if rulefmt.IsConditional(n.Attributes) {
			continue
		}
		var msg string
		if b, ok := n.Attributes["public_network_access_enabled"].(bool); ok && b {
			msg = fmt.Sprintf("Storage account %s has public_network_access_enabled=true; the data plane is reachable from the public internet.", n.ID)
		} else if b, ok := n.Attributes["allow_nested_items_to_be_public"].(bool); ok && b {
			msg = fmt.Sprintf("Storage account %s has allow_nested_items_to_be_public=true; container-level anonymous access is permitted.", n.ID)
		} else {
			continue
		}
		if len(reasons) >= rulefmt.DefaultRulePerResourceCap {
			break
		}
		reasons = append(reasons, api.RiskReason{
			RuleID:     "storage_account_public",
			Message:    msg,
			Impact:     "exposure",
			Weight:     4.0,
			ResourceID: n.ID,
		})
	}
	return reasons
}

func (storageAccountPublicRule) ReviewFocus(reason api.RiskReason, _ delta.Delta) string {
	return fmt.Sprintf(
		"Re-verify the storage account %s -- public_network_access_enabled or allow_nested_items_to_be_public both expose blob/file/table/queue contents to anonymous callers; scope via private endpoints or network rules.",
		reason.ResourceID,
	)
}
