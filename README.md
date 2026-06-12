
<p align="center">
  <picture>
    <!-- Logo displayed to Dark Mode users -->
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/logo-dark.png">
    <!-- Default Logo displayed to Light Mode users -->
    <img src="docs/images/logo.png" alt="Yocto Lens Logo" width="100%" style="max-width: 920px;">
  </picture>
</p>


<p align="center">

  <a href="https://github.com/prashantdivate/yocto-lens/actions">
    <img src="https://img.shields.io/github/actions/workflow/status/prashantdivate/yocto-lens/release.yml?label=build&logo=githubactions" />
  </a>

  <a href="https://github.com/prashantdivate/yocto-lens/releases">
    <img src="https://img.shields.io/github/v/release/prashantdivate/yocto-lens?logo=github" />
  </a>

  <a href="https://github.com/prashantdivate/yocto-lens/stargazers">
    <img src="https://img.shields.io/github/stars/prashantdivate/yocto-lens?logo=github" />
  </a>

  <a href="https://github.com/prashantdivate/yocto-lens/network/members">
    <img src="https://img.shields.io/github/forks/prashantdivate/yocto-lens?logo=github" />
  </a>

  <a href="LICENSE">
    <img src="https://img.shields.io/github/license/prashantdivate/yocto-lens" />
  </a>

  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go" />
  <img src="https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-success" />

</p>

<br>

`yocto-lens` is a fast terminal-based Static Analysis, Style Review & Recipe Health Auditing for Yocto/OpenEmbedded layers.

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
s    toggle mode
```

The current mode is shown in the header:

```text
Mode  [ Static Analysis ]   Style Check
```

Only findings from the active mode are shown in the Findings list and Inspector panel.

---

## Installation

Download the latest release:

https://github.com/prashantdivate/yocto-lens/releases

Linux / macOS:

```bash
chmod +x yocto-lens
./yocto-lens <path_to_yocto_layer>
```

Windows:

```powershell
.\yocto-lens.exe <path_to_yocto_layer>
```

## Usage about release

Scan current workspace:

`yocto-lens <path_to_yocto_layer>`

NOTE: This tool can work on recursive 
Scan multiple layers:

`yocto-lens meta-custom meta-product meta-security`

Run without TUI:

```bash
yocto-lens --no-tui /path/to/meta-custom
```

Run only static-analysis findings in console mode:

```bash
yocto-lens --no-tui --mode static /path/to/meta-custom
```

Run only style-check findings in console mode:

```bash
yocto-lens --no-tui --mode style /path/to/meta-custom
```

Export JSON:

`yocto-lens <path_to_yocto_layer> --json report.json`

Export SARIF:

`yocto-lens <path_to_yocto_layer> --sarif report.sarif`

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

## 🛠 Build From Source

```bash
git clone https://github.com/prashantdivate/yocto-lens.git

cd yocto-lens

go build -o yocto-lens ./cmd/yocto-lens
```

Run:

```bash
./yocto-lens /path/to/meta-custom
```

---

## Real World Usage

Yocto Lens is already being used to automate metadata quality checks in Yocto BSP development workflows.

| Project | Usage |
|----------|--------|
| Dynamic Devices BSP | CI validation and metadata quality checks |

## Used In CI

Yocto Lens is already being integrated into automated Yocto validation pipelines.

Example:

- Dynamic Devices BSP Layer
  - https://github.com/DynamicDevices/meta-dynamicdevices-bsp

CI Integration Script:

https://github.com/DynamicDevices/meta-dynamicdevices-bsp/blob/main/scripts/yocto-lens-ci.sh

## CI Example

```bash
./yocto-lens --no-tui --mode static --json yocto-lens.json --sarif yocto-lens.sarif .
```
---

## 🗺️ Roadmap

* [x] Static Analysis
* [x] Style Review
* [x] License Compliance
* [x] Patch Quality Auditor
* [x] Recipe Health Scoring
* [ ] Layer Dependency Graph
* [ ] License Dashboard
* [ ] Upgrade Readiness Report
* [ ] SPDX/SBOM Export
* [ ] CVE Integration

---

## 🤝 Contributing

Issues, discussions, feature requests and pull requests are welcome.

If Yocto Lens helps your workflow, consider giving the project a ⭐.

---

## 📄 License

MIT License

