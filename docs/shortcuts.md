# Keyboard shortcuts

## Phase 0 demo

| Keys | Action |
| --- | --- |
| `?` | Open or close shortcut help |
| `Esc` | Close shortcut help |
| `Ctrl+C` | Exit and restore the terminal |

`q` does not quit. It is reserved for the web application's quote action when that feature ships.

## Account connection

Open the screen with `devnull --connect`. Help and footer hints reflect the current state.

| Keys | Action |
| --- | --- |
| `Enter` | Activate the selected action, or submit a pasted callback link |
| `Tab`, `Shift+Tab` | Select the next or previous action |
| `b` | Reopen the browser while waiting |
| `u` | Show the authorization URL while waiting |
| `p` | Focus the masked full-callback input while waiting |
| `r` | Retry after a connection error |
| `v` | Verify a saved device connection |
| `i` | Show or hide device details while connected |
| `d` | Ask to delete the local device key |
| `Esc` | Leave input or an overlay first, cancel an attempt, or dismiss disconnection confirmation |
| `?` | Open or close contextual help when not typing |
| `Ctrl+C` | Exit, clean up temporary return handling, and restore the terminal |

Typing and paste belong to the callback field. They do not invoke account shortcuts. Disconnection confirmation defaults to keeping the key. Select the delete action explicitly before pressing `Enter`. Deleting the local connection does not revoke the device on the server.

A successful verification shows "Connection verified just now" for five seconds. The connected screen hides the fingerprint by default. Use **Device details** or `i` to reveal it. Browser approval always shows the full fingerprint.

Navigation, composing, search, and engagement shortcuts are not active yet. Later phases add them alongside their features. Modifier shortcuts depend on what the terminal delivers, and must have usable fallback controls.
