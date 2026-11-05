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

The TUI also checks for a new release each time it starts and offers to update and restart. See [update from the TUI](installation.md#update-from-the-tui).

## Logs

Use `Tab` or `1`, `2`, and `3` to choose Following, Latest, or Trending. Move with `j`, `k`, or the mouse wheel. `PageUp` and `PageDown` move five commits. `g g` or `Home` selects the first commit, and `G` or `End` selects the last loaded commit.

Older pages load near the end of the list, or when you press `n`. Switching logs preserves each log's selection and scroll position.

`Ctrl+R` fetches the first page while keeping the current content visible, and also refreshes the notifications and Remotes badges. The log's footer shows the shortcut. Success selects the first commit and scrolls to the top. Failure keeps your position. Trending returns to its first ranked commit, which may not be the newest.

Following and Latest check for new commits every 15 seconds while the terminal is focused, and every 30 seconds while it isn't. New commits go above the ones you're reading, and your row stays where it was. If you're at the top of a focused terminal, they simply appear. Otherwise a **New commits** button with up to three author avatars appears at the top of the log. Click it or scroll up to read them. Your own commits never raise it. Trending has no chronological arrival check.

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

`Ctrl+R` refreshes and selects the first notification after success. A failed refresh keeps your position. `Esc` returns to the log. The unread badge uses a separate count check every 15 seconds while the terminal is focused and every 30 seconds while it isn't. The check does not mark the inbox read.

## Profiles

Press `u` on a commit or notification to open its author's or actor's profile. `g p` or the sidebar's **Profile** row opens your own. On an organization's commit, `u` or **Profile** opens the organization's page instead. It shows the organization's identity, links, followers, and admins, with **Commits** and **Mentions** tabs on `1` and `2`. Press `f` to follow it. A Following list shows the organizations you follow above the people.

Profiles show available identity details, counts, activity, streaks, and achievements. A pinned commit leads the Commits tab. `Tab`, `1`, or `2` switches between Commits and Replies. Press `F` or `O`, or click a count, to open Followers or Following. The counts show those keys beside them.

Press `f` to follow or unfollow. Counts change after confirmation. A refused request leaves the profile unchanged, while an unknown outcome reloads it. In a relationship list, `f` acts on the selected person and `Enter` opens their profile.

Threads and profiles stack above the originating screen. `Esc` returns one screen at a time without losing selection.

## Go live

Press `L` anywhere outside text fields and Remotes to go live. Paste a link to your stream on YouTube or Twitch, then choose how many hours to show it, from 1 to 12, with `Left` and `Right`. `Enter` starts the stream. While you're live, your avatar gets a red ring, your commits and profile show **LIVE**, and `L` asks whether to end the stream. The aside's **Live now** lists up to three people you follow who are streaming. Click one to watch.

## Starred

Press `g s` or choose **Starred** below Profile in the sidebar. **Only you** identifies this private list, ordered by when you starred each commit, newest first.

Every explicit open fetches the first page. Returning with `Esc` from a stacked thread or profile keeps the loaded list and selection. Older pages load near the end or with `n`.

Unstarring with `s` removes the entry after confirmation. Selection moves to the next entry, or the previous one at the end. Confirmed unstars from other screens also remove loaded Starred entries. Starring elsewhere does not insert into an already loaded list. A failed unstar leaves the entry unchanged.

`Ctrl+R` refreshes the first page, keeping your position on failure. `Esc` from Starred returns to the log.

## Remotes

Remotes are direct messages and orgs. **Remotes** appears between Notifications and Profile in the sidebar, with the unread count, only when Remotes is turned on in [Settings](settings.md). Press `g r` or click it to open the inbox. The badge refreshes every 15 seconds while Remotes is closed and the terminal is focused, and every 30 seconds while it isn't.

The inbox has the web's tabs. **Open** holds your conversations, **Fetched** holds message requests and appears only when you accept requests, with the number waiting, and **Stashed** holds the conversations you put away. Switch tabs with `Tab`, `[`, and `]`, or click them. Press `/` to search by name or @handle, `Enter` to search, and `Esc` to clear the search. On the Open tab, a search also lists up to five people under **Start a conversation**. Select one and press `Enter`, or click them, to open a conversation with them.

Each row shows the partner's @handle and name, or an org's name, falling back to its other members' first names, with **org** and the member count. The second line is the latest message, prefixed with **you:** or the org member's first name. Your own direct messages show `✓` once pushed and `✓✓` once the partner pulled them. The right side shows the pin, then **N ahead** for unread messages, or the time of the latest message. Pinned conversations sort first.

At 120 columns or wider, the inbox and the open conversation sit side by side. Narrower terminals show one at a time. `Enter` opens the selected conversation, and `Esc` closes it and returns to the inbox. The status line lists the inbox's shortcuts.

Conversations show day markers, org events such as **\* @grace created the org**, your messages on the right, and everyone else's on the left under their name. Images, link cards, replies, reactions in outlined chips below the message, and the typing line follow the web. Older messages load at the top of the conversation or with `n`.

Reading a conversation moves your read receipt to its newest message, pings included, and clears its unread count. Like the web, the receipt only moves while the conversation is on screen and scrolled to the newest message. Scrolled-up conversations and conversations covered by a profile or Settings don't mark anything read until you scroll back down. Message mentions link to profiles only when the server marks them as mentions. The open conversation checks for new messages every 1.5 seconds for a minute after activity, then every 4 seconds. The inbox refreshes every 15 seconds, or every 6 while a conversation is open. While the terminal is unfocused, polling slows to every 30 seconds and never moves read receipts. It returns to its focused pace when the terminal gets focus back. Turning Remotes off in Settings closes the screen.

### Writing messages

The composer sits at the bottom of the conversation. Press `w` or `Enter`, or click it, to write, and `Enter` to push. `Shift+Enter`, `Alt+Enter`, or `Ctrl+J` start a new line. Messages hold up to 2,000 characters, and the count appears from 1,600. A paste that would go past the limit is refused. Shortcodes such as `:rocket:` become emoji as you type, with suggestions above the editor.

A pushed message shows **◌ staged** until the server stores it. If the server refuses it, the message stays with **✕ rejected** and the reason: select it and press `r` to push it again or `d` to discard it. A retry reuses the message's client id, so the server never stores it twice. Pushing to a stashed conversation moves it back to Open, as the web does.

Press `Ctrl+O` to attach up to three images, or paste or drop image paths into the composer. Attached images show thumbnails when terminal graphics and space are available, or a text list otherwise. Press `Tab` to edit images, `Enter` to add a description, and `Delete` to remove one. Press `Tab` again to return to writing. The partner sees **typing** while you write, at most every 3 seconds, once the conversation has started.

When you can't push to the partner, the web's access bar takes the composer's place: a request waiting for your merge, your own request waiting for theirs, a partner who doesn't follow you, or a partner who isn't taking remotes. On a request, press `m` or click **Merge** to accept it and start writing, or `d` or click **Drop** to remove it from Fetched after a confirmation. The sender isn't told either way.

Unsent text is saved as a draft for each conversation and comes back when you open it again, with its reply target. Drafts are the only message text the TUI writes to disk. They live in an owner-only file under the config directory, hold the reply target as an id only, and are deleted when you push, disconnect, delete your account, or the device is revoked.

### Replies, reactions, and pings

`j` and `k` move a cursor through the messages, marked with a bar on the left. Past the newest message the cursor follows new messages again. The message keys act on the selected message, or on the newest one when nothing is selected:

- `r` replies. The composer shows **Replying to** with the message, and `Esc` cancels the reply.
- `1`, `2`, and `3` toggle the three quick reactions. They start as 😂 ✅ ❤️ and slowly follow the emoji you use most, like the web.
- `e` opens the reaction row. `+` opens the picker, where typing searches the emoji. Clicking a reaction chip toggles it too.
- `p` pings the partner, or everyone in an org. A conversation needs a first message before it can be pinged.

Reactions show at once and go back if the server refuses them. You can have up to 12 reactions on a message.

### Voice notes

A voice note shows its length and its waveform. Select it and press `Space` to play it, or click its ▶. The first play downloads the note, and the waveform fills as it plays. A note rewinds when it ends, and `Space` on a note paused at its end plays it from the start. Only one note plays at a time, and leaving the conversation stops it. While a voice note is selected, the status line lists its keys. While a note plays or is paused:

- `Left` and `Right` seek 2 seconds, and clicking the waveform seeks to that point.
- `x` switches between 1×, 1.5×, and 2×. The voice keeps its pitch, and the clock counts in the chosen speed, so 0:14 at 2× reads 0:07.
- `-` and `+` change the volume. The volume is saved in [`tui.json`](settings.md#tui).

If a download fails, the note shows **Retry**, and `Space` tries again. Audio stays in memory and is never written to disk.

To record, press `Ctrl+T` in a conversation, from the messages or the composer. The composer shows "waiting for the microphone" until the microphone sends sound, which takes about a second on a Bluetooth headset. Then a meter and a clock run up to one minute, and the clock turns yellow for the last 10 seconds. If no sound arrives within 10 seconds, recording stops and asks whether the microphone is muted. `Enter` stops and `Esc` discards. A recording under half a second is dropped, as on the web. Recording over typed text or images asks first, because a voice note goes alone. It can still reply to a message. After stopping, `Space` plays the recording back, `Enter` pushes it, and `d` discards it. A rejected voice note can be retried with `r`, which sends the same recording again under the same client id.

Recordings aren't saved with drafts, so leaving the conversation discards one that wasn't pushed. The TUI uploads 16 kHz mono WAV, about 1.9 MB a minute.

Sound needs a sound server or device:

- **Linux:** playback and recording go through PulseAudio, or PipeWire's PulseAudio service, with nothing else to install.
- **macOS and Windows:** playback uses the system's audio. Recording needs `ffmpeg` or `sox` on your `PATH`. With `ffmpeg` it records from the first microphone `ffmpeg` lists, and with `sox` only from the system's default input.

Without sound output, a note shows **Open on the web**, and `Space` copies the conversation's web link. Without a recorder, `Ctrl+T` says so.

### Conversation actions

Press `.` or click **⋯** in the conversation header for its actions, in the web's order:

| Keys | Action |
| --- | --- |
| `u` | View the partner's profile (direct conversations only) |
| `P` | Pin or unpin |
| `S` | Stash, or pop a stashed conversation back into Open |
| `M` | Mute for one hour, until tomorrow, for one week, or forever, or unmute |
| `B` | Block the partner, or leave the org, after a confirmation |

Blocking removes the conversation for both of you. Leaving an org removes it and its history from your inbox. A maintainer would have to add you back.

### New remotes and orgs

From the inbox, press `N` for **New remote** or `O` for **New org**. Type a name or @handle, and results appear after a short pause. People who can't be added show why, such as "only takes remotes from people they follow", and can't be chosen.

- **New remote:** `Down` moves into the results, and `Enter` opens the conversation with the selected person, reusing an existing one.
- **New org:** name the org (optional, up to 60 characters), then `Tab` to the search. `Space` or `Enter` picks or unpicks people, up to 99. `Ctrl+S` creates the org and opens it. You maintain it, and everyone you add contributes.

If the server refuses, the dialog keeps your picks and shows its reason.

### Members

In an org, press `I` or click the member count in the header to see its members and their roles. Press `u` to open the highlighted member's profile, and `Esc` there returns to the list. Anyone can leave from their own row. Maintainers can also:

- Press `r` to rename the org. An empty name clears it, and the org falls back to its members' first names.
- Press `a` to add people. They join as contributors. Turn on **Share history** to let them read what was pushed before they joined. An org holds up to 100 people.
- Press `Enter` on a member to make them a maintainer or a contributor, or to remove them after a confirmation. An org always keeps at least one maintainer.

### Pinned messages

An org's pinned messages show as one row under the conversation header: how many there are and the latest one. Press `L` or click the row to list them all. `Enter` jumps to a message. When it is further back, the list stays open while older messages load, and `Esc` cancels.

Maintainers press `!` on a selected message to pin or unpin it for the whole org, and `!` in the list unpins. Everyone in the org sees pins, including people added without Share history, so pinning asks first. Check **Don't show this again** to skip the question. That choice is saved in [`tui.json`](settings.md#tui). An org can pin up to 25 messages.

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

### Post as an organization

Press `Ctrl+A` in the composer to choose who the commit is from. The list shows you and the organizations you're a member of. Pick one with `j`, `k`, and `Enter`, or click it. On a server that doesn't list your organizations, type the organization's handle instead. The TUI checks its page, and only members can post as it. The composer title shows the chosen organization.

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

Search is not implemented. Organization followers lists are web only. Profile README content is web only. The API currently returns one inline reply preview per commit, so lists can show fewer replies than the web app.

Follow, star, and pastél rate-limit toasts show the server's retry delay or UTC retry date when supplied. `Ctrl+R` is the refresh shortcut. Neither `r` nor `R` refreshes the workspace.

Return to the [README](../README.md) for the documentation index.
