# Codex Agent Instructions

You are working on Yocto Lens, a Go static analysis tool for Yocto Project and OpenEmbedded metadata.

Follow the shared agent guide in `docs/AI_AGENTS.md` before changing analyzer rules, parser behavior, exports, CLI behavior, or documentation.

Important defaults:

- Prefer official Yocto Project and BitBake documentation before making rule changes.
- Reduce false positives aggressively; CI users need high-signal findings.
- Keep analyzer output deterministic.
- Add tests for new rules and for false-positive cases.
- Use `gofmt` and `go test ./...` when Go tooling is available.
- Commit each implemented improvement separately when the user asks for trackable changes.

Useful commands:

```powershell
gofmt -w .
go test ./...
go run .\cmd\yocto-lens --no-tui .
go run .\cmd\yocto-lens --profile .
git diff --check
```

Do not push unless the user explicitly asks.
