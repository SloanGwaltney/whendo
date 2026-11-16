<script lang="ts">
  import type {Step} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {DesignerView} from "./designerModel";
  import StepItem from "./StepItem.svelte";

  // A sequence of steps: the automation's own, or one path of a step that
  // splits. Only top-level steps are numbered; see
  // docs/design/mvp-ui.md#automation-designer.
  let {steps, depth, view}: {steps: Step[] | null; depth: number; view: DesignerView} = $props();
</script>

<ol class="steps">
  <!-- Keyed by position as well as ID, since a file edited by hand could
       give two steps the same ID, which the problems list then reports. -->
  {#each steps ?? [] as step, stepIndex (`${stepIndex}:${step.id}`)}
    <StepItem {step} number={depth === 0 ? stepIndex + 1 : null} {depth} {view}/>
  {/each}
</ol>

<style>
  .steps {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
