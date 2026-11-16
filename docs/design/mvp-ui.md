# MVP UI Design

- **Status:** Draft
- **Last updated:** 2026-11-16

This document describes the screens and automation model for Whendo's MVP. For why we chose this stack, see the ADRs in [docs/adr](../adr). For the redeems the MVP must support, see [MVP User Redeems](mvp-user-redeems.md).

## Guiding Principles
- **The MVP user's use case comes first.** The MVP is scoped to fully supporting our first user's redeems; see [MVP User Redeems](mvp-user-redeems.md). Broader features come after that.
- **Designed for the average streamer.** When a choice is between flexibility and simplicity, choose simplicity.
- **Never create or change anything in the user's Twitch account without their explicit agreement.**

## Screens

### Navigation
A persistent navigation menu links to Home, Redeems, Run History and Settings. It also shows the connection status for Twitch and OBS, so users can see at a glance whether automations will be able to run.

### First Launch
Until the user logs in with Twitch, the main page shows a Twitch login prompt and nothing else. Without Twitch, there's nothing for Whendo to do.

### Home
Once the user is logged in:
- **Automation list:** every automation the user has, with controls to open it in the designer, enable or disable it, test it (see [Test Runs](#test-runs)), or delete it. Deleting asks for confirmation.
- **Sample gallery:** three or four built-in sample automations, plus a **Blank automation** option. Picking one asks the user for a name, then opens the new automation in the designer.

**Samples that use a redeem:**
- **Monetized accounts:** ask the user to pick one of their existing redeems to use.
- **Unmonetized accounts:** explain that the sample needs channel points, which require a monetized account.
- **In both cases:** never create a redeem on the user's behalf without asking.

### Automation Designer
The designer shows an automation as sentences: **When** the trigger happens, **do** a list of steps. For what the sentences describe, see [Automation Model](#automation-model) below. A clickable prototype of this design was well received by our first user.

- **Trigger:** shown at the top in its own card, with the values it provides listed underneath. The common values show first and the rest sit behind a "more" link.
- **Editing values:** every setting is a highlighted value inside its sentence. Clicking a value opens a small popover under it for editing. A popover keeps the sentence in view, so editing feels like finishing a sentence rather than filling in a form. Message editors get wider on larger screens. Whendo assumes at least an HD monitor and accepts some crowding below that, and the window has a minimum size of 1024×640.
- **Messages:** text with values inserted at the cursor. Each value shows as a pill that's deleted like a single character. Chat messages show their length against Twitch's 500-character limit.
- **Adding steps:** "Add a step" at the end of every list, plus an insert point between steps. Both open a searchable picker that groups steps by what they act on, such as Chat, OBS or Sound.
- **Moving steps:** Move up and Move down in each step's menu, within its own list. Steps can't be moved into or out of a path. We'll add more only if streamers show a need these don't meet.
- **Paths:** each outcome of a step that splits has its own indented path, labelled as a sentence, such as "If they type `!a1`." Each path ends with "stop here" or "carry on below," and new paths carry on below. A path with no steps takes one line. Paths are never merged, so every outcome keeps its own place to add steps.
- **Numbering:** only top-level steps are numbered. Numbers across paths would suggest an order between steps that never both run.
- **Read it back:** a toggle shows the whole automation as plain text, for reviewing it before going live.
- **Saving:** there's an explicit Save button. A saved edit applies to the next run straight away, so saving automatically could make a half-finished automation fire during a stream.

#### Problems
The designer lists problems above the steps and marks each step that has one. Clicking a problem jumps to its step.

- **Must fix:** problems with the automation itself, for example:
  - a required value is missing
  - a step uses a value that isn't available on every path that reaches it
  - a chat message is over Twitch's limit
  - a step that acts on a redemption is used without a redeem trigger
  - a list of choices has a blank or repeated choice
- **Good to know:** problems that don't mean the automation is wrong, for example:
  - OBS isn't connected
  - the redeem wasn't created by Whendo, so refunding or fulfilling it will fail
  - steps can never run because every path above them stops
- **Problems outside Whendo are never must-fix.** Twitch or OBS being down never makes an automation broken. In particular, a redeem trigger is marked as using a missing redeem only when Twitch confirms that the redeem is gone, not when the list of redeems can't be loaded.
- **Broken automations can be saved,** so unfinished work is never lost. A broken automation is always saved turned off, and it can't be turned on or test run until its must-fix problems are fixed. Saving a broken edit to an automation that's on turns it off and tells the user so. Home marks broken automations as needing fixing.

### Redeems
Lists the user's Twitch channel point redeems. For accounts that aren't monetized, the page is empty and explains why.

To detect monetization, Whendo requests the channel's redeems. For unmonetized channels, Twitch returns an error saying the channel isn't monetized, and Whendo uses that error rather than a separate check.

Twitch only lets an app edit redeems that the same app created, so redeems fall into two groups:
- **Created by Whendo:** fully editable, including enabled or disabled, visible or hidden, cooldown, cost and the other standard redeem settings.
- **Created elsewhere** (for example, in the Twitch dashboard): shown read-only. These can still be used as triggers. The user can choose to **recreate** the redeem in Whendo, which makes it editable. Recreating it gives the redeem a new ID, so the user must confirm first.

### Run History
A table with one row for every time any automation is triggered. Each row shows the automation's name, when it was triggered, its status and, for failures, the reason.

- **Statuses:** queued, running, succeeded, failed or cancelled. Cancelled runs were removed from the queue when their automation was disabled or deleted, or were interrupted by Whendo shutting down. Test runs are marked as tests.
- **Failures:** a failed run shows which step failed and why.
- **Run details:** opening a run shows each step it executed, in order, with that step's outcome. For steps that split, this includes which branch the run took. This makes it possible to debug runs that succeeded but didn't do what was expected, for example when "wait for a reply" timed out. The table itself stays simple; the step details appear only when a run is opened.
- **Retention:** runs are kept for a set number of days rather than a fixed count. The default is 30 days.

### Settings
- **Twitch:** the logged-in account, with options to log out or reconnect.
- **OBS:** the WebSocket address and password, plus connection status.
- **Audio:** the output device used for sound playback.
- **Run history:** the retention period in days.
- **Theme:** the colour theme; see [Look and Feel](#look-and-feel).

## Look and Feel
- **Hand-written CSS, no component library.** The trickiest parts, the value popover and the message editor with value pills, aren't something component libraries cover well. Libraries also bring their own theming systems, which would fight ours. Avoiding one also follows the dependency policy in [AGENTS.md](../../AGENTS.md#dependencies).
- **Every colour is a theme variable.** Components never use a literal colour. A theme is one set of values for those variables, applied by an attribute on the page, so switching themes is instant and adding one is just another set of values.
- **Themes are presets.** Users pick a whole theme rather than individual colours, because each preset is checked for readability once. Separate colour choices could make text unreadable, and the problem, value and status colours would have to stay readable on any background.
- **The MVP ships two themes:** a neutral dark default and a violet theme, which our first user asked for. The violet is our own, not Twitch's brand purple, so Whendo never looks like an official Twitch product. More presets, or a choice of accent colour, can come later.

## Automation Model

### Structure
An automation has one **trigger** and a sequence of **steps**. MVP triggers are channel point redeems, follows, and chat messages that start with given text, such as `!discord`. That last trigger is how users make chat commands: Whendo doesn't define what a command is, but its description says it's how to make one, so users looking for "commands" find it.

A chat message only matches when it **starts with** the text. A match anywhere in the message would let Whendo react to its own posts, for example when Do a Voice's prompt lists `!a1` and the next step is waiting for `!a1`. Whendo doesn't try to prevent every loop a user could build, since that would mean analyzing every combination of steps, but this rule removes the common case.

Matching ignores capitalization and compares whole words, so `!a1` matches "!a1 please" but not "!a10." Only messages from the streamer's own chat count, not other channels in a shared chat session. See [Twitch EventSub](twitch-eventsub.md#chat-messages) for the details.

The filter lives in the trigger rather than in a step. A trigger for every chat message followed by an "if it starts with" step would create a run for every message, so real runs would queue behind runs that do nothing and run history would fill with noise.

### Branching
- Most steps have a single outcome, and the automation simply continues to the next step.
- Some steps have multiple **outcomes**. For example, "wait for a reply" has "replied" and "timed out."
- Each outcome contains its own nested sequence of steps, which forms a **branch**. Branches can nest.
- When a branch finishes, it either **ends the automation** or **continues with the step after the split**.
- A branch can't jump anywhere else. This keeps every automation readable as a sentence rather than a flowchart.

### Failures
If a step fails, for example because OBS is disconnected or the audio device is missing, the run ends and is recorded in run history as failed, with the reason. Failures don't create branches.

A future version might add a "when an automation fails" trigger so users can handle failures themselves.

### Variables
Users never create or name variables themselves. Every variable available at a given step is determined automatically, and the user picks one by name to insert it into a sentence, as in "say thanks, *redeemer name*."

Variables come from:
- **The trigger.** Each trigger declares up front which variables it provides, and everything in the event is made available, including IDs. The designer shows the common ones first, such as the redeemer's name and the text they entered. See [Twitch EventSub](twitch-eventsub.md#trigger-variables) for each Twitch trigger's variables.
- **Earlier steps on the same path.** For example, "wait for a reply" provides the reply's text inside its "replied" branch.

**After a split:** when branches rejoin the main sequence, only the variables that *every* branch provides remain available. A variable that exists in only some branches can't be used after the split, because it might not have a value.

### Running and Queuing
- Each automation has its own queue.
- An automation processes one run at a time. A trigger that arrives while the automation is running is queued and runs afterward, in order.
- Different automations run in parallel with each other.
- Disabling an automation empties its queue. A run already in progress finishes.
- Edits apply right away: each run uses the automation as it is when the run starts.

See [Automation Engine](automation-engine.md#runs-and-queues) for the details.

### Test Runs
Any automation can be run by hand with made-up trigger values. Streamers use this to check an automation before going live, and developers use it to test without real Twitch events, such as redeems on an account without channel points.

- **The form comes from the trigger.** The test form asks for a value for each variable the trigger declares, prefilled with sensible defaults such as the streamer's own name, so a quick check is one click.
- **Steps really happen.** A test run posts real chat messages, really shows and hides OBS sources and plays real sound. The test form says so.
- **Steps that act on the redemption itself fail.** Refunding or fulfilling a redemption needs a real redemption from Twitch, so in a test run those steps fail like any other failed step. Run history still shows that everything before them worked.
- **Test runs use the normal queue,** so they behave exactly like real runs, including waiting behind a run in progress.
- **Disabled automations can be tested,** so an automation can be checked before it's turned on.
- **Run history marks test runs,** so they aren't mistaken for real activity.

In the engine, a test run fires an existing trigger with values from the form instead of from Twitch. It isn't a source with triggers of its own. The engine runs it exactly as it would a real event; the only difference is the test mark in run history. Test runs don't check the connection to Twitch, since they start after it.

## Open Questions
- **Sample automations:** which three or four samples should ship with the app?
