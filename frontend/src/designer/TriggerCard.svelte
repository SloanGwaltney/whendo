<script lang="ts">
  import type {TriggerSelection} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {VariableSource} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {triggerElementID, triggerOwnerID} from "./designerModel";
  import type {DesignerView} from "./designerModel";
  import Sentence from "./Sentence.svelte";
  import ValuePill from "./ValuePill.svelte";

  // "When" and the trigger's sentence, with the values it gives the steps.
  let {trigger, view}: {trigger: TriggerSelection; view: DesignerView} = $props();

  let isShowingEveryVariable = $state(false);

  const definition = $derived(view.triggerDefinition);
  const variables = $derived(definition?.variables ?? []);
  const shownVariables = $derived(isShowingEveryVariable ? variables : variables.filter((variable) => variable.isCommon));
  const hiddenVariableCount = $derived(variables.length - shownVariables.length);
</script>

<div class="trigger-card" id={triggerElementID}>
  <div class="trigger-line">
    <span class="keyword">When</span>
    {#if definition}
      <Sentence sentence={definition.sentence} settings={definition.settings} values={trigger.settings} ownerID={triggerOwnerID} {view} appearance="slots"/>
    {:else if trigger.id === ""}
      <span class="missing">pick what starts it</span>
    {:else}
      <span class="missing">a trigger Whendo doesn't have: {trigger.id}</span>
    {/if}
  </div>
  {#if variables.length > 0}
    <div class="gives">
      Gives you
      {#each shownVariables as variable (variable.id)}
        <ValuePill reference={{source: VariableSource.VariableSourceTrigger, id: variable.id}} {view} isBroken={false}/>
      {/each}
      {#if hiddenVariableCount > 0}
        <button type="button" class="more" onclick={() => (isShowingEveryVariable = true)}>+ {hiddenVariableCount} more</button>
      {:else if isShowingEveryVariable}
        <button type="button" class="more" onclick={() => (isShowingEveryVariable = false)}>Show fewer</button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .trigger-card {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px var(--space-4);
    border: 1px solid var(--color-trigger-border);
    border-radius: var(--radius-large);
    background: var(--color-trigger-soft);
  }

  .trigger-card:global(.is-flashing) {
    animation: flash 1.2s ease-out;
  }

  @keyframes flash {
    from {
      box-shadow: 0 0 0 3px var(--color-accent-hover);
    }
    to {
      box-shadow: none;
    }
  }

  .trigger-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
  }

  .keyword {
    color: var(--color-trigger);
    font-family: var(--font-display);
    font-size: 13px;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .missing {
    padding: 0 7px;
    border-bottom: 1.5px dashed var(--color-danger);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-text-muted);
    font-size: var(--font-size-sentence);
    font-style: italic;
  }

  .gives {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .more {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
