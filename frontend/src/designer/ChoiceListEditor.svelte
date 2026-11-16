<script lang="ts">
  import {tick, untrack} from "svelte";
  import type {Choice} from "./designerModel";
  import {newChoiceID} from "./draftEditing";

  // Edits a list of choices, such as the commands "wait for a chat message"
  // waits for. Each choice keeps its ID as its text changes, so its path in
  // the automation stays with it.
  let {value, placeholder, onChange, onDone}: {
    value: unknown;
    placeholder: string;
    // Called with null when no choice has any text.
    onChange: (newValue: unknown) => void;
    onDone: () => void;
  } = $props();

  // The editor starts from the value when it opens, then owns the list, so
  // a choice that's still blank stays on screen while it's typed.
  let choices = $state<Choice[]>(untrack(() => (Array.isArray(value) && value.length > 0 ? (value as Choice[]).map((choice) => ({...choice})) : [{id: newChoiceID(), text: ""}])));

  // Blank choices aren't saved, so a choice being typed never shows up as a
  // "blank choice" problem, and clearing a choice's text removes it.
  function publish(): void {
    const filledChoices = choices.filter((choice) => choice.text.trim() !== "").map((choice) => ({...choice}));
    onChange(filledChoices.length === 0 ? null : filledChoices);
  }

  let listElement: HTMLUListElement | undefined = $state();

  // The new choice gets the keyboard, which also scrolls it into view when
  // the list is longer than the popover.
  async function addChoice(): Promise<void> {
    choices.push({id: newChoiceID(), text: ""});
    await tick();
    const inputs = listElement?.querySelectorAll<HTMLInputElement>("input");
    inputs?.[inputs.length - 1]?.focus();
  }

  function removeChoice(choiceIndex: number): void {
    choices.splice(choiceIndex, 1);
    publish();
  }

  function finishOnEnter(event: KeyboardEvent): void {
    if (event.key === "Enter") {
      event.preventDefault();
      onDone();
    }
  }
</script>

<ul class="choices" bind:this={listElement}>
  {#each choices as choice, choiceIndex (choice.id)}
    <li class="choice">
      <input
        class="field"
        bind:value={choice.text}
        oninput={publish}
        onkeydown={finishOnEnter}
        placeholder={choiceIndex === 0 ? "type a choice, like !a1" : "type another choice"}
        aria-label={`Choice ${choiceIndex + 1}`}
        data-autofocus={choiceIndex === 0 ? true : undefined}
      />
      <button type="button" class="button remove" aria-label={`Remove choice ${choiceIndex + 1}`} title="Remove" onclick={() => removeChoice(choiceIndex)}>✕</button>
    </li>
  {/each}
</ul>
<button type="button" class="button" onclick={addChoice}>Add a choice</button>
{#if choices.length === 0}
  <p class="help">No choices yet: {placeholder}.</p>
{/if}

<style>
  .choices {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .choice {
    display: flex;
    gap: var(--space-2);
  }

  .remove {
    flex: none;
  }

  .help {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }
</style>
