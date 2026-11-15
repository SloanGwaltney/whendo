# Twitch Steps

- **Status:** Draft
- **Last updated:** 2026-11-15

The steps that act on Twitch: sending a chat message now, and later waiting for a chat message, refunding and fulfilling redemptions. For the step contract they follow, see [Automation Engine](automation-engine.md#steps). For the login they act as, see [Twitch Login](twitch-login.md).

## Acting as the Streamer

- **Every Twitch step acts as the logged-in streamer, in the streamer's own channel.** There's no bot account in the MVP.
- **A rejected token is renewed once.** Access tokens expire every few hours, so if Twitch answers HTTP 401, the step asks the login for a new token and tries once more. If renewing fails, the user is logged out, as in [Twitch Login](twitch-login.md#token-lifetime-and-refresh), and the step fails.
- **Not being logged in fails the step.** Test runs need a login for Twitch steps too, since they really happen.

## Send a Chat Message

`twitch.sendChatMessage` posts a message with variables inserted, using [Send Chat Message](https://dev.twitch.tv/docs/api/reference/chat#send-chat-message): `POST /helix/chat/messages` with `broadcaster_id` and `sender_id` both set to the streamer. It needs the `user:write:chat` scope, which is already requested at login.

| Setting | Kind |
|---|---|
| Message | Template |

**Failures,** each recorded in run history with the reason:

| Problem | Why it fails rather than adapting |
|---|---|
| Longer than 500 characters once variables are inserted | Cutting it short could say something the streamer never meant. It's checked before calling Twitch, counting characters rather than bytes. |
| Empty once variables are inserted | There's nothing to send. Also checked before calling Twitch. |
| Twitch answers 200 but `is_sent` is false | The message was dropped, for example by slow mode or AutoMod. The step fails with Twitch's `drop_reason`. |
| HTTP 403 | The streamer's account can't chat in the channel right now. |
| HTTP 429 | The chat rate limit was reached. Retrying could pile up messages, so the step fails. |
| Any other error | Twitch's message is recorded. |

## Sources
- [Twitch: Send Chat Message](https://dev.twitch.tv/docs/api/reference/chat#send-chat-message)
