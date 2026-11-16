package twitcheventsub

import (
	"fmt"
	"strings"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// TestEventID is the ID test runs give their event, since they have no real
// event behind them. Steps that act on the event itself, such as refunding a
// redemption, fail on it; see docs/design/mvp-ui.md#test-runs.
const TestEventID = "test"

// The reward's own details come from Twitch, which a test run doesn't ask
// yet, so these stand in until the redeem picker can look them up.
const (
	testRewardTitle = "Test reward"
	testRewardCost  = "100"
)

func testTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (redeemTrigger) TestValues(settings automationsetting.Values, streamer trigger.Streamer) map[string]string {
	rewardID, _ := settings.Text(RewardSetting)
	return map[string]string{
		RedemptionIDVariable:        TestEventID,
		RedeemerUserIDVariable:      streamer.UserID,
		RedeemerLoginVariable:       streamer.Login,
		RedeemerDisplayNameVariable: streamer.DisplayName,
		UserInputVariable:           "",
		RewardIDVariable:            rewardID,
		RewardTitleVariable:         testRewardTitle,
		RewardCostVariable:          testRewardCost,
		RewardPromptVariable:        "",
		RedeemedAtVariable:          testTime(),
	}
}

// The message starts as just the text the automation watches for, which
// the streamer can add to, to try out the rest of the message.
func (chatMessageTrigger) TestValues(settings automationsetting.Values, streamer trigger.Streamer) map[string]string {
	startText, _ := settings.Text(StartsWithSetting)
	return map[string]string{
		MessageIDVariable:         TestEventID,
		SenderUserIDVariable:      streamer.UserID,
		SenderLoginVariable:       streamer.Login,
		SenderDisplayNameVariable: streamer.DisplayName,
		MessageTextVariable:       strings.TrimSpace(startText),
	}
}

func (chatMessageTrigger) ExplainMismatch(variables map[string]string, settings automationsetting.Values) string {
	startText, _ := settings.Text(StartsWithSetting)
	return fmt.Sprintf("The message has to start with “%s”, followed by a space or nothing.", strings.TrimSpace(startText))
}

func (followTrigger) TestValues(settings automationsetting.Values, streamer trigger.Streamer) map[string]string {
	return map[string]string{
		FollowerUserIDVariable:      streamer.UserID,
		FollowerLoginVariable:       streamer.Login,
		FollowerDisplayNameVariable: streamer.DisplayName,
		FollowedAtVariable:          testTime(),
	}
}
