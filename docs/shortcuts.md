# Controls and shortcuts

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
| `g n` | Open Notifications and mark all categories read |
| `g g`, `Home` | Select the first commit and scroll to the top |
| `G`, `End` | Select the last loaded commit |
| `PageUp`, `PageDown`, `Ctrl+U`, `Ctrl+D` | Move five commits |
| Mouse wheel | Select the next or previous commit, or scroll help while it is open |
| Left click | Open a commit's thread, or select a log tab, sidebar action, or sidebar shortcut. Click an image to view it |
| `n` | Load older commits when another page exists |
| `Ctrl+R` | Refresh the first page, then select the first commit and scroll to the top after success |
| `y` | Copy the selected commit's link, with a manual fallback if clipboard access fails |
| `a` | Open account connection controls |
| `c` | Open the composer modal |
| `Enter` | Open the selected commit's thread |
| `r` | Reply to the selected commit |
| `q` | Quote the selected commit |
| `v` | View the selected commit's images |
| `D` | Ask to delete your own selected commit |
| `s` | Star or unstar the selected commit |
| `p` | Give the selected commit a pastél, when pastéis mode is on and you have some left |
| `?` | Open or close centered shortcut help. Arrow and page keys scroll it |
| `Esc` | Close shortcut help |
| `Ctrl+C` | Request exit, protecting a nonempty draft |
| `F2` | Switch between light and dark for the current session |

`q` quotes. It never quits.

The selected log entry shows clickable **Thread** (`Enter`), **Reply** (`r`), and **Quote** (`q`) actions beneath its stats. **View** (`v`) appears only when it has images, and **Delete** (`D`) appears only on your own commits. **Star** (`s`) appears on other people's commits, and **Unstar** on any commit you starred. **Pastél** (`p`) appears on other people's commits while pastéis mode is on and you have pastéis left. Counts change only after the server confirms. These controls use the same styling as thread actions and follow the selection as you navigate.

The left sidebar has a **Commit** button. The right sidebar groups **Account** and **Refresh** under **Actions**, with their keys beside the buttons. Its **Shortcuts** section holds navigation, copy, theme, help, and exit keys without repeating those actions. The workspace footer shows status only. When the terminal is too narrow for the right sidebar, keyboard controls and `?` help remain available.

Neither `r` nor `R` refreshes the workspace. Lowercase `r` replies. Refresh progress and success or failure appear as toasts, not footer messages. During refresh, existing content stays visible. Failure keeps your current position. Trending returns to its first ranked commit rather than its most recently published commit.

The `g` sequence expires after one second. Changing screens, opening help, or changing focus clears it. Paste never invokes workspace shortcuts. Entering another log cancels the previous log's in-flight request, and stale responses cannot overwrite the active state.

## Notifications

Opening Notifications marks all categories read, including categories excluded by the current filter. The unread badge uses a separate, non-mutating count request once a minute while focused.

| Keys | Action |
| --- | --- |
| `1` through `6` | Toggle Replies, Quotes, Stars, Mentions, Follows, or Moderation independently |
| `j`, `k`, `Down`, `Up`, mouse wheel | Select a notification or expanded member |
| `Space` | Expand or collapse the selected group |
| `n` | Load older members of the selected expanded group, or older notifications otherwise |
| `Enter` | Open the referenced commit's thread, when available |
| `r`, `q` | Reply to or quote the referenced commit |
| `y` | Copy the referenced commit's link |
| `v` | View attached images when returned by the API |
| `Home`, `G`, `End` | Select the first or last loaded entry |
| `PageUp`, `PageDown`, `Ctrl+U`, `Ctrl+D` | Move five entries |
| `Ctrl+R` | Refresh and select the first notification after success |
| `Esc`, `g l` | Return to the log |
| Left click | Toggle a category, expand a group, load older entries, or open a referenced commit |

`HEAD@{last visit}` separates new notifications above from older notifications below. Returning from a thread restores the same notification and scroll position. Follow notifications have no commit destination. Profile navigation and follow actions are not implemented yet. `c`, `a`, `F2`, `?`, and `Ctrl+C` keep their workspace meanings.

## Thread

The thread replaces the log column. `Esc` returns to the previous thread, then to the same log row.

| Keys | Action |
| --- | --- |
| `j`, `k`, `Down`, `Up` | Select the next or previous entry |
| `PageUp`, `PageDown`, `Ctrl+U`, `Ctrl+D` | Move five entries |
| `Home`, `G`, `End` | Select the first or last entry |
| `Enter` | Open the selected entry's thread |
| `Space` | Show or hide replies. On a reply, show or hide its nested replies |
| `n` | Load more replies |
| `r`, `q` | Reply to or quote the selected entry |
| `v` | View the selected entry's images |
| `D` | Ask to delete your own selected entry |
| `s` | Star or unstar the selected entry |
| `p` | Give the selected entry a pastél, when available |
| `y` | Copy the selected entry's link |
| `Ctrl+R` | Reload the thread |
| `c` | Open the composer |
| `Esc` | Go back |
| Left click | Select an entry, open the selected entry's thread, activate an action, or view an image |

