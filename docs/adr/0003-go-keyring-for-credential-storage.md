# ADR 0003: Use go-keyring for Credential Storage

- **Status:** Accepted
- **Date:** 2026-11-14

## Context

Whendo stores secrets on the user's machine. The first are Twitch access and refresh tokens (see [Twitch Login](../design/twitch-login.md)); later secrets include the TTS Monster overlay URL and the OBS WebSocket password. Secrets must go in the operating system's credential store, never in plain files:
- **Windows:** Credential Manager.
- **macOS:** Keychain.
- **Linux:** the Secret Service API, over D-Bus.

Whendo supports all three platforms (see [ADR 0001](0001-go-and-wails-for-desktop-app.md)). However, the maintainer's primary development machine runs Windows. Testing on macOS and Linux is possible through dual-booting or help from others, but it isn't part of the everyday workflow.

We want to avoid third-party dependencies where we reasonably can, because each one adds supply chain risk. Any dependency we do take on should be well established.

We considered three options:

1. **go-keyring:** [`github.com/zalando/go-keyring`](https://github.com/zalando/go-keyring), a cross-platform Go library for the operating system credential stores.
2. **Assemble per-platform libraries ourselves:** use existing libraries that each talk to one operating system's credential store, such as `danieljoos/wincred` for Windows and `godbus/dbus` for Linux, plus something equivalent for macOS. Wrap them in our own cross-platform interface.
3. **Write our own bindings:** call each operating system's credential API directly, without third-party libraries:
   - **Windows:** Credential Manager, through `golang.org/x/sys/windows`.
   - **macOS:** the Security framework, through cgo.
   - **Linux:** `libsecret`, through cgo.

## Decision

Use **go-keyring** for all credential storage.

## Rationale

### Why not assemble per-platform libraries ourselves (option 2)
It wouldn't meaningfully reduce our dependencies. go-keyring is essentially this option already done for us: it wraps `danieljoos/wincred` and `godbus/dbus` behind one cross-platform interface. We'd take on roughly the same dependencies, then write and maintain the same glue code that go-keyring already provides.

### Why not write our own bindings (option 3)
This is the only option that removes the third-party dependencies, but we'd be responsible for making three separate platform bindings work correctly, and we can only routinely test on Windows. The two we can't easily test are the hardest:
- **Linux:** Secret Service behavior varies between distributions and desktop environments, so we'd expect edge cases we'd struggle to reproduce.
- **macOS:** the maintainer has little experience with it, so the Keychain would be entirely new ground.

The cost of writing, testing and maintaining these bindings outweighs the supply chain exposure we'd avoid.

### Why go-keyring (option 1)
- **It's already tested on all three platforms**, by a much larger user base than we could reach ourselves.
- **The package is well established.** GitHub's own CLI (`cli/cli`) depends on it to store tokens, which is the strongest signal. It's also maintained by the open-source organization of Zalando, a large, publicly traded European e-commerce company, rather than by a single individual. Its only direct dependencies are `danieljoos/wincred` (Windows) and `godbus/dbus` (Linux).

## Consequences

### Positive
- No platform-specific binding code to write, test and maintain.
- Behavior on macOS and Linux that has been tested by far more users than we could reach ourselves.

### Negative and Accepted Risks
- **Supply chain exposure.** We depend on go-keyring and, through it, on `danieljoos/wincred`, `godbus/dbus` and `golang.org/x/sys`. We have no concerns about these packages themselves. But every dependency, direct or indirect, is another possible route for a supply chain attack. We consider the added exposure acceptable given the package's reputation and the projects that depend on it. Versions are pinned through `go.sum`, and changes should be reviewed before upgrading.
- **Less control.** If go-keyring has a platform-specific bug, we depend on its maintainers or have to work around it. The engine should access credential storage through a small interface of its own, so we could replace go-keyring without changes elsewhere.
- **Credential size limits.** Windows Credential Manager limits how much data a single credential can hold. Store each secret, such as the access token and the refresh token, as its own entry rather than bundling them into one large value.
