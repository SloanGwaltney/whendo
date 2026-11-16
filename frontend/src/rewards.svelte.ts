// The streamer's channel point redeems, shared by the redeem picker and the
// sentences that name a redeem. It's loaded when first needed and on
// refresh, not kept in sync with Twitch. See
// docs/design/twitch-redeems.md#listing-redeems.
import {RewardsService} from "../bindings/github.com/SloanGwaltney/whendo";
import type {RewardList} from "../bindings/github.com/SloanGwaltney/whendo";
import type {Kind} from "../bindings/github.com/SloanGwaltney/whendo/engine/automationsetting";
import type {Reward} from "../bindings/github.com/SloanGwaltney/whendo/engine/twitchrewards";

// These mirror twitcheventsub.RewardSettingKind and PlaceholderRewardID,
// which the bindings don't generate because they're Go constants rather
// than enums.
export const rewardSettingKind = "twitch.reward" as Kind;
export const placeholderRewardID = "whendo.placeholder";

export const placeholderRewardLabel = "A redeem I'll set up later";

interface RewardsState {
  // The last list that loaded, or null before one has.
  list: RewardList | null;
  isLoading: boolean;
  errorMessage: string;
}

export const rewardsState: RewardsState = $state({list: null, isLoading: false, errorMessage: ""});

export async function loadRewards(): Promise<void> {
  if (rewardsState.isLoading) {
    return;
  }
  rewardsState.isLoading = true;
  rewardsState.errorMessage = "";
  try {
    rewardsState.list = await RewardsService.ListRewards();
  } catch (error) {
    rewardsState.errorMessage = `Couldn't load your redeems: ${error}`;
  } finally {
    rewardsState.isLoading = false;
  }
}

export function loadRewardsIfNeeded(): void {
  if (rewardsState.list === null && !rewardsState.isLoading) {
    loadRewards();
  }
}

export function findReward(rewardID: string): Reward | undefined {
  return rewardsState.list?.rewards?.find((reward) => reward.id === rewardID);
}

// describeReward names a redeem for a sentence. Before the list loads, or if
// it couldn't, it says "your redeem" rather than show an ID.
export function describeReward(rewardID: string): string {
  if (rewardID === placeholderRewardID) {
    return placeholderRewardLabel.toLowerCase();
  }
  return findReward(rewardID)?.title ?? "your redeem";
}
