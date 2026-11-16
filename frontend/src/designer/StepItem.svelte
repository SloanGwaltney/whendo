<script lang="ts">
  import {AfterBranch, ProblemLevel} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Step} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {VariableSource} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {branchListKey} from "./draftEditing";
  import {stepElementID, useDesignerActions} from "./designerModel";
  import type {DesignerView} from "./designerModel";
  import Sentence from "./Sentence.svelte";
  import StepList from "./StepList.svelte";
  import ValuePill from "./ValuePill.svelte";

  let {step, number, depth, view}: {
    step: Step;
    // The step's number, or null for steps inside a path, which aren't
    // numbered.
    number: number | null;
    depth: number;
    view: DesignerView;
  } = $props();

  const definition = $derived(view.stepDefinitionsByType.get(step.type));
  const stepReport = $derived(view.report.steps?.[step.id]);
  const outcomes = $derived(stepReport?.outcomes ?? []);
  const isSplit = $derived(outcomes.length > 1);
  const problemLevel = $derived(view.problemLevelsByStepID.get(step.id));
  const isUnreachable = $derived(stepReport !== undefined && !stepReport.isReachable);
  const actions = useDesignerActions();
  const isMenuOpen = $derived(actions?.openAnchor?.dataset.stepMenu === step.id);
</script>

