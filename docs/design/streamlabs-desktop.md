# Streamlabs Desktop

- **Status:** Draft
- **Last updated:** 2026-11-16

How Whendo connects to Streamlabs Desktop, the streaming software the MVP supports. For what Whendo does with the connection, the show and hide steps, and how the streamer chooses Streamlabs, see [Streaming Software](streaming-software.md). Like Twitch, the connection is part of the engine, not the Wails layer.

## How Streamlabs Desktop Can Be Reached

Streamlabs Desktop is built on OBS but doesn't include OBS's WebSocket server, and it doesn't load OBS plugins, so Whendo can't use the [OBS](obs.md) connection. It has its own API instead, the [Streamlabs Desktop API](https://stream-labs.github.io/streamlabs-desktop-api-docs/docs/index.html): [JSON-RPC 2.0](https://www.jsonrpc.org/specification) requests that call methods on Streamlabs' services and objects. It accepts them three ways ([`tcp-server.ts`](https://github.com/streamlabs/desktop/blob/master/app/services/api/tcp-server/tcp-server.ts)):

| Way in | Address | Login | On by default |
|---|---|---|---|
| Plain TCP | `127.0.0.1:28194`, this PC only | None; programs on the same PC are trusted | Yes, always |
| Named pipe, Windows only | `\\.\pipe\slobs` | None | Yes |
| WebSocket, through SockJS | Port 59650, optionally from other PCs | A token, copied from Settings → Remote Control | No |

**Whendo uses plain TCP.** The streamer has nothing to set up: no token to find and copy, nothing to turn on. It needs only Go's standard `net` package, and it works on Windows and macOS, the two platforms Streamlabs Desktop runs on.

### Why We Trust the TCP Connection
Streamlabs' published documentation describes the named pipe and the WebSocket, but not the TCP connection. We rely on it anyway:

- **It's deliberate.** The code skips the login only for TCP clients on the same PC, with a comment explaining why WebSocket clients still need one.
- **It's been there since at least 2019.**
- **Streamlabs chose to keep it.** In October 2026, after a security report, Streamlabs replaced it with owner-only sockets ([#6287](https://github.com/streamlabs/desktop/pull/6287)), then put it back nine days later as "an intentional design decision" ([#6340](https://github.com/streamlabs/desktop/pull/6340)). The official Elgato Stream Deck plugin depends on it, partly because the named pipe can't be reached when Streamlabs runs as administrator and the other program doesn't.

**If it's removed,** the change in #6287 shows the replacement: the named pipe on Windows, limited to the same user, and a Unix socket on macOS. Neither needs a token, so the streamer's setup wouldn't change. Only the code that opens the connection would. Keeping it to one small part of `engine/streamlabs` keeps that swap cheap. Watch Streamlabs Desktop's releases for the change.

### What the TCP Connection Can't Do
- **Same PC only.** Two-PC setups would need the WebSocket and a token. That's left out of the MVP; most Streamlabs streamers use one PC.

## Connecting
1. Open a TCP connection to `127.0.0.1:28194`.
2. Ask for the active scene's ID with `activeSceneId`. An answer means the connection works. Streamlabs sends nothing when a client connects, so this is how Whendo knows it's talking to Streamlabs and not some other program on the port. It's a cheap request, unlike `getScenes`; see [Rate Limit](#rate-limit).

Connecting has 10 seconds to finish before it counts as failed.

- **Streamlabs accepts connections before it answers.** It opens the port early while starting up, but sends no answers until it has finished loading. So while Streamlabs starts, connecting times out, and Whendo retries as usual.

### Retrying
- **Streamlabs often isn't running yet,** so failing to connect is normal and not an error. Whendo tries again every 5 seconds for as long as Streamlabs Desktop is the chosen program.
- **A steady 5 seconds rather than backing off.** Streamlabs is on the same PC, so a failed attempt costs almost nothing, and a streamer who opens Streamlabs expects Whendo to notice in seconds. Twitch's retries back off because Twitch is a shared server; Streamlabs isn't.
- **Streamlabs closing** ends the connection, and Whendo goes back to retrying.

## Requests

Each request and each answer is one line of JSON ending in a newline. A request names the object it calls in `params.resource`, such as `ScenesService` or a resource ID Streamlabs returned earlier, and passes arguments in `params.args`:

```json
{"jsonrpc": "2.0", "id": 1, "method": "getScenes", "params": {"resource": "ScenesService", "args": []}}
```

- **One request at a time.** Streamlabs reads whatever arrives as text, splits it on newlines and parses each piece. If two requests arrived in one read with the second cut short, the cut piece would fail to parse, and Streamlabs disconnects any client that sends something it can't parse, as a security measure. Sending the next request only after the last one is answered avoids that. Requests take milliseconds, so waiting costs nothing noticeable.
- **Answers are read line by line,** since one read from the connection can hold part of a line or several lines.
- **10 seconds to answer.** A request that gets no answer fails rather than holding up its run.
- **"API server is busy":** Streamlabs refuses every request with this error while it loads a scene collection, and while the streamer drags or resizes a source in its editor. Whendo retries every half second, within the same 10 seconds, since both take seconds at most and the streamer would rather the source showed late than not at all.
- **Errors** are JSON-RPC errors with a code and a message. The message is recorded as the step's failure reason.
- **Losing the connection fails the waiting request** straight away.
- **No subscriptions.** Streamlabs can send events, but Whendo asks for what it needs when it needs it, so there's nothing to keep in sync. Events come later with triggers such as "when the scene changes."

### The Requests Whendo Uses

| Purpose | Method | Resource | Answer |
|---|---|---|---|
| Check the connection | `activeSceneId` | `ScenesService` | The active scene's ID |
| One scene, for the steps | `getScene`, with the scene ID | `ScenesService` | The scene, or `null` if there's no scene with that ID |
| Every scene, for the picker and the name fallback | `getScenes` | `ScenesService` | Every scene |
| Show or hide an item | `setVisibility`, with `true` or `false` | The item's `resourceId` | `null` |

### What a Scene Looks Like
This comes from Streamlabs Desktop's source rather than from its published examples, which are cut short and leave fields out ([`scene.ts`](https://github.com/streamlabs/desktop/blob/master/app/services/api/external-api/scenes/scene.ts), [`scene-node.ts`](https://github.com/streamlabs/desktop/blob/master/app/services/api/external-api/scenes/scene-node.ts), [`scene-item.ts`](https://github.com/streamlabs/desktop/blob/master/app/services/api/external-api/scenes/scene-item.ts)).

- **A scene** has `id`, `name` and `nodes`, every item and folder in the scene, including those inside folders, as a flat list.
- **Every node** has `id`, `sceneId`, `sceneNodeType` (`item` or `folder`) and, if it's in a folder, `parentId`.
- **An item** also has `sceneItemId`, which is the same as its `id`; `sourceId`; `name`, which is its source's name; `visible`; `display`, which is `horizontal` or `vertical`; and `resourceId`, the address to call `setVisibility` on.
- **A folder** also has `name` and `childrenIds`.

So one `getScene` gives the step everything it needs, and showing a source takes two requests: `getScene`, then `setVisibility` on each matching item.

- **Resource IDs are used as Streamlabs returns them,** never built by hand, and never kept between runs. A resource ID for an item that's since been deleted makes Streamlabs answer with an internal error, so each run gets fresh ones from `getScene`.
- **Missing scenes come back as `null`,** not as an error. The step treats `null` as "scene not found" and falls back to names.
- **IDs survive restarting Streamlabs.** Scene, item and source IDs are saved in the scene collection and loaded back as they were.
- **Visibility only.** Streamlabs also has separate "visible on stream" and "visible in recording" settings. Whendo only changes the eye icon's visibility, as the streamer would by clicking it.

### Dual Output
Streamlabs' dual output adds a vertical canvas, for platforms like TikTok. Each source shown on both canvases has two items in the scene, one with `display` set to `horizontal` and one with `vertical`, both with the same `sourceId`. Changing one's visibility through the API doesn't change the other's.

**Whendo shows and hides both,** which falls out of acting on every item in the scene that shows the source; see [Which Source](streaming-software.md#which-source). A streamer who triggers Head Pets expects it on whatever they're streaming. Picking one canvas can come later if a streamer asks.

### Rate Limit
Streamlabs limits `getScenes` to about two calls a second, shared by every program connected to it, such as a Stream Deck. Over the limit, it answers with an error saying so. Whendo uses `getScenes` only when it needs every scene: for the picker, and for the name fallback when a scene's ID isn't found. Steps use `getScene`, which has no limit.

## Packages
`engine/streamlabs`: the TCP connection with its retries and status, requests and answers, and the [streaming software interface](streaming-software.md#the-interface) built on them. Only the part that opens the connection knows it's TCP.

## Testing
- **The connection:** Go tests run it against a fake Streamlabs TCP server in the test process. They cover connecting, retrying, answers split across reads or several in one read, "API server is busy," timeouts, errors, and Streamlabs closing with a request waiting.
- **Real messages:** the fake server's answers follow [What a Scene Looks Like](#what-a-scene-looks-like), which comes from Streamlabs' source, since the maintainer doesn't run Streamlabs. Our first user's setup is the first real check.

## First Slice
1. The connection: requests and answers, retrying and status, against the fake server.
2. The streaming software interface on top of it. Then continue with [Streaming Software](streaming-software.md#first-slice).

## What Is Still Unverified
Everything here was worked out from Streamlabs Desktop's source, not by running it. What the source can't settle:

- **How long the "busy" spells last** in practice. The half-second retries within 10 seconds assume seconds at most.
- **Answers in our first user's setup,** including dual output, if they use it.

## Sources
- [Streamlabs Desktop API documentation](https://stream-labs.github.io/streamlabs-desktop-api-docs/docs/index.html) ([repository](https://github.com/stream-labs/streamlabs-desktop-api-docs))
- [Streamlabs Desktop: `tcp-server.ts`](https://github.com/streamlabs/desktop/blob/master/app/services/api/tcp-server/tcp-server.ts)
- [Streamlabs Desktop: the external scenes API](https://github.com/streamlabs/desktop/tree/master/app/services/api/external-api/scenes), and its [rate limit](https://github.com/streamlabs/desktop/blob/master/app/services/api/external-api.ts)
- [Streamlabs Desktop #6287: Restrict socket to owner](https://github.com/streamlabs/desktop/pull/6287), and its revert, [#6340](https://github.com/streamlabs/desktop/pull/6340)
