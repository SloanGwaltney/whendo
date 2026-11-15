# Twitch Login

- **Status:** Draft
- **Last updated:** 2026-11-15

How Whendo logs users in to Twitch and keeps them logged in. Login is part of the engine, not the Wails layer. Wails only displays the code and verification link and opens the browser.

## Twitch Application

Whendo's Twitch application must use the **Public** client type. The client ID is app configuration and lives with the code, not in this document.

Developers can log in through their own Twitch application by setting the `WHENDO_TWITCH_CLIENT_ID` environment variable to its client ID before starting Whendo, for example `$env:WHENDO_TWITCH_CLIENT_ID = "<client ID>"; wails3 dev` in PowerShell. Their application must also use the **Public** client type.

## Flow: Device Code Grant

Whendo uses Twitch's Device Code Grant flow with a **Public** client type. Twitch's authentication overview recommends this flow for apps that have limited input or lack a suitable browser, and names Electron applications as an example. A public client has no client secret. That matters because anything shipped inside a desktop app can be extracted, and Whendo has no server of its own that could keep a secret.

1. **Request a device code:** `POST https://id.twitch.tv/oauth2/device` with `client_id` and `scopes`. The response contains `device_code`, `user_code`, `verification_uri`, `expires_in` (1800 seconds in Twitch's example) and `interval`.
2. **Show the user** the `user_code` and `verification_uri`, and open the link in their browser.
3. **Poll for the token:** `POST https://id.twitch.tv/oauth2/token` with `client_id`, `scopes`, `device_code` and `grant_type=urn:ietf:params:oauth:grant-type:device_code`.
   - Twitch returns `authorization_pending` until the user approves.
   - Poll no faster than `interval`.
   - Stop when the device code expires and offer to start over.
4. **Store** the access token and refresh token.

Device codes can be used only once.

## Scopes

Request every scope the MVP needs at first login. Adding scopes later forces the user to log in again.

| Scope | Used for |
|---|---|
| `channel:read:redemptions` | Receiving redeem events and listing redeems |
| `channel:manage:redemptions` | Creating and editing Whendo redeems, refunding and fulfilling redemptions |
| `user:read:chat` | Receiving chat messages, for the chat message trigger and the "wait for a chat message" step |
| `user:write:chat` | Posting chat messages as the streamer |
| `moderator:read:followers` | Receiving follow events |

Confirm each one against Twitch's API reference when implementing the endpoint or EventSub subscription that needs it.

## Token Lifetime and Refresh

These rules apply to tokens from a public client:
- **Access tokens** last about **4 hours**.
- **Refresh tokens can be used only once.** Each refresh returns a new refresh token, which must be stored immediately and replace the old one. Reusing an old refresh token fails with `Invalid refresh token`.
- **Refresh tokens expire after 30 days if unused.** After that, the user has to log in again with the device code flow. A streamer who doesn't open Whendo for a month will have to log in again, and the UI should present that as normal, not as an error.
- **Refreshing must happen in one place.** Because each refresh token works only once, two refreshes running at the same time would invalidate each other. The engine must make sure only one refresh runs at a time and share the result with everything else that needs the token.
- **Refreshing can fail** with HTTP 400 or 401 if the user changed their password or disconnected the app in Twitch. Treat it as a logout and send the user back to the login prompt.

## Hourly Validation

Twitch requires every app that keeps an OAuth session to validate its token **at startup and every hour after that**.

- **Request:** `GET https://id.twitch.tv/oauth2/validate` with the header `Authorization: OAuth <access token>`.
- **200:** the token is valid. The response includes `client_id`, `scopes`, `expires_in`, `login` and `user_id`.
- **401:** the token is invalid. Try a refresh. If that also fails, treat it as a logout.

Build this immediately after proving we can get a token.

## Token Storage

Store tokens in the operating system's credential store, never in plain files:
- Windows Credential Manager.
- The macOS Keychain.
- The Secret Service on Linux.

**Decision:** use [`github.com/zalando/go-keyring`](https://github.com/zalando/go-keyring). See [ADR 0003](../adr/0003-go-keyring-for-credential-storage.md) for the options we considered and why.

## Sources
- [Twitch: Authentication overview (flow table)](https://dev.twitch.tv/docs/authentication/)
- [Twitch: Getting OAuth access tokens (Device Code Grant Flow)](https://dev.twitch.tv/docs/authentication/getting-tokens-oauth/)
- [Twitch: Refreshing access tokens](https://dev.twitch.tv/docs/authentication/refresh-tokens/)
- [Twitch: Validating tokens](https://dev.twitch.tv/docs/authentication/validate-tokens/)
