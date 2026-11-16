<script lang="ts">
  import {isStreamingSoftwareChosen, isStreamingSoftwareConnected, streamingSoftwareState} from "./streamingSoftware.svelte";

  const isChosen = $derived(isStreamingSoftwareChosen(streamingSoftwareState.current));
  const isConnected = $derived(isStreamingSoftwareConnected(streamingSoftwareState.current));
</script>

<!-- Nothing shows until a program is chosen, so streamers who don't use
     one are never nagged. Streamlabs Desktop is the only program so far. -->
{#if isChosen}
  <div class="connection-status" role="status">
    <span class="dot" class:is-connected={isConnected}></span>
    <span class="name">Streamlabs</span>
    <span class="detail">{isConnected ? "Connected" : "Waiting… Is Streamlabs Desktop open?"}</span>
  </div>
{/if}

<style>
  .connection-status {
    display: grid;
    grid-template-columns: 10px 1fr;
    gap: 2px var(--space-3);
    align-items: center;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--color-warning);
  }

  .dot.is-connected {
    background: var(--color-success);
  }

  .name {
    font-weight: 600;
    font-size: 13px;
  }

  .detail {
    grid-column: 2;
    min-width: 0;
    font-size: var(--font-size-small);
    color: var(--color-text-muted);
    overflow-wrap: anywhere;
  }
</style>
