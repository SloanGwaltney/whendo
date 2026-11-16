<script lang="ts">
  import {untrack} from "svelte";
  import type {AvailableVariable} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {Kind} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import type {Definition as SettingDefinition, VariableReference} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import type {DesignerView} from "./designerModel";
  import OptionList from "./OptionList.svelte";
  import VariablePicker from "./VariablePicker.svelte";

  // The popover's contents for editing one setting. Changes apply as they're
  // made, so the sentence and problems update while editing.
  let {definition, value, availableVariables, view, onChange, onDone}: {
    definition: SettingDefinition;
    value: unknown;
    availableVariables: AvailableVariable[];
    view: DesignerView;
    // Called with null when the value is cleared.
    onChange: (newValue: unknown) => void;
    onDone: () => void;
  } = $props();

  const millisecondsPerSecond = 1000;
  const millisecondsPerMinute = 60_000;

  // Durations show in minutes when they're whole minutes, else seconds. The
  // editor starts from the value when it opens, then owns what's typed.
  const initialMilliseconds = untrack(() => (typeof value === "number" ? value : null));
  const isInitiallyMinutes = initialMilliseconds !== null && initialMilliseconds >= millisecondsPerMinute && initialMilliseconds % millisecondsPerMinute === 0;
  let durationUnitMilliseconds = $state(isInitiallyMinutes ? millisecondsPerMinute : millisecondsPerSecond);
  let durationAmount = $state(initialMilliseconds === null ? "" : String(initialMilliseconds / (isInitiallyMinutes ? millisecondsPerMinute : millisecondsPerSecond)));

  function updateDuration(): void {
    const amount = Number.parseFloat(durationAmount);
    onChange(Number.isFinite(amount) && amount > 0 ? Math.round(amount * durationUnitMilliseconds) : null);
  }

  function updateWholeNumber(event: Event): void {
    const text = (event.currentTarget as HTMLInputElement).value;
    const number = Number.parseInt(text, 10);
    onChange(Number.isNaN(number) ? null : number);
  }

  function updateText(event: Event): void {
    const text = (event.currentTarget as HTMLInputElement).value;
    onChange(text === "" ? null : text);
  }

  function finishOnEnter(event: KeyboardEvent): void {
    if (event.key === "Enter") {
      event.preventDefault();
      onDone();
    }
  }

  const isEngineKind = $derived((Object.values(Kind) as string[]).includes(definition.kind));
</script>

<span class="title">{definition.displayName}</span>

{#if definition.kind === Kind.KindOption}
  <OptionList
    options={(definition.options ?? []).map((option) => ({id: option.id, label: option.displayName, isSelected: option.id === value}))}
    onChoose={(optionID) => {
      onChange(optionID);
      onDone();
    }}
  />
{:else if definition.kind === Kind.KindVariable}
  <VariablePicker
    {availableVariables}
    selected={(value as VariableReference | null) ?? null}
    {view}
    onPick={(reference) => {
      onChange(reference);
      onDone();
    }}
  />
{:else if definition.kind === Kind.KindWholeNumber}
  <input class="field" type="number" step="1" value={value ?? ""} oninput={updateWholeNumber} onkeydown={finishOnEnter} data-autofocus/>
{:else if definition.kind === Kind.KindDuration}
  <div class="row">
    <input class="field" type="number" min="1" step="1" bind:value={durationAmount} oninput={updateDuration} onkeydown={finishOnEnter} aria-label="How many" data-autofocus/>
    <select class="field unit" bind:value={durationUnitMilliseconds} onchange={updateDuration} aria-label="Unit">
      <option value={millisecondsPerSecond}>seconds</option>
      <option value={millisecondsPerMinute}>minutes</option>
    </select>
  </div>
{:else}
  <input class="field" value={value ?? ""} placeholder={definition.placeholder} oninput={updateText} onkeydown={finishOnEnter} data-autofocus/>
  {#if !isEngineKind}
    <!-- An integration's own kind, such as a reward, until it has a picker. -->
    <p class="help">Whendo can't list these yet, so type the value.</p>
  {/if}
{/if}

<style>
  .title {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .row {
    display: flex;
    gap: var(--space-2);
  }

  .unit {
    width: auto;
  }

  .help {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }
</style>
