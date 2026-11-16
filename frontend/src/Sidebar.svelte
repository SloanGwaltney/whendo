<script lang="ts">
  import {navigationItems} from "./navigation";
  import type {PageId} from "./navigation";
  import StreamingSoftwareStatus from "./StreamingSoftwareStatus.svelte";
  import TwitchConnectionStatus from "./TwitchConnectionStatus.svelte";
  import Wordmark from "./Wordmark.svelte";

  let {currentPageId, accountLogin, onNavigate}: {
    currentPageId: PageId;
    accountLogin: string;
    onNavigate: (pageId: PageId) => void;
  } = $props();
</script>

<aside class="sidebar">
  <div class="brand"><Wordmark/></div>
  <nav aria-label="Main">
    {#each navigationItems as item (item.pageId)}
      <button
        type="button"
        class="navigation-item"
        aria-current={item.pageId === currentPageId ? "page" : undefined}
        onclick={() => onNavigate(item.pageId)}
      >{item.label}</button>
    {/each}
  </nav>
  <div class="statuses">
    <TwitchConnectionStatus {accountLogin}/>
    <StreamingSoftwareStatus/>
  </div>
</aside>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    width: 220px;
    flex: none;
    padding: var(--space-5) var(--space-3);
    background: var(--color-sidebar);
    border-right: 1px solid var(--color-border);
  }

  .brand {
    padding: 0 var(--space-2);
  }

  nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .navigation-item {
    text-align: left;
    padding: 7px 10px;
    border-radius: var(--radius);
    color: var(--color-text-muted);
    font-weight: 500;
  }

  .navigation-item:hover {
    color: var(--color-text);
  }

  .navigation-item[aria-current="page"] {
    background: var(--color-surface-raised);
    color: var(--color-text);
  }

  .statuses {
    margin-top: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3) 10px 0;
    border-top: 1px solid var(--color-border);
  }
</style>
