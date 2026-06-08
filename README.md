# Yocto Lens

`yocto-lens` is a terminal-based static analysis and style review tool for Yocto / OpenEmbedded layers.

It scans Yocto metadata such as:

* `.bb`
* `.bbappend`
* `.patch`
* `conf/layer.conf`

It is designed to work with any Yocto project layout. It does not depend on KAS, Docker, repo manifests, or company-specific directory names.

A directory is treated as a Yocto layer only when it contains:

```text
conf/layer.conf
```

---

## Features

### Static Analysis

Static analysis focuses on build correctness, security, reproducibility, and integration risks.

Examples:

* missing `LICENSE`
* missing `LIC_FILES_CHKSUM`
* floating `SRCREV`
* `AUTOREV` usage
* insecure `http://` fetch URLs
* orphan `.bbappend`
* risky `.bbappend` behavior
* duplicate recipe names
* host-specific absolute paths
* possible hardcoded secrets
* patch files missing `Upstream-Status`
* CVE patch metadata issues

### Style Check

Style check focuses on maintainability, metadata quality, and review hygiene.

Examples:

* missing `SUMMARY`
* wildcard `.bbappend`
* old override syntax
* unclear or broad append behavior

---

## TUI Mode Toggle

The TUI supports two clean review modes:

```text
Static Analysis
Style Check
```

Use:

```text
tab  toggle mode
m    toggle mode
```

The current mode is shown in the header:

```text
Mode  [ Static Analysis ]   Style Check
```

Only findings from the active mode are shown in the Findings list and Inspector panel.

---

## Usage

Scan a single Yocto layer:

```bash
go run ./cmd/yocto-lens /path/to/meta-custom
```

Scan a workspace containing many Yocto layers:

```bash
go run ./cmd/yocto-lens /path/to/yocto-workspace
```

Scan multiple explicit layers:

```bash
go run ./cmd/yocto-lens /path/to/meta-board /path/to/meta-product
```

Run without TUI:

```bash
go run ./cmd/yocto-lens --no-tui /path/to/meta-custom
```

Run only static-analysis findings in console mode:

```bash
go run ./cmd/yocto-lens --no-tui --mode static /path/to/meta-custom
```

Run only style-check findings in console mode:

```bash
go run ./cmd/yocto-lens --no-tui --mode style /path/to/meta-custom
```

Export JSON and SARIF:

```bash
go run ./cmd/yocto-lens --no-tui --json report.json --sarif report.sarif /path/to/meta-custom
```

---

## Keyboard Shortcuts

```text
↑ / k      move up
↓ / j      move down
/          fuzzy search
tab        toggle Static Analysis / Style Check
m          toggle Static Analysis / Style Check
enter      open finding detail
esc        return from detail
q          quit
```

---

## Fuzzy Search

Press `/` inside the TUI to search findings.

Search works across:

* rule ID
* severity
* category
* title
* file name
* full path
* layer
* message

Examples:

```text
license
autorev
bbappend
patch
secret
style
HIGH
srcuri
```

---

## Generic Yocto Discovery

`yocto-lens` does not assume a fixed project structure.

It discovers layers by looking for:

```text
conf/layer.conf
```

It skips generated or heavy directories:

```text
.git
.repo
tmp
downloads
sstate-cache
cache
deploy
work
sysroots
sysroots-components
pkgdata
stamps
logs
node_modules
__pycache__
```

---

## Build

```bash
go mod tidy
go build -o yocto-lens ./cmd/yocto-lens
```

Run:

```bash
./yocto-lens /path/to/meta-custom
```

---

## CI Example

```bash
./yocto-lens --no-tui --mode static --json yocto-lens.json --sarif yocto-lens.sarif .
```

---

## Design Goal

`yocto-lens` is intended to be a professional embedded Linux review tool:

* fast on large Yocto workspaces
* clear progress while scanning
* useful for developers and CI
* generic across Yocto environments
* focused on metadata that affects security, reproducibility, and maintainability

