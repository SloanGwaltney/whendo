# Streaming Software

- **Status:** Draft
- **Last updated:** 2026-11-16

How Whendo works with the streamer's streaming software, and the steps that show and hide sources, which Head Pets needs; see [MVP User Redeems](mvp-user-redeems.md#head-pets). The MVP supports **Streamlabs Desktop**, which our first user streams with; see [Streamlabs Desktop](streamlabs-desktop.md) for how Whendo connects to it. **OBS** comes later; see [OBS](obs.md). For the step contract the steps follow, see [Automation Engine](automation-engine.md#steps).

## One Set of Steps for Every Program

- **Automations don't name the program.** "Show *Head Pets* in *Gameplay*" means the same in Streamlabs Desktop and OBS, so the steps, their setting and the picker belong to Whendo, not to one program. Each program has its own connection behind a small interface, which is all that differs.
- **Why not separate steps per program:** a streamer who moves from Streamlabs to OBS would otherwise have to rebuild every automation. Both programs are built on OBS underneath and share the same idea of scenes, sources and visibility, so one set of steps loses nothing in the MVP.
- **Program-specific steps can come later** for things only one program has, in that program's own group in the step picker.

### The Interface
Each program's connection provides:

- **Its status;** see [Connection Status](#connection-status).
- **The scenes,** each with its ID, its name and its items. An item has its own ID, its source's ID and name, and the name of the folder or group it's in, if any.
- **Showing or hiding one item** in one scene.

Each item carries whatever its program needs to find it again, so the steps never know which program they're talking to. Finding the right items from what an automation stores, including the name fallback below, is shared code rather than written once per program.

## Choosing the Program

The Settings page has a **Streaming software** section with one choice: **None** or **Streamlabs Desktop**. OBS joins the list when it's supported.

- **None is the default.** Streamers who only use chat and sound steps never see a connection status or a nag. Whendo doesn't try to connect to anything until a program is chosen.
- **Choosing Streamlabs Desktop is all the setup there is.** Whendo connects without a token or password; see [Streamlabs Desktop](streamlabs-desktop.md#connecting). The section shows the connection status under the choice, and says there's nothing to set up in Streamlabs, since streamers used to other tools may look for a token.
- **Choosing a program connects straight away,** and choosing None disconnects.

## Connection Status

When a program is chosen, the navigation shows its status under Twitch's, named after the program.

| State | Shown as |
|---|---|
| Connected | "Streamlabs: connected" |
| Waiting | "Streamlabs: waiting… Is Streamlabs Desktop open?" |

- **Waiting covers every failure to connect,** most often the program not being open yet. Each program's connection can add states of its own, such as OBS's wrong password.
- **No separate offline state,** for the same reason as Twitch: until it connects, the steps fail, and that's all the streamer needs to know.

## Show and Hide a Source

`scenes.showSource` and `scenes.hideSource` make a source visible or invisible in one scene, like clicking its eye icon. They sit in the step picker's **Scenes** group and read:

> Show *Head Pets* in *Gameplay*

> Hide *Head Pets* in *Gameplay*

Two steps rather than one with a show or hide setting, like refund and fulfill: each reads as a plain sentence in the picker, and Head Pets uses both.

| Setting | Kind | Notes |
|---|---|---|
| Source | `scenes.sceneSource` | Required. One value holding both the scene and the source; see [Picking a Source](#picking-a-source). |

### Which Source
- **The setting stores the scene's and the source's IDs and names.** Both programs keep IDs when things are renamed, so renaming a scene or a source doesn't break an automation. The names are what the designer shows when the program isn't connected.
- **Every item showing that source in the scene is affected.** A source added to one scene twice is rare, and showing only one of the copies would be a surprise.
- **Falling back to names.** If the scene's ID isn't found, the step looks for a scene with the stored name, and likewise for the source within the scene. That covers a source the streamer deleted and recreated with the same name, which they'd reasonably expect to keep working. Run history still records the step as having worked.
- **Sources in folders or groups** are found like any other source in the scene.
- **Sources in nested scenes** aren't reached through the outer scene. The streamer picks the nested scene itself, since the picker lists every scene.
- **Scene collections:** both programs can switch between scene collections, and only the one loaded is visible to Whendo. A source in another collection is missing until the streamer switches back.

### Picking a Source
- **One picker for both.** The popover lists every scene as a heading with its sources under it, sources in folders indented under their folder. It has a search box, since streamers often have dozens of sources. Picking a source picks its scene too, so there's never a source picked without a scene, or a scene change that has to clear the source.
- **Names come from the program when it's connected.** A renamed scene or source shows its new name in the sentence. The setting keeps the names it was picked with until the source is picked again; they're only used while the program isn't connected, and to find a source whose IDs are gone.
- **Not connected:** the popover says "Open Streamlabs Desktop to pick a source," or "Choose your streaming software in Settings" if none is chosen. The step keeps showing its stored names.

### Good to Know
These are never must-fix; see [Problems](mvp-ui.md#problems).

| When | Note |
|---|---|
| No program is chosen | "Choose your streaming software in Settings, so this step has something to show and hide." |
| A program is chosen but not connected | "Streamlabs Desktop isn't connected, so this step will fail until it is." |
| Connected, and the scene or source is gone, even by name | "“Head Pets” isn't in “Gameplay” any more. Pick the source again." |

Like [missing redeems](twitch-redeems.md#missing-redeems), a source is only called missing when the program says so, never because the list couldn't be loaded. The last list is loaded when the program connects and whenever the picker lists scenes, and dropped when the program disconnects. The designer checks the automation again when the connection or the list changes, so the notes stay current.

### Failures
Each is recorded in run history with the reason:

| Problem | Reason given |
|---|---|
| No program chosen | "No streaming software is chosen. Choose it in Settings." |
| Not connected | "Streamlabs Desktop isn't connected." |
| Scene not found | "The “Gameplay” scene isn't in Streamlabs Desktop any more." |
| Source not in the scene | "“Head Pets” isn't in “Gameplay” any more." |
| No answer within 10 seconds | "Streamlabs Desktop didn't answer." |
| Any other error | The program's own message |

**Already showing isn't a failure.** Showing a source that's already visible, or hiding one that's hidden, does nothing and the step works. The streamer asked for an end state, and it's true.

### Test Runs
The steps really show and hide sources in test runs, as the [test form](mvp-ui.md#test-runs) already says.

### Head Pets and Browser Sources
Hiding and showing a browser source doesn't restart what it shows unless the source's **"Shutdown source when not visible"** option is ticked; both programs have it, from OBS. Without it, Head Pets' animation would carry on from wherever it was, or show nothing if it has finished. The step's description in the picker mentions it: "Tip: for a browser source to start over each time it's shown, tick “Shutdown source when not visible” in its properties." Whether to set it automatically is left until a streamer asks.

## Packages
- `engine/scenes`: the interface, the two steps, the `scenes.sceneSource` setting kind with its checks and good-to-know notes, and finding items from a stored source.
- `engine/streamlabs`: the Streamlabs Desktop connection; see [Streamlabs Desktop](streamlabs-desktop.md#packages).
- `streamingsoftwareservice.go`: the Wails service for the Settings section, the status and the picker.

## Testing
- **The steps:** Go tests against a fake program behind the interface, covering IDs, the name fallback, folders, a source in a scene twice, already showing, and each failure.
- **Each program's connection** has its own tests; see [Streamlabs Desktop](streamlabs-desktop.md#testing).
- **Real use:** the maintainer builds Head Pets in the designer and tests it from end to end against Streamlabs Desktop on Windows.

## First Slice
1. The Streamlabs Desktop connection, with its status; see [Streamlabs Desktop](streamlabs-desktop.md#first-slice).
2. The Streaming software section in Settings and the status in the navigation.
3. The interface, the `scenes.sceneSource` setting kind, its picker and the scene list.
4. The show and hide steps, with their failures and good-to-know notes.
5. Build Head Pets in the designer and test it from end to end.
