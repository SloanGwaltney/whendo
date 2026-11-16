<script lang="ts" module>
  export interface ListOption {
    id: string;
    label: string;
    description?: string;
    isSelected?: boolean;
    isDanger?: boolean;
    isDisabled?: boolean;
  }
</script>

<script lang="ts">
  // A list of choices inside a popover, such as a setting's options or a
  // step's menu.
  let {options, onChoose}: {options: ListOption[]; onChoose: (optionID: string) => void} = $props();
</script>

<div class="option-list" role="listbox">
  {#each options as option (option.id)}
    <button
      type="button"
      class="option"
      class:is-danger={option.isDanger}
      role="option"
      aria-selected={option.isSelected ?? false}
      disabled={option.isDisabled}
      onclick={() => onChoose(option.id)}
    >
      <span>{option.label}</span>
      {#if option.description}<span class="description">{option.description}</span>{/if}
    </button>
  {/each}
</div>

<style>
  .option-list {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .option {
    display: flex;
    flex-direction: column;
    width: 100%;
    padding: 7px var(--space-2);
    border-radius: var(--radius);
    text-align: left;
  }

  .option:hover:not(:disabled),
  .option:focus-visible {
    background: var(--color-surface);
  }

  .option[aria-selected="true"] {
    background: var(--color-accent-soft);
  }

  .option.is-danger {
    color: var(--color-danger);
  }

  .option:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .description {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }
</style>
