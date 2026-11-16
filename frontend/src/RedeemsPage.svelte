<script lang="ts">
  import {AutomationsService, RewardsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {RewardLimits} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {Reward} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchrewards";
  import Dialog from "./Dialog.svelte";
  import Page from "./Page.svelte";
  import RedeemFormDialog from "./RedeemFormDialog.svelte";
  import RedeemRow from "./RedeemRow.svelte";
  import {loadRewards, placeholderRewardLabel, rewardsState} from "./rewards.svelte";
  import Toast from "./Toast.svelte";

  // The streamer's channel point redeems, in two groups: the ones Whendo
  // made, which it can change, and the ones made elsewhere, which it can only
  // use. See docs/design/mvp-ui.md#redeems.

  // Twitch is asked each time the page opens, since redeems may have changed
  // in the Twitch dashboard. See docs/design/twitch-redeems.md#listing-redeems.
  loadRewards();

  let limits = $state<RewardLimits | null>(null);
  let isFormOpen = $state(false);
  let editedReward = $state<Reward | null>(null);
  let rewardToDelete = $state<Reward | null>(null);
  // The automations that use the redeem about to be deleted, or null while
  // they're being looked up.
  let automationsUsingDeleted = $state<string[] | null>(null);
  let actionErrorMessage = $state("");
  let toastMessage = $state("");

  loadLimits();

  async function loadLimits(): Promise<void> {
    try {
      limits = await RewardsService.Limits();
    } catch (error) {
      actionErrorMessage = `Couldn't load Twitch's limits on redeems: ${error}`;
    }
  }

  const rewards = $derived(rewardsState.list?.rewards ?? []);
  const whendoRewards = $derived(rewards.filter((reward) => reward.isMadeByWhendo));
  const otherRewards = $derived(rewards.filter((reward) => !reward.isMadeByWhendo));
  const hasChannelPoints = $derived(rewardsState.list?.hasChannelPoints ?? true);
  const isChannelFull = $derived(limits !== null && rewards.length >= limits.maximumRewardsPerChannel);
  const canCreate = $derived(rewardsState.list !== null && hasChannelPoints && limits !== null && !isChannelFull);

  function openForm(reward: Reward | null): void {
    actionErrorMessage = "";
    editedReward = reward;
    isFormOpen = true;
  }

  async function finishSaving(saved: Reward): Promise<void> {
    const verb = editedReward ? "Saved" : "Created";
    isFormOpen = false;
    editedReward = null;
    toastMessage = `${verb} “${saved.title}”.`;
    await loadRewards();
  }

  // runAction changes a redeem, then reloads the list so it shows what Twitch
  // actually has, whether the change worked or not.
  async function runAction(failureMessage: string, action: () => Promise<unknown>): Promise<void> {
    actionErrorMessage = "";
    try {
      await action();
    } catch (error) {
      actionErrorMessage = `${failureMessage}: ${error}`;
    }
    await loadRewards();
  }

  function togglePause(reward: Reward): Promise<void> {
    const verb = reward.isPaused ? "resume" : "pause";
    return runAction(`Couldn't ${verb} “${reward.title}”`, () => RewardsService.SetRewardPaused(reward.id, !reward.isPaused));
  }

  async function askToDelete(reward: Reward): Promise<void> {
    actionErrorMessage = "";
    automationsUsingDeleted = null;
    rewardToDelete = reward;
    try {
      automationsUsingDeleted = (await AutomationsService.AutomationsUsingReward(reward.id)) ?? [];
    } catch (error) {
      rewardToDelete = null;
      actionErrorMessage = `Couldn't check which automations use “${reward.title}”: ${error}`;
    }
  }

  async function confirmDelete(): Promise<void> {
    const deleted = rewardToDelete;
    rewardToDelete = null;
    if (!deleted) {
      return;
    }
    await runAction(`Couldn't delete “${deleted.title}”`, () => RewardsService.DeleteReward(deleted.id));
    if (!actionErrorMessage) {
      toastMessage = `Deleted “${deleted.title}”.`;
    }
  }
</script>

<Page title="Redeems">
  <div class="redeems">
    <div class="toolbar">
      <p class="intro">Your channel point redeems on Twitch. Pick one as an automation's trigger to make it do something.</p>
      <div class="toolbar-actions">
        <button type="button" class="button" disabled={rewardsState.isLoading} onclick={loadRewards}>
          {rewardsState.isLoading ? "Loading…" : "Refresh"}
        </button>
        {#if hasChannelPoints}
          <button type="button" class="button is-primary" disabled={!canCreate} onclick={() => openForm(null)}>New redeem</button>
        {/if}
      </div>
    </div>

    {#if isChannelFull && limits}
      <p class="hint">You have {limits.maximumRewardsPerChannel} redeems, the most Twitch allows, counting hidden ones. Delete one to make another.</p>
    {/if}

    {#if rewardsState.errorMessage}
      <p class="error is-selectable" role="alert">{rewardsState.errorMessage}</p>
    {/if}
    {#if actionErrorMessage}
      <p class="error is-selectable" role="alert">{actionErrorMessage}</p>
    {/if}

    {#if rewardsState.list === null}
      <!-- Loading takes a moment at most, so there's nothing to show. -->
    {:else if !hasChannelPoints}
      <div class="empty">
        <p>Your Twitch account doesn't have channel points yet.</p>
        <p class="hint">Channel points come with Twitch affiliate or partner status. Until then, you can still build automations for your future redeems: pick “{placeholderRewardLabel}” as the redeem, test them, and switch to the real one once channel points are turned on.</p>
      </div>
    {:else if rewards.length === 0}
      <div class="empty">
        <p>You don't have any redeems yet.</p>
        <p class="hint">Make one here with New redeem, or in the Twitch dashboard.</p>
      </div>
    {:else}
      {#if whendoRewards.length > 0}
        <section class="group">
          <h2>Made in Whendo</h2>
          <ul class="redeem-list">
            {#each whendoRewards as reward (reward.id)}
              <RedeemRow {reward} actions={{onEdit: () => openForm(reward), onTogglePause: () => togglePause(reward), onDelete: () => askToDelete(reward)}}/>
            {/each}
          </ul>
        </section>
      {/if}
      {#if otherRewards.length > 0}
        <section class="group">
          <h2>Made in the Twitch dashboard</h2>
          <p class="hint">Whendo can start automations from these, but Twitch only lets the app that made a redeem change it.</p>
          <ul class="redeem-list">
            {#each otherRewards as reward (reward.id)}
              <RedeemRow {reward}/>
            {/each}
          </ul>
        </section>
      {/if}
    {/if}
  </div>
</Page>

{#if limits}
  <RedeemFormDialog isOpen={isFormOpen} {editedReward} {limits} onSaved={finishSaving} onClose={() => (isFormOpen = false)}/>
{/if}

<Dialog title="Delete this redeem?" isOpen={rewardToDelete !== null} onClose={() => (rewardToDelete = null)}>
  <p>“{rewardToDelete?.title}” will be deleted from Twitch. This can't be undone.</p>
  <p>Twitch completes any redemptions of it still waiting in your request queue, so those viewers can't be refunded.</p>
  {#if automationsUsingDeleted === null}
    <p class="hint">Checking which automations use it…</p>
  {:else if automationsUsingDeleted.length > 0}
    <p>These automations use it, and won't run until you pick another redeem for them: {automationsUsingDeleted.join(", ")}.</p>
  {/if}
  <div class="dialog-actions">
    <button type="button" class="button" onclick={() => (rewardToDelete = null)}>Cancel</button>
    <button type="button" class="button is-danger" disabled={automationsUsingDeleted === null} onclick={confirmDelete}>Delete</button>
  </div>
</Dialog>

{#if toastMessage}
  <Toast message={toastMessage} onDismiss={() => (toastMessage = "")}/>
{/if}

<style>
  .redeems {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    max-width: 880px;
  }

  .toolbar-actions {
    display: flex;
    gap: var(--space-2);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .intro {
    color: var(--color-text-muted);
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h2 {
    font-size: 13px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .redeem-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .empty {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-6) var(--space-5);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-large);
    text-align: center;
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
