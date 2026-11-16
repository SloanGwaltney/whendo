<script lang="ts">
  import type {AvailableVariable} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Definition as SettingDefinition, Sentence, Values} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {isEditableKind, isEmptyValue, settingKey, useDesignerActions} from "./designerModel";
  import type {DesignerView} from "./designerModel";
  import SettingValue from "./SettingValue.svelte";

  // A trigger's, step's or outcome's sentence, with each setting's value in
  // its place. "slots" marks each value as something to edit; "prose" is
  // plain text for Read it back.
  let {sentence, settings = [], values = {}, ownerID = "", availableVariables = null, view, appearance}: {
    sentence: Sentence;
    settings?: SettingDefinition[] | null;
    values?: Values;
    ownerID?: string;
    availableVariables?: AvailableVariable[] | null;
    view: DesignerView;
    appearance: "slots" | "prose";
  } = $props();

  const settingsByID = $derived(new Map((settings ?? []).map((definition) => [definition.id, definition])));
  const actions = useDesignerActions();
</script>

<span class="sentence" class:is-prose={appearance === "prose"}>
  {#each sentence ?? [] as part, partIndex (partIndex)}
    {#if appearance === "prose" && partIndex > 0}{" "}{/if}
    {#if part.words}
      <span class="words">{part.words}</span>
    {:else if part.userText}
      <span class="user-text">{part.userText}</span>
    {:else if part.settingId}
      {@const definition = settingsByID.get(part.settingId)}
      {@const value = values?.[part.settingId]}
      {#if definition}
        {@const isEmpty = isEmptyValue(definition.kind, value)}
        {@const key = settingKey(ownerID, part.settingId)}
        {@const settingID = part.settingId}
        {#if appearance === "slots" && actions && isEditableKind(definition.kind)}
          <button
            type="button"
            class="value"
            class:is-empty={isEmpty}
            class:has-problem={view.settingsWithProblems.has(key)}
            class:is-open={actions.openAnchor?.dataset.setting === key}
            data-setting={key}
            onclick={(event) => actions.editSetting(ownerID, settingID, event.currentTarget)}
          >
            {#if isEmpty}
              {definition.placeholder || "not set"}
            {:else}
              <SettingValue {definition} {value} {availableVariables} {view}/>
            {/if}
          </button>
        {:else}
          <span class="value" class:is-empty={isEmpty} class:has-problem={view.settingsWithProblems.has(key)} data-setting={key}>
            {#if isEmpty}
              {definition.placeholder || "not set"}
            {:else}
              <SettingValue {definition} {value} {availableVariables} {view}/>
            {/if}
          </span>
        {/if}
      {/if}
    {/if}
  {/each}
</span>

<style>
  .sentence {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) 6px;
    min-width: 0;
    font-size: var(--font-size-sentence);
    line-height: 1.7;
  }

  .value {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1);
    max-width: 100%;
    padding: 0 7px;
    border-bottom: 1.5px dashed var(--color-accent-line);
    border-radius: var(--radius);
    background: var(--color-accent-soft);
    font-weight: 500;
  }

  button.value {
    text-align: left;
    line-height: 1.7;
  }

  button.value:hover {
    background: var(--color-accent-hover);
  }

  .value.is-open {
    outline: 2px solid var(--color-accent);
    outline-offset: 1px;
  }

  .value.is-empty {
    color: var(--color-text-muted);
    font-style: italic;
    font-weight: 400;
  }

  .value.has-problem {
    border-bottom-color: var(--color-danger);
    background: var(--color-danger-soft);
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

  /* Read it back: the values stand out by colour only, so it reads as text. */
  .sentence.is-prose {
    display: inline;
    line-height: 1.75;
  }

  .is-prose .value {
    display: inline;
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent);
  }

  .is-prose .value.is-empty,
  .is-prose .value.has-problem {
    color: var(--color-danger);
    background: none;
  }
</style>
