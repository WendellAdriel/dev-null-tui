# /dev/null CLI

A terminal client for [dev-null.sh](https://dev-null.sh). Read logs, threads, and notifications, publish text commits, revisit Starred, and follow developers.

- [Get started](#get-started)
- [Documentation](#documentation)
- [License](#license)

## Get started

Login needs a terminal with at least 60 columns and 18 rows. Your device key is stored unencrypted. Read [account security](docs/account.md#device-key-storage) before connecting.

With `devnull` on your `PATH`, start the app:

```shell
devnull
```

On the first launch, press `Enter` to connect your account. Approve the device in your browser, then return to the terminal. Later launches verify your saved connection and open the workspace.

Move with `j` and `k`, press `Enter` to open a thread, and use `c` to write a commit. Press `?` for help or `Ctrl+C` to exit. If your font is missing icons, run `devnull --icons plain`.

For now, see [building from source](docs/development.md#build-from-source) to obtain the executable. Binary installation instructions are not available yet.

## Documentation

### Using the app

- [Account setup and security](docs/account.md)
- [Using the TUI](docs/usage.md)
- [Controls and shortcuts](docs/shortcuts.md)
- [Terminal setup and troubleshooting](docs/terminal.md)

### Development

- [Source builds, quality checks, and CI](docs/development.md)

## License

This project is licensed under the [MIT license](LICENSE).