Replies hang off a dashed line below their parent, with each author's avatar on its node when the terminal supports graphics. Quotes carry a **quoted** tag after the author. Nesting shows one inline level. `Space` on a nested reply asks you to press `Enter` to open its thread.

Delete confirmation offers **Keep commit** (`Esc`) and **Delete commit** (`Ctrl+D`). `Tab`, `Shift+Tab`, `Left`, and `Right` move between them. `Enter` activates the focused action, which starts on Keep commit. A **Deleting commit** toast stays until the server answers, then the result toast replaces it. A commit that is already gone counts as deleted.

## Image viewer

| Keys | Action |
| --- | --- |
| `Left`, `Right`, `h`, `l` | Previous or next image. Stops at the ends |
| `j`, `k`, `Down`, `Up`, mouse wheel | Scroll the commit and replies panel |
| `PageUp`, `PageDown`, `Ctrl+U`, `Ctrl+D`, `Home`, `End` | Scroll the panel by five lines or to either end |
| `n` | Load more replies into the panel |
| `p` | Show or hide the commit and replies panel |
| `y` | Copy the image link |
| `Esc` | Close the viewer |
| Left click | Activate a toolbar action or arrow |

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
| `F2` | Switch between light and dark |

Typing and paste belong to the callback field. They do not invoke account shortcuts. Disconnection confirmation defaults to keeping the key. Select the delete action explicitly before pressing `Enter`. Deleting the local connection does not revoke the device on the server.

A successful verification shows "Connection verified just now" for five seconds. The connected screen hides the fingerprint by default. Use **Device details** or `i` to reveal it. Browser approval always shows the full fingerprint.

## Composer

The modal keeps the log visible without changing its reading position. It needs at least 48 columns and 18 rows. A smaller terminal keeps the draft and shows a resize hint.

| Keys | Action |
| --- | --- |
| `Tab`, `Shift+Tab` | Move between the text field, Preview, Submit, and Cancel |
| `Enter` | Insert a new line in the text field, or activate the focused control |
| `Esc` | Leave preview, or request cancellation without implicit submission |
| `Up`, `Down`, `PageUp`, `PageDown` | Scroll while previewing |
| `Ctrl+C` | Request exit with draft protection |
| `Ctrl+Enter` | Submit if the terminal distinguishes this combination |
| `Ctrl+S` | Submit without requiring an enhanced Enter key |
| `Ctrl+P` | Toggle preview and editing |
| `Ctrl+Shift+P` | Preview alias in terminals that distinguish it |
| Left click | Place the caret, choose an emoji, or activate a control |
| Drag | Select text in the editor |
| Mouse wheel | Move the editor caret by one line, choose an emoji, or scroll the preview |

The modal header shows the commit, reply, or quote title once, beside your lane dot and handle. Replies and quotes show their target in a callout below the header. The textarea aligns with the modal content, and the character counter sits at the footer's right edge.

The modal footer shows **Preview** (`Ctrl+P`), **Submit** (`Ctrl+Enter` or `Ctrl+S`), and **Cancel** (`Esc`). Preview becomes **Edit** while previewing. The mouse wheel in the editor moves the caret and scrolls to keep it visible. Clicks and wheel events outside the composer do not change the log. **Cancel** and the modal's **×** protect nonempty drafts with a separate confirmation.

Draft Protection has three footer actions:

| Action | Shortcut | Result |
| --- | --- | --- |
| Keep draft | `Ctrl+K` | Close the composer and keep the in-memory text. Press `c` to reopen it |
| Discard draft | `Ctrl+D` | Remove the text. Also exit if you requested exit |
| Back | `Esc` | Return to editing |

These shortcuts belong to the confirmation, not the editor. The separate duplicate-risk confirmation uses **Submit again** (`Ctrl+S`) instead of Discard draft. It never submits through `Ctrl+D`.

Account action buttons and account-screen footer shortcuts also accept clicks. Shortcut help has a clickable **×**. All toasts expire after three seconds, including refresh progress and errors. Click a toast's **×** to dismiss it sooner. Errors keep the draft and recovery message after the toast expires. Pending submissions say **Submitted for review** because they are not public yet.

Typing and paste never invoke workspace shortcuts while the composer is open. Use focusable controls when modifier shortcuts are unavailable. Nonempty draft cancellation and exit default to keeping the draft. Unknown submission outcomes require a separate confirmation before another submission. Drafts are not saved to disk.

While emoji suggestions are open, `Up` and `Down` select an emoji, `Enter` or `Tab` inserts it, and `Esc` dismisses the list. Type at least two characters after `:`, such as `:ro`. Completed shortcodes are replaced outside code. The picker uses the web app's bundled catalog without network requests or persistent usage tracking.

Ghostty's default `Ctrl+Enter` fullscreen binding intercepts submission. Set `keybind = ctrl+enter=unbind` in Ghostty and reload its configuration, or use `Ctrl+S`. The client never changes your terminal configuration.

`r` and `q` reopen a kept draft with its original target instead of starting another. Search shortcuts are not active yet. Later phases add them alongside their features.
