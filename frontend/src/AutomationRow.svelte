<script lang="ts">
  import type {AutomationSummary} from "../bindings/github.com/SloanGwaltney/whendo";
  import Switch from "./Switch.svelte";

  let {summary, triggerName, onToggle, onDelete}: {
    summary: AutomationSummary;
    // The trigger's name, or empty when the automation has none yet.
    triggerName: string;
    onToggle: () => void;
    onDelete: () => void;
  } = $props();

  const needsFixing = $derived(summary.problem !== "" && !summary.isUnreadable);
</script>

<li class="automation-row">
  <div class="details">
    <span class="name">{summary.name}</span>
    {#if summary.isUnreadable}
      <span class="problem">
        <span class="badge is-danger">Couldn't be read</span>
        <span class="is-selectable">{summary.problem}</span>
      </span>
    {:else}
      <span class="trigger">{triggerName ? `When: ${triggerName}` : "No trigger yet"}</span>
      {#if needsFixing}
        <span class="problem">
          <span class="badge is-warning">Needs fixing</span>
          <span>{summary.problem}</span>
        </span>
      {/if}
    {/if}
  </div>
  <div class="controls">
    {#if !summary.isUnreadable}
      <Switch
        isOn={summary.isEnabled}
        label={`${summary.name} is ${summary.isEnabled ? "on" : "off"}`}
        isDisabled={needsFixing}
        {onToggle}
      />
    {/if}
    <button type="button" class="button" onclick={onDelete}>Delete</button>
  </div>
</li>

<style>
  .automation-row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-large);
    background: var(--color-surface);
  }

  .details {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .name {
    font-weight: 600;
    font-size: 15px;
    overflow-wrap: anywhere;
  }

  .trigger {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .problem {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .badge {
    flex: none;
    padding: 0 var(--space-2);
    border-radius: 999px;
    font-size: var(--font-size-small);
    font-weight: 600;
  }

  .badge.is-warning {
    background: var(--color-warning-soft);
    color: var(--color-warning);
  }

  .badge.is-danger {
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }

  .controls {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
</style>
