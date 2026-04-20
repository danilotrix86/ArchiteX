# 05 — Library mode: `count = var.create ? 1 : 0`

This is the canonical "module-author repo" shape: every resource is gated behind a `count = var.<flag> ? 1 : 0` (or `length(var.X) > 0 ? 1 : 0`). Without library mode the parser warns-and-skips every block and produces an empty graph.

With library mode (configured via the co-located `.architex.yml`), the parser materializes one **conditional phantom** per gate. The diagram shows them with a `?` prefix; risk rules refuse to score them.

## Expected output

- The diagram includes the conditional phantoms (look for `? compute:`, `? storage:`, etc.).
- **No risk rules fire**, even though the head fixture adds a public-endpoint EKS cluster — the rule layer correctly treats the phantom as non-existent.

**Total: 0.0 / 10 — `low` / `pass`**

## Run

```bash
./architex report ./examples/05-library-mode/base ./examples/05-library-mode/head --out ./.architex/
```
