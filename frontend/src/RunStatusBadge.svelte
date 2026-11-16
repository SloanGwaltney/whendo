<script lang="ts">
  import type {RunStatus} from "../bindings/github.com/SloanGwaltney/whendo/engine/runner";
  import {statusLabels} from "./runHistory";

  let {status, isTest}: {status: RunStatus; isTest: boolean} = $props();
</script>

<span class="badges">
  <span class="badge is-{status}">{statusLabels[status]}</span>
  {#if isTest}
    <!-- Marked so a test isn't mistaken for real activity. -->
    <span class="badge is-test">Test</span>
  {/if}
</span>

<style>
  .badges {
    display: inline-flex;
    gap: var(--space-1);
  }

  .badge {
    padding: 0 var(--space-2);
    border-radius: 999px;
    font-size: var(--font-size-small);
    font-weight: 600;
    white-space: nowrap;
    background: var(--color-surface-raised);
    color: var(--color-text-muted);
  }

  .badge.is-succeeded {
    color: var(--color-success);
  }

  .badge.is-failed {
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }

  .badge.is-test {
    background: var(--color-accent-soft);
    color: var(--color-accent);
  }
</style>
