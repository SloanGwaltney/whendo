<script lang="ts">
  import {AfterBranch} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Step} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {useDesignerActions} from "./designerModel";
  import type {DesignerView} from "./designerModel";
  import StepItem from "./StepItem.svelte";

  // A sequence of steps: the automation's own, or one path of a step that
  // splits. Only top-level steps are numbered; see
  // docs/design/mvp-ui.md#automation-designer.
  let {steps, listKey, depth, view}: {
    steps: Step[] | null;
    // Which list this is, for adding steps to it; see draftEditing.ts.
    listKey: string;
    depth: number;
    view: DesignerView;
  } = $props();

  const actions = useDesignerActions();

  // After a split where every path stops, nothing more can run, so the list
  // says so instead of offering to add steps there.
  const doesEveryPathStop = $derived.by(() => {
    const lastStep = steps?.at(-1);
    const outcomes = lastStep ? (view.report.steps?.[lastStep.id]?.outcomes ?? []) : [];
    return outcomes.length > 1 && outcomes.every((outcome) => lastStep?.outcomes?.[outcome.id]?.afterBranch === AfterBranch.AfterBranchEnd);
  });
</script>

<ol class="steps">
  <!-- Keyed by position as well as ID, since a file edited by hand could
       give two steps the same ID, which the problems list then reports. -->
  {#each steps ?? [] as step, stepIndex (`${stepIndex}:${step.id}`)}
    {#if actions}
      <li class="insert-row">
        <button type="button" class="insert" aria-label="Add a step here" onclick={(event) => actions.openStepPicker(listKey, stepIndex, event.currentTarget)}></button>
      </li>
    {/if}
    <StepItem {step} number={depth === 0 ? stepIndex + 1 : null} {depth} {view}/>
  {/each}
  {#if doesEveryPathStop}
    <li class="every-path-stops">Every path above stops, so nothing after this runs.</li>
  {:else if actions}
    <li>
      <button type="button" class="add-step" onclick={(event) => actions.openStepPicker(listKey, steps?.length ?? 0, event.currentTarget)}>+ Add a step</button>
    </li>
  {/if}
</ol>

<style>
  .steps {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* A thin line between steps that shows a + on hover, for adding a step
     there. */
  .insert {
    position: relative;
    display: block;
    width: calc(100% - 44px);
    height: 10px;
    margin: -2px 0 -2px 44px;
    opacity: 0;
  }

  .insert::before {
    content: "";
    position: absolute;
    left: 0;
    right: 40px;
    top: 50%;
    border-top: 1.5px solid var(--color-accent);
  }

  .insert::after {
    content: "+";
    position: absolute;
    left: -26px;
    top: 50%;
    width: 18px;
    height: 18px;
    transform: translateY(-50%);
    border-radius: 50%;
    background: var(--color-accent);
    color: var(--color-on-accent);
    font-size: 14px;
    font-weight: 700;
    line-height: 18px;
    text-align: center;
  }

  .insert:hover,
  .insert:focus-visible {
    opacity: 1;
  }

  .add-step {
    margin: var(--space-1) 0 0 44px;
    padding: var(--space-1) 10px;
    border: 1px dashed var(--color-accent-line);
    border-radius: var(--radius);
    color: var(--color-accent);
    font-weight: 500;
  }

  .add-step:hover {
    background: var(--color-accent-soft);
  }

  .every-path-stops {
    margin-left: 44px;
    color: var(--color-text-muted);
    font-size: 13px;
    font-style: italic;
  }
</style>
