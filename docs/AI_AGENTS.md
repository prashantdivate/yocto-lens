# AI Agent Guide for Yocto Lens

This guide is the shared operating manual for AI coding agents working on Yocto Lens. It is referenced by `AGENTS.md` for Codex-style agents and `CLAUDE.md` for Claude Code.

Agents may be asked to modify this repository, explain Yocto Lens findings, help integrate the tool into a Yocto workspace, or answer general Yocto Project questions. Handle all of those tasks with the same principles: use official Yocto/BitBake documentation first, prefer high-signal guidance, and make any code changes traceable.

## Project Purpose

Yocto Lens is a static analysis and style review tool for Yocto Project and OpenEmbedded metadata. It scans layers, recipes, appends, patches, include files, and classes for CI-friendly findings.

Core files:

- `cmd/yocto-lens/main.go`: CLI, TUI startup, JSON/SARIF export, profiling.
- `internal/analyzer/analyzer.go`: layer discovery, parsing, rule checks, config, suppressions.
- `internal/model/types.go`: report, finding, progress, and parsed metadata models.
- `internal/tui/app.go`: interactive terminal UI.
- `README.md`: user-facing usage and configuration docs.

Supported metadata surfaces:

- Layers with `conf/layer.conf`.
- Recipes: `.bb`.
- Appends: `.bbappend`.
- Patches: `.patch`.
- Includes and classes: `.inc`, `.bbclass`.

Yocto Lens is not BitBake and does not fully evaluate BitBake metadata. Rules must be honest about this limitation.

## Canonical Documentation Sources

Prefer official Yocto Project and BitBake documentation before changing rules or answering Yocto questions:

- BitBake metadata syntax, operators, overrides, includes, and classes: https://docs.yoctoproject.org/bitbake/bitbake-user-manual/bitbake-user-manual-metadata.html
- BitBake execution model, providers, preferences, dependencies, task execution: https://docs.yoctoproject.org/bitbake/
- Yocto layer structure and `LAYERSERIES_COMPAT`: https://docs.yoctoproject.org/dev-manual/layers.html
- Yocto variables glossary: https://docs.yoctoproject.org/ref-manual/variables.html
- Recipe style guide and patch `Upstream-Status`: https://docs.yoctoproject.org/contributor-guide/recipe-style-guide.html
- Yocto manuals index for release-specific docs: https://docs.yoctoproject.org/

When a question depends on a Yocto release, ask or infer the target release. If uncertain, explain that behavior can vary across releases and point to the correct release-specific manual.

## Third-Party Skill Files Policy

Do not vendor or copy random Yocto agent skill files from GitHub into this repository by default.

Reasons:

- Licensing may be unclear.
- Quality varies widely.
- Yocto release behavior changes over time.
- Third-party prompts can encode project-specific assumptions that are wrong for another layer.
- Copying external prompt content makes future maintenance and attribution harder.

Acceptable use:

- Read public third-party material for general ideas only when the user asks for research.
- Re-express useful patterns in this guide using original wording.
- Prefer official Yocto/BitBake docs for technical truth.
- Add organization-specific guidance only when the user provides it or asks for it.
- Keep optional external skill packs outside the core repository unless they have clear licensing and review.

Recommended stance for users:

> Yocto Lens ships with a conservative built-in agent guide. Teams can add local project-specific instructions next to it, but the built-in guide should stay official-doc-based and broadly applicable.

## Agent Operating Modes

### Answering Yocto Questions

Use this workflow when the user asks general Yocto questions:

1. Identify the domain: layer structure, recipes, appends, variables, patches, providers, images, SDK, CI, reproducibility, licensing, or performance.
2. Ask for the target Yocto release if the answer may be release-specific.
3. Use official docs as the primary source.
4. Give a practical answer with commands or metadata snippets when useful.
5. Clearly separate confirmed facts from inference.
6. Avoid claiming Yocto Lens fully matches BitBake parse behavior.

Good answers usually mention:

- The relevant file type: `.bb`, `.bbappend`, `.inc`, `.bbclass`, `layer.conf`, `local.conf`, `bblayers.conf`.
- The relevant variable or mechanism: `SRC_URI`, `SRCREV`, `LICENSE`, `LIC_FILES_CHKSUM`, `LAYERSERIES_COMPAT`, `BBFILE_COLLECTIONS`, `BBFILE_PATTERN`, `BBFILE_PRIORITY`, `PROVIDES`, `PREFERRED_PROVIDER`, `RDEPENDS`, `DEPENDS`, `FILESEXTRAPATHS`, `OVERRIDES`.
- How to validate with BitBake when static analysis is not enough.

### Explaining Yocto Lens Findings

Use this workflow when the user asks why a finding appeared:

