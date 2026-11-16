<script lang="ts">
  import {startPageId} from "./navigation";
  import type {PageId} from "./navigation";
  import DesignerPage from "./designer/DesignerPage.svelte";
  import Dialog from "./Dialog.svelte";
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
  let hasUnsavedChanges = $state(false);
  // Where to go once the user agrees to leave the designer without saving.
  let pendingPageId = $state<PageId | null>(null);

  function leaveDesignerFor(pageId: PageId): void {
    currentPageId = pageId;
    openAutomationID = "";
    hasUnsavedChanges = false;
    pendingPageId = null;
  }

  // Leaving the designer with unsaved changes asks first, so a stray click
  // doesn't throw away an edit.
  function navigate(pageId: PageId): void {
    if (openAutomationID && hasUnsavedChanges) {
      pendingPageId = pageId;
      return;
    }
    leaveDesignerFor(pageId);
  }
</script>

<div class="shell">
  <Sidebar {currentPageId} {accountLogin} onNavigate={navigate}/>
  <main>
    {#if currentPageId === "home" && openAutomationID}
      {#key openAutomationID}
        <DesignerPage automationID={openAutomationID} onClose={() => navigate("home")} bind:hasUnsavedChanges/>
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
  <Dialog title="Leave without saving?" isOpen={pendingPageId !== null} onClose={() => (pendingPageId = null)}>
    <p>Your changes to this automation haven't been saved, and will be lost.</p>
    <div class="dialog-actions">
      <button type="button" class="button" onclick={() => (pendingPageId = null)}>Keep editing</button>
      <button type="button" class="button is-danger" onclick={() => pendingPageId && leaveDesignerFor(pendingPageId)}>Discard changes</button>
    </div>
  </Dialog>
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
