# MVP UI Design

- **Status:** Draft
- **Last updated:** 2026-11-14

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
- **Automation list:** every automation the user has, with controls to open it in the designer, enable or disable it, or delete it. Deleting asks for confirmation.
- **Sample gallery:** three or four built-in sample automations, plus a **Blank automation** option. Picking one asks the user for a name, then opens the new automation in the designer.

**Samples that use a redeem:**
- **Monetized accounts:** ask the user to pick one of their existing redeems to use.
- **Unmonetized accounts:** explain that the sample needs channel points, which require a monetized account.
- **In both cases:** never create a redeem on the user's behalf without asking.

### Automation Designer
See [Automation Model](#automation-model) below. The user picks a trigger, then adds steps. Each step reads as part of a sentence.

### Redeems
Lists the user's Twitch channel point redeems. For accounts that aren't monetized, the page is empty and explains why.

To detect monetization, Whendo requests the channel's redeems. For unmonetized channels, Twitch returns an error saying the channel isn't monetized, and Whendo uses that error rather than a separate check.

Twitch only lets an app edit redeems that the same app created, so redeems fall into two groups:
- **Created by Whendo:** fully editable, including enabled or disabled, visible or hidden, cooldown, cost and the other standard redeem settings.
- **Created elsewhere** (for example, in the Twitch dashboard): shown read-only. These can still be used as triggers. The user can choose to **recreate** the redeem in Whendo, which makes it editable. Recreating it gives the redeem a new ID, so the user must confirm first.

### Run History
A table with one row for every time any automation is triggered. Each row shows the automation's name, when it was triggered, its status and, for failures, the reason.

- **Statuses:** queued, running, succeeded or failed.
- **Failures:** a failed run shows which step failed and why.
- **Run details:** opening a run shows each step it executed, in order, with that step's outcome. For steps that split, this includes which branch the run took. This makes it possible to debug runs that succeeded but didn't do what was expected, for example when "wait for a reply" timed out. The table itself stays simple; the step details appear only when a run is opened.
- **Retention:** runs are kept for a set number of days rather than a fixed count. The default is 30 days.

### Settings
- **Twitch:** the logged-in account, with options to log out or reconnect.
- **OBS:** the WebSocket address and password, plus connection status.
- **Audio:** the output device used for sound playback.
- **Run history:** the retention period in days.

## Automation Model

### Structure
An automation has one **trigger** and a sequence of **steps**. MVP triggers include channel point redeems, chat commands and follows.

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
- **The trigger.** A redeem provides the redeemer's name and the text they entered; a follow provides the follower's name.
- **Earlier steps on the same path.** For example, "wait for a reply" provides the reply's text inside its "replied" branch.

**After a split:** when branches rejoin the main sequence, only the variables that *every* branch provides remain available. A variable that exists in only some branches can't be used after the split, because it might not have a value.

### Running and Queuing
- Each automation has its own queue.
- An automation processes one run at a time. A trigger that arrives while the automation is running is queued and runs afterward, in order.
- Different automations run in parallel with each other.

## Open Questions
- **Sample automations:** which three or four samples should ship with the app?