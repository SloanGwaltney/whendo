<script lang="ts">
  import {AutomationsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {AutomationSummary, CatalogDefinitions} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {Automation} from "../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import AutomationRow from "./AutomationRow.svelte";
  import Dialog from "./Dialog.svelte";
  import Page from "./Page.svelte";

  // The automation list. See docs/design/mvp-ui.md#home. The sample gallery
  // comes later.
  let {onOpenAutomation}: {onOpenAutomation: (automationID: string) => void} = $props();

  let summaries = $state<AutomationSummary[]>([]);
  let catalog = $state<CatalogDefinitions | null>(null);
  let hasLoaded = $state(false);
  let errorMessage = $state("");

  let automationToDelete = $state<AutomationSummary | null>(null);
  let isCreating = $state(false);
  let newAutomationName = $state("");

  const triggerNamesByID = $derived(new Map((catalog?.triggers ?? []).map((definition) => [definition.id, definition.displayName])));

  loadPage();

  async function loadPage(): Promise<void> {
    try {
      catalog = await AutomationsService.Catalog();
      await loadAutomations();
    } catch (error) {
      errorMessage = `Couldn't load your automations: ${error}`;
    }
  }

  async function loadAutomations(): Promise<void> {
    summaries = (await AutomationsService.ListAutomations()) ?? [];
    hasLoaded = true;
  }

  // runAction runs one change, then reloads the list so it shows what was
  // actually saved, whether the change worked or not.
  async function runAction(failureMessage: string, action: () => Promise<unknown>): Promise<void> {
    errorMessage = "";
    try {
      await action();
    } catch (error) {
      errorMessage = `${failureMessage}: ${error}`;
    }
    try {
      await loadAutomations();
    } catch (error) {
      errorMessage = `Couldn't reload your automations: ${error}`;
    }
  }

  function toggle(summary: AutomationSummary): Promise<void> {
    const verb = summary.isEnabled ? "turn off" : "turn on";
    return runAction(`Couldn't ${verb} “${summary.name}”`, () => AutomationsService.SetAutomationEnabled(summary.id, !summary.isEnabled));
  }

  async function confirmDelete(): Promise<void> {
    const deleted = automationToDelete;
    automationToDelete = null;
    if (!deleted) {
      return;
    }
    await runAction(`Couldn't delete “${deleted.name}”`, () => AutomationsService.DeleteAutomation(deleted.id));
  }

  function startCreating(): void {
    newAutomationName = "";
    isCreating = true;
  }

  // A new automation has no trigger yet, so it's saved turned off and
  // needing fixing until it's built, then opens in the designer. See
  // docs/design/mvp-ui.md#home.
  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const name = newAutomationName.trim();
    if (!name) {
      return;
    }
    isCreating = false;
    let newAutomationID = "";
    await runAction(`Couldn't create “${name}”`, async () => {
      newAutomationID = await AutomationsService.NewAutomationID();
      const newAutomation: Automation = {
        id: newAutomationID,
        name,
        isEnabled: false,
        trigger: {id: "", settings: {}},
        steps: [],
      };
      await AutomationsService.SaveAutomation(newAutomation);
    });
    if (!errorMessage) {
      onOpenAutomation(newAutomationID);
    }
  }
</script>

<Page title="Home">
  <div class="home">
    <div class="toolbar">
      <h2>Your automations</h2>
      <button type="button" class="button is-primary" onclick={startCreating}>New automation</button>
    </div>

    {#if errorMessage}
      <p class="error is-selectable" role="alert">{errorMessage}</p>
    {/if}

    {#if hasLoaded && summaries.length === 0}
      <div class="empty">
        <p>You don't have any automations yet.</p>
        <p class="hint">An automation reacts to something on your stream, like a redeem or a chat command, with steps you choose.</p>
      </div>
    {:else}
      <ul class="automation-list">
        {#each summaries as summary (summary.id)}
          <AutomationRow
            {summary}
            triggerName={triggerNamesByID.get(summary.triggerId) ?? ""}
            onOpen={() => onOpenAutomation(summary.id)}
            onToggle={() => toggle(summary)}
            onDelete={() => (automationToDelete = summary)}
          />
        {/each}
      </ul>
    {/if}
  </div>
</Page>

<Dialog title="Delete this automation?" isOpen={automationToDelete !== null} onClose={() => (automationToDelete = null)}>
  <p>“{automationToDelete?.name}” will be deleted. Runs waiting in its queue are cancelled. This can't be undone.</p>
  <div class="dialog-actions">
    <button type="button" class="button" onclick={() => (automationToDelete = null)}>Cancel</button>
    <button type="button" class="button is-danger" onclick={confirmDelete}>Delete</button>
  </div>
</Dialog>

<Dialog title="New automation" isOpen={isCreating} onClose={() => (isCreating = false)}>
  <form class="new-automation" onsubmit={create}>
    <label for="new-automation-name">Name</label>
    <!-- svelte-ignore a11y_autofocus -->
    <input id="new-automation-name" class="field" bind:value={newAutomationName} placeholder="Do a Voice" autofocus />
    <div class="dialog-actions">
      <button type="button" class="button" onclick={() => (isCreating = false)}>Cancel</button>
      <button type="submit" class="button is-primary" disabled={!newAutomationName.trim()}>Create</button>
    </div>
  </form>
</Dialog>

<style>
  .home {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 880px;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  h2 {
    font-size: 13px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .automation-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .empty {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-6) var(--space-5);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-large);
    text-align: center;
  }

  .hint {
    color: var(--color-text-muted);
  }

  .error {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }

  .new-automation {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  label {
    color: var(--color-text-muted);
    font-size: 13px;
  }
</style>
