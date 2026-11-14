# AGENTS.md

## Project Overview

Whendo is a stream automation tool in the same space as Streamer.bot and Mix It Up. Those tools are powerful, but their interfaces are designed around their engine internals rather than around what makes sense to an average user. That's a deliberate trade for maximum flexibility, but it gives them a steep learning curve for non-technical users, and many streamers are less technical than people assume. Here, "non-technical" means comfortable with computers but not thinking like a programmer, roughly the average gamer; see [docs/overview.md](docs/overview.md#who-its-for). Whendo deliberately offers fewer features in exchange for an interface that is far more accessible. Every automation reads like a natural-language sentence: **when** a trigger happens, **do** a series of steps. That's where the name comes from. Whendo isn't meant to fully replace Streamer.bot or Mix It Up for very complex automations. The goal is to cover the 90% case so most streamers don't need them. For the remaining 10%, Whendo should run comfortably alongside a tool like Streamer.bot rather than compete with it.

For a fuller overview and an index of all project documents, see [docs/overview.md](docs/overview.md).

## Naming the Product
- In prose, docs and the UI, write the product name as **Whendo**.
- In code, module paths, package names and identifiers, use the lowercase form `whendo`, following Go conventions.

## Tech Stack
- **Backend:** Go.
- **Desktop shell:** Wails v3, which renders the UI in the operating system's native web view (WebView2 on Windows) rather than bundling a browser.
- **Frontend:** TypeScript and Svelte 5 (runes mode) with Vite, running in the web view. Not SvelteKit.
- **Platforms:** cross-platform (Windows, macOS, Linux). Windows is the primary target and is expected to be 80–90% of users, so prioritize and test it first.
- **Initial integrations:** Twitch, OBS (through the OBS WebSocket protocol) and audio playback to a chosen output device.
- **Secrets:** stored in the operating system's credential store through `github.com/zalando/go-keyring`, never in plain files. See [ADR 0003](docs/adr/0003-go-keyring-for-credential-storage.md).
- **cgo:** cgo and a C toolchain are expected. Prefer pure Go dependencies when they're good enough, but don't contort the design to avoid cgo.

See [ADR 0001](docs/adr/0001-go-and-wails-for-desktop-app.md) for why we chose Go and Wails, and [ADR 0002](docs/adr/0002-svelte-for-frontend.md) for why we chose Svelte.

## Architecture
- The automation engine (triggers, steps, integrations) is plain Go and must not import or depend on Wails. Its public API should be usable from any host: the Wails app, tests or a future CLI.
- Wails is a thin layer on top of the engine. It connects the engine to the UI and handles desktop concerns such as windows, the system tray and updates.
- Automations must keep running when the window is closed and the app is in the system tray.

## Coding Practices

### Naming
- Name everything explicitly and descriptively: variables, functions, types, structs, packages, modules and files.
- Do not abbreviate. Write `deltaTime`, not `dt`; `requestCount`, not `cnt`; `userRepository`, not `usrRepo`.
- A name should say what the thing is or does, so a reader doesn't need a comment or the surrounding code to understand it.
- Name booleans as yes/no questions: `isEnabled`, `hasPermission`, `shouldRetry`, not `enabled`, `flag` or `check`.

### Constants
- No magic numbers or strings. Give literal values a named constant that explains their meaning: `maxRetryCount`, not a bare `3`.

### Functions
- Each function should do one thing and do it well.
- That one thing should be clear from the function's name. If the name needs "and" to be accurate, split the function.
- Return early. Handle guard conditions and error cases first, and keep the main path at the lowest nesting level.

### Comments
- Comments explain *why*, not *what*. The code and its names should already make clear what it does.
- Use comments for intent, constraints, trade-offs and anything that would surprise a reader.

### Error Handling
- Handle every error explicitly. Never ignore or swallow one.
- Either return the error to the caller with added context, or handle it where it occurs and log it with enough context to diagnose.

### Unit Tests
- Test only exported (public) members. Test unexported or internal behavior through the public API that uses it.
- Name each test for the behavior it checks: `TestParseDate_ReturnsErrorOnEmptyInput`, not `TestParseDate2`.
- Keep tests independent. A test must not share mutable state with other tests or depend on the order they run in.

### Frontend (Svelte)
- Use Svelte 5 runes only: `$state`, `$derived`, `$effect` and `$props`. Never use Svelte 4 syntax such as `export let`, `$:` reactive statements, `on:event` directives, or stores for component-local state.
- Use `$derived` for any value computed from other state. Use `$effect` only to sync with something outside Svelte, such as the DOM, timers or third-party libraries, never to set state that could be derived.
- Don't add SvelteKit or any server-side frontend concepts. The frontend talks to the engine only through the generated Wails bindings.
- Before adding a dependency, check whether it's React-only. React libraries won't work here; look for a Svelte equivalent or a framework-agnostic library instead.

### Dependencies
- Every dependency, including the indirect ones it brings in, is another possible route for a supply chain attack. This isn't a judgment of any particular package; it's simply how supply chain attacks work. Weigh each new dependency carefully.
- Before adding a dependency, consider whether the standard library or a small amount of our own code would do the job.
- Only add dependencies that are well established: actively maintained, widely used and depended on by reputable projects.
- Don't add a dependency without the maintainer's approval. Explain what it's for, what it brings in indirectly and why it's worth it. Decisions about significant dependencies are recorded in an ADR.

### Code Hygiene
- Delete dead code instead of commenting it out. Version control keeps the history.
- Follow the language's conventions and the repo's existing style. Use the language's standard formatter and linter, such as `gofmt` or `prettier`.

### Change Scope
- Keep each change focused on its task. Don't refactor, reformat or "clean up" code that is unrelated to the task.
- Code is related when the task requires changing it, even if it belongs to a different feature. Make those changes as part of the task; only leave alone what the task doesn't need.
