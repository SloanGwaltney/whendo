<script lang="ts">
  import {TwitchLoginService} from "../bindings/github.com/SloanGwaltney/whendo";
  import Page from "./Page.svelte";

  let {accountLogin}: {accountLogin: string} = $props();

  let logoutErrorMessage = $state("");

  async function logOut(): Promise<void> {
    logoutErrorMessage = "";
    try {
      await TwitchLoginService.Logout();
    } catch (error) {
      logoutErrorMessage = String(error);
    }
  }
</script>

<!-- OBS, audio, run history retention and the theme join Twitch here as
     they're built. See docs/design/mvp-ui.md#settings. -->
<Page title="Settings">
  <section class="setting-group">
    <h2>Twitch</h2>
    <div class="setting-row">
      <p>Logged in as <strong class="is-selectable">{accountLogin}</strong></p>
      <button type="button" class="button" onclick={logOut}>Log out</button>
    </div>
    {#if logoutErrorMessage}
      <p class="error is-selectable" role="alert">Logged out, but something went wrong: {logoutErrorMessage}</p>
    {/if}
  </section>
</Page>

<style>
  .setting-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    max-width: 640px;
  }

  h2 {
    font-size: 13px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-large);
    background: var(--color-surface);
  }

  .error {
    color: var(--color-danger);
    font-size: 13px;
  }
</style>
