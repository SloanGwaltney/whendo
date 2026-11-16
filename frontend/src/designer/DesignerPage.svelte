<script lang="ts">
  import {AutomationsService} from "../../bindings/github.com/SloanGwaltney/whendo";
  import type {CatalogDefinitions} from "../../bindings/github.com/SloanGwaltney/whendo";
  import {ProblemSubject} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import type {Automation, Problem, Report} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {buildDesignerView, stepElementID, triggerElementID} from "./designerModel";
  import DesignerProblems from "./DesignerProblems.svelte";
  import ReadBack from "./ReadBack.svelte";
  import StepList from "./StepList.svelte";
  import TriggerCard from "./TriggerCard.svelte";

  // The automation designer. See docs/design/mvp-ui.md#automation-designer.
  // Editing and saving come next; for now it shows an automation as it's
  // saved, with its problems.
  let {automationID, onClose}: {automationID: string; onClose: () => void} = $props();

  type DesignerMode = "build" | "readBack";

  let catalog = $state<CatalogDefinitions | null>(null);
  let draft = $state<Automation | null>(null);
  let report = $state<Report | null>(null);
  let mode = $state<DesignerMode>("build");
  let errorMessage = $state("");

  const view = $derived(catalog && report && draft ? buildDesignerView(catalog, report, draft) : null);

  loadAutomation();

  async function loadAutomation(): Promise<void> {
    try {
      const [loadedCatalog, loadedAutomation] = await Promise.all([AutomationsService.Catalog(), AutomationsService.GetAutomation(automationID)]);
      report = await AutomationsService.CheckAutomation(loadedAutomation);
      catalog = loadedCatalog;
      draft = loadedAutomation;
    } catch (error) {
      errorMessage = `Couldn't open this automation: ${error}`;
    }
  }

  // Jumping to a problem switches to Build, since Read it back has no
  // problem marks to find.
  async function jumpTo(problem: Problem): Promise<void> {
    mode = "build";
    const isAboutStep = problem.subject === ProblemSubject.ProblemSubjectStep && problem.stepId;
    const elementID = isAboutStep ? stepElementID(problem.stepId!) : triggerElementID;
    // Wait a frame, so the Build view is on the page after switching.
    await new Promise(requestAnimationFrame);
    const element = document.getElementById(elementID);
    if (!element) {
      return;
    }
    element.scrollIntoView({block: "center", behavior: "smooth"});
    element.classList.remove("is-flashing");
    // Reading the size restarts the animation when the same step is
    // flashed twice in a row.
    void element.offsetWidth;
    element.classList.add("is-flashing");
  }
</script>

<section class="designer">
  <header class="designer-header">
    <button type="button" class="back" onclick={onClose}>Home ›</button>
    <h1>{draft?.name ?? ""}</h1>
    {#if draft}
      <span class="state" class:is-on={draft.isEnabled}>{draft.isEnabled ? "On" : "Off"}</span>
    {/if}
    <div class="mode-tabs" role="group" aria-label="View">
      <button type="button" aria-pressed={mode === "build"} onclick={() => (mode = "build")}>Build</button>
      <button type="button" aria-pressed={mode === "readBack"} onclick={() => (mode = "readBack")}>Read it back</button>
    </div>
  </header>

  <div class="designer-body">
    {#if errorMessage}
      <p class="error is-selectable" role="alert">{errorMessage}</p>
    {:else if draft && view}
      <DesignerProblems problems={view.report.problems ?? []} onJump={jumpTo}/>
      {#if mode === "build"}
        <div class="canvas">
          <TriggerCard trigger={draft.trigger} {view}/>
          <div>
            <div class="keyword">Do</div>
            {#if draft.steps?.length}
              <StepList steps={draft.steps} depth={0} {view}/>
            {:else}
              <p class="no-steps">No steps yet.</p>
            {/if}
          </div>
        </div>
      {:else}
        <ReadBack {draft} {view}/>
      {/if}
    {/if}
  </div>
</section>

<style>
  .designer {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .designer-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3) var(--space-4);
    padding: var(--space-4) var(--space-5);
    border-bottom: 1px solid var(--color-border);
  }

  .back {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .back:hover {
    color: var(--color-text);
  }

  h1 {
    min-width: 0;
    font-size: var(--font-size-heading);
    overflow-wrap: anywhere;
  }

  .state {
    padding: 1px var(--space-2);
    border-radius: 999px;
    background: var(--color-surface-raised);
    color: var(--color-text-muted);
    font-size: var(--font-size-small);
    font-weight: 600;
  }

  .state.is-on {
    color: var(--color-success);
  }

  .mode-tabs {
    display: inline-flex;
    margin-left: auto;
    overflow: hidden;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius);
  }

  .mode-tabs button {
    padding: 5px 12px;
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .mode-tabs button[aria-pressed="true"] {
    background: var(--color-surface-raised);
    color: var(--color-text);
  }

  .designer-body {
    flex: 1;
    overflow: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    padding: var(--space-5);
  }

  .designer-body > :global(*) {
    max-width: 900px;
  }

  .canvas {
    display: flex;
    flex-direction: column;
    gap: 22px;
  }

  .keyword {
    margin: 0 0 6px 6px;
    color: var(--color-accent);
    font-family: var(--font-display);
    font-size: 13px;
    font-weight: 700;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .no-steps {
    padding: var(--space-2) 6px;
    color: var(--color-text-muted);
  }

  .error {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }
</style>
