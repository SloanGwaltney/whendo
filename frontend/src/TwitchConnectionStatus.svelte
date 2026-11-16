<script lang="ts">
  import {Events} from "@wailsio/runtime";
  import {TwitchConnectionStateChanged} from "../bindings/events";
  import {TwitchEventsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import {ConnectionStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitcheventsub";
  import type {ConnectionState} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitcheventsub";

  let {accountLogin}: {accountLogin: string} = $props();

  let connectionState = $state<ConnectionState | null>(null);

  $effect(() => {
    // Subscribe before asking for the current state, so a change that
    // happens in between isn't missed.
    const unsubscribe = Events.On(TwitchConnectionStateChanged, (event) => {
      connectionState = event.data;
    });
    TwitchEventsService.GetConnectionState()
      .then((currentState) => {
        connectionState ??= currentState;
      })
      .catch((error: unknown) => console.error("Couldn't load the Twitch connection state", error));
    return unsubscribe;
  });

  const isConnected = $derived(connectionState?.status === ConnectionStatus.ConnectionStatusConnected);

  const statusText = $derived.by(() => {
    switch (connectionState?.status) {
      case ConnectionStatus.ConnectionStatusConnected:
        return `Connected as ${accountLogin}`;
      case ConnectionStatus.ConnectionStatusProblem:
        return "Some events can't be received";
      default:
        return "Reconnecting…";
    }
  });

  const problems = $derived(connectionState?.problems ?? []);
</script>

<div class="connection-status" role="status">
  <span class="dot" class:is-connected={isConnected}></span>
  <span class="name">Twitch</span>
  {#if problems.length > 0}
    <details class="detail">
      <summary>{statusText}</summary>
      <ul class="is-selectable">
        {#each problems as problem (problem)}
          <li>{problem}</li>
        {/each}
      </ul>
    </details>
  {:else}
    <span class="detail">{statusText}</span>
  {/if}
</div>

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

  summary {
    cursor: pointer;
  }

  ul {
    margin: var(--space-1) 0 0;
    padding-left: var(--space-4);
  }
</style>
