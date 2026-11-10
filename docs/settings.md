# Settings and safety

Settings opens with `g ,` or the sidebar's **Settings** row. It uses the existing signed device connection. Account changes apply after the server confirms them.

Settings has six tabs: Profile, Region, Appearance, Safety, Devices, and TUI. Use Left and Right or click a tab to switch.

GitHub sync, pastéis, Remotes, and automatic translation changes show progress and results through the shared timed toasts. Failed writes retain recovery guidance in Settings after the toast expires.

## Profile

The Profile tab shows your GitHub identity, available profile fields, and last sync time. **Sync from GitHub** refreshes the mirrored profile. **View profile** opens your profile in the TUI.

**Color** offers Automatic, six palette lanes, and a custom lowercase `#rrggbb` color. Custom colors use the same value in both themes. **Pastéis mode** controls giving actions and counters without erasing pastéis already given or received.

The color picker starts on your saved choice. Click a color or use the arrow keys, then choose **Save color** or press `Ctrl+S`. Clicking the custom field selects Custom and focuses its input. Within that field, Left and Right move the caret; Up and Down still change the selected color.

**Remotes** turns messaging availability on or off without deleting conversations. While enabled, the server's capabilities determine whether **Who can push** includes everyone. **Share pull receipts** controls read receipts. While Remotes is on, the sidebar shows [Remotes](usage.md#remotes).

## Region

The Region tab loads your timezone and translation settings when you open it.

**Timezone** takes a name such as `Europe/Lisbon`. Others see your timezone when they hover your commits. Press `Enter` to save. Leave the field empty to unset it, which follows this device without sharing it. The server rejects names it does not recognize. While a timezone is set, **Show local time on profile** shows the current time in your timezone to anyone who opens your profile. Unsetting the timezone also turns this off.

**Translation language** opens a searchable list of the server's languages. Type part of an English or native name, choose with Up and Down, then press `Enter` or `Ctrl+S`. **Follow device language** clears the choice. **Automatically show translations** controls whether translated commits appear first. This client does not translate commits yet.

## Appearance

The Appearance tab sets the event theme and whether Who to follow shows. Its event theme shows dark or light colors to match your theme. Pick the theme itself in the TUI tab, or press `F2` in the workspace, outside popups and the composer. Both are saved in `tui.json`, and `--theme` overrides the theme for one run.

## Safety

The Safety tab shows **Muted**, followed by **Blocked**, in one scrollable view. Both lists load when you open the tab. Each section has its own refresh control, loading and error messages, and **Load more** pagination. People are ordered newest first within each list. Unmute and unblock require confirmation.

Mute is silent and preserves follows. Muted people's commits remain reachable through their profile and direct links, while discovery and reply lists hide them. Block ends follows in both directions and hides content everywhere. Unblocking does not restore follows. Confirmed safety changes remove affected cached content and invalidate pending reads. Threads rooted in blocked commits and blocked profiles close. Unrelated log entries retain selection where possible. Removed content is not restored from an old cache after unmuting or unblocking; refresh the relevant screen to fetch current content.

`M` and `B` open mute and block confirmation on another person's profile. `!` reports another person's published commit. Reports offer the server's four reasons and up to 500 characters of optional details. Reporting the same commit again retains the first report on the server.

Uncertain writes are never replayed automatically. Settings blocks further writes until a successful account refresh. For an uncertain safety change, affected content stays hidden locally until a confirmed reversal or a new session. Check the web before retrying an uncertain report or deletion.

## Devices

The Devices tab lists every device approved to sign requests as you, newest approval first. Each row shows the device and app name, approval and last-used times, and its key fingerprint. The device this client signs with is marked **This device**.

Select a device and press Enter to revoke it. The dialog starts on Cancel. Revoking another device signs the app on it out and removes it from the list. Revoking this device signs this client out and removes its saved key after the server confirms with HTTP 204. You then need to approve the client again on the web.

## TUI

The TUI tab holds preferences for this client only. They are saved in `tui.json` in the app's configuration directory, take effect without the server, and survive disconnecting.

**Check for updates** is on by default. When on, devnull looks for a new release each time it starts and offers to install it. See [update from the TUI](installation.md#update-from-the-tui). The same file remembers any version you skipped, the voice note volume set with `-` and `+` in a conversation, and whether to skip the warning before pinning an org message.

## Account deletion

**Delete account** permanently removes your /dev/null account and its data. Threads you started and their replies are removed. Some replies to other people's threads remain as anonymous tombstones. Your GitHub account is unchanged.

The dialog starts on Cancel and requires your exact handle before confirmation. Only a confirmed HTTP 204 response counts as deletion. The client then removes its saved device key and clears account content and drafts. If local key removal fails, the client reports the failure and offers local disconnection. A network error does not count as successful deletion and does not erase the saved key.

## API limits

Data export has no route in the current signed API and is not exposed here. Use the web to export your data. Browser-only appearance preferences are not copied into account settings. Existing account connection and explicit terms-consent controls remain separate.
