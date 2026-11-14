# ADR 0001: Use Go and Wails v3 for the Desktop App

- **Status:** Accepted
- **Date:** 2026-11-14

## Context

Whendo is a desktop stream automation tool for streamers, many of whom aren't technical. It runs during live streams, next to OBS and usually a game, so it must stay light on memory and CPU. Its main differentiator is an easy-to-use UI, which is best built with web technology.

Platform requirements:
- Cross-platform support (Windows, macOS, Linux) is required.
- Windows is expected to be 80–90% of users and gets the most effort.

Integrations needed at launch are Twitch, OBS (through OBS WebSocket) and audio playback to a chosen output device. Global hotkeys and local text-to-speech are wanted but not essential. Much of the engine will be event-driven and concurrent.

We considered three desktop stacks:

1. **Electron** (TypeScript)
2. **Tauri** (Rust)
3. **Wails v3** (Go)

## Decision

Build Whendo with **Go** and **Wails v3**, with a **TypeScript** frontend running in the operating system's native web view.

Keep the automation engine independent of Wails. The engine is plain Go with a public API that doesn't reference Wails, and Wails is a thin layer on top of it.

## Rationale

### Why not Electron
Electron bundles its own browser with every app. That costs a lot of memory and disk, and none of it is shared with other apps. Native web views avoid bundling a browser. On Windows, WebView2 is a single runtime installed system-wide and shared by every app that uses it. For an app that runs alongside OBS and a game during a stream, this footprint rules Electron out.

### Why Go over Rust (Wails over Tauri)
Tauri is better funded and has broader support, and Wails took a long time to ship v3. Rust's compile-time guarantees are a real advantage for concurrency-heavy code. We still chose Go, for these reasons:

- **Review cost.** Even with AI writing much of the code, Rust would take noticeably more of the maintainer's time to review than Go. Review time is the actual bottleneck.
- **Concurrency.** Goroutines and channels fit an event-driven automation engine well. We're confident we can design concurrency safely without Rust's borrow checker, and we'll back that up with Go's race detector.
- **Performance needs are modest.** Whendo's workload isn't heavy enough for garbage-collection pauses to matter, and Go compiles ahead of time, so there's no JIT warm-up. Rust's performance advantage wouldn't make a noticeable difference here.
- **Wails v3 is now viable.** It has shipped with system tray support, distribution and auto-updates, which were the main gaps compared with Tauri.

## Consequences

### Positive
- Small resource footprint during streams.
- Code that is faster to review and maintain.
- Concurrency that fits the event-driven engine.
- Because the engine is separate from Wails, the desktop shell could be replaced without rewriting the engine if Wails stalls.

### Negative and Accepted Risks
- **Smaller ecosystem than Tauri.** Wails has less funding and fewer contributors, and has historically depended on a small number of maintainers. We reduce this risk by keeping the engine independent of Wails.
- **cgo is required.** Some system integrations (audio, hotkeys, platform APIs) won't have adequate pure Go libraries. The race detector also needs cgo. Builds and CI need a C toolchain on every platform.
- **Web view differences between platforms.** WebView2 (Windows), WebKit (macOS) and WebKitGTK (Linux) render differently. Windows gets tested first; the other platforms will need extra UI testing.
- **Native Windows APIs are clunkier from Go than from Rust.** For example, text-to-speech goes through COM. This is acceptable because text-to-speech isn't essential at launch.
- **Antivirus and SmartScreen warnings.** Unsigned or newly signed executables trigger warnings until they build a reputation. This affects every option equally, so it didn't influence the decision, but signing and reputation-building should be planned early.

### Out of Scope for Now
- **Advanced automations.** For automations beyond Whendo's scope, the plan is to run alongside a tool like Streamer.bot and make that coexistence painless. An embedded scripting language is a possible long-term extension, not a current goal.
- **Browser sources and overlays.** Whendo doesn't serve browser sources, so it doesn't need a local HTTP or WebSocket server for overlays. OBS control goes through the standard OBS WebSocket connection.
