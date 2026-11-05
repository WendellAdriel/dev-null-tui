# Install devnull

This archive holds the `devnull` executable for one OS and architecture, and its MIT license. Go is not required.

Before you run the executable, compare the archive's SHA-256 hash with its line in `checksums.txt` from the same release. The checksum detects a damaged or changed download. It does not prove who published the file.

## Linux and macOS

```shell
mkdir -p ~/.local/bin
mv devnull ~/.local/bin/
```

If `~/.local/bin` is not on your `PATH`, add `export PATH="$HOME/.local/bin:$PATH"` to your shell configuration and open a new terminal.

The binary is not signed or notarized. If macOS blocks it after a browser download, run `xattr -d com.apple.quarantine ~/.local/bin/devnull`, or allow it under System Settings > Privacy & Security.

## Windows

Create `%LOCALAPPDATA%\Programs\devnull` if it does not exist, move `devnull.exe` into it, add that folder to your user `Path` through Environment Variables, and open a new terminal.

The executable is not signed, so SmartScreen may warn you the first time. Choose More info, then Run anyway.

## Next steps

Run `devnull --version` to check the install, then `devnull` to connect your account. `devnull update` installs later releases.

The full guide covers updates, removal, and terminal setup: https://github.com/WendellAdriel/dev-null-tui/blob/main/docs/installation.md
