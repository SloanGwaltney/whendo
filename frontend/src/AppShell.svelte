<script lang="ts">
  import {startPageId} from "./navigation";
  import type {PageId} from "./navigation";
  import PlaceholderPage from "./PlaceholderPage.svelte";
  import SettingsPage from "./SettingsPage.svelte";
  import Sidebar from "./Sidebar.svelte";

  // The app once the user is logged in: the sidebar and the current page.
  let {accountLogin}: {accountLogin: string} = $props();

  let currentPageId: PageId = $state(startPageId);
</script>

<div class="shell">
  <Sidebar {currentPageId} {accountLogin} onNavigate={(pageId) => (currentPageId = pageId)}/>
  <main>
    {#if currentPageId === "home"}
      <PlaceholderPage title="Home" futureContents="Your automations and the sample gallery will be here."/>
    {:else if currentPageId === "redeems"}
      <PlaceholderPage title="Redeems" futureContents="Your channel point redeems will be here."/>
    {:else if currentPageId === "runHistory"}
      <PlaceholderPage title="Run History" futureContents="Every time an automation runs will be listed here."/>
    {:else}
      <SettingsPage {accountLogin}/>
    {/if}
  </main>
</div>

<style>
  .shell {
    display: flex;
    height: 100%;
  }

  main {
    flex: 1;
    min-width: 0;
  }
</style>
