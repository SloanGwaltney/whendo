<script lang="ts">
  import {AutomationsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {Definition as TriggerDefinition} from "../bindings/github.com/SloanGwaltney/whendo/engine/trigger";
  import Dialog from "./Dialog.svelte";

  // The test run form: the trigger's values, starting from ones its trigger
  // works out, for running a saved automation by hand. See
  // docs/design/mvp-ui.md#test-runs.
  let {automationID, automationName, triggerDefinition, onStarted, onClose}: {
    // The automation to test, or empty when the form is closed.
    automationID: string;
    automationName: string;
    triggerDefinition: TriggerDefinition | null;
    onStarted: (runID: string) => void;
    onClose: () => void;
  } = $props();

  let values = $state<Record<string, string>>({});
  let isShowingEveryValue = $state(false);
  let isLoading = $state(false);
  let isStarting = $state(false);
  let mismatchReason = $state("");
  let errorMessage = $state("");

  // Values the trigger's settings decide, or its filter adds, can't be
  // changed in a way that still makes sense, so the form doesn't ask.
  const askedVariables = $derived((triggerDefinition?.variables ?? []).filter((variable) => !variable.isAddedByFilter && !variable.isSetBySettings));
  const shownVariables = $derived(isShowingEveryValue ? askedVariables : askedVariables.filter((variable) => variable.isCommon));
  const hiddenVariableCount = $derived(askedVariables.length - shownVariables.length);

  $effect(() => {
    if (automationID) {
      loadTestValues(automationID);
    }
  });

  async function loadTestValues(testedAutomationID: string): Promise<void> {
    isLoading = true;
    isShowingEveryValue = false;
    mismatchReason = "";
    errorMessage = "";
    try {
      const testValues = (await AutomationsService.TestValues(testedAutomationID)) ?? {};
      values = Object.fromEntries(Object.entries(testValues).map(([variableID, value]) => [variableID, value ?? ""]));
    } catch (error) {
      errorMessage = `Couldn't prepare the test: ${error}`;
    } finally {
      isLoading = false;
    }
  }

  async function startTestRun(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (isStarting || isLoading) {
      return;
    }
    isStarting = true;
    mismatchReason = "";
    errorMessage = "";
    try {
      const started = await AutomationsService.TestRun(automationID, $state.snapshot(values));
      if (started.mismatchReason) {
        mismatchReason = started.mismatchReason;
        return;
      }
      onStarted(started.runId);
    } catch (error) {
      errorMessage = `Couldn't start the test: ${error}`;
    } finally {
      isStarting = false;
    }
  }
</script>

<Dialog title="Test run" isOpen={automationID !== ""} {onClose}>
  <form class="test-form" onsubmit={startTestRun}>
    <p class="intro">Runs “{automationName}” as if a {triggerDefinition?.displayName.toLowerCase() ?? "real event"} came in with these values. It waits behind any run already going.</p>
    <p class="warning"><strong>Steps really happen.</strong> Chat messages are really sent.</p>

    {#each shownVariables as variable (variable.id)}
      <label class="value-field">
        <span>{variable.displayName}</span>
        <input class="field" bind:value={values[variable.id]} disabled={isLoading}/>
      </label>
    {/each}
    {#if hiddenVariableCount > 0}
      <button type="button" class="more" onclick={() => (isShowingEveryValue = true)}>+ {hiddenVariableCount} more values</button>
    {/if}

    {#if mismatchReason}
      <p class="mismatch" role="alert">{mismatchReason}</p>
    {/if}
    {#if errorMessage}
      <p class="mismatch is-selectable" role="alert">{errorMessage}</p>
    {/if}

    <div class="dialog-actions">
      <button type="button" class="button" onclick={onClose}>Cancel</button>
      <button type="submit" class="button is-primary" disabled={isLoading || isStarting}>{isStarting ? "Starting…" : "Run test"}</button>
    </div>
  </form>
</Dialog>

<style>
  .test-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .intro {
    color: var(--color-text-muted);
  }

  .warning {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-warning-soft);
    color: var(--color-text);
  }

  .value-field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    font-size: var(--font-size-small);
    color: var(--color-text-muted);
  }

  .value-field input {
    font-size: var(--font-size-body);
    color: var(--color-text);
  }

  .more {
    align-self: flex-start;
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .mismatch {
    color: var(--color-danger);
  }
</style>
