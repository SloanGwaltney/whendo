<script lang="ts">
  import Page from "./Page.svelte";
  import RedeemRow from "./RedeemRow.svelte";
  import {loadRewards, placeholderRewardLabel, rewardsState} from "./rewards.svelte";

  // The streamer's channel point redeems, read only for now, in two groups:
  // the ones Whendo made, which it will be able to edit, and the ones made
  // elsewhere, which it can only use. See docs/design/mvp-ui.md#redeems.

  // Twitch is asked each time the page opens, since redeems may have changed
  // in the Twitch dashboard. See docs/design/twitch-redeems.md#listing-redeems.
  loadRewards();

  const rewards = $derived(rewardsState.list?.rewards ?? []);
  const whendoRewards = $derived(rewards.filter((reward) => reward.isMadeByWhendo));
  const otherRewards = $derived(rewards.filter((reward) => !reward.isMadeByWhendo));
  const hasChannelPoints = $derived(rewardsState.list?.hasChannelPoints ?? true);
</script>

<Page title="Redeems">
  <div class="redeems">
    <div class="toolbar">
      <p class="intro">Your channel point redeems on Twitch. Pick one as an automation's trigger to make it do something.</p>
      <button type="button" class="button" disabled={rewardsState.isLoading} onclick={loadRewards}>
        {rewardsState.isLoading ? "Loading…" : "Refresh"}
      </button>
    </div>

    {#if rewardsState.errorMessage}
      <p class="error is-selectable" role="alert">{rewardsState.errorMessage}</p>
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
        <p class="hint">Redeems you make in the Twitch dashboard show up here.</p>
      </div>
    {:else}
      {#if whendoRewards.length > 0}
        <section class="group">
          <h2>Made in Whendo</h2>
          <ul class="redeem-list">
            {#each whendoRewards as reward (reward.id)}
              <RedeemRow {reward}/>
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

<style>
  .redeems {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    max-width: 880px;
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
