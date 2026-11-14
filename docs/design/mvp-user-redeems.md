# MVP User Redeems

- **Status:** Draft
- **Last updated:** 2026-11-14

These are the channel point redeems used by our first MVP user. **The MVP must be able to build every redeem in this document that needs Whendo.** Redeems that Twitch handles entirely by itself are listed so the inventory is complete, but they don't need any automation.

Related: [MVP UI Design](mvp-ui.md).

## Redeems That Need Whendo

### D20 Mute Me
1. Roll a 20-sided die.
2. If the roll is 20, post a fixed message to chat that includes the roll and the redeemer's name.
3. Otherwise, post a different fixed message to chat that includes the roll and the redeemer's name.

The cooldown is set on the reward itself.

### Mute Me
1. Post a fixed message to chat telling the streamer to mute themselves.
2. Trigger TTS Monster.

### Check In
Post a fixed message to chat that includes the redeemer's name.

### Hydrate
1. Post a message to chat.
2. Trigger TTS Monster with a fixed message.

### Chug It, Posture Check and Stretch
Same as Hydrate, each with its own messages. Chug It is a more expensive version of Hydrate.

### Cute
Same as Hydrate, with its own message.

### Head Pets
1. Show a browser source in OBS.
2. Wait a set amount of time.
3. Hide the browser source.

### Do a Voice
1. Post a message to chat from the streamer's account. It names the redeemer and lists the voice commands, for example "`!a1` for an Irish voice, `!a2` for a Scottish voice." There are three or four voices.
2. Wait for **the redeemer** to use one of the commands in chat, for up to a few minutes. The time limit is set on the step when the automation is configured. Commands from anyone else are ignored.
3. **If the redeemer uses a command:**
   1. Post a message to chat telling the streamer which voice was chosen. The streamer then does the voice.
   2. Mark the redemption as fulfilled.
4. **If the time limit runs out:** refund the redemption.

The user is open to changing how this one works.

## Redeems Handled by Twitch
These need no automation.

- **Voices:** the viewer enters text when they redeem it, and Twitch handles the rest.
- **Highlight My Message:** a built-in Twitch reward.

## Capabilities the MVP Needs
Building the redeems above requires the following.

**Triggers**
- Channel point redeem.

