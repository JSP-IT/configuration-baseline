# Configuration Baseline

[![CI](https://github.com/JSP-IT/configuration-baseline/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/JSP-IT/configuration-baseline/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/JSP-IT/configuration-baseline)](https://github.com/JSP-IT/configuration-baseline/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`config-baseline` checks a JSON or YAML configuration file against an approved
baseline and reports which settings are **missing**, **unexpected**, or
**modified**. It is a single static binary, never changes the files it reads,
and never prints configuration values, so its output is safe to keep in CI logs
even when the configuration contains secrets.

Typical uses:

- Fail a CI pipeline when a deployed or generated configuration drifts from the
  approved one.
- Review an exported configuration snapshot against a hardening baseline.
- Keep environment-specific files (dev, staging, production) within agreed
  limits.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Usage](#usage)
- [Writing a baseline](#writing-a-baseline)
- [Using it in CI](#using-it-in-ci)
- [Security](#security)
- [Limitations](#limitations)
- [Development](#development)

## Install

**Prebuilt binary.** Download the archive for your platform (Linux, macOS, or
Windows; amd64 or arm64) from
[Releases](https://github.com/JSP-IT/configuration-baseline/releases), extract
it, and put `config-baseline` on your `PATH`. Each release includes
`checksums.txt` with SHA-256 hashes:

```sh
sha256sum --ignore-missing -c checksums.txt          # Linux
shasum -a 256 --ignore-missing -c checksums.txt      # macOS
```

```powershell
Get-FileHash .\config-baseline_v0.1.0_windows_amd64.zip -Algorithm SHA256   # Windows
```

**With Go 1.26 or newer:**

```sh
go install github.com/JSP-IT/configuration-baseline/cmd/config-baseline@latest
```

Check the installation with `config-baseline -version`.

## Quick start

The repository contains a synthetic [baseline](examples/baseline.yaml) and a
[drifted configuration](examples/config-drift.yaml):

```yaml
# examples/baseline.yaml
schema_version: 1
allow_unexpected: false
rules:
  - path: /service/host
    type: equals
    value: 0.0.0.0
  - path: /service/port
    type: equals
    value: 8443
  - path: /tls/enabled
    type: equals
    value: true
  - path: /auth/api_token
    type: exists
  - path: /debug
    type: absent
```

```yaml
# examples/config-drift.yaml
service:
  host: 0.0.0.0
  port: 8080
  experimental_flag: true
tls:
  enabled: true
auth:
  api_token: EXAMPLE-ONLY-NOT-A-REAL-SECRET
debug: true
```

Compare them:

```sh
config-baseline -baseline examples/baseline.yaml -config examples/config-drift.yaml
```

```text
Configuration drift detected.
UNEXPECTED /debug
UNEXPECTED /service/experimental_flag
MODIFIED /service/port
Summary: 0 missing, 2 unexpected, 1 modified (3 total)
```

- `/service/port` is `8080` but the baseline requires `8443`.
- `/debug` is present although an `absent` rule forbids it.
- `/service/experimental_flag` is not covered by any rule.
- The API token is only checked for presence, and its value is not printed.

## Usage

```text
config-baseline -baseline FILE -config FILE [-format text|json]
config-baseline -version
```

| Flag | Description |
| --- | --- |
| `-baseline FILE` | Approved baseline (`.json`, `.yaml`, or `.yml`). Required. |
| `-config FILE` | Configuration to inspect (`.json`, `.yaml`, or `.yml`). Required. |
| `-format text\|json` | Report format. Default `text`. |
| `-version` | Print the version and exit. |

The two files may use different formats, for example a YAML baseline for a
JSON configuration.

### Findings

| Status | Meaning |
| --- | --- |
| `MISSING` | A path required by the baseline is not in the configuration. |
| `UNEXPECTED` | A path exists that an `absent` rule forbids, or that no rule covers (unless `allow_unexpected` is set). |
| `MODIFIED` | A path exists but its value does not match the rule. |

Findings are sorted by path, so the same input always gives the same report.
Paths are [JSON Pointers](https://www.rfc-editor.org/rfc/rfc6901). In the text
report the whole document is shown as `(root)`, and a path containing control
characters is printed as a quoted string, so a crafted key cannot add lines to
the report.

### JSON report

With `-format json`, the same result is written as JSON for scripts:

```json
{
  "findings": [
    { "path": "/debug", "status": "UNEXPECTED" },
    { "path": "/service/experimental_flag", "status": "UNEXPECTED" },
    { "path": "/service/port", "status": "MODIFIED" }
  ],
  "summary": { "missing": 0, "unexpected": 2, "modified": 1, "total": 3 }
}
```

The root path is `""` in JSON output.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The configuration matches the baseline. |
| `1` | Drift was found. |
| `2` | Usage error, unreadable or malformed file, unsupported format, or invalid baseline. |

## Writing a baseline

A baseline is a JSON or YAML file with `schema_version: 1`, an optional
`allow_unexpected` flag, and a list of rules. Each rule has a `path` (a JSON
Pointer) and a `type`:

```yaml
schema_version: 1
allow_unexpected: false     # default; set true to ignore settings no rule covers
rules:
  - path: /server/port
    type: equals
    value: 8443
  - path: /server/tls       # whole objects and arrays can be compared
    type: equals
    value:
      enabled: true
      min_version: "1.2"
  - path: /auth/api_token
    type: exists
  - path: /debug
    type: absent
  - path: /logging/level
    type: one_of
    values: [info, warning]
```

| Type | Passes when | Fields |
| --- | --- | --- |
| `equals` | The path exists and equals `value`. Objects and arrays are compared recursively, and each differing child is reported separately. | `value` |
| `exists` | The path exists. Its value and children are not inspected. | none |
| `absent` | The path does not exist. | none |
| `one_of` | The path exists and equals one of `values`. | `values` (non-empty list) |

**Paths.** `/server/port` selects key `port` inside `server`, and `/items/0`
selects the first element of the `items` array. Escape `~` as `~0` and `/` as
`~1`: the key `a/b` is written `/a~1b`. The empty path `""` means the whole
document.

**Coverage.** A rule covers its path and everything below it. With
`allow_unexpected: false`, every configuration value that no rule covers is
reported as `UNEXPECTED`, so the baseline acts as an allow-list.

**Value comparison.**

- Numbers are compared by value: `8443` equals `8443.0`, but the string
  `"8443"` is not a number and does not match.
- YAML timestamps such as `2024-01-01` are compared as text, so they match the
  JSON string `"2024-01-01"` but not `2024-01-01T00:00:00Z`.
- Strings, booleans, and `null` must match exactly.

**Validation.** A baseline is rejected (exit code `2`) if it has unknown
fields, unknown rule types, duplicate rule paths, invalid pointers, or fields
that do not belong to the rule type.

## Using it in CI

Because drift returns exit code `1`, the tool can gate a pipeline directly.
GitHub Actions example:

```yaml
- uses: actions/setup-go@v7
  with:
    go-version: "1.26.x"
- name: Check configuration drift
  run: |
    go install github.com/JSP-IT/configuration-baseline/cmd/config-baseline@v0.1.0
    config-baseline -baseline baselines/production.yaml -config deploy/production.yaml
```

Pin a release tag rather than `@latest` so that results do not change
unexpectedly. Use `-format json` if a later step needs to process the
findings.

## Security

- **Read-only.** The tool reads the two named files and writes the report to
  standard output. It does not modify files, run commands, or open network
  connections.
- **No values in output.** Reports and error messages contain paths and
  statuses only, never configuration or baseline values. Parse errors do not
  echo file contents.
- **Strict parsing.** Duplicate keys in JSON or YAML, non-string YAML keys,
  non-finite numbers, and multi-document YAML are rejected rather than guessed
  at.
- **Out of scope.** The tool does not verify who approved a baseline, encrypt
  files, or stop other processes from changing them. Keep baselines under
  review (for example, in version control) and protect configuration files as
  usual.

## Limitations

- Only JSON and single-document YAML are supported. TOML, XML, INI, HCL, and
  `.env` files are not.
- Rules use exact paths; wildcards, transformations, and environment-variable
  expansion are not supported.
- Reports do not show expected or actual values. Look up a reported path in
  the files to see the difference.
- The tool compares files you provide. It does not discover or export
  configuration from hosts or services.

## Development

Requires Go 1.26 or newer.

```sh
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
```

The test suite checks that the example output in
[examples/expected-report.txt](examples/expected-report.txt) is still correct.
If you change the output format, update that file and this README together.
Releases are published as described in [RELEASING.md](RELEASING.md).

## License

MIT, see [LICENSE](LICENSE). The only third-party dependency is
[`gopkg.in/yaml.v3`](https://github.com/go-yaml/yaml) (MIT and Apache-2.0).
