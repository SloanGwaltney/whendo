# ADR 0004: Use coder/websocket for Twitch EventSub

- **Status:** Accepted
- **Date:** 2026-11-15

## Context

Whendo receives Twitch events through EventSub's WebSocket transport (see [Twitch EventSub](../design/twitch-eventsub.md)). Go's standard library has no WebSocket client, so we need either a library or our own implementation.

Every dependency, including what it brings in indirectly, is another possible route for a supply chain attack, so we want as few as reasonably possible (see [AGENTS.md](../../AGENTS.md#dependencies)).

We considered three options:

1. **coder/websocket:** [`github.com/coder/websocket`](https://github.com/coder/websocket), maintained by Coder. It was created as `nhooyr/websocket` in 2019, and Coder took over maintenance in 2024.
2. **Another WebSocket library,** such as `gorilla/websocket`.
3. **Write our own client** on top of the standard library's HTTP and networking packages.

## Decision

Use **coder/websocket**, as a direct dependency.

## Rationale

### It's already in our build
Wails v3 depends on coder/websocket, so it's already compiled into Whendo. Using it directly adds no new code to the build and no new supply chain exposure. Any other library would add a dependency we don't otherwise have.

### It has no dependencies of its own
Its `go.mod` lists none, so depending on it directly brings nothing else in.

### It fits how the engine is written
Its API is built around `context.Context`, which the engine already uses for cancellation and shutdown. It answers pings while reading, which EventSub requires, and it passes the Autobahn WebSocket conformance test suite.

### Why not write our own (option 3)
The WebSocket protocol has enough detail, such as framing, masking, fragmentation, ping and pong, and the close handshake, that a hand-written client would be a real source of bugs. Since a well-tested implementation is already in our build, writing our own would add risk without removing any dependency.

## Consequences

### Positive
- No new dependencies, direct or indirect.
- A well-tested implementation of the protocol.

### Negative and Accepted Risks
- **We now depend on it directly,** so a future Wails release dropping it wouldn't remove it from our build. That's acceptable: it has no dependencies of its own, and we'd evaluate it on its own merits at that point.
- **Version coupling with Wails.** Go's module system picks one version for both Wails and Whendo. An upgrade one of them needs could affect the other. Review changes before upgrading, as with any dependency.
