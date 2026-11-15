package twitcheventsub_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

const (
	automationRewardID = "92af127c-7326-4483-a52b-b0da0be61c01"
	otherRewardID      = "5d1e3c0a-0000-4000-8000-000000000000"
)

func findTrigger(t *testing.T, triggerID string) trigger.Trigger {
	t.Helper()
	for _, twitchTrigger := range twitcheventsub.Triggers() {
		if twitchTrigger.Definition().ID == triggerID {
			return twitchTrigger
		}
	}
	t.Fatalf("no trigger with ID %q", triggerID)
	return nil
}

func textSettings(t *testing.T, settingID string, text string) automationsetting.Values {
	t.Helper()
	encodedText, err := json.Marshal(text)
	if err != nil {
		t.Fatalf("encoding setting: %v", err)
	}
	return automationsetting.Values{settingID: encodedText}
}

func redeemEvent(rewardID string) trigger.Event {
	return trigger.Event{
		TriggerID: twitcheventsub.RedeemTriggerID,
		Variables: map[string]string{
			twitcheventsub.RewardIDVariable:            rewardID,
			twitcheventsub.RedeemerDisplayNameVariable: "Cooler_User",
		},
	}
}

func chatMessageEvent(messageText string) trigger.Event {
	return trigger.Event{
		TriggerID: twitcheventsub.ChatMessageTriggerID,
		Variables: map[string]string{
			twitcheventsub.MessageTextVariable:       messageText,
			twitcheventsub.SenderDisplayNameVariable: "VIPTest",
		},
	}
}

func filterChatMessage(t *testing.T, startText string, messageText string) (map[string]string, bool) {
	t.Helper()
	chatTrigger := findTrigger(t, twitcheventsub.ChatMessageTriggerID)
	variables, isMatch, err := chatTrigger.Filter(chatMessageEvent(messageText), textSettings(t, twitcheventsub.StartsWithSetting, startText))
	if err != nil {
		t.Fatalf("Filter returned an error: %v", err)
	}
	return variables, isMatch
}

func TestRedeemFilter_MatchesTheAutomationsReward(t *testing.T) {
	redeemTrigger := findTrigger(t, twitcheventsub.RedeemTriggerID)

	variables, isMatch, err := redeemTrigger.Filter(redeemEvent(automationRewardID), textSettings(t, twitcheventsub.RewardSetting, automationRewardID))

	if err != nil || !isMatch {
		t.Fatalf("got match %v, error %v; want a match", isMatch, err)
	}
	if variables[twitcheventsub.RedeemerDisplayNameVariable] != "Cooler_User" {
		t.Errorf("got variables %v, want the event's variables", variables)
	}
}

func TestRedeemFilter_IgnoresOtherRewards(t *testing.T) {
	redeemTrigger := findTrigger(t, twitcheventsub.RedeemTriggerID)

	_, isMatch, err := redeemTrigger.Filter(redeemEvent(otherRewardID), textSettings(t, twitcheventsub.RewardSetting, automationRewardID))

	if err != nil || isMatch {
		t.Errorf("got match %v, error %v; want no match", isMatch, err)
	}
}

func TestRedeemFilter_ReturnsErrorWhenRewardIsMissing(t *testing.T) {
	redeemTrigger := findTrigger(t, twitcheventsub.RedeemTriggerID)

	_, _, err := redeemTrigger.Filter(redeemEvent(automationRewardID), automationsetting.Values{})

	if !errors.Is(err, automationsetting.ErrSettingMissing) {
		t.Errorf("got error %v, want ErrSettingMissing", err)
	}
}

func TestChatMessageFilter_MatchesFirstWordAndAddsRestOfMessage(t *testing.T) {
	variables, isMatch := filterChatMessage(t, "!so", "!so coolstreamer")

	if !isMatch {
		t.Fatal("got no match")
	}
	if variables[twitcheventsub.RestOfMessageVariable] != "coolstreamer" {
		t.Errorf("got rest of message %q, want %q", variables[twitcheventsub.RestOfMessageVariable], "coolstreamer")
	}
	if variables[twitcheventsub.SenderDisplayNameVariable] != "VIPTest" {
		t.Errorf("got variables %v, want the event's variables too", variables)
	}
}

func TestChatMessageFilter_IgnoresCapitalization(t *testing.T) {
	_, isMatch := filterChatMessage(t, "!discord", "!Discord")

	if !isMatch {
		t.Error("got no match")
	}
}

