// Package rules is the aggregator that wires every migrated rule
// implementation into the architex/risk/api registry.
//
// Why a separate aggregator (vs each subpackage init()-registering
// itself):
//
//   - Init order across packages with no dependencies is implementation-
//     defined in Go ("the build system is encouraged to present them in
//     lexical filename order"). For the byte-identical contract of the
//     readability refactor, "encouraged" is too weak; we need the
//     registry to be populated in the EXACT same order pre-refactor
//     EvaluateWithBaseline appended its rule outputs.
//   - Centralizing registration in one init() makes the rule sequence
//     trivially auditable -- one file to read, one diff to review.
//
// Consumers of the registry (cmd/architex/main.go, the golden-snapshot
// tests, anything that calls risk.EvaluateWithBaseline) blank-import
// this package to trigger registration. The risk package itself does
// NOT import this package -- doing so would create the very import
// cycle the architex/risk/api leaf was designed to avoid.
package rules

import (
	"architex/risk/api"
	"architex/risk/rules/availability"
	"architex/risk/rules/data"
	"architex/risk/rules/exposure"
	"architex/risk/rules/identity"
	"architex/risk/rules/lifecycle"
	"architex/risk/rules/observability"
)

// init registers every migrated rule in the SAME order the pre-refactor
// risk.EvaluateWithBaseline appended their outputs. That order does not
// affect the final sorted reason list (which is sorted by weight desc,
// ruleID asc) but it DOES affect the order applyConfig sees the
// reasons in -- and applyConfig preserves input order for equal
// weights. Keeping registration order identical to pre-refactor is the
// simplest way to guarantee byte-identical output across the
// migration.
//
// Order summary (matches pre-refactor risk.EvaluateWithBaseline body):
//
//	v1.0:  PublicExposure, NewData, NewEntryPoint, PotentialDataExposure, Removal
//	Phase 6 (v1.1): S3BucketPublic, IAMAdminAttached, LambdaPublicURL
//	Phase 7 PR4 (v1.2): CloudFrontNoWAF, EBSUnencrypted, MessagingTopicPublic, NACLAllowAllIngress
//	Phase 8 (v1.3): EKSPublicEndpoint, EKSNoLogging, ASGUnrestrictedScaling
//	Phase 9 (v1.4): NSGAllowAllIngress, StorageAccountPublic, MSSQLDatabasePublic
//
// Note: the Phase 7 PR5 baseline-anomaly rules (first_time_*) are NOT
// in the registry. They live in architex/risk/rules/baseline/ as plain
// exported functions because they need *baseline.Baseline as a second
// input -- a richer signature than the api.Rule interface provides.
// risk.EvaluateWithBaseline calls them by name after the registry loop.
// See package architex/risk/rules/baseline for the rationale.
func init() {
	// v1.0 rules
	api.Register(exposure.PublicExposure)
	api.Register(data.NewData)
	api.Register(exposure.NewEntryPoint)
	api.Register(data.PotentialDataExposure)
	api.Register(lifecycle.Removal)

	// Phase 6 (v1.1) -- AWS Top 10 rules
	api.Register(exposure.S3BucketPublic)
	api.Register(identity.IAMAdminAttached)
	api.Register(exposure.LambdaPublicURL)

	// Phase 7 PR4 (v1.2) -- Coverage tranche 2 rules
	api.Register(exposure.CloudFrontNoWAF)
	api.Register(data.EBSUnencrypted)
	api.Register(exposure.MessagingTopicPublic)
	api.Register(exposure.NACLAllowAllIngress)

	// Phase 8 (v1.3) -- Coverage tranche 3 rules
	api.Register(exposure.EKSPublicEndpoint)
	api.Register(observability.EKSNoLogging)
	api.Register(availability.ASGUnrestrictedScaling)

	// Phase 9 (v1.4) -- Azure tranche-0 rules (registered alongside
	// their AWS counterparts in the same domain folders, NOT in a
	// separate azure/ folder, because rules are organized by
	// signal-domain, not by provider).
	api.Register(exposure.NSGAllowAllIngress)
	api.Register(data.StorageAccountPublic)
	api.Register(data.MSSQLDatabasePublic)
}
