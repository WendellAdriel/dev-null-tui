<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/dev-null-logo-dark.svg">
    <img src="docs/images/dev-null-logo-light.svg" alt="/dev/null logo" width="400">
  </picture>
</p>

<h1 align="center">/dev/null TUI</h1>

> [!WARNING]
> Pull requests and issues are disabled until the [MVP roadmap](#mvp-roadmap) is complete.

A terminal client for [dev-null.sh](https://dev-null.sh). Read logs, threads, and notifications, publish text commits, revisit Starred, and follow developers.

- [Get started](#get-started)
- [Documentation](#documentation)
- [MVP roadmap](#mvp-roadmap)
- [License](#license)

## Get started

Login needs a terminal with at least 60 columns and 18 rows. Your device key is stored unencrypted. Read [account security](docs/account.md#device-key-storage) before connecting.

### Install on Linux, macOS, or WSL

```shell
curl -fsSL https://raw.githubusercontent.com/WendellAdriel/dev-null-tui/main/scripts/install.sh | sh
```

For Windows and other installation options, see [installation and updates](docs/installation.md).

### Start the app

```shell
devnull
```

On the first launch, press `Enter` to connect your account. Approve the device in your browser, then return to the terminal. Later launches verify your saved connection and open the workspace.

Move with `j` and `k`, press `Enter` to open a thread, and use `c` to write a commit. Press `?` for help or `Ctrl+C` to exit. If your font is missing icons, run `devnull --icons plain`.

## Documentation

### Using the app

- [Installation and updates](docs/installation.md)
- [Account setup and security](docs/account.md)
- [Using the TUI](docs/usage.md)
- [Controls and shortcuts](docs/shortcuts.md)
- [Terminal setup and troubleshooting](docs/terminal.md)

### Development

- [Source builds, quality checks, and CI](docs/development.md)
- [Preparing and publishing releases](docs/releases.md)

## MVP roadmap

- Settings and safety views
- Polls and image attachments
- Remotes support

## License

This project is licensed under the [MIT license](LICENSE).
