# Whendo Overview

- **Last updated:** 2026-11-15

This is the starting point for understanding Whendo. It explains what the project is and why it exists, and links to the documents that cover each area in detail.

## What Whendo Is

Whendo is a desktop stream automation tool for Twitch streamers, in the same space as Streamer.bot and Mix It Up. It reacts to things that happen on a stream, such as channel point redeems, chat commands and follows, and runs automations in response: posting to chat, playing text-to-speech, showing and hiding OBS sources, and so on.

Every automation reads like a sentence: **when** something happens, **do** these steps. That's where the name comes from.

## Who It's For

Whendo's target user is roughly the average gamer. They browse the web, play games, install software and do basic troubleshooting without trouble. They're comfortable with computers.

What they aren't is programmers. They don't naturally break a problem into triggers, actions and sub-actions, and they don't know or care how software works internally. "Less technical" in these docs means exactly that. It doesn't mean someone who struggles to use a computer, and Whendo's UI should never talk down to its users.

A surprisingly large share of streamers fit this description.

## The Problem

Streamer.bot and Mix It Up are powerful, but their interfaces are designed around their own engine internals rather than around what makes sense to an average user. Building an automation means working directly with concepts like actions, sub-actions, arguments and variables.

That's largely a deliberate choice: exposing the engine's internals gives users as much flexibility as possible. Breaking automation down into triggers, actions and nested sub-actions is exactly how a programmer would model the problem, and how they'd build an engine for it. But showing that model directly to users imposes a steep learning curve on anyone who doesn't think that way, and that's a large share of streamers. Whendo aims to be far more accessible to them.

Streamers also often end up chaining several tools together. For example, our first user's text-to-speech currently runs from a Twitch redeem to Streamer.bot, then through a webhook to Mix It Up, then to TTS Monster. Each extra tool is one more thing to install, configure and keep running.

## The Approach

**Fewer features, much easier to use.** Whendo deliberately doesn't try to match Streamer.bot or Mix It Up feature for feature. It aims to cover the roughly 90% of automations most streamers actually need, with an interface a non-technical streamer can use without help.

**Run alongside, not replace.** For the remaining 10%, Whendo is meant to run comfortably alongside tools like Streamer.bot rather than compete with them.

**Key design choices:**
- **Automations read as sentences.** A trigger is followed by steps. Some steps split into outcomes, such as "replied" or "timed out," and each outcome has its own branch. A branch either ends the automation or continues with the step after the split, so automations never turn into flowcharts.
- **No variables to manage.** The values available at each step, such as the redeemer's name or a dice roll, are determined automatically. The user picks one by name to insert it into a sentence.
- **Predictable runs.** Each automation runs one at a time, and new triggers queue behind the current run. Different automations run in parallel.
- **Simple failures.** If a step fails, the run ends and run history shows which step failed and why.
- **Test runs.** Any automation can be run by hand with made-up values, to check it before going live. It runs exactly like a real trigger.
- **Nothing changes in the user's Twitch account without their agreement.**

See [MVP UI Design](design/mvp-ui.md#automation-model) for the full automation model.

## The MVP

The MVP is scoped to fully supporting the channel point redeems of our first user. Broader features come after that.

That means:
- **Twitch** login, with triggers for redeems, chat commands and follows.
- **Chat messages** with variables inserted.
- **OBS** control through the OBS WebSocket protocol.
- **Audio playback** to a chosen output device.
- **TTS Monster** text-to-speech, replacing the user's current Streamer.bot and Mix It Up chain.
- Screens for automations, redeems, run history and settings.

## Tech Stack

- **Backend:** Go.
- **Desktop shell:** Wails v3, using the operating system's native web view.
- **Frontend:** TypeScript and Svelte 5, built with Vite. No SvelteKit.
- **Platforms:** Windows, macOS and Linux. Windows is the priority, because we expect 80–90% of users to be on it.
- **Architecture:** the automation engine is plain Go with no dependency on Wails. Wails is a thin layer on top of it.

## Documents

### Project Guidelines
- **[AGENTS.md](../AGENTS.md):** coding practices, naming rules, the tech stack and architecture rules. Read it before writing any code.

### Architecture Decisions
- **[ADR 0001: Go and Wails v3](adr/0001-go-and-wails-for-desktop-app.md):** why we chose Go and Wails over Electron and Tauri, and the risks we accepted.
- **[ADR 0002: Svelte 5](adr/0002-svelte-for-frontend.md):** why we chose Svelte over plain TypeScript, React, Vue, Astro and HTMX, including the cost of a smaller ecosystem.
- **[ADR 0003: go-keyring](adr/0003-go-keyring-for-credential-storage.md):** why we store secrets in the operating system's credential store through go-keyring rather than our own platform bindings.

### Design
- **[MVP UI Design](design/mvp-ui.md):** the screens, navigation, and the automation model (branching, failures, variables and queuing).
- **[MVP User Redeems](design/mvp-user-redeems.md):** our first user's redeems, the capabilities the MVP needs to support them, and how the TTS Monster integration works.
- **[Twitch Login](design/twitch-login.md):** the device code flow, scopes, how tokens are refreshed and validated, and where they're stored.

## Open Questions
- **Sample automations:** which three or four samples should ship with the app? See [MVP UI Design](design/mvp-ui.md#open-questions).
- **Chat message matching:** whether chat triggers ignore capitalization, and whether `!a1` matches `!a10`. See [MVP UI Design](design/mvp-ui.md#open-questions).
- **TTS Monster:** the audio format, and whether TTS Monster accepts Whendo using the overlay method. See [MVP User Redeems](design/mvp-user-redeems.md#what-is-still-unverified).
