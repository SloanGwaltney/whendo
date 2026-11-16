<script lang="ts">
  import {startPageId} from "./navigation";
  import type {PageId} from "./navigation";
  import DesignerPage from "./designer/DesignerPage.svelte";
  import HomePage from "./HomePage.svelte";
  import PlaceholderPage from "./PlaceholderPage.svelte";
  import SettingsPage from "./SettingsPage.svelte";
  import Sidebar from "./Sidebar.svelte";

  // The app once the user is logged in: the sidebar and the current page.
  let {accountLogin}: {accountLogin: string} = $props();

  let currentPageId: PageId = $state(startPageId);
  // The automation open in the designer, which opens from Home; empty when
  // none is.
  let openAutomationID = $state("");

  function navigate(pageId: PageId): void {
    currentPageId = pageId;
    openAutomationID = "";
  }
</script>

<div class="shell">
  <Sidebar {currentPageId} {accountLogin} onNavigate={navigate}/>
  <main>
    {#if currentPageId === "home" && openAutomationID}
      {#key openAutomationID}
        <DesignerPage automationID={openAutomationID} onClose={() => (openAutomationID = "")}/>
      {/key}
    {:else if currentPageId === "home"}
      <HomePage onOpenAutomation={(automationID) => (openAutomationID = automationID)}/>
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
