# Twitch Steps

- **Status:** Draft
- **Last updated:** 2026-11-16

The steps that act on Twitch: sending a chat message, refunding and fulfilling redemptions, and later waiting for a chat message. For the step contract they follow, see [Automation Engine](automation-engine.md#steps). For the login they act as, see [Twitch Login](twitch-login.md).

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

## Refund and Fulfill

`twitch.refundRedemption` gives the viewer back the channel points they spent, and `twitch.fulfillRedemption` marks the redemption as done, so it can no longer be refunded. Both use [Update Redemption Status](https://dev.twitch.tv/docs/api/reference/channel-points#update-redemption-status): `PATCH /helix/channel_points/custom_rewards/redemptions` with `broadcaster_id`, `reward_id` and `id`, and a `status` of `CANCELED` or `FULFILLED`. They need the `channel:manage:redemptions` scope, which is already requested at login. They read "Refund the redemption" and "Fulfill the redemption", and sit in the step picker's **Redeems** group.

- **No settings.** Each step acts on the redemption that started the automation, using the redeem trigger's redemption ID and reward ID. Twitch needs both, and picking an ID from a list of variables would mean nothing to most streamers. The redeem trigger is the only source of redemptions, so there's nothing else to choose.
- **They only work after the redeem trigger.** A step definition can name the trigger it needs. Using one of these steps in an automation that starts another way is a must-fix problem: "“Refund the redemption” only works in automations started by “Channel point redeem”." The check skips this while there's no trigger yet, like the other trigger-dependent checks.
- **Test runs fail without asking Twitch.** A test run's redemption ID is `test`, so the step fails at once with "a test run has no real redemption to refund, so this step only works when someone really redeems", rather than with whatever Twitch says about a made-up ID.

**Good to know,** going by the last list of redeems that loaded, the same list [missing redeems](twitch-redeems.md#missing-redeems) use. A step can add notes about how it's used, like a trigger can about its settings; they're never must-fix, and only appear once the step is with the right trigger. A redeem the list doesn't have, such as the placeholder, gets no note, since the trigger already explains it.

| The redeem | Note |
|---|---|
| Wasn't made by Whendo | "Whendo didn't make this redeem, so Twitch won't let it refund redemptions. Recreate the redeem in Whendo on the Redeems page." (or "fulfill") |
| Can't be refunded, refund step | "This redeem can't be refunded. Turn on “Viewers can be refunded” in Redeems." |
| Can't be refunded, fulfill step | "This redeem is fulfilled as soon as it's redeemed, so there's nothing left to fulfill. Turn on “Viewers can be refunded” in Redeems if you want this step to do it instead." |

**Failures,** each recorded in run history with the reason:

| Problem | Reason given |
|---|---|
| HTTP 403 | Twitch only lets Whendo change redemptions of redeems Whendo made, so recreate the redeem in Whendo. Twitch's message follows. |
| HTTP 404 | No redemption is waiting: it was already fulfilled or refunded, for example in the Twitch dashboard, or the redeem doesn't let viewers be refunded. |
| Any other error | Twitch's message is recorded. |

## Sources
- [Twitch: Send Chat Message](https://dev.twitch.tv/docs/api/reference/chat#send-chat-message)
- [Twitch: Update Redemption Status](https://dev.twitch.tv/docs/api/reference/channel-points#update-redemption-status)