**Steps**
- **Send a chat message** from the streamer's account, with variables inserted.
- **Roll a random number** within a range. This provides the roll as a variable.
- **Check a condition**, such as "the roll is 20." This step has two outcomes, one for when the condition is true and one for when it's false.
- **Wait** for a set amount of time.
- **Show or hide an OBS source.**
- **Wait for a chat command** from a list of choices, with a time limit, optionally accepting it only from a specific user such as the redeemer. This step has one outcome per command, plus one for timing out.
- **Refund a redemption.**
- **Mark a redemption as fulfilled.**
- **Play TTS Monster speech.** Generate the audio for a message and play it on the chosen output device. See [TTS Monster](#tts-monster).

**Variables**
- The redeemer's name, from the redeem trigger.
- The roll, from the random number step.

## TTS Monster

**Current setup:**
1. A redeem triggers Streamer.bot.
2. Streamer.bot sends a webhook to Mix It Up.
3. Mix It Up uses the TTS Monster overlay URL to call what appears to be an internal TTS Monster API.
4. The API returns the TTS audio file, and Mix It Up plays it over the speakers.

**Goal:** Whendo replaces both Streamer.bot and Mix It Up in this chain. It calls TTS Monster directly, then downloads and plays the audio on the chosen output device.

**Two ways to integrate:**
- **The overlay URL method** is what Mix It Up uses. It needs only the user's overlay URL. It isn't an officially documented API, so it could change without notice.
- **The official API** is documented, but it's a paid product and works differently from the overlay URL.

**MVP decision:** use the overlay URL method, because it's what the MVP user already relies on and it doesn't need a paid plan. The official API is a separate integration we might add later, but it isn't part of the MVP.

### How the Overlay URL Method Works
**Source:** Mix It Up's `TTSMonsterService.cs` and `TTSMonsterServiceControlViewModel.cs`. We checked two repositories:
- [mixitupapp/Desktop-Archive](https://github.com/mixitupapp/Desktop-Archive), last commit 2025-10-13, MIT licensed.
- [mixitupapp/Desktop](https://github.com/mixitupapp/Desktop), last commit 2026-08-19, under the Mix It Up Business Source License.

Both use the same protocol. The original repository, `SaviorXTanren/mixer-mixitup`, has been taken down. **Implement the method from this description and don't copy Mix It Up's code**, because the current repository isn't open source.

**1. Credentials.** The overlay URL has the form `https://tts.monster/overlay/{userId}/{key}`. Whendo removes the prefix, splits the rest on `/` and requires exactly two parts. The URL is effectively a password, so store it as a secret.

**2. Listing voices.** Send a `POST` to `https://wutface.tts.monster/` with this JSON body:
```json
{ "userId": "<userId>", "apiKey": "<key>" }
```
The response looks like `{ "status": ..., "message": { "plan": { "name": ... }, "voices": [ "<name>", ... ], "customVoices": [ { "name": ... } or "<name>", ... ] } }`.
- Mix It Up lists custom voices first and treats a custom voice as either an object with a `name` or a plain string.
- Mix It Up hides some built-in voices: `diana`, `dagoth`, `tate`, `gordon` and `megan`. The code doesn't say why; it might be content policy or because they're unsupported. We should decide whether to hide the same ones.
- Mix It Up uses this call to validate the URL. If no voices come back, the connection is treated as failed.

**3. Generating speech.** Send a `POST` to `https://us-central1-tts-monster.cloudfunctions.net/generateTTS` with this JSON body:
```json
{ "data": { "userId": "<userId>", "key": "<key>", "message": "<voice>: <text>", "ai": true, "details": { "provider": "mix-it-up" } } }
```
- The voice isn't a separate field. It goes at the start of the message, as `voice: text`.
- Mix It Up sets a 10-second timeout.

**4. Playing the audio.** A successful response has `status` 200, and `data.link` is a URL to the generated audio file. Mix It Up downloads the file, saves it to a temporary folder under the name taken from the URL's path, then plays it on the chosen output device.

### What Is Still Unverified
- **Whether the protocol works:** confirmed in practice. The MVP user's current setup already uses it through Mix It Up. The current Mix It Up code, from 2026-08-19, uses the same protocol as the 2025 archive.
- **Audio format:** the code doesn't say what format the audio file is in; it just uses the URL's filename. Check the file extension and content type in a real response.
- **The `provider` field:** Whendo sends `whendo` rather than pretend to be Mix It Up.
- **If TTS Monster blocks Whendo:** this includes rejecting the `whendo` provider value, rate limits or terms of use. Whendo then can't support the overlay method, and users who want TTS Monster keep using Mix It Up for it. This would be less than ideal but acceptable.

## Testing Redeems Without Channel Points

The maintainer's Twitch account isn't monetized, so it has no channel point redeems to test with. Before redeem support is built, developers need a way to trigger a **simulated redeem** and run automations against it.

Requirements:
- **Simulate the trigger.** A developer can fire a fake redeem event with a chosen reward, redeemer name and user input. It flows through the engine exactly like a real one: queuing, variables, branches and run history.
- **Simulate the redemption actions too.** Refunding and fulfilling a redemption call Twitch's API, which will fail for a reward that doesn't exist. Simulated redemptions need a fake implementation of those actions so automations like Do a Voice can run end to end.
- **Developer use at minimum.** It might later become a user-facing "test this automation" feature, but that isn't required.

This isn't designed yet; work it out when redeem support is built. One option to evaluate then is the [Twitch CLI](https://dev.twitch.tv/docs/cli/), which can send mock EventSub events, including channel point redemptions, to a local mock server.

## Constraints

**Do a Voice must be a Whendo-created redeem.** Twitch only lets an app refund or fulfill a redemption if that app created the reward. If the existing Do a Voice redeem was made in the Twitch dashboard, the user recreates it in Whendo using the Redeems page's recreate option. See [MVP UI Design](mvp-ui.md#redeems).

## Open Questions
- **TTS Monster:** see [What Is Still Unverified](#what-is-still-unverified).
