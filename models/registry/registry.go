// Package registry centralizes per-provider Terraform resource metadata
// (supported types, abstract-role mapping, edge labels, attribute
// promotion) into one place per provider. It exists so adding a
// resource means editing ONE entry in ONE provider file -- no longer
// touching models/models.go (two giant maps) AND graph/graph.go
// (edgeTypeMap + deriveAttributes switch).
//
// Design constraint: this package MUST NOT import architex/models. The
// dependency runs in the other direction -- models reads our tables at
// init() to populate models.SupportedResources and models.AbstractionMap
// for backward compatibility. To keep AttrPromoter usable from graph,
// it takes a raw attribute map (map[string]any) instead of
// models.RawResource. graph.deriveAttributes calls
// AttrPromoterFor(res.Type)(res.Attributes).
//
// Provider files (aws.go, azure.go) register their resources via init()
// in deterministic order, mirroring the pre-refactor map literal order
// so first-iteration tests and golden snapshots stay byte-identical.
package registry

// AttrPromoter takes a raw resource's attribute map and returns the
// promoted attribute map that lives on the architecture-graph node.
//
// Promoters are pure functions: same input -> same output, no side
// effects, no I/O. They are responsible for all per-resource attribute
// extraction logic that previously lived inside graph.deriveAttributes
// switch arms (e.g. promoting `policy_arn` for IAM attachments,
// computing `public` from cidr_blocks for SG rules, passing
// `web_acl_id` through for CloudFront).
//
// The conditional flag (set by parser library mode) is handled
// orthogonally in graph.deriveAttributes -- promoters never need to
// look at it.
type AttrPromoter func(attrs map[string]any) map[string]any

// Resource is one provider-type registration: the literal Terraform
// type string (e.g. "aws_lb"), the abstract architectural role it
// maps to (e.g. "entry_point"), and the function that promotes its
// attributes for the graph node.
type Resource struct {
	ProviderType string
	AbstractType string
	Promoter     AttrPromoter
}

// Edge is one source+target containment / placement / governance
// relationship the graph builder labels with something more specific
// than the generic "references" fallback.
type Edge struct {
	Source string
	Target string
	Label  string
}

var (
	resources = map[string]Resource{}
	edges     = map[string]string{} // "source|target" -> label
)

// Register adds a resource type to the registry. Called from each
// provider file's init(). Order matters: callers MUST register in the
// same order entries appeared in the pre-refactor models.go literal,
// so iteration order (when callers happen to range over the maps)
// stays stable and golden snapshots remain byte-identical.
func Register(r Resource) {
	if r.Promoter == nil {
		r.Promoter = defaultPromoter
	}
	resources[r.ProviderType] = r
}

// RegisterEdge declares a labelled relationship between a source and
// target provider type. Pairs not registered here fall through to the
// generic "references" label in graph.inferEdgeType.
func RegisterEdge(source, target, label string) {
	edges[source+"|"+target] = label
}

// AttrPromoterFor returns the promoter for a provider type. Unknown
// types get the default promoter, which mirrors the pre-refactor
// `default:` arm of graph.deriveAttributes (just sets public=false).
func AttrPromoterFor(providerType string) AttrPromoter {
	if r, ok := resources[providerType]; ok {
		return r.Promoter
	}
	return defaultPromoter
}

// EdgeLabelFor returns the registered label for a (source, target)
// pair, or "" if the pair isn't registered. Callers (graph.inferEdgeType)
// substitute "references" when "" is returned. Splitting that fallback
// out of the lookup keeps the registry's intent explicit -- "" means
// "no special label".
func EdgeLabelFor(source, target string) string {
	return edges[source+"|"+target]
}

// SupportedTypes returns a fresh map of all registered provider types
// suitable for use as models.SupportedResources. Built fresh at every
// call so the caller can mutate freely; in practice it's only called
// once at models package init().
func SupportedTypes() map[string]bool {
	out := make(map[string]bool, len(resources))
	for k := range resources {
		out[k] = true
	}
	return out
}

// AbstractTypes returns a fresh map of provider type -> abstract role
// suitable for use as models.AbstractionMap. Same one-shot semantics
// as SupportedTypes.
func AbstractTypes() map[string]string {
	out := make(map[string]string, len(resources))
	for k, r := range resources {
		out[k] = r.AbstractType
	}
	return out
}

// defaultPromoter mirrors the pre-refactor `default:` arm of
// graph.deriveAttributes: every unregistered resource type gets a node
// with `public = false`. Phase 6 resources without their own switch
// case (aws_s3_bucket, aws_s3_bucket_public_access_block, IAM-* except
// the policy attachment, aws_lambda_function) intentionally land here
// -- their behavior is governed by sibling resources, not by a single
// derived attribute.
func defaultPromoter(_ map[string]any) map[string]any {
	return map[string]any{"public": false}
}
