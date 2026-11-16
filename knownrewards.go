package main

import (
	"sync"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

// missingRedeemMessage is the problem an automation gets when its redeem was
// deleted in Twitch.
const missingRedeemMessage = "The redeem this uses was deleted in Twitch. Pick another one."

// knownRewards remembers which rewards the last list that loaded had, so an
// automation whose redeem is gone can be pointed out without asking Twitch
// again. A redeem only counts as missing once a list has loaded: Twitch
// being unreachable, or an account without channel points, never marks one
// missing. See docs/design/twitch-redeems.md#missing-redeems.
type knownRewards struct {
	mutex sync.Mutex
	// rewardIDs is nil until a list has loaded.
	rewardIDs map[string]bool
}

func (known *knownRewards) remember(rewards []twitchrewards.Reward) {
	rewardIDs := make(map[string]bool, len(rewards))
	for _, reward := range rewards {
		rewardIDs[reward.ID] = true
	}
	known.mutex.Lock()
	defer known.mutex.Unlock()
	known.rewardIDs = rewardIDs
}

// forget drops the last list, for an account that turns out to have no
// channel points, so its automations aren't marked missing.
func (known *knownRewards) forget() {
	known.mutex.Lock()
	defer known.mutex.Unlock()
	known.rewardIDs = nil
}

func (known *knownRewards) isMissing(rewardID string) bool {
	if rewardID == "" || rewardID == twitcheventsub.PlaceholderRewardID {
		return false
	}
	known.mutex.Lock()
	defer known.mutex.Unlock()
	return known.rewardIDs != nil && !known.rewardIDs[rewardID]
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
