<script lang="ts">
  import type {Definition as TriggerDefinition} from "../bindings/github.com/SloanGwaltney/whendo/engine/trigger";
  import {RunStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/runner";
  import type {RunRecord} from "../bindings/github.com/SloanGwaltney/whendo/engine/runner";
  import Dialog from "./Dialog.svelte";
  import {formatElapsed, formatTime, isTimeSet, stepName} from "./runHistory";
  import RunStatusBadge from "./RunStatusBadge.svelte";

  // One run opened from Run History: what set it off, and each step it ran
  // with the branch it took or why it failed. See
  // docs/design/mvp-ui.md#run-history.
  let {record, triggerDefinition, onClose}: {
    // The run to show, or null when the dialog is closed.
    record: RunRecord | null;
    // The trigger that set the run off, or null if Whendo no longer has it.
    triggerDefinition: TriggerDefinition | null;
    onClose: () => void;
  } = $props();

  interface ShownVariable {
    id: string;
    name: string;
    value: string;
  }

  // The trigger's variables in the order it declares them, with any the
  // record has that the trigger no longer declares after them.
  const shownVariables: ShownVariable[] = $derived.by(() => {
    const values = record?.triggerVariables ?? {};
    const declaredVariables = triggerDefinition?.variables ?? [];
    const declaredIDs = new Set(declaredVariables.map((variable) => variable.id));
    const shown = declaredVariables.map((variable) => ({id: variable.id, name: variable.displayName, value: values[variable.id] ?? ""}));
    for (const [variableID, value] of Object.entries(values)) {
      if (!declaredIDs.has(variableID)) {
        shown.push({id: variableID, name: variableID, value: value ?? ""});
      }
    }
    return shown;
  });

  const hasStarted = $derived(record !== null && isTimeSet(record.startedAt));
  // A failure with no failed step happened before any step ran, such as a
  // full queue.
  const runProblem = $derived(record && record.status !== RunStatus.RunStatusSucceeded && !record.steps?.some((stepRecord) => stepRecord.failureReason) ? (record.failureReason ?? "") : "");
</script>

<Dialog title={record?.automationName ?? ""} isOpen={record !== null} {onClose} isWide>
  {#if record}
    <div class="summary">
      <RunStatusBadge status={record.status} isTest={record.isTest}/>
      <span>Triggered at {formatTime(record.queuedAt)}</span>
      {#if hasStarted}
        <span>Took {formatElapsed(record.startedAt, record.endedAt)}</span>
      {:else}
        <span>Never started</span>
      {/if}
    </div>

    {#if runProblem}
      <p class="problem is-selectable">{runProblem}</p>
    {/if}

    <section>
      <h3>{triggerDefinition ? `When: ${triggerDefinition.displayName}` : "What set it off"}</h3>
      {#if shownVariables.length > 0}
        <dl class="variables">
          {#each shownVariables as variable (variable.id)}
            <dt>{variable.name}</dt>
            <dd class="is-selectable" class:is-empty={variable.value === ""}>{variable.value || "empty"}</dd>
          {/each}
        </dl>
      {:else}
        <p class="muted">No values.</p>
      {/if}
    </section>

    <section>
      <h3>Steps</h3>
      {#if record.steps?.length}
        <ol class="steps">
          {#each record.steps as stepRecord, stepIndex (stepIndex)}
            <li class="step" class:has-failed={stepRecord.failureReason}>
              <span class="mark" aria-hidden="true">{stepRecord.failureReason ? "✕" : "✓"}</span>
              <div class="step-details">
                <span class="step-name">{stepName(stepRecord)}</span>
                {#if stepRecord.failureReason}
                  <span class="failure is-selectable">{stepRecord.failureReason}</span>
                {:else if stepRecord.branchLabel}
                  <span class="branch">Went down the path “{stepRecord.branchLabel}”</span>
                {/if}
                {#each stepRecord.values ?? [] as providedValue (providedValue.id)}
                  <span class="provided-value">{providedValue.name}: <span class="value is-selectable" class:is-empty={providedValue.value === ""}>{providedValue.value || "empty"}</span></span>
                {/each}
                {#each stepRecord.details ?? [] as detail, detailIndex (detailIndex)}
                  <span class="provided-value">{detail.name}: <span class="value is-selectable" class:is-empty={detail.value === ""}>{detail.value || "empty"}</span></span>
                {/each}
              </div>
            </li>
          {/each}
        </ol>
      {:else}
        <p class="muted">No steps ran.</p>
      {/if}
    </section>

    <div class="dialog-actions">
      <button type="button" class="button" onclick={onClose}>Close</button>
    </div>
  {/if}
</Dialog>

<style>
  .summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    color: var(--color-text-muted);
  }

  .problem {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }

  section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h3 {
    font-size: 13px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .variables {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: var(--space-1) var(--space-4);
    margin: 0;
  }

  dt {
    color: var(--color-text-muted);
  }

  dd {
    margin: 0;
    color: var(--color-value);
    overflow-wrap: anywhere;
  }

  dd.is-empty,
  .muted {
    color: var(--color-text-faint);
  }

  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .step {
    display: flex;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border);
    border-radius: var(--radius);
    background: var(--color-surface);
  }

  .step.has-failed {
    border-color: var(--color-danger);
  }

  .mark {
    flex: none;
    width: 1em;
    color: var(--color-success);
    font-weight: 700;
  }

  .has-failed .mark {
    color: var(--color-danger);
  }

  .step-details {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .step-name {
    font-weight: 600;
  }

  .branch {
    color: var(--color-text-muted);
  }

  .provided-value {
    color: var(--color-text-muted);
  }

  .value {
    color: var(--color-value);
    overflow-wrap: anywhere;
  }

  .value.is-empty {
    color: var(--color-text-faint);
  }

  .failure {
    color: var(--color-danger);
    overflow-wrap: anywhere;
  }
</style>
