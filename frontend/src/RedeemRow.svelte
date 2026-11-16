<script lang="ts">
  import type {Reward} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchrewards";

  // One redeem on the Redeems page. Redeems Whendo made can be changed;
  // any other can only be recreated in Whendo, since Twitch doesn't let
  // Whendo change it.
  let {reward, actions, onRecreate}: {
    reward: Reward;
    actions?: {onEdit: () => void; onTogglePause: () => void; onDelete: () => void};
    onRecreate?: () => void;
  } = $props();

  const secondsPerMinute = 60;
  const secondsPerHour = 60 * secondsPerMinute;

  function pluralize(count: number, unit: string): string {
    return `${count.toLocaleString()} ${unit}${count === 1 ? "" : "s"}`;
  }

  function formatCooldown(seconds: number): string {
    if (seconds % secondsPerHour === 0) {
      return pluralize(seconds / secondsPerHour, "hour");
    }
    if (seconds % secondsPerMinute === 0) {
      return pluralize(seconds / secondsPerMinute, "minute");
    }
    return pluralize(seconds, "second");
  }

  // The limits Twitch puts on redeeming it, in words. Limits of zero are off.
  const limits: string[] = $derived.by(() => {
    const described: string[] = [];
    if (reward.cooldownSeconds > 0) {
      described.push(`${formatCooldown(reward.cooldownSeconds)} cooldown`);
    }
    if (reward.maximumPerStream > 0) {
      described.push(`${pluralize(reward.maximumPerStream, "time")} per stream`);
    }
    if (reward.maximumPerViewerPerStream > 0) {
      described.push(`${pluralize(reward.maximumPerViewerPerStream, "time")} per viewer per stream`);
    }
    return described;
  });
</script>

<li class="redeem-row">
  <!-- The redeem's own colour, from Twitch, so the streamer can recognize
       it. It's the one colour on the page that isn't a theme variable. -->
  <span class="swatch" style:background-color={reward.backgroundColor || null} aria-hidden="true"></span>
  <div class="details">
    <div class="heading">
      <span class="title">{reward.title}</span>
      <span class="cost">{reward.cost.toLocaleString()} points</span>
    </div>
    {#if reward.prompt}
      <p class="prompt">{reward.prompt}</p>
    {/if}
    <div class="facts">
      {#if !reward.isShownToViewers}
        <span class="badge is-muted">Hidden from viewers</span>
      {/if}
      {#if reward.isPaused}
        <span class="badge is-warning">Paused</span>
      {/if}
      {#if reward.isUserInputRequired}
        <span class="badge">Viewers type a message</span>
      {/if}
      {#if reward.canBeRefunded}
        <span class="badge">Viewers can be refunded</span>
      {/if}
      {#each limits as limit (limit)}
        <span class="limit">{limit}</span>
      {/each}
    </div>
  </div>
  {#if actions}
    <div class="controls">
      <!-- Pausing is here rather than in the form, since it's what a
           streamer reaches for mid-stream. -->
      <button type="button" class="button" onclick={actions.onTogglePause}>{reward.isPaused ? "Resume" : "Pause"}</button>
      <button type="button" class="button" onclick={actions.onEdit}>Edit</button>
      <button type="button" class="button" onclick={actions.onDelete}>Delete</button>
    </div>
  {:else if onRecreate}
    <div class="controls">
      <button type="button" class="button" title="Make it a Whendo redeem, so Whendo can change it" onclick={onRecreate}>Recreate in Whendo</button>
    </div>
  {/if}
</li>

<style>
  .redeem-row {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-large);
    background: var(--color-surface);
  }

  .controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .swatch {
    flex: none;
    width: 28px;
    height: 28px;
    margin-top: 2px;
    border-radius: var(--radius);
    background-color: var(--color-surface-raised);
  }

  .details {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .title {
    font-weight: 600;
    font-size: 15px;
    overflow-wrap: anywhere;
  }

  .cost {
    flex: none;
    color: var(--color-text-muted);
    font-variant-numeric: tabular-nums;
  }

  .prompt {
    color: var(--color-text-muted);
    font-size: 13px;
    overflow-wrap: anywhere;
  }

  .facts {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--space-1) var(--space-2);
    margin-top: var(--space-1);
  }

  .facts:empty {
    display: none;
  }

  .badge {
    padding: 0 var(--space-2);
    border-radius: 999px;
    background: var(--color-accent-soft);
    color: var(--color-accent);
    font-size: var(--font-size-small);
    font-weight: 600;
  }

  .badge.is-muted {
    background: var(--color-surface-raised);
    color: var(--color-text-muted);
  }

  .badge.is-warning {
    background: var(--color-warning-soft);
    color: var(--color-warning);
  }

  .limit {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }
</style>
