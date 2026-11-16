<script lang="ts">
  import {untrack} from "svelte";
  import type {Item} from "../../bindings/github.com/SloanGwaltney/whendo/engine/scenes";
  import {loadScenes, scenesState} from "../scenes.svelte";
  import type {SourceInScene} from "../scenes.svelte";
  import {isStreamingSoftwareChosen, isStreamingSoftwareConnected, streamingSoftwareState} from "../streamingSoftware.svelte";

  // Picks a source and its scene in one go: every scene is a heading with
  // its sources under it. See
  // docs/design/streaming-software.md#picking-a-source.
  let {selected, onPick}: {selected: SourceInScene | null; onPick: (source: SourceInScene) => void} = $props();

  let query = $state("");

  const isChosen = $derived(isStreamingSoftwareChosen(streamingSoftwareState.current));
  const isConnected = $derived(isStreamingSoftwareConnected(streamingSoftwareState.current));

  // Load when the picker opens, and again once the program connects, so a
  // list that failed while it was closed doesn't stay failed.
  $effect(() => {
    if (isConnected) {
      // Loading reads and writes the shared state, which mustn't make this
      // effect run again.
      untrack(loadScenes);
    }
  });

  // A source can be in a scene more than once, such as once on each of
  // Streamlabs' canvases, but the steps act on every copy, so it's listed
  // once.
  function uniqueSources(items: Item[]): Item[] {
    const seenSourceIDs = new Set<string>();
    return items.filter((item) => {
      if (seenSourceIDs.has(item.sourceId)) {
        return false;
      }
      seenSourceIDs.add(item.sourceId);
      return true;
    });
  }

  const groups = $derived.by(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return (scenesState.list ?? [])
      .map((scene) => {
        const isSceneMatch = scene.name.toLowerCase().includes(normalizedQuery);
        const items = uniqueSources(scene.items ?? []).filter(
          (item) => isSceneMatch || [item.sourceName, ...(item.folderPath ?? [])].join(" ").toLowerCase().includes(normalizedQuery),
        );
        return {scene, items};
      })
      .filter((group) => group.items.length > 0);
  });

  function pick(sceneID: string, sceneName: string, item: Item): void {
    onPick({sceneId: sceneID, sceneName, sourceId: item.sourceId, sourceName: item.sourceName});
  }

  function isSelected(sceneID: string, item: Item): boolean {
    return selected?.sceneId === sceneID && selected?.sourceId === item.sourceId;
  }
</script>

<div class="source-picker">
  {#if !isChosen}
    <p class="note">Choose your streaming software in Settings to pick a source.</p>
  {:else if !isConnected}
    <p class="note">Open Streamlabs Desktop to pick a source.</p>
  {:else}
    <input class="field" placeholder="Find a source or scene" aria-label="Find a source or scene" bind:value={query} data-autofocus/>
    {#if scenesState.isLoading && scenesState.list === null}
      <p class="note">Loading your scenes…</p>
    {:else if scenesState.errorMessage}
      <p class="error is-selectable" role="alert">{scenesState.errorMessage}</p>
    {/if}
    <div class="groups" role="listbox">
      {#each groups as group (group.scene.id)}
        <div class="scene-label">{group.scene.name}</div>
        {#each group.items as item (item.sourceId)}
          <button
            type="button"
            class="source-option"
            role="option"
            aria-selected={isSelected(group.scene.id, item)}
            onclick={() => pick(group.scene.id, group.scene.name, item)}
          >
            {#if item.folderPath?.length}<span class="folders">{item.folderPath.join(" / ")} /</span>{/if}
            <span>{item.sourceName}</span>
          </button>
        {/each}
      {:else}
        {#if scenesState.list !== null}
          <p class="note">{query.trim() ? `No sources match “${query}”.` : "Your scenes have no sources yet."}</p>
        {/if}
      {/each}
    </div>
    <button type="button" class="refresh" disabled={scenesState.isLoading} onclick={loadScenes}>
      {scenesState.isLoading ? "Loading…" : "Refresh the list"}
    </button>
  {/if}
</div>

<style>
  .source-picker {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-width: 340px;
  }

  .groups {
    display: flex;
    flex-direction: column;
    gap: 1px;
    max-height: 320px;
    overflow-y: auto;
  }

  .scene-label {
    padding: var(--space-2) var(--space-2) 2px;
    color: var(--color-text-faint);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .source-option {
    display: flex;
    gap: var(--space-1);
    padding: 7px var(--space-2);
    border-radius: var(--radius);
    text-align: left;
  }

  .source-option:hover,
  .source-option:focus-visible {
    background: var(--color-surface);
  }

  .source-option[aria-selected="true"] {
    background: var(--color-accent-soft);
  }

  .folders,
  .note {
    color: var(--color-text-muted);
  }

  .note {
    font-size: 13px;
  }

  .error {
    color: var(--color-danger);
    font-size: 13px;
  }

  .refresh {
    align-self: flex-start;
    color: var(--color-accent);
    font-size: 13px;
  }

  .refresh:hover:not(:disabled) {
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
