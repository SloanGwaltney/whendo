<script lang="ts">
  import {RewardsService} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {RewardLimits} from "../bindings/github.com/SloanGwaltney/whendo";
  import type {Reward, Settings} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchrewards";
  import Dialog from "./Dialog.svelte";
  import {rewardsState} from "./rewards.svelte";

  // Creates a redeem, or edits one Whendo made. It changes the streamer's
  // Twitch account, so nothing is sent until they press Save. See
  // docs/design/twitch-redeems.md#creating.
  let {isOpen, editedReward, limits, onSaved, onClose}: {
    isOpen: boolean;
    // The redeem being edited, or null to create one.
    editedReward: Reward | null;
    limits: RewardLimits;
    onSaved: (savedReward: Reward) => void;
    onClose: () => void;
  } = $props();

  const secondsPerMinute = 60;
  // The colour a new redeem starts from when the streamer picks one.
  const startingColour = "#9147FF";

  let settings = $state<Settings | null>(null);
  // Twitch picks a random colour for a new redeem unless one is chosen.
  let isColourChosen = $state(false);
  let colour = $state(startingColour);
  // Cooldowns are typed in minutes when they're whole minutes, else seconds.
  let cooldownAmount = $state(0);
  let cooldownUnitSeconds = $state(secondsPerMinute);
  let isCooldownOn = $state(false);
  let isPerStreamLimitOn = $state(false);
  let isPerViewerLimitOn = $state(false);
  let isSaving = $state(false);
  let errorMessage = $state("");

  const isCreating = $derived(editedReward === null);
  // Twitch keeps the old description when sent a blank one, so a description
  // can be changed but not cleared.
  const hasDescriptionToKeep = $derived((editedReward?.prompt ?? "") !== "");

  $effect(() => {
    if (isOpen) {
      startForm();
    }
  });

  async function startForm(): Promise<void> {
    errorMessage = "";
    isSaving = false;
    const startingSettings = editedReward ? settingsOf(editedReward) : await RewardsService.NewRewardSettings();
    isColourChosen = editedReward !== null;
    colour = editedReward?.backgroundColor || startingColour;
    isCooldownOn = startingSettings.cooldownSeconds > 0;
    const isWholeMinutes = startingSettings.cooldownSeconds > 0 && startingSettings.cooldownSeconds % secondsPerMinute === 0;
    cooldownUnitSeconds = isWholeMinutes || startingSettings.cooldownSeconds === 0 ? secondsPerMinute : 1;
    cooldownAmount = startingSettings.cooldownSeconds > 0 ? startingSettings.cooldownSeconds / cooldownUnitSeconds : 1;
    isPerStreamLimitOn = startingSettings.maximumPerStream > 0;
    isPerViewerLimitOn = startingSettings.maximumPerViewerPerStream > 0;
    settings = {
      ...startingSettings,
      maximumPerStream: startingSettings.maximumPerStream || 1,
      maximumPerViewerPerStream: startingSettings.maximumPerViewerPerStream || 1,
    };
  }

  function settingsOf(reward: Reward): Settings {
    return {
      title: reward.title,
      prompt: reward.prompt,
      cost: reward.cost,
      backgroundColor: reward.backgroundColor,
      isShownToViewers: reward.isShownToViewers,
      isUserInputRequired: reward.isUserInputRequired,
      canBeRefunded: reward.canBeRefunded,
      maximumPerStream: reward.maximumPerStream,
      maximumPerViewerPerStream: reward.maximumPerViewerPerStream,
      cooldownSeconds: reward.cooldownSeconds,
    };
  }

  // problemWith checks what's quick to check here, so the streamer hears
  // straight away. The engine checks the same rules again, and Twitch checks
  // the rest, such as AutoMod.
  function problemWith(formSettings: Settings): string {
    const title = formSettings.title.trim();
    if (title === "") {
      return "Give the redeem a name.";
    }
    if ([...title].length > limits.maximumTitleLength) {
      return `The name can be at most ${limits.maximumTitleLength} characters long.`;
    }
    const isTitleTaken = (rewardsState.list?.rewards ?? []).some((reward) => reward.id !== editedReward?.id && reward.title.toLowerCase() === title.toLowerCase());
    if (isTitleTaken) {
      return "Another redeem already has this name.";
    }
    if (!Number.isInteger(formSettings.cost) || formSettings.cost < limits.minimumCost || formSettings.cost > limits.maximumCost) {
      return `The cost has to be a whole number from ${limits.minimumCost.toLocaleString()} to ${limits.maximumCost.toLocaleString()} points.`;
    }
    if (hasDescriptionToKeep && formSettings.prompt.trim() === "") {
      return "Twitch can't clear a redeem's description, only change it. Write a new one instead.";
    }
    const isLimitInvalid = (isOn: boolean, value: number) => isOn && (!Number.isInteger(value) || value < 1);
    if (isLimitInvalid(isCooldownOn, cooldownAmount * cooldownUnitSeconds) || isLimitInvalid(isPerStreamLimitOn, formSettings.maximumPerStream) || isLimitInvalid(isPerViewerLimitOn, formSettings.maximumPerViewerPerStream)) {
      return "Each limit that's on needs a whole number of at least 1.";
    }
    return "";
  }

  function settingsToSave(formSettings: Settings): Settings {
    return {
      ...formSettings,
      title: formSettings.title.trim(),
      backgroundColor: isColourChosen ? colour.toUpperCase() : "",
      cooldownSeconds: isCooldownOn ? Math.round(cooldownAmount * cooldownUnitSeconds) : 0,
      maximumPerStream: isPerStreamLimitOn ? formSettings.maximumPerStream : 0,
      maximumPerViewerPerStream: isPerViewerLimitOn ? formSettings.maximumPerViewerPerStream : 0,
    };
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!settings || isSaving) {
      return;
    }
    errorMessage = problemWith(settings);
    if (errorMessage) {
      return;
    }
    isSaving = true;
    try {
      const saved = editedReward
        ? await RewardsService.UpdateReward(editedReward.id, settingsToSave(settings))
        : await RewardsService.CreateReward(settingsToSave(settings));
      onSaved(saved);
    } catch (error) {
      errorMessage = `Couldn't save the redeem: ${error}`;
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog title={isCreating ? "New redeem" : `Edit “${editedReward?.title}”`} {isOpen} {onClose} isWide>
  {#if settings}
    <form class="redeem-form" onsubmit={save}>
      <div class="field-group">
        <label for="redeem-title">Name</label>
        <input id="redeem-title" class="field" bind:value={settings.title} maxlength={limits.maximumTitleLength} placeholder="Hydrate"/>
      </div>

      <div class="field-group">
        <label for="redeem-cost">Cost in channel points</label>
        <input id="redeem-cost" class="field short" type="number" min={limits.minimumCost} max={limits.maximumCost} step="1" bind:value={settings.cost}/>
      </div>

      <div class="field-group">
        <label for="redeem-prompt">Description</label>
        <input id="redeem-prompt" class="field" bind:value={settings.prompt} placeholder="Shown to viewers when they redeem it"/>
      </div>

      <div class="choices">
        <label class="choice"><input type="checkbox" bind:checked={settings.isShownToViewers}/> Shown to viewers</label>
        <label class="choice"><input type="checkbox" bind:checked={settings.isUserInputRequired}/> Viewers type a message</label>
        <div>
          <label class="choice"><input type="checkbox" bind:checked={settings.canBeRefunded}/> Viewers can be refunded</label>
          <p class="hint">Keeps each redemption waiting in your Twitch request queue until it's completed or refunded. Most redeems don't need this.</p>
        </div>
      </div>

      <div class="field-group">
        <span class="label">Colour</span>
        <div class="row">
          {#if isCreating}
            <label class="choice"><input type="checkbox" bind:checked={isColourChosen}/> Choose a colour</label>
          {/if}
          {#if isColourChosen}
            <input class="colour" type="color" bind:value={colour} aria-label="Colour"/>
          {:else}
            <span class="hint">Twitch picks one.</span>
          {/if}
        </div>
      </div>

      <div class="field-group">
        <span class="label">Limits</span>
        <div class="limit">
          <label class="choice"><input type="checkbox" bind:checked={isCooldownOn}/> Cooldown</label>
          {#if isCooldownOn}
            <input class="field short" type="number" min="1" step="1" bind:value={cooldownAmount} aria-label="Cooldown length"/>
            <select class="field unit" bind:value={cooldownUnitSeconds} aria-label="Cooldown unit">
              <option value={1}>seconds</option>
              <option value={secondsPerMinute}>minutes</option>
            </select>
          {/if}
        </div>
        <div class="limit">
          <label class="choice"><input type="checkbox" bind:checked={isPerStreamLimitOn}/> Limit per stream</label>
          {#if isPerStreamLimitOn}
            <input class="field short" type="number" min="1" step="1" bind:value={settings.maximumPerStream} aria-label="Times per stream"/>
            <span class="hint">times</span>
          {/if}
        </div>
        <div class="limit">
          <label class="choice"><input type="checkbox" bind:checked={isPerViewerLimitOn}/> Limit per viewer per stream</label>
          {#if isPerViewerLimitOn}
            <input class="field short" type="number" min="1" step="1" bind:value={settings.maximumPerViewerPerStream} aria-label="Times per viewer per stream"/>
            <span class="hint">times</span>
          {/if}
        </div>
      </div>

      {#if errorMessage}
        <p class="error is-selectable" role="alert">{errorMessage}</p>
      {/if}

      <div class="dialog-actions">
        <button type="button" class="button" onclick={onClose}>Cancel</button>
        <button type="submit" class="button is-primary" disabled={isSaving}>{isSaving ? "Saving…" : isCreating ? "Create redeem" : "Save"}</button>
      </div>
    </form>
  {/if}
</Dialog>

<style>
  .redeem-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  label,
  .label {
    color: var(--color-text-muted);
    font-size: 13px;
  }

  .choices {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .choice {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--color-text);
    font-size: var(--font-size-body);
  }

  .row,
  .limit {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-height: 34px;
  }

  .short {
    width: 120px;
  }

  .unit {
    width: auto;
  }

  .colour {
    width: 48px;
    height: 30px;
    padding: 0;
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius);
    background: none;
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
