package risk

// Test-only blank import of the rules aggregator. The risk package's
// production code deliberately does NOT depend on architex/risk/rules
// (to avoid the import cycle the architex/risk/api leaf package was
// designed to break). But the risk-package's own unit tests assume the
// v1.0+ rules are registered when EvaluateWithBaseline runs, so the
// test binary needs the same wiring the architex binary and the golden
// harness use. Blank-importing here triggers the aggregator's init()
// only inside the test binary; production builds of `architex/risk`
// remain rule-less and cycle-free.
import _ "architex/risk/rules"
