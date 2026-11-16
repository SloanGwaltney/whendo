<script lang="ts">
  import type {AvailableVariable} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {Kind} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import type {Definition as SettingDefinition, VariableReference} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {describeReward, loadRewardsIfNeeded, rewardSettingKind} from "../rewards.svelte";
  import {formatDuration, variableKey} from "./designerModel";
  import type {Choice, DesignerView, TemplatePart} from "./designerModel";
  import ValuePill from "./ValuePill.svelte";

  // One setting's value, shown the way it reads in its sentence.
  let {definition, value, availableVariables, view}: {
    definition: SettingDefinition;
    value: unknown;
    // The variables available where the setting is used, or null for the
    // trigger's settings, which can't use variables.
    availableVariables: AvailableVariable[] | null;
    view: DesignerView;
  } = $props();

  function isAvailable(reference: VariableReference): boolean {
    if (availableVariables === null) {
      return true;
    }
    const key = variableKey(reference);
    return availableVariables.some((variable) => variableKey(variable.reference) === key);
  }

  const templateParts = $derived(definition.kind === Kind.KindTemplate ? (value as TemplatePart[]) : []);
  const choices = $derived(definition.kind === Kind.KindChoiceList ? (value as Choice[]) : []);
  const isReward = $derived(definition.kind === rewardSettingKind);
  // A redeem is named by its title, which only Twitch knows.
  $effect(() => {
    if (isReward) {
      loadRewardsIfNeeded();
    }
  });

  const optionName = $derived((definition.options ?? []).find((option) => option.id === value)?.displayName ?? String(value));
</script>

{#if definition.kind === Kind.KindTemplate}
  <span class="quote">“</span>{#each templateParts as part, partIndex (partIndex)}{#if part.variable}<ValuePill reference={part.variable} {view} isBroken={!isAvailable(part.variable)}/>{:else}<span class="template-text">{part.text}</span>{/if}{/each}<span class="quote">”</span>
{:else if definition.kind === Kind.KindDuration}
  {formatDuration(value as number)}
{:else if definition.kind === Kind.KindOption}
  {optionName}
{:else if definition.kind === Kind.KindVariable}
  <ValuePill reference={value as VariableReference} {view} isBroken={!isAvailable(value as VariableReference)}/>
{:else if isReward}
  {describeReward(String(value))}
{:else if definition.kind === Kind.KindChoiceList}
  {#each choices as choice, choiceIndex (choice.id)}{#if choiceIndex > 0}{choiceIndex === choices.length - 1 ? " or " : ", "}{/if}<span class="user-text">{choice.text}</span>{/each}
{:else}
  <!-- Text, whole numbers, and kinds an integration adds that the designer
       has no picker for yet, which show their stored value. -->
  {String(value)}
{/if}

<style>
  .quote {
    color: var(--color-text-faint);
  }

  .template-text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .user-text {
    display: inline-block;
    padding: 0 6px;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-small);
    background: var(--color-surface);
    font-size: 0.92em;
    line-height: 1.55;
  }
</style>
