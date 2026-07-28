# AI Agent Guide for Yocto Lens

This guide is the shared operating manual for AI coding agents working on Yocto Lens. It is referenced by `AGENTS.md` for Codex-style agents and `CLAUDE.md` for Claude Code.

## Project Purpose

Yocto Lens is a static analysis and style review tool for Yocto Project and OpenEmbedded metadata. It scans layers, recipes, appends, patches, include files, and classes for CI-friendly findings.

Core files:

- `cmd/yocto-lens/main.go`: CLI, TUI startup, JSON/SARIF export, profiling.
- `internal/analyzer/analyzer.go`: layer discovery, parsing, rule checks, config, suppressions.
- `internal/model/types.go`: report, finding, progress, and parsed metadata models.
- `internal/tui/app.go`: interactive terminal UI.
- `README.md`: user-facing usage and configuration docs.

## Official Yocto and BitBake References

Prefer official Yocto Project documentation before changing rules:

- BitBake metadata syntax, operators, include/class behavior: https://docs.yoctoproject.org/bitbake/bitbake-user-manual/bitbake-user-manual-metadata.html
- BitBake execution model, providers, preferences, dependencies: https://docs.yoctoproject.org/bitbake/
- Yocto layer structure and `LAYERSERIES_COMPAT`: https://docs.yoctoproject.org/dev-manual/layers.html
- Yocto variables glossary: https://docs.yoctoproject.org/ref-manual/variables.html
- Recipe style guide and patch `Upstream-Status`: https://docs.yoctoproject.org/contributor-guide/recipe-style-guide.html

When a rule depends on release-specific behavior, check the documentation for that target release instead of assuming current development-branch behavior applies to old LTS layers.

## Agent Skill: Yocto Metadata Rule Work

Use this workflow when adding or changing analyzer rules:

1. Identify whether the rule is static correctness, style guidance, security, patch quality, layer compatibility, or CI/export behavior.
2. Check the official Yocto or BitBake reference that governs the behavior.
3. Prefer high-confidence findings. Avoid rules based only on broad substrings.
4. Parse assignment keys and values when possible instead of scanning entire lines.
5. Ignore comments unless the rule intentionally checks comments.
6. Treat `.bbappend` findings carefully; a missing target recipe can be valid when only one layer is scanned.
7. Treat patch findings carefully; patch references can be indirect through variables, includes, appends, and classes.
8. Add a regression test for both the positive case and at least one realistic false-positive case.
9. Keep rule IDs stable. Changing a rule ID breaks CI suppression and SARIF history.
10. Update README or this guide when user-facing behavior changes.

## Agent Skill: Performance Work

Use this workflow when changing parsing, discovery, caching, or profiling:

1. Preserve deterministic output ordering for CI reports.
2. Keep worker pools bounded.
3. Do not introduce unbounded goroutines per file.
4. Avoid repeated full-tree walks inside per-file parsing.
5. Reuse the layer file index and metadata parse cache when possible.
6. Keep progress updates meaningful for both TUI and `--no-tui`.
7. Use `--profile` to compare discovery, parsing, rules, and total scan time.
8. Add focused tests for ordering, counts, cache safety, and skipped paths.

## Agent Skill: False-Positive Reduction

Use this workflow when users report noisy findings:

1. Reproduce the pattern with a minimal metadata snippet in `internal/analyzer/analyzer_test.go`.
2. Decide whether the rule should be removed, downgraded, made configurable, or made stricter.
3. Prefer stricter evidence over broader matching.
4. Keep intentionally weak signals at `INFO` or make them opt-in through config.
5. Do not duplicate findings between static and style modes.
6. Support inline suppressions for intentional exceptions:

```bitbake
# yocto-lens-disable-next-line static/license-closed
LICENSE = "CLOSED"
```

## Config and Suppression Contract

Yocto Lens auto-discovers `.yocto-lens.json` or `yocto-lens.json` from the scan root or current directory.

Supported keys:

```json
{
  "target_release": "scarthgap",
  "exclude": ["recipes-test/**"],
  "disabled_rules": ["style/*"],
  "severity": {
    "static/license-closed": "HIGH"
  }
}
```

Do not remove or rename these keys without a migration path.

## Local Verification

Preferred verification before committing:

```powershell
gofmt -w .
go test ./...
go run .\cmd\yocto-lens --no-tui .
go run .\cmd\yocto-lens --profile .
```

If Go is unavailable in the current environment, at minimum run:

```powershell
git diff --check
git status --short
```

Then clearly report that `gofmt` and `go test` were not run.

## Commit Discipline

Keep commits focused:

- One behavior change per commit.
- Include tests in the same commit as the behavior.
- Do not push from an agent session unless the human explicitly asks and credentials are available.
