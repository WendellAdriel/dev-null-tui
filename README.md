# /dev/null CLI

A terminal client for [dev-null.sh](https://dev-null.sh), built with Go, Bubble Tea, Lip Gloss, and Bubbles.

Run `devnull` to open the application. It verifies your saved connection or shows login, then opens your workspace with Following, Latest, and Trending. You can read logs and threads, view commit images, submit text commits, reply, quote, and delete your own commits. Engagement actions are not available yet.

## Connect an account

The device key is stored **unencrypted** on your computer. Anyone who obtains the file can act as your account until you revoke the device. Do not share the key, authorization callback, or a backup containing the key.

```sh
go run ./cmd/devnull
```

1. Press `Enter` to start connection.
2. In your browser, sign in through GitHub if needed. Compare the device fingerprint with the terminal, then approve the device.
3. Return to the terminal. After a signed `GET /api/v1/me` verifies your identity, the application opens automatically.

The client temporarily registers a `devnull://` handler when no existing owner is found. It does not replace another application's handler. Linux uses desktop integration utilities, macOS uses built-in application registration utilities, and Windows uses a per-user registry entry. If automatic return is unavailable, press `p` and paste the complete `devnull://auth/callback?...` link. Bare codes are not accepted. On the current website, obtaining the approval callback may require inspecting the approval response's `Location` header in your browser's developer tools. Do not share or save that header. This fallback is less convenient than automatic return.

If the browser does not open, press `u` to display the authorization URL and open it yourself. Press `Esc` to cancel an attempt, or `r` to retry after an error. Attempts expire after ten minutes. Failed enrollment is never retried automatically.

On later launches, the client verifies the saved key with its original server. If a saved device needs attention, the primary action verifies it before starting another browser approval. A revoked device requires reconnection.

Press `a` in the workspace to open your account connection. Use **Device details** or `i` to show or hide the fingerprint. It remains visible during browser approval. Press `Esc` from the connected account view to return to the workspace.

Use `Tab` and `Shift+Tab` to select an action, then `Enter` to activate it. Press `d` to open local disconnection confirmation. It defaults to keeping the key. Select **Delete local key**, then press `Enter`, to delete it. This does not revoke server approval. Revoke the device separately in [Settings > Devices](https://dev-null.sh/settings/devices).

Keys are stored in `connection.json` under the OS configuration directory's `dev-null-cli` subdirectory. Linux and macOS require an owner-only directory and file. Windows applies a protected ACL for the current user. No keyring service, passphrase, or credentials from another TUI are used. Temporary handler records support cleanup on the next account-screen launch after an interruption.

Canceling before enrollment removes a newly generated key. Once enrollment starts, the key is retained because a network error can leave approval uncertain. Verify the saved connection with `v`, retry approval, or disconnect locally and revoke it on the web.

For an approved isolated environment, use `--server https://your-test-host`. Plain HTTP is accepted only for `localhost` or a loopback IP. The saved server must match the selected `--server` origin before any signed restore request is sent. Tests use local HTTP fixtures, not production enrollment.

## Read your workspace

With Go **1.27.3** installed, run these commands from the project root:

```sh
go run ./cmd/devnull
go run ./cmd/devnull --theme light
go run ./cmd/devnull --theme dark
```

The default `auto` theme follows the terminal's background when the terminal reports it. The client uses your terminal font and requires at least 30 columns and 10 rows. Below 100 columns, navigation becomes an icon strip. The account aside appears at 124 columns.

Press `F2` to switch between dark and light while reading, composing, connecting, or confirming an action. The on-screen hint names the theme you switch to. This overrides automatic appearance for the current session without writing a preference file.

Use `Tab` or `1`, `2`, and `3` to choose Following, Latest, or Trending. Move between commits with `j`, `k`, or the mouse wheel. Click a row to open its thread. `PageUp` and `PageDown` move five commits. `g g` or `Home` selects the first commit. `G` or `End` selects the last loaded commit. Older pages load near the end of the list, or when you press `n`.

Press `Ctrl+R` to refresh. Existing content stays visible while the client fetches the first page. After success, the client selects the first commit and scrolls to the top. On failure, your position stays unchanged. In Trending, the first commit is the highest ranked, not necessarily the newest. Switching logs still preserves each log's reading position.

A chronological log checks for new commits once a minute while the workspace is focused, but does not insert them until you refresh. Trending is ranked and does not check for chronological arrivals. Neither `r` nor `R` refreshes the workspace. Lowercase `r` replies to the selected commit.

Wide layouts center the feed between navigation and an account aside. The aside displays your returned profile counts, bio, company, and location when available. Missing counts are omitted, not replaced with zeroes. Author identities use lane-colored dots or round avatars, compact ages, and commit hashes. Each commit's lane line runs from its avatar to a ring beside its stats, and reply previews hang off a dashed line below it, like the web log. Quotes stay grouped with their parent commit.

Press `y` to copy the selected commit's web link. Clipboard access uses optional native utilities: `pbcopy` on macOS, PowerShell on Windows, or `wl-copy`, `xclip`, or `xsel` on Linux. If none works, the client displays the link for manual copying. No clipboard utility is required to read the logs.

Terminals that confirm Kitty graphics support and provide true color can display optional avatars and inline images. Other terminals show dots and image descriptions. Media downloads are unsigned, limited to two concurrent requests, 10 MiB per response, and eight million decoded pixels. Images stay in memory, and the client deletes its terminal image IDs when they leave the visible feed or the program exits. Uploads and voting controls are not available. Suggestions on an empty Following log are labeled separately, not presented as followed content. There are no persistent feed caches.

Press `?` for a centered help overlay. Use the arrow keys or mouse wheel to scroll it, `Esc` to close it, and `Ctrl+C` to exit. The key `q` quotes the selected commit. It never quits. See [shortcuts](docs/shortcuts.md).

`--help` and `--version` work without opening a TUI or requiring an interactive terminal. The old `--demo` and `--connect` flags are removed:

```sh
go run ./cmd/devnull --help
go run ./cmd/devnull --version
```

## Read a thread

Press `Enter`, or click a commit, to open its thread in place of the log. The thread shows up to 20 ancestors, the focused commit, and its replies oldest first. Replies hang off a dashed line below the focused commit, with each author's avatar on its node when the terminal supports graphics. Quotes of the focused commit appear among the replies with a **quoted** tag after the author. Deleted commits appear as **Commit deleted** with no author or text. Press `n` to load more replies and `Ctrl+R` to reload.

Move with `j` and `k`. The selected entry shows its actions: **Reply** (`r`), **Quote** (`q`), **Replies** (`Space`), **View** (`v`) when it has images, and **Delete** (`D`) on your own commits. Click an action or press its key. Click a selected entry to open its thread. `Space` on a reply shows or hides one level of nested replies. Deeper replies open with `Enter` in their own thread. `Esc` returns to the previous thread, then to the same log row and scroll position.

## View images

Click an image, or press `v` on a commit with images, to open the viewer. It shows the image enlarged with its position, such as `1 / 2`. A right panel holds the commit and its replies, drawn like the log: the commit's avatar and lane line end in a ring, and replies hang off a dashed line with their authors' avatars. Use `Left` and `Right`, or click the arrows, to move between images. Navigation stops at the first and last image. Scroll the panel with `j`, `k`, or the mouse wheel, and press `n` to load more replies. Press `p` to hide or show the panel, `y` to copy the image link, and `Esc` to return to where you were. Terminals without Kitty graphics show the image description and link instead. The viewer has no zoom or download.

## Reply, quote, and delete

`r` opens the composer as a reply and `q` as a quote. The modal names the target with a short excerpt, such as **Replying to @grace**. Replies appear in the thread and update its reply count. They are not inserted into logs, because logs only hold top-level commits. Quotes appear at the top of the originating chronological log and among the quoted commit's replies. If the target was deleted before you submit, the client reports that the commit is no longer available and keeps your draft.

Only one draft exists. If you kept a draft, `c`, `r`, or `q` reopens it with its original target and asks you to finish or discard it first.

`D` asks before deleting your own commit. The confirmation defaults to **Keep commit** (`Esc`). Choose **Delete commit** (`Ctrl+D`) to delete it. Deletion cannot be undone. While the request runs, `D` cannot start another delete. The commit leaves your logs, and open threads show it as deleted.

## Write a commit

Press `c` or click **Commit** to open the composer modal over your log. Opening it preserves the reader's size and position. Composing needs at least 48 columns and 18 rows. Text entry owns typing and paste, so letters do not navigate the workspace. Click **Preview**, **Submit**, or **Cancel**, or select a control with `Tab` or `Shift+Tab` and press `Enter`. In the text field, `Enter` inserts a new line. Click to place the caret and drag to select text. Preview uses the signed server endpoint without publishing or storing a commit. Use arrow or page keys to scroll a preview, and **Edit** or `Esc` to return to editing.

The modal footer has three actions: **Preview** (`Ctrl+P`), **Submit** (`Ctrl+Enter` or `Ctrl+S`), and **Cancel** (`Esc`). Preview changes to **Edit** while the server preview is open. No duplicate action row or theme hint appears in the modal.

Ghostty binds `Ctrl+Enter` to fullscreen by default, so the app cannot receive it. To send it to the client, add this line to your Ghostty configuration and reload the configuration:

```ini
keybind = ctrl+enter=unbind
```

The client does not edit terminal settings. Use `Ctrl+S` if you prefer to keep Ghostty's fullscreen binding.

Type a shortcode prefix such as `:ro` to see emoji suggestions from the web app's catalog. Use `Up` and `Down` to choose, then `Enter` or `Tab` to insert. `Esc` dismisses suggestions before canceling the composer. Completing a known shortcode such as `:rocket:` turns it into an emoji at the caret. Code stays literal, including unfinished inline code and fenced blocks. Suggestions work offline and do not store usage history.

The budget is 4000 Unicode characters after normalization. Prose links count as at most 23 characters, while code counts literally. A link can contain at most 2048 characters, and normalized raw text must fit within 40000 characters. Server validation remains authoritative. Only text is sent. Polls and uploads are not supported.

Submissions use your approved device key. The client sends once while a request is in flight and never retries automatically. Validation, rejection, and transport errors keep your draft. If the outcome is unknown, check the web before retrying. Submitting again requires confirmation because the first commit may already exist.

Most successful submissions are **pending**, not public until approved. The originating chronological log shows the returned status without resetting other logs. Trending stays ranked and requires refresh to check for the commit. First-time posters must make their first commit through the web composer and accept the commit rules there.

Submission results appear as toasts. Pending commits say **Submitted for review**, not published. Click a toast's **×** to dismiss it, or wait for it to expire. Errors keep the draft and recovery details in the composer after the toast disappears. Clicks outside a modal do not activate the workspace or discard text.

Drafts stay in memory only. `Esc` or **Cancel** asks before discarding a nonempty draft, and `Ctrl+C` asks before exiting. In Draft Protection, **Keep draft** (`Ctrl+K`) closes the composer and retains the text for later. Press `c` to reopen it. **Discard draft** (`Ctrl+D`) removes the text and also exits if you requested exit. **Back** (`Esc`) returns to editing. Confirmations default to keeping the draft. Closing the terminal or terminating the process loses it. Exiting during submission cannot guarantee that the server cancels publication.

## Build a binary

```sh
go build -trimpath -o bin/ ./cmd/devnull
```

On macOS and Linux, run `./bin/devnull`. On Windows PowerShell, run `./bin/devnull.exe`. Users of a compiled binary do not need Go or developer quality tools installed.

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

Developers can refresh the bundled emoji catalog from a web checkout with `go run ./tools/emoji ../dev-null.sh/resources/js/lib/emoji-shortcodes.json`. Run this from the CLI repository root. The tool validates the supplied local JSON and replaces `internal/tui/emoji.json`. The compiled client does not need the web checkout.

## CI and platform verification

The read-only GitHub Actions workflow checks code quality, race behavior, coverage, and all six builds. It also defines native tests, host builds, and version/help smoke checks on Linux, macOS, and Windows. It uploads coverage and binary artifacts, but never publishes releases or pushes repository changes.

The headless application lifecycle test checks alternate-screen entry and restoration. It does not replace an interactive terminal check. CI jobs have not run until these changes are pushed through a separately authorized action.

Linux amd64 builds and local validation are available. Native macOS and Windows execution, ARM64 runtime checks, and real-account connection acceptance remain pending. Do not interpret the cross-build matrix or local HTTP fixtures as completed native runtime or live-authentication verification. Verify browser return, storage protection, cancellation, and terminal restoration on the target systems before declaring cross-platform login release acceptance complete.

## Design reference

The palette, flat rows, and graph motif follow the web project's [design guide](https://github.com/dev-null-sh/dev-null.sh/blob/production/DESIGN.md). Only shipped shortcuts appear in help. Native platform and live log acceptance checks remain separate from fixture-backed tests and cross-builds.
