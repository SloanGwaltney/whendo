# Twitch Redeems

- **Status:** Draft
- **Last updated:** 2026-11-16

How Whendo lists, creates and edits the streamer's channel point redeems, how a redeem trigger picks one, the placeholder redeem for automations whose redeem doesn't exist yet, and how Whendo is developed on an account without channel points. For the Redeems screen this supports, see [MVP UI Design](mvp-ui.md#redeems). For how redemptions arrive as events, see [Twitch EventSub](twitch-eventsub.md).

Twitch calls these **custom rewards**, and the API uses that name. Whendo's UI calls them **redeems**, the word streamers use. This document uses "reward" for the Twitch object and "redeem" for what the user sees.

## Listing Redeems

[Get Custom Reward](https://dev.twitch.tv/docs/api/reference/channel-points#get-custom-reward): `GET /helix/channel_points/custom_rewards?broadcaster_id=<streamer>`. It needs `channel:read:redemptions` or `channel:manage:redemptions`, both already requested at login.

- **Two requests.** One lists every reward. A second, with `only_manageable_rewards=true`, lists the ones Whendo can manage, which are exactly the ones Whendo created. Twitch is the record of which redeems Whendo made, so Whendo doesn't keep its own list that could drift from it.
- **Loaded when needed, not kept in sync.** The list is loaded when the Redeems page or the designer's redeem picker opens, and both have a refresh button. A redeem changed in the Twitch dashboard while Whendo runs shows up the next time the list is loaded. Twitch has EventSub events for reward changes, but keeping a live copy isn't worth it for a list that rarely changes.
- **The last list that loaded is kept** in memory, so problems such as a missing redeem can be worked out without asking Twitch again.
- **What's shown:** the title, cost, prompt, whether it's shown to viewers, paused, needs the viewer to type something, can be refunded, and its limits and cooldown.

### Accounts Without Channel Points
Channel points need a Twitch affiliate or partner account. For any other account, Get Custom Reward answers **HTTP 403** with "The broadcaster is not a partner or affiliate." That's the only 403 Twitch documents for this request, so Whendo treats any 403 on it as "this account has no channel points" rather than making a separate check. See [MVP UI Design](mvp-ui.md#redeems) for what the page shows.

A 403 is never treated as missing redeems. An account that loses affiliate status keeps its automations as they are, and nothing is marked missing; see [Missing Redeems](#missing-redeems).

## Whendo Redeems

Twitch only lets the app that created a reward update or delete it. A redeem made in the Twitch dashboard can be read, and used as a trigger, but not changed by Whendo.

### Creating
[Create Custom Rewards](https://dev.twitch.tv/docs/api/reference/channel-points#create-custom-rewards): `POST /helix/channel_points/custom_rewards`, which needs `channel:manage:redemptions`, already requested at login.

The Redeems page creates a redeem from a form, saved with an explicit button, since it changes the streamer's Twitch account. See the [guiding principles](mvp-ui.md#guiding-principles).

| Setting | Twitch field | Limits |
|---|---|---|
| Name | `title` | 1 to 45 characters, unique among the channel's rewards, ignoring capitalization |
| Cost | `cost` | 1 to 1,000,000,000 channel points |
| Description | `prompt` | Shown to viewers when they redeem it |
| Viewers type a message | `is_user_input_required` | |
| Shown to viewers | `is_enabled` | |
| Colour | `background_color` | Hex, such as `#9147FF` |
| Limit per stream | `is_max_per_stream_enabled`, `max_per_stream` | At least 1 |
| Limit per viewer per stream | `is_max_per_user_per_stream_enabled`, `max_per_user_per_stream` | At least 1 |
| Cooldown | `is_global_cooldown_enabled`, `global_cooldown_seconds` | At least 1 second. The form offers minutes and seconds. |
| Viewers can be refunded | `should_redemptions_skip_request_queue`, inverted | Off for a new redeem; see [Refunds and the Request Queue](#refunds-and-the-request-queue) |

- **Checked before calling Twitch** where Whendo knows the rule: the name's length, a name already used by another redeem, the cost and the limits. Twitch checks again, and its message is shown if it still refuses, for example when AutoMod rejects the name.
- **A channel has at most 50 rewards,** shown and hidden together. The page says so when the channel is full, rather than letting the request fail.
- **Pictures can't be set.** The API has no field for a reward's picture, so a Whendo redeem uses Twitch's default picture. Whether the streamer can add one in the Twitch dashboard to a reward an app created is [unverified](#what-is-still-unverified).

### Refunds and the Request Queue
Twitch's request queue is a list in the streamer's Twitch dashboard, not anything to do with how Whendo runs automations. A redemption of a redeem that doesn't skip it waits there, unfulfilled, until the streamer, a moderator or the app that made the redeem completes it or rejects it, which refunds the viewer. A redeem that skips it is completed the moment it's redeemed, so it can never be refunded. Either way, Whendo receives the redemption at once and runs the automation the same.

- **Whendo never says "queue" for this.** Its automations already queue runs, and the two would be confused. The setting is **Viewers can be refunded**, the opposite of Twitch's `should_redemptions_skip_request_queue`, with a hint that turning it on keeps each redemption waiting in the Twitch request queue until it's completed or refunded.
- **New redeems can't be refunded by default.** Most redeems never need a refund, and redemptions that wait in the queue pile up unless something completes them.
- **It's in plain sight on the form,** not hidden among advanced settings, since a redeem that needs refunds, such as Do a Voice, depends on it.
- **The refund and fulfill steps will warn** with a good to know problem when the redeem can't be refunded: "This redeem can't be refunded. Turn on “Viewers can be refunded” in Redeems."

### Editing
[Update Custom Reward](https://dev.twitch.tv/docs/api/reference/channel-points#update-custom-reward): `PATCH /helix/channel_points/custom_rewards?broadcaster_id=<streamer>&id=<reward>`.

- **Every limit is sent as a pair.** Twitch sets a limit's other field to off or 0 when only one of its pair is sent, so Whendo always sends both fields of every limit, never one.
- **Pausing** (`is_paused`) is available from the redeem's row rather than the form, since it's what a streamer reaches for mid-stream: viewers still see the redeem, but can't redeem it.
- **A blank description can't be saved.** Twitch keeps the old prompt when it's sent an empty one, so clearing it would look saved without being saved. The form doesn't allow it.

### Deleting
[Delete Custom Reward](https://dev.twitch.tv/docs/api/reference/channel-points#delete-custom-reward): `DELETE /helix/channel_points/custom_rewards?broadcaster_id=<streamer>&id=<reward>`.

- **Asks for confirmation,** naming the automations that use it.
- **Twitch fulfills the redemptions still waiting** in the request queue when a reward is deleted. The confirmation says so, since it means viewers can't be refunded for those.
- **Automations that use it** are left as they are and get a [missing redeem](#missing-redeems) problem, so the streamer can pick another redeem.

## Recreating a Redeem in Whendo

A redeem made in the Twitch dashboard can be recreated as a Whendo redeem, so Whendo can edit it, and, once those steps exist, refund and fulfill its redemptions. Do a Voice needs this; see [MVP User Redeems](mvp-user-redeems.md#constraints).

Two Twitch rules decide the order of the steps. Names must be unique, so the copy can't be made while the original exists. And only the app that created a reward can delete it, so Whendo can't delete the original. The streamer deletes it in the Twitch dashboard, in the middle of the flow:

1. **The streamer picks Recreate** on a redeem that wasn't made by Whendo. A dialog explains what happens: the redeem gets a new ID, its picture isn't copied, and the automations that use it are switched to the new one.
2. **Whendo keeps the redeem's settings** from the list it already loaded.
3. **The streamer deletes the original** in the Twitch dashboard, which the dialog links to, then clicks Continue.
4. **Whendo checks that it's gone.** If Get Custom Reward still finds its ID, the dialog says so and waits.
5. **Whendo creates the copy** with the same settings.
6. **Whendo switches every automation** that used the original's ID to the copy's, and saves each one. The streamer agreed to this in step 1, so it's not a separate question.

- **The settings are kept in memory only.** If Whendo closes between steps 3 and 5, the original is gone and the streamer recreates it by hand from the Redeems page. That's rare enough not to be worth saving the half-finished flow to a file.
- **Redemptions waiting in the original's queue** are fulfilled by Twitch when the streamer deletes it, the same as any deletion. The dialog says so.
- **Creating the copy first was considered.** Whendo could create it hidden, under a temporary name, and rename it once the original was deleted. Nothing would be lost if Whendo closed partway, but the streamer would see a strangely named redeem in their dashboard, and a crash would leave it there. Recreating is rare, so the simpler flow wins.

## Picking a Redeem in the Designer

The redeem trigger's reward setting has the `twitch.reward` kind. Its popover is a picker:

- **Every redeem the account has,** with its cost, Whendo redeems marked, and a refresh button.
- **The placeholder,** "A redeem I'll set up later"; see [The Placeholder Redeem](#the-placeholder-redeem).
- **For an account without channel points,** the picker says so and offers only the placeholder.
- **The sentence shows the redeem's name,** from the last list that loaded. If the list hasn't loaded, or couldn't be, the sentence says "your redeem" rather than an ID. The automation still stores only the ID, so renaming a redeem never breaks an automation.

## The Placeholder Redeem

A streamer without channel points yet, or one planning a redeem they haven't made, can build an automation on a placeholder and pick the real redeem later. Test runs work on it like on any redeem, so the automation can be checked before the real redeem exists.

- **It's a special reward ID,** `whendo.placeholder`, stored in the setting like any other. Twitch's reward IDs are UUIDs, so a real redemption can never match it.
- **Test runs need nothing new.** The redeem trigger's test values take the reward from the automation's settings, so they match its own filter as always.
- **It's never a must-fix problem.** Must-fix problems block test runs, and testing is the point of the placeholder. Instead it's a **good to know** problem: "This uses a placeholder redeem, so it only runs when you test it. Pick your real redeem once it's ready."
- **Home shows it too,** on the automation's row, so automations still waiting for their real redeem are easy to find once channel points are turned on.
- **The trigger reports it.** It's about the automation's own settings, not the world outside it, so it belongs in the engine's [check](automation-engine.md#checking-automations). Triggers gain an optional way to add problems about their own settings, which the Twitch redeem trigger uses for this.
- **Sample automations can use it,** so an account without channel points can still start from a sample that needs a redeem. See [MVP UI Design](mvp-ui.md#home).

## Missing Redeems

A redeem trigger uses a **missing redeem** when the last list that loaded doesn't have its reward. It's a **good to know** problem, "The redeem this uses was deleted in Twitch. Pick another one," shown in the designer and on Home.

- **Only a list that loaded counts.** A list that couldn't be loaded, or an account without channel points, never marks a redeem missing; see [Problems](mvp-ui.md#problems).
- **It isn't must-fix.** Problems outside the automation are never must-fix, and nothing's at risk: no redemption of a deleted reward can arrive.
- **It's worked out outside the engine's check.** It depends on Twitch, not on the automation, so the Wails layer adds it to the check's problems using the last list that loaded. The engine stays unaware of Twitch rewards.

## Developing Without Channel Points

The maintainer's account has no channel points, so it only ever gets "not a partner or affiliate." Twitch has no test channels with channel points, and the Twitch CLI's mock API is still a mock. So:

- **The reward code is tested against a fake Twitch server,** like [Send a Chat Message](twitch-steps.md). Tests cover listing, the 403, creating, editing with limits sent as pairs, deleting, and Twitch's error messages.
- **A fake reward source** stands in for Twitch while developing the UI. Setting `WHENDO_FAKE_REWARDS=1` replaces the Twitch reward client with one that keeps a few made-up redeems in memory: some marked as made by Whendo, some not. Creating, editing, deleting and recreating work on them, and reset when Whendo restarts. Like `WHENDO_TWITCH_CLIENT_ID`, it's for developers and never shown to users.
- **Fake redeems never fire on their own.** Test runs fire them, the same as any redeem.
- **The last check is on our first user's account,** which has real channel points.

## Packages

| Package | Contains |
|---|---|
| `engine/twitchapi` | The Helix request helper that renews a rejected token once, moved out of `engine/twitchsteps` so the steps and rewards share it. |
| `engine/twitchrewards` | Listing, creating, updating and deleting rewards; the "no channel points" error; the reward source interface and its Twitch and fake versions. |
| `engine/twitcheventsub` | The redeem trigger gains the placeholder and its problem. |

The Wails layer gets a rewards service for the Redeems page and the picker, and adds missing redeem problems to the check.

## First Slice

1. Move the Helix helper to `engine/twitchapi`.
2. List rewards, with the "no channel points" error and the fake reward source.
3. The redeem picker in the designer, with the placeholder and its problem.
4. The Redeems page, read only: both groups of redeems, and the message for accounts without channel points.
5. Missing redeem problems in the designer and on Home.
6. Creating, editing, pausing and deleting Whendo redeems.
7. Recreating a redeem in Whendo.

After that, the refund and fulfill steps, then wait for a chat message, which together finish Do a Voice.

## What Is Still Unverified
- **Pictures on Whendo redeems:** whether the streamer can set a picture in the Twitch dashboard on a reward an app created.
- **Error wording:** the "not a partner or affiliate" message, and that Get Custom Reward never answers 403 for any other reason, come from Twitch's reference and haven't been seen from the real API.

## Sources
- [Twitch: Get Custom Reward](https://dev.twitch.tv/docs/api/reference/channel-points#get-custom-reward)
- [Twitch: Create Custom Rewards](https://dev.twitch.tv/docs/api/reference/channel-points#create-custom-rewards)
- [Twitch: Update Custom Reward](https://dev.twitch.tv/docs/api/reference/channel-points#update-custom-reward)
- [Twitch: Delete Custom Reward](https://dev.twitch.tv/docs/api/reference/channel-points#delete-custom-reward)
