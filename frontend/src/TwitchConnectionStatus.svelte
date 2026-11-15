<script lang="ts">
  import {Events} from "@wailsio/runtime";
  import {TwitchConnectionStateChanged} from "../bindings/events";
  import {TwitchEventsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import {ConnectionStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitcheventsub";
  import type {ConnectionState} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitcheventsub";

  let connectionState: ConnectionState | null = $state(null);

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

  const statusText = $derived.by(() => {
    switch (connectionState?.status) {
      case ConnectionStatus.ConnectionStatusConnected:
        return "Twitch: connected";
      case ConnectionStatus.ConnectionStatusProblem:
        return "Twitch: some events can't be received";
      default:
        return "Twitch: reconnecting…";
    }
  });

  const problems = $derived.by(() => connectionState?.problems ?? []);
</script>

<div class="connection-status" role="status">
  {#if problems.length > 0}
    <details>
      <summary>{statusText}</summary>
      <ul>
        {#each problems as problem (problem)}
          <li>{problem}</li>
        {/each}
      </ul>
    </details>
  {:else}
    <span>{statusText}</span>
  {/if}
</div>

<style>
  .connection-status {
    color: var(--muted);
    font-size: 0.9rem;
    --wails-draggable: no-drag;
  }

  summary {
    cursor: pointer;
  }

  ul {
    margin: var(--s-1) 0 0;
    padding-left: var(--s-2);
    text-align: left;
    -webkit-user-select: text;
    user-select: text;
  }
</style>
