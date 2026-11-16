<script lang="ts">
  import type {Snippet} from "svelte";

  let {title, children}: {title: string; children: Snippet} = $props();
</script>

<section class="page">
  <header class="page-header">
    <h1>{title}</h1>
  </header>
  <div class="page-body">
    {@render children()}
  </div>
</section>

<style>
  .page {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-width: 0;
  }

  .page-header {
    padding: var(--space-4) var(--space-5);
    border-bottom: 1px solid var(--color-border);
  }

  h1 {
    font-size: var(--font-size-heading);
  }

  .page-body {
    flex: 1;
    overflow: auto;
    padding: var(--space-5);
  }
</style>
