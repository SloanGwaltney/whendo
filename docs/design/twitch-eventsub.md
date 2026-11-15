# Twitch EventSub

- **Status:** Draft
- **Last updated:** 2026-11-15

How Whendo receives events from Twitch, such as redeems, chat messages and follows, and turns them into triggers. Like login, this is part of the engine, not the Wails layer. For the automation model these triggers feed into, see [MVP UI Design](mvp-ui.md#automation-model). For the login it depends on, see [Twitch Login](twitch-login.md).

## Triggers and Event Sources

- **The engine defines the contract, not the events.** A trigger has an ID, a name, its settings, a filter that checks events against those settings, and a list of the variables it provides; see [Automation Engine](automation-engine.md#triggers). An event says which trigger fired and carries a value for each of those variables. The engine only handles that generic shape.
- **Each integration defines its own triggers.** The Twitch integration declares "redeem," "chat message starts with" and "follow." A future integration for another platform declares its own, and nothing forces them to match Twitch's, because no event is guaranteed to exist on every platform.
- **Twitch's data stays in the Twitch integration.** The integration translates each EventSub notification into one of its own triggers. The engine never sees Twitch's JSON, so when Twitch changes or retires a subscription version, only the integration changes.
- **Everything in the event becomes a variable.** Nothing is hidden from the user, including IDs. The only limit is how the designer presents them, for example by showing common variables first and IDs under "more." Steps take variables as inputs, so "refund a redemption" takes the redemption ID variable and "only from a specific user" takes a user ID variable.
- **Test runs fire any trigger** with values from a form instead of from Twitch. They aren't a source with triggers of their own. See [Test Runs](mvp-ui.md#test-runs).

## Subscriptions

Whendo subscribes to every event it supports whenever the user is logged in, and ignores events that no enabled automation uses.

| Trigger | Subscription | Version | Condition | Scope |
|---|---|---|---|---|
| Redeem | `channel.channel_points_custom_reward_redemption.add` | 1 | `broadcaster_user_id` | `channel:read:redemptions` |
| Chat message | `channel.chat.message` | 1 | `broadcaster_user_id`, and `user_id` set to the streamer | `user:read:chat` |
| Follow | `channel.follow` | 2 | `broadcaster_user_id`, and `moderator_user_id` set to the streamer | `moderator:read:followers` |

All three scopes are already requested at login.

- **Why always subscribe:** Twitch closes a connection that hasn't created a subscription within 10 seconds of connecting. With a fixed set of subscriptions, every connect and reconnect does the same thing, and enabling or disabling an automation never involves Twitch. Events no automation uses cost nothing but a little bandwidth.
- **One redeem subscription for all rewards:** the subscription can be limited to one reward, but that would mean creating and deleting subscriptions whenever a user changes which reward a trigger uses. Instead, the engine matches each redeem to the triggers for its reward.
- **Limits:** a WebSocket connection allows 300 subscriptions and a total cost of 10. Subscriptions the user has authorized cost nothing, and Whendo uses three. Revisit this if a future integration adds many event types.
- **Token expiry doesn't affect subscriptions.** Twitch checks the token when a subscription is created, and none of its reasons for revoking a subscription is the token expiring. Refreshing the access token never requires resubscribing.

## Trigger Variables

Each trigger provides everything in its event:

- **Redeem:** the redemption ID; the redeemer's user ID, login and display name; the text they entered, which is empty if the reward doesn't ask for any; the reward's ID, title, cost and prompt; and when it was redeemed.
- **Chat message:** the message ID; the sender's user ID, login and display name; the full message; and the rest of the message after the matched text.
- **Follow:** the follower's user ID, login and display name, and when they followed.

## Chat Messages

- **Only the streamer's own chat.** During a shared chat session, `channel.chat.message` also delivers messages from the other channels in the session, marked by its `source_*` fields. Whendo ignores those. Otherwise two streamers who both have `!discord` would each answer the other's viewers.
- **Matching starts at the beginning of the message.** A message matches when its first word is the trigger's text. Capitalization is ignored, so `!Discord` matches `!discord`. The whole word must match, so `!a1` matches "!a1" and "!a1 please" but not "!a10."
- **The rest of the message** is the text after the matched word, with leading spaces removed. "!so coolstreamer" gives "coolstreamer."
- **The streamer's own messages count.** Test runs depend on the streamer typing commands themselves, for example to answer Do a Voice's prompt.
- **No general loop prevention.** Matching only at the start stops the common case of Whendo reacting to its own prompt. Preventing every loop a user could build would mean analyzing every combination of steps, so Whendo doesn't try.

## Connection

Whendo uses EventSub's WebSocket transport, because a desktop app can't receive webhooks. It keeps one connection open while the user is logged in.

### Connecting
1. Connect to `wss://eventsub.wss.twitch.tv/ws` with `keepalive_timeout_seconds=10`.
2. Wait for the welcome message, which carries the session ID.
3. Create the three subscriptions for that session within 10 seconds.

### Staying Connected
- **Keepalive:** Twitch sends a keepalive message whenever no event has arrived for 10 seconds. If neither an event nor a keepalive arrives within that time plus a few seconds' grace for network delay, Whendo treats the connection as dead and reconnects. The minimum timeout is used so a dead connection is noticed in seconds rather than minutes. It costs only a small message every 10 seconds when nothing is happening.
- **Ping and pong:** Twitch also sends WebSocket pings and closes the connection if they aren't answered. The connection must keep reading at all times so the WebSocket library can answer them.

### Planned Reconnects
Twitch sometimes sends a reconnect message with a new URL, about 30 seconds before it closes the connection.

1. Open a connection to that URL, unchanged.
2. Wait for the new connection's welcome message. Subscriptions carry over, so there's nothing to create.
3. Close the old connection.

While both connections are open, either may deliver events, so duplicates are possible; see [Duplicate Events](#duplicate-events). A planned reconnect doesn't count toward Twitch's limit of 3 connections, and it doesn't change the connection status.

### Unexpected Disconnects
When the connection drops, Twitch disables its subscriptions. Whendo connects again from the start and recreates them.

- **Retries:** wait about 1 second before the first attempt, then double the wait after each failure, up to about a minute. Keep trying for as long as the user is logged in.
- **No randomness in the wait.** Randomizing retries matters when many clients could overwhelm a server by retrying at the same moment. Whendo's share of Twitch's traffic is too small for that to matter.

### Close Codes

| Code | Meaning | Whendo's response |
|---|---|---|
| 4000 | Twitch internal server error | Reconnect |
| 4001 | Whendo sent a message other than a pong | Log an error, since it's a bug in Whendo, then reconnect |
| 4002 | Whendo didn't answer a ping | Log an error, then reconnect |
| 4003 | No subscription created within 10 seconds | Log an error, then reconnect |
| 4004 | A planned reconnect wasn't finished within 30 seconds | Reconnect from the start |
| 4005 | Network timeout | Reconnect |
| 4006 | Network error | Reconnect |
| 4007 | Invalid reconnect URL | Log an error, then reconnect from the start |

Errors that point to a bug are logged so they're noticed, but Whendo still reconnects so a bug doesn't leave the user without events.

### Missed Events
Twitch doesn't replay events that happen while Whendo is disconnected, and Whendo doesn't try to recover them. The connection status shows when Whendo isn't receiving events.

A future version could recover missed redeems, since Twitch can list a reward's unfulfilled redemptions for rewards Whendo created. But running a redeem minutes late may not be what a streamer wants, so it isn't part of the MVP.

### Duplicate Events
Whendo ignores any notification whose message ID it has seen in the last few minutes. Twitch's WebSocket documentation doesn't say whether duplicates happen, but planned reconnects make them possible, and a redeem that plays TTS twice would be noticeable.

## Interaction with Login

- **Connect and disconnect with the login.** Connect when the session becomes logged in, at startup or after a login. Close the connection when it becomes logged out. Closing is enough to clean up, because Twitch disables a connection's subscriptions when it closes.
- **Token rejected while subscribing (HTTP 401):** call the session's `RenewAccessToken` with the rejected token and retry once. If renewing fails, the session logs the user out, which closes the connection.
- **Missing scope (HTTP 403):** log the user out, so they log in again with the scopes the current version requests. This can only happen if a future version adds a scope.

### Revoked Subscriptions
Twitch sends a revocation message once, with a reason, and then stops sending that event.

| Reason | Meaning | Whendo's response |
|---|---|---|
| `authorization_revoked` | The user disconnected Whendo in Twitch or changed their password | Log out |
| `user_removed` | The user's account no longer exists | Log out |
| `version_removed` | Twitch retired the subscription version Whendo uses | Log an error and show it in the connection status. The user stays logged in, because only that subscription has stopped |

None of these can be fixed by the app. `version_removed` means Whendo needs an update, so it's logged as an error in the hope that someone reports it.

## Connection Status

The navigation shows the Twitch connection status.

| State | When | Shown as |
|---|---|---|
| Connected | Connected, with all subscriptions created | "Twitch: connected" |
| Reconnecting | Connecting after login or startup, or after a drop | "Twitch: reconnecting…" |
| Problem | `version_removed`, or a subscription that keeps failing | "Twitch: some events can't be received," with details on hover or click |

- **No logged-out state:** while logged out, the app shows the login prompt instead of the main screens.
- **No separate offline state:** a failed reconnect stays "reconnecting…" until it succeeds. That's all the streamer needs to know: automations won't fire until it's back.
- **Receiving only:** the status is about receiving events. Sending chat messages or refunding redemptions can still fail on their own, and those failures show up as failed steps in run history.

## Single Instance

Whendo runs as a single instance. Opening it again brings the existing window forward. Two copies would each run every automation, posting every chat reply twice, and would use two of Twitch's three allowed connections.

## Testing

- **The EventSub client:** Go tests run it against a fake EventSub WebSocket server in the test process. They cover the welcome message, subscribing, keepalive timeouts, planned reconnects, drops and resubscribing, duplicates, revocations and close codes.
- **Real message formats:** the [Twitch CLI](https://dev.twitch.tv/docs/cli/) can run a mock EventSub WebSocket server and send it mock events, which checks Whendo's assumptions about real messages without real events.
- **Automations:** [test runs](mvp-ui.md#test-runs).

## Dependencies

Go's standard library has no WebSocket client. Whendo uses `github.com/coder/websocket`; see [ADR 0004](../adr/0004-coder-websocket-for-eventsub.md).

## Sources
- [Twitch: Handling WebSocket events](https://dev.twitch.tv/docs/eventsub/handling-websocket-events/)
- [Twitch: Managing EventSub subscriptions](https://dev.twitch.tv/docs/eventsub/manage-subscriptions/)
- [Twitch: EventSub chat reference (Channel Chat Message)](https://dev.twitch.tv/docs/eventsub/reference/chat)
- [Twitch: EventSub channel points reference (Custom Reward Redemption Add)](https://dev.twitch.tv/docs/eventsub/reference/channel-points)
- [Twitch: EventSub channel reference (Channel Follow)](https://dev.twitch.tv/docs/eventsub/reference/channel)
