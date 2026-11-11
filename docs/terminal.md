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

Without `--theme`, the app starts with the theme saved in Settings. A new install starts on **Auto**, which shows Dark or Light to match the terminal's background when the terminal reports it. `--theme` picks Auto, Light or Dark for one run without saving it:

```shell
devnull --theme light
devnull --theme dark
```

Press `F2` in the workspace, outside popups and the composer, to open the theme list: Auto, Dark, Light, Catppuccin Latte, Catppuccin Mocha, Dracula, Solarized Light, Synthwave '84, Tokyo Night and Tokyo Night Day. Each theme except Auto has one look. The choice is saved in `tui.json`. Event themes (Halloween, Christmas and New Year) show their dark or light colors to match the chosen theme.

The font and its size come from your terminal's settings. A program running inside the terminal can't change them.

### Custom themes

Press `n` in the theme list to make your own theme. Start from the theme you're using, or from the default dark or light one, then name it and set a color for each slot: the background, text, borders, accents, status colors, the six lanes, and the activity graph. The brand red stays the same. Whether the theme counts as dark or light comes from its background.

Type a color as hex, with or without `#`, in full (`#1a2b3c`) or short (`#f00`) form. Or press `Enter` on a color to open hue, saturation and lightness sliders under it: `Up` and `Down` pick a slider, `Left` and `Right` move it, `Shift` moves it in steps of ten, and you can click a bar. `Enter` or `Esc` goes back to typing. The sliders write the hex value, which is what's saved. On a terminal without true color, the bars show the nearest colors it has.

The preview on the right updates as you type or slide, and marks with `◂` the rows the selected color changes. Below it, it lists the pairs that are hard to read, with their contrast. You can still save them.

In the editor, `Up`, `Down` and `Tab` move between fields, `Ctrl+S` saves, and `Esc` returns to the list. Custom themes show in their own group in the list. `e` edits the selected one and `D` deletes it after you confirm. Pick another theme before deleting the one in use. You can keep up to 20, saved in `tui.json`.

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
| `--theme` | `auto`, `light`, or `dark` for this run. Default: the theme saved in Settings |
| `--icons` | `nerd` or `plain`. Default: `DEVNULL_ICONS`, or `nerd` when unset |
| `--server` | Server origin. Default: `https://dev-null.sh`. HTTPS is required except on loopback |
| `--help` | Print usage |
| `--version` | Print the version |

For login problems, see [account setup](account.md#browser-return-problems). The [shortcut reference](shortcuts.md) lists controls for every screen.
