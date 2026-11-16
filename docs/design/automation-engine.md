# Automation Engine

- **Status:** Draft
- **Last updated:** 2026-11-16

How the engine turns trigger events into runs: matching events to automations, the contracts for triggers and steps, variables, queuing, how automations are saved and how run history is kept. This is plain Go with no dependency on Wails. For the user-facing model this implements, see [MVP UI Design](mvp-ui.md#automation-model). For where Twitch events come from, see [Twitch EventSub](twitch-eventsub.md).

## Overview

1. An integration, such as the Twitch EventSub listener, delivers a `trigger.Event` to the engine.
2. The engine gives the event to every **step that's waiting for events**, such as "wait for a chat message."
3. The engine checks the event against the trigger of every **enabled automation**. The trigger's settings act as its filter.
4. Each match becomes a **run**, added to that automation's queue.
5. Each automation runs its queue one run at a time, and different automations run in parallel.
6. When a run ends, the engine writes it to **run history**.

Delivering an event never waits on a run. The listener has to keep reading its connection, so the engine only matches and queues the event before returning.

## Settings

Triggers and steps both need settings, such as which reward a redeem trigger uses or how long a wait step waits. They share one contract, so the designer can render both the same way.

- **A setting definition** has an ID, a display name, a kind, and whether it's required.
- **The kind** tells the designer which control to show and the engine how to read the value. Kinds are named like trigger IDs, so an integration can add its own without colliding with the engine's.
- **MVP kinds:**

| Kind | Value | Used by |
|---|---|---|
| `text` | Plain text | Chat message trigger's "starts with" |
| `template` | Text with variables inserted; see [Templates](#templates) | Send a chat message |
| `wholeNumber` | An integer | Random number's lowest and highest |
| `duration` | A length of time, stored in milliseconds, with an optional maximum | Wait, wait for a chat message |
| `variable` | A reference to one variable | Check a condition |
| `option` | One of a fixed list the definition provides | Check a condition's comparison, wait for a chat message's "from" |
| `choiceList` | A list of texts, each with a stable ID | Wait for a chat message's choices |
| `twitch.reward` | A channel point reward ID | Redeem trigger |

## Triggers

`trigger.Definition` gains settings, and each trigger gains a **filter**: a function that takes an event and an automation's settings for that trigger and says whether the event matches.

- **The filter can add variables.** The chat message trigger provides "the rest of the message" after the matched text, which depends on the setting, so only the filter can work it out.
- **Redeem:** one setting, the reward. It matches when the event's reward ID equals it.
- **Chat message:** one setting, the text the message must start with. It matches using the rules in [Twitch EventSub](twitch-eventsub.md#chat-messages) and adds the rest of the message.
- **Follow:** no settings. Every follow matches.
- **The person who started it:** a trigger can name the variable holding the user ID of the person behind its event, such as the redeemer. The engine passes that ID to every step in the run, so "wait for a chat message" can wait for that person without knowing which trigger it follows. See [Twitch Steps](twitch-steps.md#who-can-answer).
- **Filters live with the integration,** next to its triggers, for the same reason the triggers do: the engine doesn't know what a reward is.

## Steps

A step definition mirrors a trigger definition:

- **ID and display name,** with the ID prefixed by where it comes from: `core.wait`, `twitch.sendChatMessage`.
- **Settings,** as above.
- **Outcomes,** each with an ID, a display name and the variables it provides. Most steps have one outcome. Outcomes can depend on settings: "wait for a chat message" has one outcome per choice plus "timed out," so the definition provides a function from settings to outcomes rather than a fixed list.
- **Running** takes a context and the step's settings with variables already resolved. It returns the outcome taken and the values of that outcome's variables, or an error. An error fails the run; see [Failures](mvp-ui.md#failures).
- **Required trigger,** optional, for steps that act on the event that started the automation, such as refunding a redemption. Using the step after any other trigger is a must-fix problem.
- **Details** are optional notes a step returns about what it did, such as the chat message it sent, kept only for run history. Unlike variables, later steps can't use them, so the designer doesn't offer them. They're kept even when the step fails, so run history shows what it tried.

The context is cancelled when Whendo shuts down, so a step that waits must stop when it is.

### Steps That Provide Variables
Steps provide variables through their outcomes. "Roll a random number" has one outcome that provides the roll. "Wait for a chat message" provides the sender and the message in each choice outcome, and nothing in "timed out."

Steps that let users create and set their own variables aren't part of the MVP.

### Steps That Wait for Events
A step can **subscribe** to events, the same way a trigger receives them. "Wait for a chat message" subscribes to chat message events when it starts, checks each one against its choices and who it's waiting for, and unsubscribes when it finishes or times out.

- **Subscriptions are in the engine, not the listener.** Every integration already delivers its events to the engine, so this is one place to subscribe that works for any integration, and the step never touches the Twitch connection.
- **An event can do both.** A chat message that answers a waiting step can also match another automation's trigger. Both happen. Deciding which one "wins" would be surprising either way, and a user who builds that overlap can see it in run history.
- **Delivery never waits.** Each subscription has a small buffer. If a step falls so far behind that its buffer fills, further events are dropped for that step and logged. Only a bug could cause that, since checking a chat message takes microseconds.

### MVP Core Steps
Steps that need no integration live in the engine:

- **Wait:** one `duration` setting.
- **Roll a random number:** `wholeNumber` settings for the lowest and highest values, both included. Provides the roll.
- **Check a condition:** a `variable`, a comparison, and a `template` to compare against. Comparisons are "is," "is not," "is more than" and "is less than." Text comparisons ignore capitalization. "More than" and "less than" compare numbers, and fail the step if either side isn't a number. Two outcomes: yes and no.

Twitch steps (send a chat message, wait for a chat message, refund and fulfill) and the streaming software and audio steps are designed with their integrations.

## Variables

### References
A setting refers to a variable by where it comes from and its ID, never by its display name:

- **From the trigger:** `{"source": "trigger", "id": "redeemerDisplayName"}`
- **From a step:** `{"source": "step", "stepId": "3f9c…", "id": "roll"}`

Every step in a saved automation has its own ID, so two "roll a random number" steps don't conflict, and renaming a variable's display name never breaks an automation.

### Templates
A template is stored as a list of parts rather than text with placeholders, so nothing a user types needs escaping:

```json
[
  { "text": "Thanks for checking in, " },
  { "variable": { "source": "trigger", "id": "redeemerDisplayName" } },
  { "text": "!" }
]
```

### Which Variables Are Available
One engine function answers "which variables can this step use?" The designer uses it to show the choices, and the engine uses it to check automations before saving and running them.

- **The trigger's variables** are available everywhere.
- **A step's variables** are available to later steps on the same path, including inside the branch for the outcome that provides them.
- **After a split,** a variable is available only if every branch that **continues** provides it. A branch that ends the automation never reaches the steps after the split, so what it lacks doesn't matter. In Do a Voice, the reply is available after "wait for a chat message" even though "timed out" lacks it, as long as "timed out" ends the automation.

If a reference points to a variable that isn't available, for example because the step it came from was deleted, the automation is **invalid**; see [Invalid Automations](#invalid-automations).

## Runs and Queues

### Queuing
- **Each automation has its own queue,** whether it's enabled or not, so test runs of disabled automations behave like real runs.
- **The queue holds up to 2,048 runs.** A trigger that arrives when the queue is full is recorded in run history as failed, with the reason that too many runs were waiting. The limit is a constant for now and might become a setting later.

### Disabling and Deleting
- **Disabling an automation empties its queue.** Each queued run is recorded in run history as cancelled. This is the safest choice and the easiest to change if it turns out to be wrong.
- **A run already in progress finishes.** Stopping one partway through could leave things half done, like a source that's shown but never hidden in Head Pets.
- **Deleting an automation** does the same as disabling it, then removes it.
- **Shutting down stops runs in progress.** A waiting step stops right away, so quitting partway through Head Pets leaves the source showing. That's accepted: avoiding it would mean steps that undo themselves, which is a lot of work for a rare case the streamer can fix by hand.

**Later, not in the MVP:** a step that waits, such as "wait for a chat message," holds up its automation's queue for as long as it waits, which is why that step is capped at 5 minutes. Revisit whether this model still holds up with waits, and in particular whether a waiting run should step aside so later runs can start.

### Editing
**Edits apply right away.** A run uses the automation as it is when the run **starts**, so queued runs pick up an edit and the run in progress doesn't. A streamer who doesn't want an automation firing while they edit it disables it first.

### Statuses
A run is **queued**, **running**, **succeeded**, **failed** or **cancelled**. Cancelled is new to the MVP UI's list and covers runs removed from the queue by disabling or deleting, and runs interrupted by Whendo shutting down.

### Test Runs
A test run targets one automation directly, enabled or not, and joins its queue like any other run.

Every trigger supplies starting values for the test form, given the automation's settings and the streamer's account, and they must pass its own filter. Only the trigger knows its filter, so this is the one place that can guarantee a form that works as it is.

The test form's values still go through the trigger's filter, so the filter can add its variables, such as the rest of the message. If they don't match, the test run is refused. A trigger that can explain why gives the reason, for example "The message has to start with “!discord”"; others get a general one. A test that a real event would never trigger would be misleading.

Starting a test run returns the run's ID, so the designer can find its record when it ends and show what each step did.

## Saving Automations

**Users create every automation in the designer.** Whendo's code defines triggers and steps, the building blocks, but never an automation. The MVP user's redeems, such as D20 Mute Me, are built in the designer like anyone else's, not written into the app.

Each automation is one JSON file at `<user config folder>/whendo/automations/<automation ID>.json`, which is `%AppData%\whendo\automations` on Windows. The file is named by ID rather than by name, so renaming an automation doesn't move its file.

```json
{
  "formatVersion": 1,
  "id": "8b1e…",
  "name": "D20 Mute Me",
  "isEnabled": true,
  "trigger": {
    "id": "twitch.redeem",
    "settings": { "reward": "c7a2…" }
  },
  "steps": [
    {
      "id": "3f9c…",
      "type": "core.randomNumber",
      "settings": { "lowest": 1, "highest": 20 }
    },
    {
      "id": "a41d…",
      "type": "core.checkCondition",
      "settings": {
        "variable": { "source": "step", "stepId": "3f9c…", "id": "roll" },
        "comparison": "is",
        "value": [{ "text": "20" }]
      },
      "outcomes": {
        "yes": { "steps": [ … ], "afterBranch": "end" },
        "no":  { "steps": [ … ], "afterBranch": "end" }
      }
    }
  ]
}
```

- **`formatVersion`** lets a later version of Whendo recognize and upgrade older files.
- **`afterBranch`** is either `end` or `continue`; see [Branching](mvp-ui.md#branching).
- **Automation IDs** are random, made by `crypto/rand.Text`. No dependency is needed. Because an ID names a file, Whendo only accepts IDs of up to 64 letters, digits, hyphens and underscores, so an ID can never point outside the automations folder.
- **Step and choice IDs** only need to be unique within their automation or setting, so the designer can make them itself.
- **A file whose name doesn't match the ID inside it** can't be loaded. Saving it again would otherwise create a second copy under the right name.
- **Writes are atomic.** Whendo writes to a temporary file in the same folder and renames it over the old one, so a crash mid-write never leaves a half-written automation.
- **Automations are read once at startup.** Editing the files by hand while Whendo runs isn't supported.

### Invalid Automations
An automation is invalid when its file can't be read, it refers to a trigger, step or variable that doesn't exist, or a required setting is missing. Whendo never runs an invalid automation. It still lists it with the reason, so the user can fix or delete it rather than have it silently disappear.

Saving checks the same rules, but still saves an invalid automation, so unfinished work in the designer is never lost. An invalid automation is always saved turned off, and turning it on is refused until it's valid. Saving an invalid version of an automation that's on turns it off. See [Problems](mvp-ui.md#problems).

- **The ID is the exception.** An automation whose ID isn't safe as a file name is never saved, because its file could end up outside the automations folder.
- **The runner keeps the last valid version, turned off.** When an automation that was valid is saved invalid, the runner gets its last valid version with the automation turned off. That cancels the queued runs and lets a run in progress finish, the same as turning it off. Removing it from the runner instead would let a fixed version start a second run while the old one was still finishing.

### Checking Automations
The designer checks an automation while it's being built, so it can list problems and offer the right values at each step. **The rules live only in the engine.** The designer sends its draft to one function and shows what comes back. A second copy of the rules in TypeScript would slowly drift from the real ones.

The check returns:
- **Every problem,** not just the first. Each has a level, the step and setting it belongs to (or that it's about the trigger or the whole automation), and a message written for the streamer, such as "“Send a chat message” needs you to write a message."
- **For each step:** the variables it can use, its outcomes for its current settings, and whether a run can reach it.

**Levels:**
- **Must fix:** anything that makes the automation invalid, as above. Also a setting with a maximum length, such as a chat message, whose own text is already over it before any variables are inserted, and a list of choices with the same choice twice.
- **Good to know:** steps that can never run because every branch of a split before them ends the automation. Whendo used to reject these. Now they're allowed, since they're harmless and often appear midway through an edit. Only the first such step in each sequence is reported, so a long tail of steps produces one problem rather than many.

**While the automation is unfinished:**
- **With no trigger, or an unknown one,** references to trigger variables aren't checked, because every one of them would be a problem and none would help.
- **A step of an unknown type** is a problem, and its branches aren't checked, since its outcomes are unknown.
- **Branches must match outcomes exactly,** even midway through an edit. When a step's outcomes change, such as a choice being added, the designer adds or removes branches using the outcomes the check returns.

**Triggers can add notes** about their own settings, which the check reports as good to know, such as a [placeholder redeem](twitch-redeems.md#the-placeholder-redeem). They're never must-fix, since rules that make settings invalid belong in the setting definitions, and they're left out while the trigger's settings have problems of their own.

**Steps can add notes** the same way, about how they're used: their own settings and the automation's trigger. Refund and fulfill use this to point out a redeem whose redemptions Twitch won't let Whendo change; see [Twitch Steps](twitch-steps.md#refund-and-fulfill). They're left out while the step's settings have problems, or when the step needs another trigger.

Problems that depend on the world outside the automation, such as Streamlabs Desktop not being connected, aren't part of the check. They belong to the integrations, and will come with the first steps that need them.

### What Definitions Tell the Designer
Trigger and step definitions carry the text the designer shows, so an integration that adds a step also says how it reads:

- **Sentence:** a list of parts, each either fixed words or a setting, such as `"Say in chat"` followed by the message. Every setting appears in the sentence exactly once.
- **Description:** one line for the trigger or step picker.
- **Group,** for steps: what the step acts on, such as "Chat" or "Timing," which groups the step picker.
- **Placeholder,** for settings: what an empty value says, written as an instruction, such as "pick a redeem." Problems reuse it: "needs you to pick a redeem."
- **Maximum length,** for text and template settings, where the integration has one.
- **Common,** for variables: whether the designer shows it first, before a "more" link.
- **Outcome sentences:** each outcome of a step that splits has a sentence that labels its branch, such as "If so." It can include text the user wrote, so "wait for a chat message" can label a branch "If they type `!a1`."

## Run History

Run history is stored in files, one per day, at `<user config folder>/whendo/run-history/<YYYY-MM-DD>.jsonl`. Each line is one finished run.

- **A run is written once, when it ends.** Queued and running runs are kept in memory and shown from there. A run in progress when Whendo crashes is lost from history. That's acceptable for the MVP, and much simpler than updating records in place.
- **Each record holds** the automation's ID and its name at the time, whether it was a test, the trigger's ID and variables, when it was queued, started and ended, its status, and each step it ran with its outcome or failure reason.
- **Records keep their labels as text.** Each step keeps its name, the values it provided to later steps with their names, such as the number a roll landed on, any details it reported about what it did, such as the chat message it sent, and, for a step that splits, the label of the branch it took, such as "If they type !a1." That label can depend on settings edited later, or on an automation since deleted, so looking it up when history is shown could be wrong or impossible.
- **The day is the day the run ended,** in local time, since that's what the streamer will look for.
- **A line a crash cut short** is skipped when reading, and the next write starts on a new line, so the damage never spreads past that one record.
- **Retention** deletes files older than the retention period, 30 days by default, which the user can set from 1 to 365 days in Settings. It runs at startup and once a day after that, and straight away when the user changes the period, so a shorter one takes effect at once.
- **No database.** Run history is written once and read newest first, a day at a time. Files do that with the standard library alone. SQLite would be another dependency, and its strengths, such as queries and updates, aren't needed here.

## Packages

| Package | Contains |
|---|---|
| `engine/automationsetting` | Setting definitions, kinds, variable references and templates. Shared by triggers and steps. |
| `engine/trigger` | The trigger contract, now with settings and filters. |
| `engine/step` | The step contract. |
| `engine/automation` | The automation model, the file format, validation and which variables are available. |
| `engine/runhistory` | Writing, reading and pruning run history files. |
| `engine/appsettings` | The settings file, `<user config folder>/whendo/settings.json`, holding the user's choices from the Settings page. |
| `engine/runner` | Receiving events, subscriptions, matching, queues and running steps. It implements `trigger.Receiver`. |
| `engine/coresteps` | Wait, roll a random number and check a condition. |

The Wails layer gets a service that exposes automations, test runs and run history to the UI, and replaces the logging receiver with the runner.

## Testing

- **The runner** is tested with fake triggers and steps, so tests control exactly when events arrive and steps finish. They cover matching, queuing order, a full queue, disabling, edits between runs, branching and subscriptions.
- **Variable availability** is tested on automations the tests build as test data, including splits where some branches end.
- **Saving and run history** are tested in a temporary folder.
- **From end to end,** test runs drive real automations without Twitch events.

## First Slice

1. The settings, trigger and step contracts, and trigger settings and filters for the Twitch triggers.
2. The automation model, validation and variable availability.
3. The runner, with queues and subscriptions.
4. Run history.
5. The core steps.

Until the designer exists, only tests drive the engine, using automations they build as test data. Nothing in the app creates automations of its own. After the first slice, "send a chat message" comes next, which gives the designer what it needs to build Check In and D20 Mute Me.
