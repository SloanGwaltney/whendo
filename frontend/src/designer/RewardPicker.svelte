<script lang="ts">
  import {loadRewards, loadRewardsIfNeeded, placeholderRewardID, placeholderRewardLabel, rewardsState} from "../rewards.svelte";
  import OptionList from "./OptionList.svelte";
  import type {ListOption} from "./OptionList.svelte";

  // Picks the redeem a redeem trigger reacts to: one of the streamer's
  // redeems, or the placeholder for one that doesn't exist yet. See
  // docs/design/twitch-redeems.md#picking-a-redeem-in-the-designer.
  let {selectedRewardID, onPick}: {selectedRewardID: string | null; onPick: (rewardID: string) => void} = $props();

  loadRewardsIfNeeded();

  const placeholderOption: ListOption = $derived({
    id: placeholderRewardID,
    label: placeholderRewardLabel,
    description: "Build and test it now, and pick the real redeem later.",
    isSelected: selectedRewardID === placeholderRewardID,
  });

  const rewardOptions: ListOption[] = $derived(
    (rewardsState.list?.rewards ?? []).map((reward) => ({
      id: reward.id,
      label: reward.title,
      description: `${reward.cost.toLocaleString()} points${reward.isMadeByWhendo ? " · made in Whendo" : ""}`,
      isSelected: reward.id === selectedRewardID,
    })),
  );

  const hasChannelPoints = $derived(rewardsState.list?.hasChannelPoints ?? true);
</script>

<div class="reward-picker">
  {#if rewardsState.isLoading && rewardsState.list === null}
    <p class="note">Loading your redeems…</p>
  {:else if rewardsState.errorMessage}
    <p class="error is-selectable" role="alert">{rewardsState.errorMessage}</p>
  {:else if !hasChannelPoints}
    <p class="note">Your Twitch account doesn't have channel points yet. They come with affiliate or partner status. Until then, you can build this on a placeholder.</p>
  {:else if rewardOptions.length === 0}
    <p class="note">You don't have any redeems yet.</p>
  {/if}

  <div class="options">
    <OptionList options={[...rewardOptions, placeholderOption]} onChoose={onPick}/>
  </div>

  <button type="button" class="refresh" disabled={rewardsState.isLoading} onclick={loadRewards}>
    {rewardsState.isLoading ? "Loading…" : "Refresh the list"}
  </button>
</div>

<style>
  .reward-picker {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-width: 340px;
  }

  .options {
    max-height: 320px;
    overflow-y: auto;
  }

  .note {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .error {
    color: var(--color-danger);
    font-size: 13px;
  }

  .refresh {
    align-self: flex-start;
    color: var(--color-accent);
    font-size: 13px;
  }

  .refresh:hover:not(:disabled) {
    text-decoration: underline;
    text-underline-offset: 3px;
  }
</style>
