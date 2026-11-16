<script lang="ts">
  import Page from "./Page.svelte";

  // Stands in for a page that isn't built yet, saying what it will hold.
  let {title, futureContents}: {title: string; futureContents: string} = $props();
</script>

<Page {title}>
  <p class="future-contents">{futureContents}</p>
</Page>

<style>
  .future-contents {
    max-width: 60ch;
    color: var(--color-text-muted);
  }
</style>
