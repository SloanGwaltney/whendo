package main

import (
	"sync"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

// missingRedeemMessage is the problem an automation gets when its redeem was
// deleted in Twitch.
const missingRedeemMessage = "The redeem this uses was deleted in Twitch. Pick another one."

// knownRewards remembers the rewards the last list that loaded had, so an
// automation whose redeem is gone, or whose redemptions Twitch won't let
// Whendo refund, can be pointed out without asking Twitch again. A redeem only counts as missing once a list has loaded: Twitch
// being unreachable, or an account without channel points, never marks one
// missing. See docs/design/twitch-redeems.md#missing-redeems.
type knownRewards struct {
	mutex sync.Mutex
	// rewardsByID is nil until a list has loaded.
	rewardsByID map[string]twitchrewards.Reward
}

// The refund and fulfill steps look redeems up in the last list.
var _ twitchsteps.RewardLookup = (*knownRewards)(nil)

func (known *knownRewards) remember(rewards []twitchrewards.Reward) {
	rewardsByID := make(map[string]twitchrewards.Reward, len(rewards))
	for _, reward := range rewards {
		rewardsByID[reward.ID] = reward
	}
	known.mutex.Lock()
	defer known.mutex.Unlock()
	known.rewardsByID = rewardsByID
}

// forget drops the last list, for an account that turns out to have no
// channel points, so its automations aren't marked missing.
func (known *knownRewards) forget() {
	known.mutex.Lock()
	defer known.mutex.Unlock()
	known.rewardsByID = nil
}

// KnownReward returns the reward as the last list had it.
func (known *knownRewards) KnownReward(rewardID string) (twitchrewards.Reward, bool) {
	known.mutex.Lock()
	defer known.mutex.Unlock()
	reward, isKnown := known.rewardsByID[rewardID]
	return reward, isKnown
}

func (known *knownRewards) isMissing(rewardID string) bool {
	if rewardID == "" || rewardID == twitcheventsub.PlaceholderRewardID {
		return false
	}
	known.mutex.Lock()
	defer known.mutex.Unlock()
	_, isKnown := known.rewardsByID[rewardID]
	return known.rewardsByID != nil && !isKnown
}

// missingRedeemProblem returns the problem for a redeem trigger whose redeem
// is missing, if it is.
func (known *knownRewards) missingRedeemProblem(checkedAutomation automation.Automation) (automation.Problem, bool) {
	if checkedAutomation.Trigger.ID != twitcheventsub.RedeemTriggerID {
		return automation.Problem{}, false
	}
	rewardID, err := checkedAutomation.Trigger.Settings.Text(twitcheventsub.RewardSetting)
	if err != nil || !known.isMissing(rewardID) {
		return automation.Problem{}, false
	}
	return automation.Problem{
		Level:     automation.ProblemLevelGoodToKnow,
		Subject:   automation.ProblemSubjectTrigger,
		SettingID: twitcheventsub.RewardSetting,
		Message:   missingRedeemMessage,
	}, true
}