{#snippet endingControl(outcomeID: string, ending: string, isEnd: boolean)}
  {#if actions}
    <button
      type="button"
      class="ending"
      class:is-end={isEnd}
      data-ending="{step.id}/{outcomeID}"
      onclick={(event) => actions.openEndingMenu(step.id, outcomeID, event.currentTarget)}
    >{ending}</button>
  {:else}
    <span class="ending" class:is-end={isEnd}>{ending}</span>
  {/if}
{/snippet}

<li class="step-item" class:is-unreachable={isUnreachable}>
  <div class="step" id={stepElementID(step.id)}>
    <span
      class="number"
      class:is-nested={number === null}
      class:has-must-fix={problemLevel === ProblemLevel.ProblemLevelMustFix}
      class:has-good-to-know={problemLevel === ProblemLevel.ProblemLevelGoodToKnow}
    >{number ?? ""}</span>
    {#if definition}
      <Sentence
        sentence={definition.sentence}
        settings={definition.settings}
        values={step.settings}
        ownerID={step.id}
        availableVariables={stepReport?.availableVariables ?? []}
        {view}
        appearance="slots"
      />
    {:else}
      <span class="unknown-step">A step Whendo doesn't have: {step.type}</span>
    {/if}
    {#if actions}
      <button
        type="button"
        class="menu-button"
        aria-label="Step options"
        aria-expanded={isMenuOpen}
        data-step-menu={step.id}
        onclick={(event) => actions.openStepMenu(step.id, event.currentTarget)}
      >⋯</button>
    {/if}
  </div>

  {#if isSplit}
    <div class="paths">
      {#each outcomes as outcome (outcome.id)}
        {@const branch = step.outcomes?.[outcome.id]}
        {@const branchSteps = branch?.steps ?? []}
        {@const ending = branch?.afterBranch === AfterBranch.AfterBranchEnd ? "stop here" : "carry on below"}
        {@const commonVariables = (outcome.variables ?? []).filter((variable) => variable.isCommon)}
        <section class="path">
          <div class="path-head">
            <span class="path-label">
              {#if outcome.sentence?.length}
                <Sentence sentence={outcome.sentence} {view} appearance="slots"/>
              {:else}
                {outcome.displayName}
              {/if}
            </span>
            {#if branchSteps.length === 0}
              <!-- A path with no steps takes one line. -->
              <span class="path-ending">
                then {@render endingControl(outcome.id, ending, branch?.afterBranch === AfterBranch.AfterBranchEnd)}
                {#if actions}
                  <button type="button" class="add-inline" onclick={(event) => actions.openStepPicker(branchListKey(step.id, outcome.id), 0, event.currentTarget)}>+ Add a step</button>
                {/if}
              </span>
            {:else if commonVariables.length > 0}
              <span class="path-gives">
                gives you
                {#each commonVariables as variable (variable.id)}
                  <ValuePill reference={{source: VariableSource.VariableSourceStep, stepId: step.id, id: variable.id}} {view} isBroken={false}/>
                {/each}
              </span>
            {/if}
          </div>
          {#if branchSteps.length > 0}
            <StepList steps={branchSteps} listKey={branchListKey(step.id, outcome.id)} depth={depth + 1} {view}/>
            <div class="path-foot">then {@render endingControl(outcome.id, ending, branch?.afterBranch === AfterBranch.AfterBranchEnd)}</div>
          {/if}
        </section>
      {/each}
    </div>
  {/if}
</li>

<style>
  .step-item {
    display: flex;
    flex-direction: column;
  }

  .step {
    display: grid;
    grid-template-columns: 28px minmax(0, 1fr) auto;
    gap: 10px;
    align-items: start;
    padding: var(--space-2) 6px;
    border-radius: var(--radius-large);
  }

  .step:global(.is-flashing) {
    animation: flash 1.2s ease-out;
  }

  @keyframes flash {
    from {
      background: var(--color-accent-hover);
    }
    to {
      background: transparent;
    }
  }

  .is-unreachable > .step {
    opacity: 0.45;
  }

  .number {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    margin-top: 3px;
    border: 1px solid var(--color-border-strong);
    border-radius: 50%;
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .number.is-nested {
    border: 0;
  }

  .number.is-nested::before {
    content: "";
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-text-faint);
  }

  .number.has-must-fix {
    border-color: var(--color-danger);
    color: var(--color-danger);
  }

  .number.has-must-fix.is-nested::before {
    background: var(--color-danger);
  }

  .number.has-good-to-know {
    border-color: var(--color-warning);
    color: var(--color-warning);
  }

  .number.has-good-to-know.is-nested::before {
    background: var(--color-warning);
  }

  .menu-button {
    width: 28px;
    height: 28px;
    margin-top: 2px;
    border-radius: var(--radius);
    color: var(--color-text-faint);
    font-size: 18px;
    line-height: 1;
  }

  .menu-button:hover,
  .menu-button[aria-expanded="true"] {
    background: var(--color-surface-raised);
    color: var(--color-text);
  }

  .add-inline {
    padding: 0 6px;
    border-radius: var(--radius-small);
    color: var(--color-accent);
    font-size: 13px;
  }

  .add-inline:hover {
    background: var(--color-accent-soft);
  }

  .unknown-step {
    color: var(--color-danger);
    font-size: var(--font-size-sentence);
    line-height: 1.7;
  }

  .paths {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin: 2px 0 6px 18px;
    padding-left: var(--space-4);
    border-left: 1.5px solid var(--color-border-strong);
  }

  .path-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-2);
    padding: var(--space-1) 6px 0;
  }

  .path-label {
    display: inline-flex;
    align-items: center;
    font-weight: 600;
  }

  .path-label::before {
    content: "";
    width: 12px;
    margin-left: -22px;
    margin-right: var(--space-1);
    border-top: 1.5px solid var(--color-border-strong);
  }

  .path-gives,
  .path-ending {
    display: inline-flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1);
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .path-ending {
    font-size: var(--font-size-body);
  }

  .path-foot {
    padding: 2px 6px 0 44px;
    color: var(--color-text-muted);
  }

  .ending {
    padding: 0 7px;
    border-bottom: 1.5px dashed var(--color-accent-line);
    border-radius: var(--radius);
    background: var(--color-accent-soft);
    color: var(--color-text);
  }

  button.ending:hover {
    background: var(--color-accent-hover);
  }

  .ending.is-end {
    border-bottom-color: var(--color-danger);
    background: var(--color-danger-soft);
  }
</style>
