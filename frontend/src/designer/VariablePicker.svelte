<script lang="ts">
  import type {AvailableVariable} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {VariableSource} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import type {VariableReference} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {variableKey} from "./designerModel";
  import type {DesignerView} from "./designerModel";

  // The values a step can use, grouped by where they come from, with the
  // common ones first and the rest behind "more".
  let {availableVariables, selected = null, view, onPick}: {
    availableVariables: AvailableVariable[];
    selected?: VariableReference | null;
    view: DesignerView;
    onPick: (reference: VariableReference) => void;
  } = $props();

  interface VariableGroup {
    label: string;
    common: AvailableVariable[];
    others: AvailableVariable[];
  }

  const groups = $derived.by(() => {
    const groupsByLabel = new Map<string, VariableGroup>();
    for (const variable of availableVariables) {
      const label = variable.reference.source === VariableSource.VariableSourceTrigger
        ? `From “${view.triggerDefinition?.displayName ?? "the trigger"}”`
        : `From ${view.stepLabelsByID.get(variable.reference.stepId ?? "") ?? "an earlier step"}`;
      let group = groupsByLabel.get(label);
      if (!group) {
        group = {label, common: [], others: []};
        groupsByLabel.set(label, group);
      }
      (variable.isCommon ? group.common : group.others).push(variable);
    }
    return [...groupsByLabel.values()];
  });

  const selectedKey = $derived(selected ? variableKey(selected) : "");
</script>

{#if groups.length === 0}
  <p class="nothing">Nothing to pick here yet. Values come from what starts the automation and from steps above this one.</p>
{/if}
{#each groups as group (group.label)}
  <div class="group">
    <span class="group-label">{group.label}</span>
    <div class="pills">
      {#each group.common as variable (variableKey(variable.reference))}
        <button type="button" class="pill" aria-pressed={variableKey(variable.reference) === selectedKey} onclick={() => onPick(variable.reference)}>{variable.displayName}</button>
      {/each}
    </div>
    {#if group.others.length > 0}
      <details>
        <summary>{group.others.length} more</summary>
        <div class="pills">
          {#each group.others as variable (variableKey(variable.reference))}
            <button type="button" class="pill" aria-pressed={variableKey(variable.reference) === selectedKey} onclick={() => onPick(variable.reference)}>{variable.displayName}</button>
          {/each}
        </div>
      </details>
    {/if}
  </div>
{/each}

<style>
  .nothing {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .group-label {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }

  .pills {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding: 2px 0;
  }

  .pill {
    padding: 0 6px;
    border: 1px solid transparent;
    border-radius: var(--radius-small);
    background: var(--color-value-soft);
    color: var(--color-value);
    font-size: 13px;
    font-weight: 500;
    line-height: 1.6;
  }

  .pill:hover,
  .pill[aria-pressed="true"] {
    border-color: var(--color-value);
  }

  summary {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    cursor: pointer;
  }
</style>
