
# Yocto Lens

`yocto-lens` is a fast terminal-based static analysis and style review tool for Yocto / OpenEmbedded layers.

Yocto Lens helps embedded Linux developers identify security risks, maintainability issues, layer dependency problems, patch quality concerns, and metadata style violations before they become build or release issues.

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

* Detects `AUTOREV` usage
* Detects floating `SRCREV`
* Detects insecure `SRC_URI` references
* Detects possible hardcoded secrets
* Detects host-specific absolute paths
* Detects orphan `.bbappend` files
* Detects duplicate recipes across layers
* Layer dependency validation
* Missing `LAYERSERIES_COMPAT`
* Missing `BBFILE_COLLECTIONS`
* Missing layer dependencies
* Layer dependency cycle detection
* Isolated layer detection
* License compliance checks
* Missing `LICENSE` metadata
* Missing `LIC_FILES_CHKSUM`
* GPLv3-family package identification
* CLOSED license review warnings

### Style Check

Style check focuses on maintainability, metadata quality, and review hygiene.

Examples:

* Missing `SUMMARY`
* Missing `DESCRIPTION`
* Missing `LICENSE`
* Missing `LIC_FILES_CHKSUM`
* Variable ordering checks
* Assignment formatting checks
* Long line detection
* Trailing whitespace detection
* Legacy override syntax `(_append, _prepend, _remove)`
* Recipe naming convention checks
* Patch metadata validation

### Patch Quality Auditor
* Missing Upstream-Status
* Missing Signed-off-by
* Missing patch subject
* Missing author information
* Invalid patch structure detection
* CVE patch validation
* Secret detection inside patches

### Interactive TUI
* Static Analysis mode
* Style Check mode
* Recipe Health view
* Health score calculation
* Recipe health leaderboard
* Inspector panel
* Detail view
* Interactive search
* JSON export
* SARIF export

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

## Installation

Download the latest binary from GitHub Releases.

Linux:

`chmod +x yocto-lens
./yocto-lens <path_to_yocto_layer>`

macOS:

`chmod +x yocto-lens
./yocto-lens <path_to_yocto_layer>`

Windows:

`.\yocto-lens.exe <path_to_yocto_layer>`

## Usage about release

Scan current workspace:

`yocto-lens <path_to_yocto_layer>`

NOTE: This tool can work on recursive 
Scan multiple layers:

`yocto-lens meta-custom meta-product meta-security`

Export JSON:

`yocto-lens <path_to_yocto_layer> --json report.json`

Export SARIF:

`yocto-lens <path_to_yocto_layer> --sarif report.sarif`

## Usage about repo

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

| Key     | Action             |
| ------- | ------------------ |
| ↑ / ↓   | Navigate findings  |
| Enter   | Open detail view   |
| Esc     | Return             |
| /       | Search findings    |
| s / Tab | Switch mode        |
| h       | Recipe Health view |
| q       | Quit               |

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

## Why Yocto Lens?

Most Yocto validation today happens after BitBake starts parsing or building.

Yocto Lens shifts metadata review earlier by providing fast feedback on:

* Security
* Maintainability
* Layer architecture
* Patch hygiene
* License compliance
* Release readiness

directly from the terminal.
