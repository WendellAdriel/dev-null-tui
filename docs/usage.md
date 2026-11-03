# Using the TUI

- [Logs](#logs)
- [Threads](#threads)
- [Notifications](#notifications)
- [Profiles](#profiles)
- [Starred](#starred)
- [Remotes](#remotes)
- [Write a commit](#write-a-commit)
- [Reply, quote, and delete](#reply-quote-and-delete)
- [Vote on polls](#vote-on-polls)
- [View images](#view-images)
- [Current limits](#current-limits)

Run `devnull` to open the workspace. If you have not connected an account, follow [account setup](account.md) first. Press `?` for contextual help or use the [shortcut reference](shortcuts.md).

After an upgrade, the first launch opens **What's new** with a summary of the release. Scroll with `↑`, `↓`, `PageUp`, `PageDown`, `Home`, and `End`, and close it with `Esc`, `Enter`, or `q`. It shows once per version, and never on a fresh install. The last version you saw is stored in `last-seen-version` in the app's configuration directory. Disconnecting keeps it.

The TUI also checks for a new release once a day and offers to update and restart. See [update from the TUI](installation.md#update-from-the-tui).

## Logs

Use `Tab` or `1`, `2`, and `3` to choose Following, Latest, or Trending. Move with `j`, `k`, or the mouse wheel. `PageUp` and `PageDown` move five commits. `g g` or `Home` selects the first commit, and `G` or `End` selects the last loaded commit.

Older pages load near the end of the list, or when you press `n`. Switching logs preserves each log's selection and scroll position.

`Ctrl+R` fetches the first page while keeping the current content visible. Success selects the first commit and scrolls to the top. Failure keeps your position. Trending returns to its first ranked commit, which may not be the newest.

Following and Latest check for new commits once a minute while the workspace is focused. New commits are not inserted until you refresh. Trending has no chronological arrival check.

Commit bodies follow the web's display rules. Links show shortened and open in the browser in terminals that support OSC 8 hyperlinks. GitHub, GitLab, and Codeberg links show the repository and the issue or pull request, and dev/null commit links show the author and hash. Mentions link to the profile on the web. Quotes, emoji shortcodes, hex colors, and the /dev/null wordmark are styled. Code blocks show their file name or language with syntax highlighting. In pastéis mode, the words pastel and pastéis show the pastel icon.

A commit with a link shows the server's link card below the body: the site, a two-line title that opens the page, a two-line description, and the page's picture when the terminal supports graphics. A card the server is still fetching shows **Loading preview…** until the log reloads. In Remotes, the conversation asks for the card again after 1, 2, 3, 5, 8, and 13 seconds, as the web does, then stops showing the loading row. Quoted commits show their link card inside the quote. The TUI has no card picker, so the server uses the first link.

Long commits show the server's excerpt first, like the web. Press `m` or click **Show more** to see the whole text. It stays unfolded for the rest of the session. Opening a thread always shows its commit whole, while its ancestors and replies keep the excerpt.

Click a row or press `Enter` to open its thread. The selected commit shows clickable actions below its stats. `y` copies its web link. Clipboard failures display the link for manual copying.

## Threads

A thread shows up to 20 ancestors, the focused commit, and replies oldest first. Quotes of the focused commit appear among the replies with a **quoted** tag. Deleted commits show **Commit deleted**, with no author or text.

Move with `j` and `k`. Press `Space` to show or hide replies, `n` for another reply page, and `Ctrl+R` to reload. One level of nested replies can appear inline. Open a deeper reply with `Enter` to read its own thread.

Press `s` to star or unstar a commit. When pastéis mode is on and you have some left, `p` awards a pastél to someone else's commit. Counts change after the server confirms. An uncertain pastél outcome disables further awards until you verify your account again.

`Esc` returns through the previous screens with their selection and scroll position preserved.

## Notifications

> [!NOTE]
> Opening Notifications marks every notification category read on the server, including categories hidden by the current filter.

Open the inbox with `g n` or the sidebar. Press `1` through `6` to toggle Replies, Quotes, Stars, Mentions, Follows, and Moderation independently. `HEAD@{last visit}` separates new notifications from older ones.

`Space` expands or collapses a group. `n` loads older members of the selected expanded group, or older notifications otherwise. `Enter` opens the referenced commit. On a single follow, or an expanded follow member, it opens the follower's profile. `u` opens the actor's profile, using the newest actor for a group.

`Ctrl+R` refreshes and selects the first notification after success. A failed refresh keeps your position. `Esc` returns to the log. The unread badge uses a separate count check every 15 seconds while the terminal is focused, which does not mark the inbox read.

## Profiles

Press `u` on a commit or notification to open its author's or actor's profile. `g p` or the sidebar's **Profile** row opens your own. Organization profiles are not available yet, so commits and notifications from organizations have no **Profile** action and `u` does nothing on them.

Profiles show available identity details, counts, activity, streaks, and achievements. A pinned commit leads the Commits tab. `Tab`, `1`, or `2` switches between Commits and Replies. Press `3` or `4`, or click a count, to open Followers or Following.

Press `f` to follow or unfollow. Counts change after confirmation. A refused request leaves the profile unchanged, while an unknown outcome reloads it. In a relationship list, `f` acts on the selected person and `Enter` opens their profile.

Threads and profiles stack above the originating screen. `Esc` returns one screen at a time without losing selection.

## Starred

Press `g s` or choose **Starred** below Profile in the sidebar. **Only you** identifies this private list, ordered by when you starred each commit, newest first.

Every explicit open fetches the first page. Returning with `Esc` from a stacked thread or profile keeps the loaded list and selection. Older pages load near the end or with `n`.

Unstarring with `s` removes the entry after confirmation. Selection moves to the next entry, or the previous one at the end. Confirmed unstars from other screens also remove loaded Starred entries. Starring elsewhere does not insert into an already loaded list. A failed unstar leaves the entry unchanged.

`Ctrl+R` refreshes the first page, keeping your position on failure. `Esc` from Starred returns to the log.

## Remotes

Remotes are direct messages and orgs. **Remotes** appears between Notifications and Profile in the sidebar, with the unread count, only when Remotes is turned on in [Settings](settings.md). Press `g r` or click it to open the inbox. The badge refreshes every 15 seconds while Remotes is closed and the terminal is focused.

The inbox has the web's tabs. **Open** holds your conversations, **Fetched** holds message requests and appears only when you accept requests, with the number waiting, and **Stashed** holds the conversations you put away. Switch tabs with `Tab`, `[`, and `]`, or click them. Press `/` to search by name or @handle, `Enter` to search, and `Esc` to clear the search.

Each row shows the partner's @handle and name, or an org's name, falling back to its other members' first names, with **org** and the member count. The second line is the latest message, prefixed with **you:** or the org member's first name. Your own direct messages show `✓` once pushed and `✓✓` once the partner pulled them. The right side shows the pin, then **N ahead** for unread messages, or the time of the latest message. Pinned conversations sort first.

At 120 columns or wider, the inbox and the open conversation sit side by side. Narrower terminals show one at a time. `Enter` opens the selected conversation, and `Esc` closes it and returns to the inbox. The status line lists the inbox's shortcuts.

Conversations show day markers, org events such as **\* @grace created the org**, your messages on the right, and everyone else's on the left under their name. Images, link cards, replies, reactions in outlined chips below the message, and the typing line follow the web. Pinned org messages are listed at the top. Older messages load at the top of the conversation or with `n`.

Reading a conversation moves your read receipt to its newest message, pings included, and clears its unread count. Like the web, the receipt only moves while the conversation is on screen and scrolled to the newest message. Scrolled-up conversations and conversations covered by a profile or Settings don't mark anything read until you scroll back down. Message mentions link to profiles only when the server marks them as mentions. The open conversation checks for new messages every 1.5 seconds for a minute after activity, then every 4 seconds. The inbox refreshes every 15 seconds, or every 6 while a conversation is open. Polling pauses while the terminal is unfocused and checks at once when it gets focus back. Turning Remotes off in Settings closes the screen.

Press `.` or click **⋯** in the conversation header for its actions, in the web's order:

| Keys | Action |
| --- | --- |
| `u` | View the partner's profile (direct conversations only) |
| `P` | Pin or unpin |
| `S` | Stash, or pop a stashed conversation back into Open |
| `M` | Mute for one hour, until tomorrow, for one week, or forever, or unmute |
| `B` | Block the partner, or leave the org, after a confirmation |

Blocking removes the conversation for both of you. Leaving an org removes it and its history from your inbox. A maintainer would have to add you back.

## Write a commit

Press `c` or click **Commit** to open the composer. It needs at least 48 columns and 18 rows. Typing and paste stay inside the editor and never invoke workspace shortcuts.

Enter your text, then submit with `Ctrl+Enter` or the `Ctrl+S` fallback. `Ctrl+P` switches between preview and editing. Preview does not publish or store the draft. You can also select controls with `Tab` and activate them with `Enter`.

The budget is 4000 Unicode characters after normalization. Prose links count as at most 23 characters, while code counts literally. Each link is limited to 2048 characters, and normalized raw text is limited to 40000 characters. The server validates submissions.

### Images

Press `Ctrl+O` to add an image. A file browser opens inside the composer, starting in your home folder and then in the last folder you used. It lists folders first, then JPEG, PNG, and WebP files. When the terminal supports graphics, a thumbnail of the selected image and its size show beside the list. `Up` and `Down` move, `Enter` opens a folder or picks a file, `Backspace` goes up a folder, typing filters the list, and `Esc` closes it. You can also drag image files into the terminal. A paste made only of existing file paths attaches them, and any other paste stays text.

A commit can carry up to 3 images. Each must be a JPEG, PNG, or WebP file of at most 10 MB and 6000 pixels on a side. The file content decides its type, so a renamed SVG is refused. A refused file shows the reason and keeps your draft. With images attached, the text is optional.

The attachment strip below the editor shows a thumbnail of each image when the terminal supports graphics, with the selected image's number, size, and description below them. Other terminals list the file names instead. `Tab` moves focus to the strip. `Up` and `Down` choose an image, `Enter` edits its description (up to 1000 characters), `Alt+Up` and `Alt+Down` reorder it, and `Delete` removes it.

### Polls

Press `Ctrl+L` to add a poll with two empty options. `Enter` edits the selected option and saves it, and `Esc` stops editing. `Ctrl+N` adds an option, up to 4. `Delete` removes one, keeping at least 2. `Left` and `Right` change the duration between 1 hour, 1 day, 3 days, and 7 days, and `Ctrl+X` removes the poll. Options are limited to 50 characters and must differ ignoring case.

Polls can't go on replies, on quotes, or with images. The strip shows why when a poll is unavailable, and `Ctrl+O` asks you to remove a poll before adding images.

The server preview renders only the text. The composer draws the poll or images below it, with thumbnails when the terminal supports graphics. A preview of images without text never calls the server.

Emoji suggestions appear after at least two shortcode characters, such as `:ro`. Use `Up` and `Down` to choose, `Enter` or `Tab` to insert, and `Esc` to dismiss. Known completed shortcodes are replaced outside code. Suggestions work offline.

### Drafts and failures

`Esc` or **Cancel** protects a draft with text, images, or a poll. **Keep draft** (`Ctrl+K`) closes the composer and retains the draft in memory for `c` to reopen. **Discard draft** (`Ctrl+D`) removes it, and **Back** (`Esc`) resumes editing. Drafts are not saved to disk and are lost when the process ends.

The app sends once while a request is in flight. Validation errors, rejections, and transport failures keep your draft. If the outcome is unknown, check the web before retrying. Another submission requires confirmation because the first commit may already exist.

### Moderation

Most successful submissions are pending until approved. **Submitted for review** means the commit is not public yet. The originating chronological log shows the returned status, while Trending stays ranked and needs a refresh to check for the commit.

First-time posters must submit through the web composer and accept its commit rules before posting from the terminal.

## Reply, quote, and delete

`r` opens a reply and `q` opens a quote. `q` never quits. The composer shows the target. Replies update the thread and the parent's count and newest reply in every list that shows it, without a refresh. They don't enter the top-level logs. Quotes also appear in the originating chronological log. If the target becomes unavailable, the app keeps your draft and reports the error.

Only one draft exists. If you kept one, `c`, `r`, and `q` reopen it with its original target until you finish or discard it.

`D` asks before deleting your own commit. Confirmation defaults to **Keep commit** (`Esc`). Choose **Delete commit** (`Ctrl+D`) to delete it permanently. Open threads show the deleted commit without its author or body.

## Vote on polls

Commits show their polls below the text. An open poll shows its options, the time left, and **Vote to see results** until you vote. A poll awaiting moderation shows **Opens when approved**. Results show a bar and percentage for each option, with a check on yours, and closed polls show **Final results**.

Press `o` or click **Vote** on someone else's open poll. Choose an option with `Up` and `Down`, then press `Enter`. You can also click an option to go straight to the confirmation for it. The confirmation starts on **Cancel** because votes are final. Press `y` to vote or `Esc` to go back. A confirmed vote updates every loaded copy of the commit. A refused vote leaves the poll unchanged. When the outcome is unknown, the poll reloads and the vote is never sent again on its own.

## View images

Click an image or press `v` on a commit with images. The viewer shows its position, such as `1 / 2`, alongside a commit and replies panel. `Left` and `Right` move between images and stop at the ends.

Scroll the panel with `j`, `k`, or the mouse wheel. `n` loads more replies, `p` toggles the panel, and `y` copies the image link. `Esc` returns to the originating entry. Without graphics support, the viewer shows a description and link. There is no zoom or download control.

See [terminal graphics](terminal.md#images-and-avatars) for display requirements.

## Current limits

Sending messages in Remotes, voice notes, and org management are not implemented yet. Organization profiles, posting as an organization, and search are not implemented. Profile README content is web only. The API currently returns one inline reply preview per commit, so lists can show fewer replies than the web app.

Follow, star, and pastél rate-limit toasts show the server's retry delay or UTC retry date when supplied. `Ctrl+R` is the refresh shortcut. Neither `r` nor `R` refreshes the workspace.

Return to the [README](../README.md) for the documentation index.
