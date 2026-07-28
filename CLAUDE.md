# Claude Code Instructions

You are working on Yocto Lens, a Go static analysis tool for Yocto Project and OpenEmbedded metadata.

Read and follow `docs/AI_AGENTS.md` as the project skill guide. It defines the Yocto references, rule-writing workflow, false-positive reduction workflow, performance guidance, verification steps, and commit discipline.

Project priorities:

- Make findings useful for embedded Linux developers using Yocto/OpenEmbedded in CI.
- Prefer precise parsing and official documentation over broad text matching.
- Keep SARIF/JSON output deterministic and stable.
- Avoid noisy findings unless they are configurable, suppressible, or clearly low severity.
- Add tests for behavior changes.

Recommended local checks:

```powershell
gofmt -w .
go test ./...
go run .\cmd\yocto-lens --no-tui .
go run .\cmd\yocto-lens --profile .
git diff --check
```

Do not push unless the user explicitly asks.
