# Terminal setup and troubleshooting

- [Terminal size](#terminal-size)
- [Theme and icons](#theme-and-icons)
- [Images and avatars](#images-and-avatars)
- [Clipboard](#clipboard)
- [Submit keys in Ghostty](#submit-keys-in-ghostty)
- [Help and command options](#help-and-command-options)

The app supports text navigation without terminal graphics. A compatible terminal can also display avatars and attached images.

## Terminal size

The workspace needs at least 30 columns and 10 rows. Login needs 60 columns and 18 rows, and the composer needs 48 columns and 18 rows. Use at least 60 columns and 18 rows for all three.

Below 100 columns, the sidebar becomes an icon strip. The account aside appears at 124 columns. Keyboard controls and `?` help remain available when the aside is hidden.

## Theme and icons

The default `auto` theme follows the terminal's background when the terminal reports it. Choose an explicit theme when starting the app:

```shell
devnull --theme light
devnull --theme dark
```

Press `F2` to switch between light and dark during the session. This overrides automatic appearance without saving a preference.

Navigation and profile details use Nerd Font icons by default. If your font is missing glyphs, choose plain Unicode symbols:

```shell
devnull --icons plain
```

You can also set `DEVNULL_ICONS=plain` in your environment. An explicit `--icons` flag overrides it.

## Images and avatars

Avatars and inline images require true color and a terminal that confirms Kitty graphics support. Other terminals show lane dots, image descriptions, and links instead. A graphics-capable terminal is optional.

Media stays in memory. The app removes its terminal image IDs when they leave the visible content or the program exits. Feed content is not cached on disk.

If images fail under tmux or SSH, use the text fallback. Those environments still need runtime verification, along with the other checks listed in [platform verification](development.md#platform-verification).

## Clipboard

Press `y` to copy a link. Clipboard access uses optional native utilities:

| Platform | Utilities |
| --- | --- |
| macOS | `pbcopy` |
| Windows | PowerShell |
| Linux | `wl-copy`, `xclip`, or `xsel` |

If clipboard access fails, the app displays the link for manual copying. Clipboard utilities are not required to read the app.

## Submit keys in Ghostty

Ghostty binds `Ctrl+Enter` to fullscreen by default. Use `Ctrl+S` to submit, or add this setting to your Ghostty configuration and reload it:

```ini
keybind = ctrl+enter=unbind
```

The app never changes your terminal configuration. Where modifier keys are unavailable, select a composer control with `Tab` and activate it with `Enter`.

## Help and command options

`devnull --help` and `devnull --version` work without opening the TUI or requiring an interactive terminal.

| Option | Values or effect |
| --- | --- |
| `--theme` | `auto`, `light`, or `dark`. Default: `auto` |
| `--icons` | `nerd` or `plain`. Default: `DEVNULL_ICONS`, or `nerd` when unset |
| `--server` | Server origin. Default: `https://dev-null.sh`. HTTPS is required except on loopback |
| `--help` | Print usage |
| `--version` | Print the version |

For login problems, see [account setup](account.md#browser-return-problems). The [shortcut reference](shortcuts.md) lists controls for every screen.
