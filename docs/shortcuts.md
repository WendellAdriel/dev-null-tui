# Keyboard shortcuts

## Workspace

Run `devnull`. A verified saved connection opens the workspace. Otherwise, login appears and successful approval opens the workspace automatically.

| Keys | Action |
| --- | --- |
| `j`, `k`, `Down`, `Up` | Select the next or previous commit |
| `Tab`, `]`, `l`, `Right` | Switch to the next log, keeping each log's reading position |
| `Shift+Tab`, `[`, `Left` | Switch to the previous log |
| `1`, `2`, `3` | Open Following, Latest, or Trending |
| `g l` | Return to the most recently selected chronological log |
| `g e` | Open Trending |
| `g g`, `Home` | Select the first commit and scroll to the top |
| `G`, `End` | Select the last loaded commit |
| `PageUp`, `PageDown`, `Ctrl+U`, `Ctrl+D` | Move five commits |
| Mouse wheel | Select the next or previous commit, or scroll help while it is open |
| Left click | Select a commit row or log tab |
| `n` | Load older commits when another page exists |
| `Ctrl+R` | Refresh the first page, then select the first commit and scroll to the top after success |
| `y` | Copy the selected commit's link, with a manual fallback if clipboard access fails |
| `a` | Open account connection controls |
| `?` | Open or close centered shortcut help. Arrow and page keys scroll it |
| `Esc` | Close shortcut help |
| `Ctrl+C` | Exit and restore the terminal |

`q` does not quit. It is reserved for the web application's quote action when that feature ships.

Neither `r` nor `R` refreshes the workspace. Lowercase `r` is reserved for replies. During refresh, existing content stays visible. Failure keeps your current position. Trending returns to its first ranked commit rather than its most recently published commit.

The `g` sequence expires after one second. Changing screens, opening help, or changing focus clears it. Paste never invokes workspace shortcuts. Entering another log cancels the previous log's in-flight request, and stale responses cannot overwrite the active state.

## Account connection

Login appears when needed. Press `a` from the workspace to manage the connection. Help and footer hints reflect the current state.

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
| `Esc` | Leave input or an overlay first, cancel an attempt, dismiss disconnection confirmation, or return from a connected account to the workspace |
| `?` | Open or close contextual help when not typing |
| `Ctrl+C` | Exit, clean up temporary return handling, and restore the terminal |

Typing and paste belong to the callback field. They do not invoke account shortcuts. Disconnection confirmation defaults to keeping the key. Select the delete action explicitly before pressing `Enter`. Deleting the local connection does not revoke the device on the server.

A successful verification shows "Connection verified just now" for five seconds. The connected screen hides the fingerprint by default. Use **Device details** or `i` to reveal it. Browser approval always shows the full fingerprint.

Composing, search, thread navigation, and engagement shortcuts are not active yet. Later phases add them alongside their features. Modifier shortcuts depend on what the terminal delivers.
