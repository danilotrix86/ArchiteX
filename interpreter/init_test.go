package interpreter

// Test-only blank import of the rules aggregator. Several interpreter
// tests construct a delta and call risk.Evaluate / RenderWithGraph; for
// those assertions to see the v1.0+ rules fire, the registry must be
// populated. Production binaries handle this by blank-importing
// architex/risk/rules from cmd/architex/main.go; the equivalent here
// scopes that wiring to the test binary only. No effect on production
// builds of the interpreter package.
import _ "architex/risk/rules"
