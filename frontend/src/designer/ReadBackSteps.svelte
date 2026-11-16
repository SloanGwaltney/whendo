<script lang="ts">
  import {AfterBranch} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Step} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {DesignerView} from "./designerModel";
  import ReadBackSteps from "./ReadBackSteps.svelte";
  import Sentence from "./Sentence.svelte";

  // One sequence of steps as numbered plain text, with each path below the
  // step that splits.
  let {steps, view}: {steps: Step[] | null; view: DesignerView} = $props();
</script>

<ol>
  {#each steps ?? [] as step, stepIndex (`${stepIndex}:${step.id}`)}
    {@const definition = view.stepDefinitionsByType.get(step.type)}
    {@const stepReport = view.report.steps?.[step.id]}
    {@const outcomes = stepReport?.outcomes ?? []}
    <li>
      {#if definition}
        <Sentence
          sentence={definition.sentence}
          settings={definition.settings}
          values={step.settings}
          ownerID={step.id}
          availableVariables={stepReport?.availableVariables ?? []}
          {view}
          appearance="prose"
        />.
      {:else}
        A step Whendo doesn't have: {step.type}.
      {/if}
      {#if outcomes.length > 1}
        {#each outcomes as outcome (outcome.id)}
          {@const branch = step.outcomes?.[outcome.id]}
          {@const ending = branch?.afterBranch === AfterBranch.AfterBranchEnd ? "then stop" : "then carry on"}
          <div class="path">
            <strong>
              {#if outcome.sentence?.length}<Sentence sentence={outcome.sentence} {view} appearance="prose"/>{:else}{outcome.displayName}{/if}:
            </strong>
            {#if branch?.steps?.length}
              <ReadBackSteps steps={branch.steps} {view}/>
              <span>{ending}.</span>
            {:else}
              nothing extra, {ending}.
            {/if}
          </div>
        {/each}
      {/if}
    </li>
  {/each}
</ol>

<style>
  ol {
    margin: 0;
    padding-left: 22px;
  }

  li {
    margin: var(--space-1) 0;
  }

  li::marker {
    color: var(--color-text-faint);
    font-variant-numeric: tabular-nums;
  }

  .path {
    margin-top: var(--space-1);
    color: var(--color-text-muted);
  }

  .path strong {
    color: var(--color-text);
    font-weight: 600;
  }
</style>
