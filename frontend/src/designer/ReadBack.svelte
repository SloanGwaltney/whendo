<script lang="ts">
  import type {Automation} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automation";
  import {triggerOwnerID} from "./designerModel";
  import type {DesignerView} from "./designerModel";
  import ReadBackSteps from "./ReadBackSteps.svelte";
  import Sentence from "./Sentence.svelte";

  // The whole automation as plain text, for reviewing it before going live.
  let {draft, view}: {draft: Automation; view: DesignerView} = $props();
</script>

<div class="read-back">
  <p>
    When
    {#if view.triggerDefinition}
      <Sentence
        sentence={view.triggerDefinition.sentence}
        settings={view.triggerDefinition.settings}
        values={draft.trigger.settings}
        ownerID={triggerOwnerID}
        {view}
        appearance="prose"
      />:
    {:else}
      <span class="missing">something happens</span>:
    {/if}
  </p>
  {#if draft.steps?.length}
    <ReadBackSteps steps={draft.steps} {view}/>
  {:else}
    <p class="nothing">Nothing happens yet.</p>
  {/if}
</div>

<style>
  .read-back {
    max-width: 70ch;
    font-size: var(--font-size-sentence);
    line-height: 1.75;
  }

  p {
    margin-bottom: var(--space-2);
  }

  .missing {
    color: var(--color-danger);
    font-style: italic;
  }

  .nothing {
    color: var(--color-text-muted);
  }
</style>
