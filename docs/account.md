# Account setup and security

- [Connect your account](#connect-your-account)
- [Browser return problems](#browser-return-problems)
- [Manage your connection](#manage-your-connection)
- [Device key storage](#device-key-storage)
- [Disconnect and revoke access](#disconnect-and-revoke-access)

The app connects through browser approval of a device key. Login needs at least 60 columns and 18 rows.

## Connect your account

> [!WARNING]
> The device key is stored unencrypted. Anyone who obtains it can act as your account until you revoke the device. Do not share the key, authorization callback, or backups containing the key.

Start `devnull`, then connect:

1. Press `Enter` to begin.
2. Sign in through GitHub in your browser if needed. Compare the device fingerprint with the terminal, then approve the device.
3. Return to the terminal. If prompted, review **Terms** (`t`) and **Privacy** (`p`), then choose **Agree** (`a`). The workspace opens after verification and any required consent.

Consent is never automatic. `Enter` initially opens Terms, not Agree. If the terms version changes or acceptance fails, press `v` to verify again.

## Browser return problems

The app registers a temporary `devnull://` handler when no existing owner is found. It leaves an existing handler untouched.

If the browser does not open, press `u` to display the authorization URL and open it yourself. While waiting for approval, `b` reopens the browser.

If automatic return fails, press `p` and paste the complete `devnull://auth/callback?...` link. Bare codes are not accepted. Obtaining this link may require inspecting the approval response's `Location` header in your browser's developer tools. Do not share or save the header.

Press `Esc` to cancel an attempt or `r` to retry after an error. Attempts expire after ten minutes. Failed enrollment is never retried automatically.

Canceling before enrollment removes the newly generated key. Once enrollment starts, the key is retained because a network error can leave approval uncertain. Verify the saved connection before starting another approval.

## Manage your connection

Press `a` from the workspace to open account controls. Use `Tab` and `Shift+Tab` to select an action, then `Enter` to activate it. `Esc` returns to the workspace when connected.

**Device details** (`i`) shows the fingerprint. Browser approval always shows it. A successful verification displays "Connection verified just now" for five seconds.

Later launches verify the saved key with its original server. A revoked device requires reconnection. A saved connection for a different server is refused before the app sends a signed request.

## Device key storage

The key is stored in `connection.json`, inside the `dev-null-cli` subdirectory of your OS configuration directory. Linux and macOS use owner-only directory and file permissions. Windows uses a protected ACL for the current user.

There is no keyring service or passphrase. Temporary handler records allow cleanup after an interrupted login. `Ctrl+C` exits without deleting a saved key.

## Disconnect and revoke access

Press `d` in account controls to open the disconnection confirmation. It defaults to keeping the key. Select **Delete local key**, then press `Enter`, to remove the local connection.

Deleting the local key does not revoke the device on the server. Revoke it separately in [Settings > Devices](https://dev-null.sh/settings/devices).

See [account shortcuts](shortcuts.md#account-connection) for the complete control reference, or return to the [README](../README.md).