1. Locate the rule ID in `internal/analyzer/analyzer.go`.
2. Explain what the rule checks and what it does not check.
3. Show the relevant metadata line or variable.
4. Explain whether it is a true problem, a policy warning, or a weak signal.
5. Suggest one of:
   - Fix the metadata.
   - Configure severity.
   - Disable the rule in `.yocto-lens.json`.
   - Add an inline suppression for intentional exceptions.
6. If the finding is likely noisy, add or propose a regression test before changing the rule.

### Integrating Yocto Lens Into a Yocto Workspace

Yocto Lens can scan a single layer or multiple layers. For better dependency and `.bbappend` results, scan all relevant layers together.

Example local scan:

```powershell
yocto-lens --no-tui C:\work\poky\meta C:\work\meta-openembedded\meta-oe C:\work\meta-product
```

Example Linux scan:

```bash
yocto-lens --no-tui poky/meta meta-openembedded/meta-oe meta-product
```

Recommended CI outputs:

```bash
yocto-lens --no-tui --json yocto-lens-report.json --sarif yocto-lens.sarif meta-product meta-openembedded/meta-oe
```

For performance diagnosis:

```bash
yocto-lens --profile meta-openembedded/meta-oe
```

Integration guidance:

- In GitHub Actions, upload SARIF to code scanning when available.
- In GitLab/Jenkins, archive JSON/SARIF as build artifacts.
- In Yocto build containers, run Yocto Lens before expensive BitBake builds.
- In KAS or repo-based workspaces, pass the checked-out layer paths explicitly.
- If only one layer is scanned, treat orphan `.bbappend` findings as lower confidence.
- Use `.yocto-lens.json` to tune noise for the project.

Yocto Lens does not require sourcing `oe-init-build-env`, but BitBake validation does.

### Modifying Yocto Lens Code

Use this workflow when the user asks for implementation changes:

1. Inspect the existing code path before editing.
2. Keep changes small and local.
3. Preserve deterministic output for CI.
4. Add or update tests.
5. Update README or this guide when behavior changes.
6. Commit each distinct implementation separately when the user wants trackable commits.
7. Do not push unless explicitly asked.

## Rule Design Principles

### High-Confidence Static Rules

Good static rules usually check:

- Missing required metadata such as `LICENSE`.
- Explicit dangerous values such as `SRCREV = "${AUTOREV}"`.
- Missing referenced patch files from `SRC_URI`.
- Layer compatibility declarations that conflict with configured `target_release`.
- Duplicate recipe filenames across scanned layers.
- Patch files with CVE-like names but missing CVE metadata.

### Lower-Confidence Style Rules

Style rules should usually be `LOW` or `INFO`:

- Line length.
- Broad or wildcard `.bbappend`.
- Variable order.
- Missing `DESCRIPTION`.
- Old override syntax, unless target release makes it a hard incompatibility.

### Avoid These False-Positive Patterns

Avoid rules that:

- Match broad substrings across whole files.
- Treat comments as active metadata.
- Flag target paths like `/tmp` as host leaks.
- Flag placeholders like `${CI_TOKEN}` as hardcoded secrets.
- Treat every unreferenced `.patch` as stale.
- Treat normal multi-version recipes as duplicate providers.
- Duplicate the same problem in both static and style modes.

When in doubt, prefer a weaker severity or make the rule configurable.

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

## Agent Skill: CI and SARIF Work

Use this workflow when changing exports or CI behavior:

1. Keep SARIF rule IDs stable.
2. Keep SARIF output deterministic.
3. Prefer repository-relative artifact URIs when possible.
4. Keep JSON output backward compatible.
5. Avoid changing severity mapping without a clear reason.
6. Document any new CLI flags in README.

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

Inline suppressions:

```bitbake
# yocto-lens-disable-next-line rule/id
VAR = "value"

VAR = "value" # yocto-lens-disable-line rule/id
```

An empty suppression applies to all findings on that line. Prefer targeted rule IDs in examples.

## Common User Questions and Agent Response Shape

### "How do I scan my Yocto project?"

Ask which layers are part of the build. Recommend scanning all relevant layers, not just the product layer, to reduce `.bbappend` and dependency false positives.

### "Can Yocto Lens replace BitBake parsing?"

Answer no. Yocto Lens is a fast static analyzer. It catches many common issues before a build, but BitBake remains the authority for full metadata expansion, overrides, provider resolution, and task execution.

### "Why is my bbappend orphaned?"

Explain that the target recipe was not found in the scanned layers. It may be valid if the base layer was not included. Recommend scanning the provider layer too.

### "Why is AUTOREV bad?"

Explain reproducibility: builds can fetch different revisions over time. Recommend pinning `SRCREV`.

### "Why is Upstream-Status required?"

Explain patch maintenance and upstream tracking. Point to the Yocto recipe style guide.

### "Should this rule be disabled?"

Prefer project config or targeted inline suppression for intentional exceptions. Avoid globally disabling high-confidence static rules unless the project policy really differs.

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
- Do not mix formatting-only churn with logic changes.
- Do not push from an agent session unless the human explicitly asks and credentials are available.
