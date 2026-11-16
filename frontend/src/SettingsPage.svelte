<script lang="ts">
  import {SettingsService, TwitchLoginService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {RetentionLimits} from "../bindings/github.com/SloanGwaltney/whendo";
  import Page from "./Page.svelte";

  let {accountLogin}: {accountLogin: string} = $props();

  let logoutErrorMessage = $state("");

  // The saved retention period, and what's in the field, which differ while
  // the user types or when a change is refused.
  let savedRetentionDays = $state(0);
  let enteredRetentionDays = $state(0);
  let retentionLimits = $state<RetentionLimits | null>(null);
  let retentionMessage = $state("");
  let isRetentionMessageAnError = $state(false);

  loadSettings();

  async function loadSettings(): Promise<void> {
    try {
      const [settings, limits] = await Promise.all([SettingsService.GetSettings(), SettingsService.RunHistoryRetentionLimits()]);
      savedRetentionDays = settings.runHistoryRetentionDays;
      enteredRetentionDays = settings.runHistoryRetentionDays;
      retentionLimits = limits;
    } catch (error) {
      showRetentionMessage(`Couldn't load your settings: ${error}`, true);
    }
  }

  function showRetentionMessage(message: string, isError: boolean): void {
    retentionMessage = message;
    isRetentionMessageAnError = isError;
  }

  // Saves as soon as the field is committed, like the other settings,
  // rather than with a Save button.
  async function saveRetention(): Promise<void> {
    if (!retentionLimits || enteredRetentionDays === savedRetentionDays) {
      return;
    }
    const {minimumDays, maximumDays} = retentionLimits;
    if (!Number.isInteger(enteredRetentionDays) || enteredRetentionDays < minimumDays || enteredRetentionDays > maximumDays) {
      showRetentionMessage(`Choose a whole number of days from ${minimumDays} to ${maximumDays}.`, true);
      return;
    }
    try {
      await SettingsService.SetRunHistoryRetentionDays(enteredRetentionDays);
      savedRetentionDays = enteredRetentionDays;
      showRetentionMessage("Saved.", false);
    } catch (error) {
      showRetentionMessage(`Couldn't save: ${error}`, true);
    }
  }

  async function logOut(): Promise<void> {
    logoutErrorMessage = "";
    try {
      await TwitchLoginService.Logout();
    } catch (error) {
      logoutErrorMessage = String(error);
    }
  }
</script>

<!-- OBS, audio and the theme join these here as they're built. See
     docs/design/mvp-ui.md#settings. -->
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

  <section class="setting-group">
    <h2>Run history</h2>
    <div class="setting-row">
      <label for="retention-days">Keep runs for</label>
      <div class="retention-field">
        <input
          id="retention-days"
          class="field"
          type="number"
          min={retentionLimits?.minimumDays}
          max={retentionLimits?.maximumDays}
          step="1"
          disabled={!retentionLimits}
          bind:value={enteredRetentionDays}
          onchange={saveRetention}
        />
        <span>days</span>
      </div>
    </div>
    <p class="hint">Older runs are deleted, so choosing fewer days deletes them straight away.</p>
    {#if retentionMessage}
      <p class:error={isRetentionMessageAnError} class:hint={!isRetentionMessageAnError} class="is-selectable" role={isRetentionMessageAnError ? "alert" : "status"}>{retentionMessage}</p>
    {/if}
  </section>
</Page>

<style>
  .setting-group + .setting-group {
    margin-top: var(--space-5);
  }

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

  .retention-field {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .retention-field .field {
    width: 88px;
  }

  .hint {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .error {
    color: var(--color-danger);
    font-size: 13px;
  }
</style>
