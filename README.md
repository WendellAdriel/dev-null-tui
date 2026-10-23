# /dev/null CLI

A terminal client for [dev-null.sh](https://dev-null.sh), built with Go, Bubble Tea, Lip Gloss, and Bubbles.

Phase 0 provides an offline visual demo and the build/quality scaffold. Login, live logs, posting, and other social features are not available yet. All demo authors and commits are synthetic. The demo makes no network requests and stores no account data.

## Run the demo

With Go **1.27.3** installed, run these commands from the project root:

```sh
go run ./cmd/devnull --demo
go run ./cmd/devnull --demo --theme light
go run ./cmd/devnull --demo --theme dark
```

The default `auto` theme follows the terminal's background when the terminal reports it. The demo uses the terminal font. It requires at least 60 columns and 18 rows, and displays a resize notice below that size.

Press `?` for help, `Esc` to close help, and `Ctrl+C` to exit. The key `q` is reserved for quoting in a later phase, not quitting. See [shortcuts](docs/shortcuts.md).

Running without arguments prints usage. `--help` and `--version` work without opening a TUI or requiring an interactive terminal:

```sh
go run ./cmd/devnull --help
go run ./cmd/devnull --version
```

## Build a binary

```sh
go build -trimpath -o bin/ ./cmd/devnull
```

On macOS and Linux, run `./bin/devnull --demo`. On Windows PowerShell, run `./bin/devnull.exe --demo`. Users of a compiled binary do not need Go or developer quality tools installed.

Build all six targets with the same command on every operating system:

```sh
go run ./tools/build
```

| System | Architectures | Output directories |
| --- | --- | --- |
| macOS | amd64, arm64 | `dist/devnull_darwin_<arch>/devnull` |
| Linux | amd64, arm64 | `dist/devnull_linux_<arch>/devnull` |
| Windows | amd64, arm64 | `dist/devnull_windows_<arch>/devnull.exe` |

The cross-build tool sets `CGO_ENABLED=0` for those binaries. Race tests run separately with their native compiler requirements. Cross-compilation does not prove that an executable works on its target system.

The scaffold reports version `dev`. A release build can set the version through `-ldflags`, for example:

```sh
go build -trimpath -ldflags "-X github.com/WendellAdriel/dev-null-cli/internal/cli.Version=0.1.0" -o bin/ ./cmd/devnull
```

Version `0.1.0` above is an example, not a published release. Binary archives, checksums, signing decisions, and installation packages are planned for the distribution phase.

## Install quality tools

Use Go **1.27.3**, golangci-lint **2.15.0**, and govulncheck **v1.9.0**. The Go module pins the application dependencies. Huh is deferred until a form needs it.

Download the golangci-lint binary for your OS and architecture from its [v2.15.0 release](https://github.com/golangci/golangci-lint/releases/tag/v2.15.0). Verify its archive against the release checksums, extract it, and put the executable on your `PATH`. `.golangci-lint-version` records the selected version. The official binaries include the selected analyzers and goimports formatter.

Install govulncheck with this command on macOS, Linux, or Windows:

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
```

Put the Go bin directory on your `PATH`. It defaults to the `bin` directory under `go env GOPATH`, unless `GOBIN` overrides it. Vulnerability checks need network access to the Go vulnerability database.

## Quality commands

Make is optional. The direct commands below also work from PowerShell. Run them from the project root.

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

`make check` runs the formatting check, lint, module verification, tests, and vulnerability analysis. Race tests, coverage, and build checks are separate targets. On platforms where race detection requires cgo, install a compatible C compiler and enable cgo for that test command. Do not set `CGO_ENABLED=0` globally and expect race tests to run.

The enabled linters are `govet`, `staticcheck`, `errcheck`, `ineffassign`, `unused`, and `gosec`. Formatting uses `goimports`. No blanket warning exclusions or coverage threshold are configured. Inspect coverage with `go tool cover -func=coverage.out`.

## CI and platform verification

The read-only GitHub Actions workflow checks code quality, race behavior, coverage, and all six builds. It also defines native tests, host builds, and version/help smoke checks on Linux, macOS, and Windows. It uploads coverage and binary artifacts, but never publishes releases or pushes repository changes.

The headless demo lifecycle test checks alternate-screen entry and restoration. It does not replace an interactive terminal check. CI jobs have not run until these changes are pushed through a separately authorized action.

At the Phase 0 review handoff, Linux amd64 builds and local validation are available. macOS/Windows native execution and ARM64 runtime checks require their target systems. Do not interpret the cross-build matrix as completed native runtime verification. Test the demo in a macOS terminal and in Windows Terminal with PowerShell before declaring those runtime acceptance checks complete.

## Design reference

The palette, flat rows, and graph motif follow the web project's [design guide](https://github.com/dev-null-sh/dev-null.sh/blob/production/DESIGN.md). This scaffold is a preview, not full Log page parity. Only shipped shortcuts appear in help.
