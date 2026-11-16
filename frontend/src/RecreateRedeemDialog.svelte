<script lang="ts">
  import {AutomationsService, RewardsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {RecreateResult} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {Reward} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchrewards";
  import Dialog from "./Dialog.svelte";

  // Recreates a redeem made in the Twitch dashboard as one Whendo made, so
  // Whendo can change it. Twitch won't let two redeems share a name, and
  // only lets the dashboard delete the original, so the streamer deletes it
  // there partway through. See
  // docs/design/twitch-redeems.md#recreating-a-redeem-in-whendo.
  let {original, onFinished, onClose}: {
    // The redeem to recreate, or null when the dialog is closed.
    original: Reward | null;
    onFinished: () => void;
    onClose: () => void;
  } = $props();

  type Stage = "explaining" | "deleting" | "done";

  let stage = $state<Stage>("explaining");
  // The original's settings, kept here because once the streamer deletes it,
  // Twitch no longer has them, and the page's list will drop it.
  let keptOriginal = $state<Reward | null>(null);
  let automationNames = $state<string[]>([]);
  let result = $state<RecreateResult | null>(null);
  let isWorking = $state(false);
  let errorMessage = $state("");

  $effect(() => {
    if (original) {
      start(original);
    }
  });

  async function start(rewardToRecreate: Reward): Promise<void> {
    keptOriginal = structuredClone($state.snapshot(rewardToRecreate)) as Reward;
    stage = "explaining";
    result = null;
    errorMessage = "";
    try {
      automationNames = (await AutomationsService.AutomationsUsingReward(rewardToRecreate.id)) ?? [];
    } catch (error) {
      errorMessage = `Couldn't check which automations use it: ${error}`;
    }
  }

  async function openDashboard(): Promise<void> {
    if (!keptOriginal) {
      return;
    }
    errorMessage = "";
    try {
      await RewardsService.OpenRewardsDashboard(keptOriginal.id);
    } catch (error) {
      errorMessage = `Couldn't open the Twitch dashboard: ${error}`;
    }
  }

  async function recreate(): Promise<void> {
    if (!keptOriginal || isWorking) {
      return;
    }
    isWorking = true;
    errorMessage = "";
    try {
      result = await RewardsService.RecreateReward(keptOriginal);
      stage = "done";
    } catch (error) {
      // The original's settings are still kept here, so the streamer can
      // try again, for example after deleting it.
      errorMessage = String(error);
    } finally {
      isWorking = false;
    }
  }

  function finish(): void {
    onFinished();
  }

  // Closing partway through leaves the original as it is, unless the
  // streamer already deleted it; then they recreate it by hand.
  function close(): void {
    if (stage === "done") {
      onFinished();
      return;
    }
    onClose();
  }
</script>

<Dialog title={`Recreate “${keptOriginal?.title ?? ""}” in Whendo`} isOpen={original !== null} onClose={close} isWide>
  {#if keptOriginal}
    {#if stage === "explaining"}
      <p>Twitch only lets the app that made a redeem change it. Recreating this one in Whendo lets Whendo edit it, and later refund or complete its redemptions.</p>
      <ul class="facts">
        <li>You delete the original in the Twitch dashboard, then Whendo makes a copy with the same settings. Twitch doesn't allow two redeems with the same name, and only the dashboard can delete one it made.</li>
        <li>The copy is a new redeem as far as Twitch is concerned. Its picture isn't copied, since Whendo can't set pictures.</li>
        <li>Twitch completes any redemptions of the original still waiting in your request queue when you delete it, so those viewers can't be refunded.</li>
        {#if automationNames.length > 0}
          <li>These automations are switched to the copy for you: {automationNames.join(", ")}.</li>
        {/if}
      </ul>
      {#if errorMessage}
        <p class="error is-selectable" role="alert">{errorMessage}</p>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="button" onclick={close}>Cancel</button>
        <button type="button" class="button is-primary" onclick={() => (stage = "deleting")}>Start</button>
      </div>
    {:else if stage === "deleting"}
      <ol class="steps">
        <li>
          Delete “{keptOriginal.title}” in your Twitch dashboard.
          <button type="button" class="link" onclick={openDashboard}>Open the Twitch dashboard</button>
        </li>
        <li>Come back here and press Continue. Whendo checks it's gone, then makes the copy.</li>
      </ol>
      <p class="hint">Whendo remembers the redeem's settings while this window is open. If you close it after deleting the original, make the redeem again with New redeem.</p>
      {#if errorMessage}
        <p class="error is-selectable" role="alert">{errorMessage}</p>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="button" onclick={close}>Cancel</button>
        <button type="button" class="button is-primary" disabled={isWorking} onclick={recreate}>{isWorking ? "Recreating…" : "Continue"}</button>
      </div>
    {:else if result}
      <p>“{result.reward.title}” is now a Whendo redeem.</p>
      {#if result.switchedAutomations?.length}
        <p>Switched to it: {result.switchedAutomations.join(", ")}.</p>
      {/if}
      {#if result.switchProblem}
        <p class="error is-selectable" role="alert">{result.switchProblem}</p>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="button is-primary" onclick={finish}>Done</button>
      </div>
    {/if}
  {/if}
</Dialog>

<style>
  .facts,
  .steps {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding-left: var(--space-5);
  }

  .link {
    display: block;
    margin-top: var(--space-1);
    color: var(--color-accent);
  }

  .link:hover {
    text-decoration: underline;
    text-underline-offset: 3px;
  }

  .hint {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .error {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius);
    background: var(--color-danger-soft);
    color: var(--color-danger);
  }
</style>
