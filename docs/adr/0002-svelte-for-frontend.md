# ADR 0002: Use Svelte 5 (Without SvelteKit) for the Frontend

- **Status:** Accepted
- **Date:** 2026-11-14

## Context

[ADR 0001](0001-go-and-wails-for-desktop-app.md) established a Go backend with a TypeScript frontend running in a native web view through Wails v3. Wails generates typed TypeScript bindings from Go structs and methods, so the frontend calls the engine directly rather than through an HTTP API.

Most of the app is simple screens and settings. The exception is the **automation designer**, where users build "when this happens, do these steps" sentences. It needs a lot of interactivity: reordering steps, editing parts of sentences inline and holding unsaved state on the client.

Most of the code will be written by AI. The main goal for the frontend is code that the maintainer can review with confidence, without having to rewrite large parts of it. In practice, that means choosing a framework with as few footguns as possible.

We considered:

1. **Plain TypeScript** (no framework)
2. **React**
3. **Vue**
4. **Svelte 5**
5. **HTMX**
6. **Astro**

## Decision

Build the frontend with **Svelte 5** in runes mode, using **plain Svelte with Vite**. Don't use SvelteKit.

## Rationale

### Why not plain TypeScript
Without a framework, we'd have complete control. But control isn't a problem we need to solve. We'd end up building and maintaining our own way of rendering and updating the UI without gaining anything from it.

### Why not React
React has the largest ecosystem by a wide margin. But it has many footguns: it's easy to trigger unintended re-renders, misuse `useEffect` or get dependency arrays wrong. These mistakes are common in AI-written code and are hard to spot in review. React also relies on a virtual DOM, which the field has largely moved past in favor of fine-grained reactivity.

### Why not Vue
Vue's reactivity is also signal-based, but it still uses a virtual DOM by default, and the mode that removes it is still maturing. The primary maintainer doesn't know Vue well, so reviewing it with confidence would take extra effort, and it offers no clear advantage over Svelte.

### Why not Astro
Astro adds a server layer. Wails bindings already connect the frontend to Go, so a middle-layer server would be redundant.

### Why not HTMX
- **It needs a server that returns HTML.** In Wails, the frontend calls Go directly rather than through an HTTP server. Running HTMX would mean adding a handler that renders HTML in Go, which is another middle layer.
- **It would replace the typed contract with HTML strings.** The generated TypeScript bindings give a compiler-checked contract between the engine and the UI, and HTMX would throw that away.
- **The designer would need a second tool.** Its interactivity would need Alpine or hand-written scripts alongside HTMX, which means two ways of building UI in one app.

### Why Svelte 5
- **Reactivity is explicit.** Runes (`$state`, `$derived`, `$effect`) mark exactly where reactivity happens, which fits our explicit-naming rules.
- **Fewer footguns.** There's no virtual DOM and no re-render cycle to manage. The main remaining footgun, misusing `$effect`, is far smaller than React's `useEffect` and is covered by one rule.
- **The main AI failure can be caught at compile time.** AI tools often mix Svelte 4 syntax into Svelte 5 code. Enforcing runes mode globally with `compilerOptions: { runes: true }` turns Svelte 4 syntax into a compile error. `svelte-check` in CI catches type errors.

### Why not SvelteKit
SvelteKit is built around server concepts such as load functions, server-side rendering and form actions, and AI tools tend to reach for them even in a static build. A desktop app with a few screens doesn't need them. Plain Svelte with Vite keeps the frontend a pure client of the Go bindings.

## Consequences

### Positive
- Less framework-induced complexity to review.
- A typed, compiler-checked boundary between the Go engine and the UI.
- One way of building UI across the whole app, including the designer.

### Negative and Accepted Risks
- **We lose most of the frontend ecosystem.** Much of the web's ecosystem is React-specific components, hooks and integrations. Svelte can use framework-agnostic JavaScript libraries, but React libraries are unavailable unless a Svelte equivalent exists. Where none exists, we'll build it ourselves or do without it. We consider this manageable for Whendo's scope, but it's a real cost, and it will come up repeatedly.
- **AI code quality is less reliable than with React.** There's less Svelte 5 code to learn from, and AI tools still tend to produce Svelte 4 syntax. The runes compiler setting, `svelte-check` and the rules in `AGENTS.md` reduce this risk but don't eliminate it.
- **No built-in routing.** Without SvelteKit, we need a small client-side router or state-driven views. That's acceptable for a handful of screens.
