package twitcheventsub_test

import (
	"strings"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

var testStreamer = trigger.Streamer{UserID: "1234", Login: "teststreamer", DisplayName: "TestStreamer"}

func TestTestValues_PassEachTriggersOwnFilter(t *testing.T) {
	settingsByTriggerID := map[string]automationsetting.Values{
		twitcheventsub.RedeemTriggerID:      textSettings(t, twitcheventsub.RewardSetting, automationRewardID),
		twitcheventsub.ChatMessageTriggerID: textSettings(t, twitcheventsub.StartsWithSetting, " !d20 "),
		twitcheventsub.FollowTriggerID:      {},
	}
	for _, twitchTrigger := range twitcheventsub.Triggers() {
		triggerID := twitchTrigger.Definition().ID
		t.Run(triggerID, func(t *testing.T) {
			settings := settingsByTriggerID[triggerID]
			testValues := twitchTrigger.TestValues(settings, testStreamer)

			_, isMatch, err := twitchTrigger.Filter(trigger.Event{TriggerID: triggerID, Variables: testValues}, settings)

			if err != nil || !isMatch {
				t.Errorf("test values %v: got match %v and error %v, want a match", testValues, isMatch, err)
			}
		})
	}
}

func TestTestValues_PassTheRedeemFilterForThePlaceholder(t *testing.T) {
	redeemTrigger := findTrigger(t, twitcheventsub.RedeemTriggerID)
	settings := textSettings(t, twitcheventsub.RewardSetting, twitcheventsub.PlaceholderRewardID)
	testValues := redeemTrigger.TestValues(settings, testStreamer)

	_, isMatch, err := redeemTrigger.Filter(trigger.Event{TriggerID: twitcheventsub.RedeemTriggerID, Variables: testValues}, settings)

	if err != nil || !isMatch {
		t.Errorf("got match %v and error %v, want a test run on the placeholder to match", isMatch, err)
	}
}

func TestTestValues_GiveEveryVariableTheFormAsksFor(t *testing.T) {
	for _, twitchTrigger := range twitcheventsub.Triggers() {
		definition := twitchTrigger.Definition()
		testValues := twitchTrigger.TestValues(automationsetting.Values{}, testStreamer)
		for _, variable := range definition.Variables {
			if _, hasValue := testValues[variable.ID]; !hasValue && !variable.IsAddedByFilter {
				t.Errorf("%s: no test value for %q", definition.ID, variable.ID)
			}
		}
	}
}

func TestTestValues_UseTheStreamersOwnName(t *testing.T) {
	testValues := findTrigger(t, twitcheventsub.FollowTriggerID).TestValues(automationsetting.Values{}, testStreamer)

	if testValues[twitcheventsub.FollowerDisplayNameVariable] != testStreamer.DisplayName {
		t.Errorf("got follower %q, want the streamer", testValues[twitcheventsub.FollowerDisplayNameVariable])
	}
}

func TestExplainMismatch_ChatMessageNamesTheStartText(t *testing.T) {
	chatTrigger := findTrigger(t, twitcheventsub.ChatMessageTriggerID)
	explainer, canExplain := chatTrigger.(trigger.MismatchExplainer)
	if !canExplain {
		t.Fatal("the chat message trigger can't explain mismatches")
	}

	reason := explainer.ExplainMismatch(map[string]string{twitcheventsub.MessageTextVariable: "hello"}, textSettings(t, twitcheventsub.StartsWithSetting, "!d20"))

	if !strings.Contains(reason, "“!d20”") {
		t.Errorf("got reason %q, want it to name !d20", reason)
	}
}