func TestChatMessageFilter_MatchesMessageThatIsOnlyTheText(t *testing.T) {
	variables, isMatch := filterChatMessage(t, "!a1", "!a1")

	if !isMatch || variables[twitcheventsub.RestOfMessageVariable] != "" {
		t.Errorf("got match %v with rest of message %q; want a match with nothing left", isMatch, variables[twitcheventsub.RestOfMessageVariable])
	}
}

func TestChatMessageFilter_RequiresWholeWord(t *testing.T) {
	_, isMatch := filterChatMessage(t, "!a1", "!a10")

	if isMatch {
		t.Error(`"!a1" matched "!a10"`)
	}
}

func TestChatMessageFilter_IgnoresTextLaterInMessage(t *testing.T) {
	_, isMatch := filterChatMessage(t, "!a1", "type !a1 for an Irish voice")

	if isMatch {
		t.Error("matched text that isn't at the start of the message")
	}
}

func TestChatMessageFilter_RemovesAllSpacesBeforeRestOfMessage(t *testing.T) {
	variables, _ := filterChatMessage(t, "!so", "!so   coolstreamer")

	if variables[twitcheventsub.RestOfMessageVariable] != "coolstreamer" {
		t.Errorf("got rest of message %q, want %q", variables[twitcheventsub.RestOfMessageVariable], "coolstreamer")
	}
}

func TestChatMessageFilter_IgnoresSpacesAroundSettingText(t *testing.T) {
	_, isMatch := filterChatMessage(t, " !discord ", "!discord")

	if !isMatch {
		t.Error("got no match")
	}
}

func TestChatMessageFilter_MatchesLettersWhoseCasesDifferInLength(t *testing.T) {
	// The Kelvin sign is an uppercase "k" that takes three bytes rather than
	// one, so comparing byte slices of equal length would miss it.
	_, isMatch := filterChatMessage(t, "!kick", "!Kick")

	if !isMatch {
		t.Error("got no match")
	}
}

func TestChatMessageFilter_ReturnsErrorWhenTextIsBlank(t *testing.T) {
	chatTrigger := findTrigger(t, twitcheventsub.ChatMessageTriggerID)

	_, _, err := chatTrigger.Filter(chatMessageEvent("anything"), textSettings(t, twitcheventsub.StartsWithSetting, "   "))

	if !errors.Is(err, automationsetting.ErrSettingMissing) {
		t.Errorf("got error %v, want ErrSettingMissing", err)
	}
}

func TestChatMessageFilter_LeavesEventVariablesUnchanged(t *testing.T) {
	chatTrigger := findTrigger(t, twitcheventsub.ChatMessageTriggerID)
	event := chatMessageEvent("!so coolstreamer")

	_, _, err := chatTrigger.Filter(event, textSettings(t, twitcheventsub.StartsWithSetting, "!so"))

	if err != nil {
		t.Fatalf("Filter returned an error: %v", err)
	}
	if _, hasRestOfMessage := event.Variables[twitcheventsub.RestOfMessageVariable]; hasRestOfMessage {
		t.Error("Filter added a variable to the event itself, which other automations share")
	}
}

func TestFollowFilter_MatchesEveryFollow(t *testing.T) {
	followTrigger := findTrigger(t, twitcheventsub.FollowTriggerID)
	event := trigger.Event{
		TriggerID: twitcheventsub.FollowTriggerID,
		Variables: map[string]string{twitcheventsub.FollowerDisplayNameVariable: "Cool_User"},
	}

	_, isMatch, err := followTrigger.Filter(event, automationsetting.Values{})

	if err != nil || !isMatch {
		t.Errorf("got match %v, error %v; want a match", isMatch, err)
	}
}

func TestFilters_IgnoreEventsForOtherTriggers(t *testing.T) {
	settingsByTrigger := map[string]automationsetting.Values{
		twitcheventsub.RedeemTriggerID:      textSettings(t, twitcheventsub.RewardSetting, automationRewardID),
		twitcheventsub.ChatMessageTriggerID: textSettings(t, twitcheventsub.StartsWithSetting, "!so"),
		twitcheventsub.FollowTriggerID:      {},
	}
	otherTriggerEvent := trigger.Event{TriggerID: "other.trigger", Variables: map[string]string{}}

	for triggerID, settings := range settingsByTrigger {
		_, isMatch, err := findTrigger(t, triggerID).Filter(otherTriggerEvent, settings)
		if err != nil || isMatch {
			t.Errorf("trigger %q: got match %v, error %v; want no match", triggerID, isMatch, err)
		}
	}
}
