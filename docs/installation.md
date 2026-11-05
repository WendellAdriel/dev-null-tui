# Installation and updates

- [Linux, macOS, and WSL](#linux-macos-and-wsl)
- [Windows PowerShell](#windows-powershell)
- [Manual installation](#manual-installation)
- [Check your version](#check-your-version)
- [Update](#update)
- [Remove the app](#remove-the-app)

Install `devnull` from [GitHub releases](https://github.com/WendellAdriel/dev-null-tui/releases). Binaries are available for Linux, macOS, and Windows on amd64 and arm64. Go is not required.

## Linux, macOS, and WSL

Install the latest release:

```shell
curl -fsSL https://raw.githubusercontent.com/WendellAdriel/dev-null-tui/main/scripts/install.sh | sh
```

The [shell installer](../scripts/install.sh) detects your OS and architecture, verifies the release's SHA-256 checksum, and installs to `~/.local/bin`. It needs `curl` and either `sha256sum` or `shasum`. WSL installs the Linux binary inside your Linux environment.

If the directory is not on your `PATH`, add this line to your shell configuration, then open a new terminal:

```shell
export PATH="$HOME/.local/bin:$PATH"
```

To choose a different directory or a specific version:

```shell
curl -fsSL https://raw.githubusercontent.com/WendellAdriel/dev-null-tui/main/scripts/install.sh | INSTALL_DIR="$HOME/bin" sh -s -- v0.1.0
```

Use a directory you own. Neither installer requires administrator access.

## Windows PowerShell

Install the latest release in PowerShell:

```powershell
irm https://raw.githubusercontent.com/WendellAdriel/dev-null-tui/main/scripts/install.ps1 | iex
```

The [PowerShell installer](../scripts/install.ps1) detects the Windows OS architecture, verifies the checksum, and installs to `%LOCALAPPDATA%\Programs\devnull`. Add that directory to your user `Path` through **Environment Variables**, then open a new terminal. The script does not change your `Path`.

To choose a directory or version:

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/WendellAdriel/dev-null-tui/main/scripts/install.ps1))) -Version v0.1.0 -InstallDir "$HOME\bin"
```

## Manual installation

Each release has an archive and a plain executable for every platform. Pick amd64 for Intel and AMD processors, and arm64 for Apple silicon and other ARM machines. On Windows, **Settings > System > About** shows the system type. On Linux and macOS, `uname -m` prints `x86_64` for amd64 and `aarch64` or `arm64` for arm64.

### Archives

Archives are named with the version, such as `devnull_v0.5.0_linux_amd64.tar.gz`:

| OS | amd64 | arm64 |
| --- | --- | --- |
| Linux | `devnull_<version>_linux_amd64.tar.gz` | `devnull_<version>_linux_arm64.tar.gz` |
| macOS | `devnull_<version>_darwin_amd64.tar.gz` | `devnull_<version>_darwin_arm64.tar.gz` |
| Windows | `devnull_<version>_windows_amd64.zip` | `devnull_<version>_windows_arm64.zip` |

Each archive holds the `devnull` or `devnull.exe` executable, the license, and short install notes. Extract it with `tar -xzf <archive>` on Linux and macOS, or **Extract All** on Windows, then move the executable to a directory on your `PATH`.

### Plain executables

The installers and `devnull update` use these files, which have no version in their names:

| OS | amd64 | arm64 |
| --- | --- | --- |
| Linux | `devnull_linux_amd64` | `devnull_linux_arm64` |
| macOS | `devnull_darwin_amd64` | `devnull_darwin_arm64` |
| Windows | `devnull_windows_amd64.exe` | `devnull_windows_arm64.exe` |

Rename the executable to `devnull` or `devnull.exe` and place it in a directory on your `PATH`. On Linux and macOS, make it executable with `chmod +x devnull`.

### Verify the download

Download `checksums.txt` from the same release, and compare your file's SHA-256 hash with its line. Use `sha256sum` on Linux, `shasum -a 256` on macOS, or `Get-FileHash -Algorithm SHA256` in PowerShell. Checksums detect a damaged or changed download, but they do not prove who published the file.

### Operating system warnings

Release binaries are not code-signed or notarized.

- **macOS:** a file downloaded through a browser can be blocked on first launch. Allow it under **System Settings > Privacy & Security**, or run `xattr -d com.apple.quarantine <path-to-devnull>`.
- **Windows:** SmartScreen may warn about an unrecognized app the first time it runs. Choose **More info**, then **Run anyway**.

The Linux binaries are statically linked, so they need no system libraries. See [terminal setup](terminal.md) for terminal size, fonts, and image support.

## Check your version

```shell
devnull --version
```

The command prints your installed version without opening the TUI.

## Update

Close other `devnull` sessions, then run:

```shell
devnull update
```

The command checks the latest stable GitHub release and updates the executable you ran. It does not downgrade, install prereleases, or send your account key to GitHub. A checksum failure leaves your installed executable unchanged. Account data is not modified.

The executable's directory must be writable. If permissions prevent replacement, reinstall into a directory you own. For a package-manager installation, use that manager's update command instead.

On Windows, the previous binary remains as `devnull.exe.old`. The next update removes that backup. You can also delete it after all old app processes exit. Restart `devnull` to use the new version.

If GitHub is unavailable or its API rate limit is reached, retry later or install a verified download manually. Builds explicitly labeled `dev` cannot self-update.

### Update from the TUI

Each time the TUI starts, it checks GitHub for a newer stable release. A failed check is silent. If a release is available, a dialog shows its version with three choices:

- **Update and restart** installs it the same way as `devnull update`, closes the TUI, and starts the new version with the same flags. On Windows, the choice is **Update**, and you start `devnull` again yourself.
- **Later** closes the dialog. It shows again the next time you start the TUI.
- **Skip this version** hides that version. A newer release shows again.

If the install fails, the dialog shows the error, and you can retry or run `devnull update` in a terminal. Turn the check off in [Settings > TUI](settings.md#tui). Builds labeled `dev` never check.

## Remove the app

Delete the installed executable and any `.old` backup. Remove the installation directory from your `PATH` if it contains no other tools.

Removing the binary does not delete your account key or revoke the device. See [disconnect and revoke access](account.md#disconnect-and-revoke-access) before removing account data.
