<script lang="ts">
  import type {Definition as StepDefinition} from "../../bindings/github.com/SloanGwaltney/whendo/engine/step";

  // Every step, grouped by what it acts on and searchable.
  let {definitions, onChoose}: {definitions: StepDefinition[]; onChoose: (stepType: string) => void} = $props();

  let query = $state("");

  const groups = $derived.by(() => {
    const normalizedQuery = query.trim().toLowerCase();
    const matching = definitions.filter((definition) => `${definition.displayName} ${definition.description}`.toLowerCase().includes(normalizedQuery));
    const groupsByName = new Map<string, StepDefinition[]>();
    for (const definition of matching) {
      const group = groupsByName.get(definition.group) ?? [];
      group.push(definition);
      groupsByName.set(definition.group, group);
    }
    return [...groupsByName.entries()];
  });
</script>

<input class="field" placeholder="Find a step" aria-label="Find a step" bind:value={query} data-autofocus/>
<div class="groups">
  {#each groups as [groupName, groupDefinitions] (groupName)}
    <div class="group-label">{groupName}</div>
    {#each groupDefinitions as definition (definition.id)}
      <button type="button" class="step-option" onclick={() => onChoose(definition.id)}>
        <span>{definition.displayName}</span>
        <span class="description">{definition.description}</span>
      </button>
    {/each}
  {:else}
    <p class="nothing">No steps match “{query}”.</p>
  {/each}
</div>

<style>
  .groups {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .group-label {
    padding: var(--space-2) var(--space-2) 2px;
    color: var(--color-text-faint);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }

  .step-option {
    display: flex;
    flex-direction: column;
    padding: 7px var(--space-2);
    border-radius: var(--radius);
    text-align: left;
  }

  .step-option:hover,
  .step-option:focus-visible {
    background: var(--color-surface);
  }

  .description,
  .nothing {
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
  }
</style>
