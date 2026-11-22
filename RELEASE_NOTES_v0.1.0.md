# Configuration Baseline v0.1.0

First release of Configuration Baseline, a read-only Go CLI for comparing JSON
or YAML configuration files with explicit approved-baseline rules.

## Supported

- JSON and single-document YAML input.
- `equals`, `exists`, `absent`, and `one_of` rules using RFC 6901 JSON Pointer
  paths.
- Missing, unexpected, and modified drift findings, with deterministic text or
  JSON reports. Exit codes: `0` no drift, `1` drift found, `2` usage or input
  error.
- Reports omit configuration values; inputs are only read, never modified.
- Text reports quote paths that contain control characters, so crafted keys
  cannot forge report lines. The whole document is shown as `(root)`.
- YAML timestamps are compared as their literal text and match the same JSON
  string.
- `config-baseline -version` prints the release tag, including for builds made
  with `go install`.
- Release builds are provided for Linux, macOS, and Windows on amd64 and arm64,
  with SHA-256 checksums.

## Install

```sh
go install github.com/JSP-IT/configuration-baseline/cmd/config-baseline@v0.1.0
```

Or download an archive below and verify it against `checksums.txt`.

## Known limitations

- TOML, XML, INI, HCL, remote scanning, wildcard rules, baseline approval
  workflows, and before/after values in reports are not supported.
- An `exists` rule checks presence only and does not inspect the value or its
  descendants.
- The tool compares files supplied by the caller; it does not discover or
  export system configuration.

See the README for rule examples, exit codes, and security considerations.
