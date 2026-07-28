# Codex Agent Instructions

You are working on Yocto Lens, a Go static analysis tool for Yocto Project and OpenEmbedded metadata.

Follow the shared agent guide in `docs/AI_AGENTS.md` before changing analyzer rules, parser behavior, exports, CLI behavior, or documentation.
Also use that guide when answering user questions about Yocto Project, BitBake metadata, Yocto Lens findings, or CI integration.

Important defaults:

- Prefer official Yocto Project and BitBake documentation before making rule changes.
- Prefer official Yocto Project and BitBake documentation before answering Yocto behavior questions.
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
