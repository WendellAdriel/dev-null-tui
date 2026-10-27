# Development

- [Build from source](#build-from-source)
- [Cross-platform builds](#cross-platform-builds)
- [Quality tools](#quality-tools)
- [Quality checks](#quality-checks)
- [Test servers](#test-servers)
- [Bundled emoji catalog](#bundled-emoji-catalog)
- [CI](#ci)
- [Platform verification](#platform-verification)
- [Runtime constraints](#runtime-constraints)

This guide covers working on the Go codebase. For app usage, see the [user guide](usage.md) and [shortcut reference](shortcuts.md).

## Build from source

Install Go **1.27.3**, then run commands from the repository root. To start without creating a separate binary:

```shell
go run ./cmd/devnull
```

The app opens login or verifies your saved connection. It uses the same account storage as a compiled executable.

To build for your current platform:

```shell
go build -trimpath -o bin/ ./cmd/devnull
```

Run `./bin/devnull` on Linux or macOS. In Windows PowerShell, run `./bin/devnull.exe`. Compiled binaries do not require Go or the quality tools at runtime.

The default version is `dev`. To set the reported version, pass the version variable through `-ldflags`:

```shell
go build -trimpath -ldflags "-X github.com/WendellAdriel/dev-null-cli/internal/cli.Version=0.1.0" -o bin/ ./cmd/devnull
```

`0.1.0` is an example, not a published release. Packaging and binary installation instructions are not available yet.

## Cross-platform builds

Build all six targets with the same command on Linux, macOS, or Windows:

```shell
go run ./tools/build
```

The tool writes each executable to its own directory:

| System | Architectures | Executable paths |
| --- | --- | --- |
| macOS | amd64, arm64 | `dist/devnull_darwin_<arch>/devnull` |
| Linux | amd64, arm64 | `dist/devnull_linux_<arch>/devnull` |
| Windows | amd64, arm64 | `dist/devnull_windows_<arch>/devnull.exe` |

These builds use `CGO_ENABLED=0`. Cross-compilation does not prove native runtime behavior. Race tests run separately and may require cgo and a compatible C compiler.

## Quality tools

The pinned tools are Go **1.27.3**, golangci-lint **2.15.0**, and govulncheck **v1.9.0**. Application dependencies are pinned in [`go.mod`](../go.mod). Huh is not a dependency yet.

Download the golangci-lint binary for your platform from its [v2.15.0 release](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0). Verify the archive against the release checksums, extract it, and put the executable on your `PATH`. [`.golangci-lint-version`](../.golangci-lint-version) records the version used by CI.

Install govulncheck with the same command on every platform:

```shell
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
```

Put Go's binary directory on your `PATH`. It defaults to the `bin` directory under `go env GOPATH`, unless `GOBIN` overrides it. Vulnerability checks need access to the Go vulnerability database.

## Quality checks

Make is optional. Run these commands from the repository root. The direct commands also work in PowerShell.

| Check | Make shortcut | Direct command |
| --- | --- | --- |
| Format and organize imports | `make fmt` | `golangci-lint fmt` |
| Check formatting without changes | `make fmt-check` | `golangci-lint fmt --diff` |
| Static and security analysis | `make lint` | `golangci-lint run ./...` |
| Tests | `make test` | `go test ./...` |
| Race detection | `make test-race` | `go test -race ./...` |
| Coverage report | `make coverage` | `go test -coverprofile=coverage.out ./...` |
| Known vulnerabilities | `make vuln` | `govulncheck ./...` |
| Module integrity | `make verify` | `go mod verify` |
| Host build | `make build` | `go build -trimpath -o bin/ ./cmd/devnull` |
| Six-target build | `make build-all` | `go run ./tools/build` |

`make check` runs the formatting check, lint, module verification, tests, and vulnerability analysis. Race tests, coverage, and builds are separate. Do not set `CGO_ENABLED=0` globally when running race tests that require cgo.

[`.golangci.yml`](../.golangci.yml) enables `govet`, `staticcheck`, `errcheck`, `ineffassign`, `unused`, and `gosec`. Formatting uses `goimports`. There are no blanket warning exclusions or minimum coverage thresholds. Inspect coverage with `go tool cover -func=coverage.out`.

## Test servers

Use `--server` to connect to an isolated environment:

```shell
go run ./cmd/devnull --server https://your-test-host
```

Plain HTTP is accepted only for `localhost` or a loopback IP. A saved connection must match the selected server before any signed restore request is sent. Tests use local HTTP fixtures and do not enroll production devices.

## Bundled emoji catalog

To refresh the catalog from a local web checkout, run:

```shell
go run ./tools/emoji ../dev-null.sh/resources/js/lib/emoji-shortcodes.json
```

The tool validates the JSON and replaces `internal/tui/emoji.json`. The compiled app does not need the web checkout. Suggestions work offline and do not keep usage history.

## CI

The read-only [Checks workflow](../.github/workflows/checks.yml) runs quality checks, race tests, coverage, and six-target builds. Native Linux, macOS, and Windows jobs run tests, host builds, and version/help smoke checks. Coverage and binaries are uploaded as artifacts.

The workflow does not publish releases or push changes. Its configuration alone is not evidence that remote jobs have passed.

## Platform verification

Linux amd64 builds, local tests, and synthetic Ghostty checks have run. Native macOS and Windows, ARM64 runtime, real-account connection, remote CI, real tmux, and SSH checks remain open.

The headless lifecycle test checks alternate-screen entry and restoration, but does not replace an interactive terminal check. Verify browser return, key-storage protection, cancellation, and terminal restoration on the target platform before claiming runtime support.

## Runtime constraints

Media requests are unsigned and limited to two concurrent downloads, 10 MiB per response, and eight million decoded pixels. Images and feed content stay in memory. Terminal image cleanup removes only the app's own IDs.

The palette, flat rows, and graph motif follow the web project's [design guide](https://github.com/dev-null-sh/dev-null.sh/blob/production/DESIGN.md).
