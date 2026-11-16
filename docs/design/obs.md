# OBS

- **Status:** Later, not in the MVP. The MVP supports Streamlabs Desktop instead; see [Streamlabs Desktop](streamlabs-desktop.md).
- **Last updated:** 2026-11-16

How Whendo will connect to OBS. The steps that show and hide sources, choosing the program in Settings and the shared parts of the connection status are the same for every program; see [Streaming Software](streaming-software.md). Like Twitch, the connection is part of the engine, not the Wails layer.

## How Whendo Talks to OBS

OBS has had a WebSocket server built in since version 28. Whendo connects to it as a client, using version 5 of the [OBS WebSocket protocol](https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md) with JSON messages.

- **Running alongside other tools.** OBS accepts many clients at once, so Whendo and Streamer.bot can both be connected. Whendo never changes OBS's WebSocket settings.
- **Only what steps need.** The protocol can control almost everything in OBS. Whendo only reads scenes and their sources, and shows and hides sources, through the [streaming software interface](streaming-software.md#the-interface).

## Setting Up

Choosing OBS in Settings' Streaming software section shows the three values OBS's **Tools → WebSocket Server Settings → Show Connect Info** window shows, in the same order, so the streamer can copy them across one by one:

| Setting | Default | Stored in |
|---|---|---|
| Server IP | `127.0.0.1` | Settings file |
| Server port | `4455`, OBS's default | Settings file |
| Server password | Empty | Credential store; see [ADR 0003](../adr/0003-go-keyring-for-credential-storage.md) |

- **Instructions on the page:** OBS's WebSocket server is off by default, so the section says how to turn it on: in OBS, open Tools → WebSocket Server Settings, tick "Enable WebSocket server", then click "Show Connect Info." It doesn't assume the streamer knows what a WebSocket is.
- **Saving connects.** Saving the values closes any current connection and connects with the new ones.
- **An empty password is allowed,** for streamers who turned off authentication in OBS.
- **Choosing None** deletes the password and stops connecting.
- **Two-PC setups** work by entering the streaming PC's IP. Whendo doesn't scan the network for OBS.

## Connection

### Connecting
1. Open `ws://<IP>:<port>` with the `obswebsocket.json` subprotocol.
2. Wait for **Hello**. If it asks for authentication, work out the answer from the password, the salt and the challenge as the protocol describes: base64 of SHA-256 of password plus salt, then base64 of SHA-256 of that plus challenge. The password itself is never sent.
3. Send **Identify** with RPC version 1 and no event subscriptions; see [Events](#events).
4. Wait for **Identified**, then check OBS is new enough; see [Versions](#versions).

Connecting, from opening to Identified, has 10 seconds to finish before it counts as failed.

### Retrying
- **OBS often isn't running yet.** Whendo usually starts with Windows or before OBS does, so failing to connect is normal and not an error. Whendo tries again every 5 seconds for as long as OBS is set up.
- **A steady 5 seconds rather than backing off.** OBS is usually on the same PC, so a failed attempt costs almost nothing, and a streamer who opens OBS expects Whendo to notice in seconds, not after a backoff has grown to a minute. Twitch's retries back off because Twitch is a shared server; OBS isn't.
- **A wrong password stops retrying.** OBS closes with code 4009, and the status says the password is wrong until the streamer saves the OBS settings again. Retrying can't fix it.
- **OBS closing** ends the connection, and Whendo goes back to retrying every 5 seconds.

### Close Codes

| Code | Meaning | Whendo's response |
|---|---|---|
| 4009 | Authentication failed | Stop retrying; status "wrong password" |
| 4010 | Unsupported RPC version | Stop retrying; status "OBS needs updating" |
| 4002–4008, 4012 | Whendo sent something OBS didn't understand | Log an error, since it's a bug in Whendo, then retry |
| Anything else | OBS closed or the network dropped | Retry |

### Requests
- **Each request has its own ID,** and its response is matched by that ID, so steps in different automations can send requests at the same time over the one connection.
- **10 seconds to answer.** A request that gets no answer fails rather than holding up its run. OBS normally answers in milliseconds, so this only catches a hung OBS.
- **The connection stays read at all times,** so pings are answered and responses are delivered, as for Twitch.
- **Losing the connection fails waiting requests** straight away, with "OBS disconnected."

### Events
Whendo subscribes to no OBS events in the MVP. The step and the designer ask OBS for what they need when they need it, so there's nothing to keep in sync. Events come later with OBS triggers, such as "when the scene changes."

### Versions
Whendo identifies scenes and sources by their UUIDs; see [Which Source](streaming-software.md#which-source). UUIDs in scene and scene item requests came with obs-websocket 5.5, which shipped with OBS 30.2 (unverified, see [What Is Still Unverified](#what-is-still-unverified)). After connecting, Whendo checks `GetVersion`'s `obsWebSocketVersion`, and an older OBS gets the status "OBS needs updating, to version 30.2 or later" and isn't used. OBS 30.2 came out in mid-2024, so asking streamers to update is reasonable.

## Connection Status

On top of the [shared states](streaming-software.md#connection-status), connected and waiting, OBS adds:

| State | When | Shown as |
|---|---|---|
| Wrong password | Closed with 4009 | "OBS: wrong password," linking to Settings |
| Needs updating | OBS is too old | "OBS: needs updating," with the version needed on hover or click |

Waiting's hint is "Is OBS open, with its WebSocket server turned on?"

## Finding Items
- **The scenes** come from `GetSceneList`, and each scene's items from `GetSceneItemList`, by scene UUID. Items in groups come from `GetGroupSceneItemList`, and are shown and hidden in the group rather than the scene.
- **Showing and hiding** is `SetSceneItemEnabled` with the scene's UUID and the item's ID.
- **Request failures** are OBS's status code and comment, recorded as the step's failure reason; a scene or source that's gone is code 600, resource not found.

## Packages
- `engine/obswebsocket`: the protocol client and the connection that retries, with its status, and the [streaming software interface](streaming-software.md#the-interface) built on them.

## Testing
- **The client:** Go tests run it against a fake OBS WebSocket server in the test process. They cover authentication, a wrong password, close codes, retrying, requests answered out of order, timeouts, and losing the connection with requests waiting.
- **Finding items:** against the same fake server, covering groups.
- **Real OBS:** the maintainer checks against OBS on Windows, including Head Pets from end to end.

## Dependencies
None new. The client is a few hundred lines over `github.com/coder/websocket`, which Whendo already uses for EventSub; see [ADR 0004](../adr/0004-coder-websocket-for-eventsub.md). Existing Go clients for OBS, such as `goobs`, wrap the whole protocol, almost none of which Whendo uses, and would bring in their own dependencies.

## First Slice
1. The protocol client: connecting, authentication, requests, close codes. Tested against the fake server.
2. The connection that retries, with its status, and the version check.
3. OBS in Settings' Streaming software section, with the password in the credential store.
4. The streaming software interface on top of the client, so the existing show and hide steps work with OBS.

## What Is Still Unverified
- **Which OBS version first has UUIDs** in scene item requests and responses: obs-websocket 5.5 in OBS 30.2 is the working assumption. Check it before writing the version check.
- **The fields of a scene item** in `GetSceneItemList`'s response, in particular `sourceUuid` and `isGroup`. The protocol document doesn't list them; check against real OBS.

## Sources
- [OBS WebSocket protocol](https://github.com/obsproject/obs-websocket/blob/master/docs/generated/protocol.md)
