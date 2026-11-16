<script lang="ts">
  import type {VariableReference} from "../../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
  import {variableKey} from "./designerModel";
  import type {DesignerView} from "./designerModel";

  // A variable inserted into a sentence, such as the redeemer's name.
  let {reference, view, isBroken}: {
    reference: VariableReference;
    view: DesignerView;
    // The variable isn't available where it's used.
    isBroken: boolean;
  } = $props();

  const label = $derived(view.variableNamesByKey.get(variableKey(reference)) ?? "deleted value");
</script>

<span class="value-pill" class:is-broken={isBroken}>{label}</span>

<style>
  .value-pill {
    display: inline-flex;
    align-items: center;
    padding: 0 6px;
    border-radius: var(--radius-small);
    background: var(--color-value-soft);
    color: var(--color-value);
    font-size: 0.9em;
    font-weight: 500;
    line-height: 1.6;
    white-space: nowrap;
  }

  .value-pill.is-broken {
    background: var(--color-danger-soft);
    color: var(--color-danger);
    text-decoration: line-through;
  }
</style>
