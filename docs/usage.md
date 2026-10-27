# Using the TUI

- [Logs](#logs)
- [Threads](#threads)
- [Notifications](#notifications)
- [Profiles](#profiles)
- [Starred](#starred)
- [Write a commit](#write-a-commit)
- [Reply, quote, and delete](#reply-quote-and-delete)
- [View images](#view-images)
- [Current limits](#current-limits)

Run `devnull` to open the workspace. If you have not connected an account, follow [account setup](account.md) first. Press `?` for contextual help or use the [shortcut reference](shortcuts.md).

## Logs

Use `Tab` or `1`, `2`, and `3` to choose Following, Latest, or Trending. Move with `j`, `k`, or the mouse wheel. `PageUp` and `PageDown` move five commits. `g g` or `Home` selects the first commit, and `G` or `End` selects the last loaded commit.

Older pages load near the end of the list, or when you press `n`. Switching logs preserves each log's selection and scroll position.

`Ctrl+R` fetches the first page while keeping the current content visible. Success selects the first commit and scrolls to the top. Failure keeps your position. Trending returns to its first ranked commit, which may not be the newest.

Following and Latest check for new commits once a minute while the workspace is focused. New commits are not inserted until you refresh. Trending has no chronological arrival check.

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

`Ctrl+R` refreshes and selects the first notification after success. A failed refresh keeps your position. `Esc` returns to the log. The unread badge uses a separate count check that does not mark the inbox read.

## Profiles

Press `u` on a commit or notification to open its author's or actor's profile. `g p` or the sidebar's **Profile** row opens your own.

Profiles show available identity details, counts, activity, streaks, and achievements. A pinned commit leads the Commits tab. `Tab`, `1`, or `2` switches between Commits and Replies. Press `3` or `4`, or click a count, to open Followers or Following.

Press `f` to follow or unfollow. Counts change after confirmation. A refused request leaves the profile unchanged, while an unknown outcome reloads it. In a relationship list, `f` acts on the selected person and `Enter` opens their profile.

Threads and profiles stack above the originating screen. `Esc` returns one screen at a time without losing selection.

## Starred

Press `g s` or choose **Starred** below Profile in the sidebar. **Only you** identifies this private list, ordered by when you starred each commit, newest first.

Every explicit open fetches the first page. Returning with `Esc` from a stacked thread or profile keeps the loaded list and selection. Older pages load near the end or with `n`.

Unstarring with `s` removes the entry after confirmation. Selection moves to the next entry, or the previous one at the end. Confirmed unstars from other screens also remove loaded Starred entries. Starring elsewhere does not insert into an already loaded list. A failed unstar leaves the entry unchanged.

`Ctrl+R` refreshes the first page, keeping your position on failure. `Esc` from Starred returns to the log.

## Write a commit

Press `c` or click **Commit** to open the composer. It needs at least 48 columns and 18 rows. Typing and paste stay inside the editor and never invoke workspace shortcuts.

Enter your text, then submit with `Ctrl+Enter` or the `Ctrl+S` fallback. `Ctrl+P` switches between preview and editing. Preview does not publish or store the draft. You can also select controls with `Tab` and activate them with `Enter`.

The budget is 4000 Unicode characters after normalization. Prose links count as at most 23 characters, while code counts literally. Each link is limited to 2048 characters, and normalized raw text is limited to 40000 characters. The server validates submissions.

Emoji suggestions appear after at least two shortcode characters, such as `:ro`. Use `Up` and `Down` to choose, `Enter` or `Tab` to insert, and `Esc` to dismiss. Known completed shortcodes are replaced outside code. Suggestions work offline.

### Drafts and failures

`Esc` or **Cancel** protects a nonempty draft. **Keep draft** (`Ctrl+K`) closes the composer and retains the text in memory for `c` to reopen. **Discard draft** (`Ctrl+D`) removes it, and **Back** (`Esc`) resumes editing. Drafts are not saved to disk and are lost when the process ends.

The app sends once while a request is in flight. Validation errors, rejections, and transport failures keep your draft. If the outcome is unknown, check the web before retrying. Another submission requires confirmation because the first commit may already exist.

### Moderation

Most successful submissions are pending until approved. **Submitted for review** means the commit is not public yet. The originating chronological log shows the returned status, while Trending stays ranked and needs a refresh to check for the commit.

First-time posters must submit through the web composer and accept its commit rules before posting from the terminal.

## Reply, quote, and delete

`r` opens a reply and `q` opens a quote. `q` never quits. The composer shows the target. Replies update the thread and its count without entering the top-level logs. Quotes also appear in the originating chronological log. If the target becomes unavailable, the app keeps your draft and reports the error.

Only one draft exists. If you kept one, `c`, `r`, and `q` reopen it with its original target until you finish or discard it.

`D` asks before deleting your own commit. Confirmation defaults to **Keep commit** (`Esc`). Choose **Delete commit** (`Ctrl+D`) to delete it permanently. Open threads show the deleted commit without its author or body.

## View images

Click an image or press `v` on a commit with images. The viewer shows its position, such as `1 / 2`, alongside a commit and replies panel. `Left` and `Right` move between images and stop at the ends.

Scroll the panel with `j`, `k`, or the mouse wheel. `n` loads more replies, `p` toggles the panel, and `y` copies the image link. `Esc` returns to the originating entry. Without graphics support, the viewer shows a description and link. There is no zoom or download control.

See [terminal graphics](terminal.md#images-and-avatars) for display requirements.

## Current limits

Settings, Remotes, polls, image uploads, and search are not implemented. Profile README content is web only. The API currently returns one inline reply preview per commit, so lists can show fewer replies than the web app.

Follow, star, and pastél rate-limit toasts show the server's retry delay or UTC retry date when supplied. `Ctrl+R` is the refresh shortcut. Neither `r` nor `R` refreshes the workspace.

Return to the [README](../README.md) for the documentation index.
