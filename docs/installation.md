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

Download your executable and `checksums.txt` from the same release:

| OS | amd64 | arm64 |
| --- | --- | --- |
| Linux | `devnull_linux_amd64` | `devnull_linux_arm64` |
| macOS | `devnull_darwin_amd64` | `devnull_darwin_arm64` |
| Windows | `devnull_windows_amd64.exe` | `devnull_windows_arm64.exe` |

Compare the file's SHA-256 hash with its entry in `checksums.txt`. Use `sha256sum` on Linux, `shasum -a 256` on macOS, or `Get-FileHash -Algorithm SHA256` in PowerShell. These checksums detect changed downloads, but are not independent publisher signatures.

Rename the executable to `devnull` or `devnull.exe` and place it in a directory on your `PATH`. On Linux and macOS, make it executable with `chmod +x devnull`. macOS may require approval under **System Settings > Privacy & Security** for a downloaded executable. Release binaries are not code-signed or notarized by this tooling.

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

## Remove the app

Delete the installed executable and any `.old` backup. Remove the installation directory from your `PATH` if it contains no other tools.

Removing the binary does not delete your account key or revoke the device. See [disconnect and revoke access](account.md#disconnect-and-revoke-access) before removing account data.
