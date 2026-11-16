# Twitch Steps

- **Status:** Draft
- **Last updated:** 2026-11-16

The steps that act on Twitch: sending a chat message, waiting for a chat message, and refunding and fulfilling redemptions. For the step contract they follow, see [Automation Engine](automation-engine.md#steps). For the login they act as, see [Twitch Login](twitch-login.md).

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

## Wait for a Chat Message

`twitch.waitForChatMessage` waits for a chat message that starts with one of a list of choices, such as `!a1`, `!a2` and `!a3`, and splits the automation with one path per choice plus one for running out of time. It's what lets Do a Voice ask the redeemer to pick a voice. It reads:

> Wait up to *2 minutes* for *the person who started this* to type *!a1, !a2 or !a3*

| Setting | Kind | Notes |
|---|---|---|
| How long | Duration | Required. More than zero and at most 5 minutes; see [Time Limit](#time-limit). |
| From | Option | "the person who started this" or "anyone". New steps start on "the person who started this", since that's what a "pick one" prompt almost always wants. A setting definition can now give an initial value, which the designer copies into a new step; Whendo refuses to start if a setting's initial value is one the setting itself would refuse. |
| Choices | Choice list | Required. At least one; blank and repeated choices are already must-fix. |

### Who Can Answer
- **No IDs to pick.** Rather than a user ID variable, the step offers "the person who started this" or "anyone". Picking the right ID from a list of variables would mean nothing to most streamers, the same reason [refund and fulfill](#refund-and-fulfill) take no settings.
- **Each trigger says who started it.** A trigger definition can name the variable holding the user ID of the person behind its event: the redeemer for a redeem, the sender for a chat message and the follower for a follow. The engine passes that ID to the step as part of its input, so the step never knows which trigger it follows.
- **A trigger without a person** can't be used with "the person who started this". A step can check how it's used, given its settings and the trigger, and report must-fix problems, unlike its good to know notes. This one is: "“Wait for a chat message” can't wait for the person who started this, because “…” isn't started by a person." All three MVP triggers have one, so this only guards future triggers.
- **Messages from anyone else are ignored,** and the step keeps waiting.

### Matching
- **Choices match like the chat message trigger:** the message must start with the choice as a whole word, ignoring capitalization, so `!a1` matches "!a1 please" but not "!a10". See [Twitch EventSub](twitch-eventsub.md#chat-messages). The trigger's matching function is shared rather than copied, so the two can't drift apart.
- **Only the streamer's own chat counts,** since shared chat messages from other channels never become events.
- **The first match wins.** Messages that match no choice are ignored. If a message could match two choices, which can only happen when one choice starts with another followed by a space, such as `!a` and `!a 1`, the earlier choice in the list wins.
- **Only messages sent after the step starts count.** The step subscribes to chat message events when it starts and unsubscribes when it ends; see [Steps That Wait for Events](automation-engine.md#steps-that-wait-for-events).
- **A message can do both.** An answer that also starts another automation's chat message trigger does both, as the engine design already says.
- **The streamer's own messages count,** which is how test runs work: the test form starts the redeemer as the streamer, and the streamer types the answer in their own chat.

### Outcomes
- **One per choice,** with the choice's ID as the outcome ID, so editing a choice's text keeps its path. Its branch reads "If they type `!a1`" for "the person who started this", or "If someone types `!a1`" for "anyone".
- **Timed out,** with the ID `timedOut`, reads "If they don't answer in time" or "If no one answers in time". A choice whose ID is `timedOut` is a must-fix problem. The designer never makes one, so only a hand-edited file could.
- **Each choice's path provides the answer:** the sender's name, the message and the rest of the message after the choice, which are common, and the sender's login name, user ID and message ID. They use the same IDs and names as the chat message trigger's variables. "Timed out" provides nothing, so in Do a Voice the answer is available after the split only if "timed out" ends the automation, which it does.
- **Run history** records which path the step took, as for every step that splits, and the answer's values as for any step's variables.

### Time Limit
- **At most 5 minutes.** An automation runs one run at a time, so while this step waits, later runs of the same automation wait behind it. A cap stops a slip, such as 2 hours instead of 2 minutes, from holding up every later redeem. Over the cap is a must-fix problem: "“Wait for a chat message” is over its 5-minute limit.", worded like the character limit's. A duration setting can now declare a maximum, like a text setting's maximum length. The designer's duration editor says "At most 5 minutes." and its arrows stop there.
- **Revisit later, not in the MVP:** whether a waiting run should hold up its automation's queue at all, or step aside so later runs can start. See [Running and Queuing](automation-engine.md#runs-and-queues).
- **Losing the connection to Twitch while waiting** isn't a failure. No messages arrive, so the step times out as if no one answered. Run history then shows "timed out" during a time the connection status shows as down.
- **Shutting down** stops the wait at once, like the wait step.

### Designer
- **The choice list gets its editor,** which the designer has been waiting on since choice lists were added. Each choice is a text field with a remove button, plus an "Add a choice" button. The designer makes each new choice's ID, as for step IDs.
- **Blank choices aren't saved.** A choice still being typed would otherwise show a "blank choice" problem, and clearing a choice's text removes it, like its remove button.
- **Adding a choice adds a path,** and removing one removes its path if it's empty, using the existing branch reconciling. A path that still has steps stays, with a must-fix problem, so steps are never lost silently.

### Failures
The step has no failures of its own beyond its settings. Running out of time is an outcome, not a failure, because what to do then is the streamer's choice, such as refunding in Do a Voice.

### First Slice
1. Triggers name their person, and the engine passes the person's user ID to steps.
2. A maximum for duration settings, with its problem.
3. Share the chat message trigger's matching function.
4. The step itself: settings, outcomes with their sentences and variables, and running it.
5. The choice list editor, and the duration editor's maximum.
6. Build Do a Voice in the designer and test it from end to end, typing the answer in chat.

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
