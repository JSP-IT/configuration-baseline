# Configuration Baseline

Configuration Baseline is a lightweight, read-only Go command-line utility for
comparing a configuration file with an approved baseline. It reports missing,
unexpected, and modified settings without changing either input.

## Installation

Once the first release is published, download the archive for your operating
system and architecture from
[GitHub Releases](https://github.com/JSP-IT/configuration-baseline/releases).
Each release archive has a SHA-256 entry in `checksums.txt`.

After the approved PR is merged, create a draft GitHub Release for the merge
commit with `RELEASE_NOTES_v0.1.0.md`. Manually run the **Release binaries**
workflow with that draft's tag. The workflow verifies that the tagged commit
is already on `main`, builds the archives and checksums, and uploads them to
the draft without publishing it. Review the assets and publish the release
manually.

With Go installed, install a published version with:

```powershell
go install github.com/JSP-IT/configuration-baseline/cmd/config-baseline@v0.1.0
```

Or build from a checkout:

```powershell
go build -o config-baseline ./cmd/config-baseline
```

## Quick start

Compare the synthetic example files from the repository root:

```powershell
go run ./cmd/config-baseline -baseline examples/baseline.yaml -config examples/config-drift.yaml
```

Example output:

```text
Configuration drift detected.
UNEXPECTED /debug
UNEXPECTED /service/experimental_flag
MODIFIED /service/port
Summary: 0 missing, 2 unexpected, 1 modified (3 total)
```

The report includes setting paths and statuses, never the configured or
expected values. JSON output is available with `-format json`:

```powershell
config-baseline -baseline baseline.yaml -config current.yaml -format json
```

Exit codes are `0` for no drift, `1` when drift is found, and `2` for invalid
arguments, inaccessible or malformed files, unsupported formats, or invalid
baseline rules.

## Baseline rules

Baseline and configuration files may be JSON (`.json`) or YAML (`.yaml`,
`.yml`). A baseline has `schema_version: 1` and a list of rules. Rule paths use
[RFC 6901 JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901), including
`~0` for `~` and `~1` for `/`.

```yaml
schema_version: 1
allow_unexpected: false
rules:
  - path: /server/port
    type: equals
    value: 8443
  - path: /tls/enabled
    type: equals
    value: true
  - path: /auth/api_token
    type: exists
  - path: /debug
    type: absent
  - path: /logging/level
    type: one_of
    values: [info, warning]
```

Supported rule types:

| Type | Meaning |
| --- | --- |
| `equals` | The path must exist and equal the given value. Objects and arrays are compared recursively, with individual missing, unexpected, and modified paths reported. |
| `exists` | The path must exist. Its value and descendants are not inspected. |
| `absent` | The path must not exist. |
| `one_of` | The path must exist and equal one of the listed values. |

Unexpected configuration leaves without a matching rule are reported by
default. Set `allow_unexpected: true` to ignore unruled leaves; explicit
`equals` rules still compare their entire value. Duplicate rule paths and
unknown fields or rule types are rejected.

## Examples

The `examples` directory contains a synthetic baseline, a drifted configuration
and its expected report. The drift example uses a clearly fake token value to
demonstrate that values are not printed.

## Security and read-only behavior

- The program only reads the two specified files and writes a report to
  standard output. It does not write to inspected files, invoke shell
  commands, connect to systems, or apply configuration changes.
- Reports omit all configuration and baseline values, including values at
  paths such as `password`, `token`, and `secret`. Parse errors are sanitized
  rather than echoing source text.
- Use trusted baseline files and protect access to configuration files as
  usual. The tool does not verify who approved a baseline, encrypt files, or
  prevent other processes from changing them.
- This is a file comparator, not a host discovery or remote scanning tool.
  Supply a configuration file or an exported snapshot yourself.

## Limitations

- Only JSON and single-document YAML are supported. TOML, XML, INI, HCL, and
  application-specific formats are not supported.
- Baselines use explicit JSON Pointer rules; the tool does not discover
  baseline values automatically or support wildcard patterns, transformations,
  or environment-variable expansion.
- An `exists` rule deliberately does not validate the value or children of its
  path.
- YAML mapping keys must be strings. Duplicate JSON object keys, duplicate
  YAML keys, non-finite numbers, and multiple YAML documents are rejected.
- Values are reported by path and category only; the report does not include
  before/after values.

## Development

Requires Go 1.26 or newer.

```powershell
go test ./...
go vet ./...
go test -race ./...
```

Project code is MIT-licensed. The project depends on `gopkg.in/yaml.v3` (MIT
license); Go's standard library is BSD-3-Clause. See [LICENSE](LICENSE) and
`go.mod`.
