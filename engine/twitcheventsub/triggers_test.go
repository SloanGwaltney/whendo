package twitcheventsub_test

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

const (
	redemptionType     = "channel.channel_points_custom_reward_redemption.add"
	chatMessageType    = "channel.chat.message"
	followType         = "channel.follow"
	broadcasterUserID  = "1337"
	otherBroadcasterID = "9001"
)

const redemptionEventJSON = `{
	"id": "17fa2df1-ad76-4804-bfa5-a40ef63efe63",
	"broadcaster_user_id": "1337",
	"broadcaster_user_login": "cool_user",
	"broadcaster_user_name": "Cool_User",
	"user_id": "9001",
	"user_login": "cooler_user",
	"user_name": "Cooler_User",
	"user_input": "pogchamp",
	"status": "unfulfilled",
	"reward": {
		"id": "92af127c-7326-4483-a52b-b0da0be61c01",
		"title": "title",
		"cost": 100,
		"prompt": "reward prompt"
	},
	"redeemed_at": "2020-07-15T17:16:03.17106713Z"
}`

const followEventJSON = `{
	"user_id": "1234",
	"user_login": "cool_user",
	"user_name": "Cool_User",
	"broadcaster_user_id": "1337",
	"broadcaster_user_login": "cooler_user",
	"broadcaster_user_name": "Cooler_User",
	"followed_at": "2020-07-15T18:16:11.17106713Z"
}`

func chatMessageEventJSON(sourceBroadcasterUserID string) string {
	sourceField := "null"
	if sourceBroadcasterUserID != "" {
		sourceField = `"` + sourceBroadcasterUserID + `"`
	}
	return `{
		"broadcaster_user_id": "1337",
		"broadcaster_user_login": "cool_user",
		"broadcaster_user_name": "Cool_User",
		"chatter_user_id": "4145994",
		"chatter_user_login": "viptest",
		"chatter_user_name": "VIPTest",
		"message_id": "cc106a89-1814-919d-454c-f4f2f970aae7",
		"message": {
			"text": "!a1 please",
			"fragments": [{"type": "text", "text": "!a1 please", "cheermote": null, "emote": null, "mention": null}]
		},
		"color": "#00FF7F",
		"badges": [],
		"message_type": "text",
		"cheer": null,
		"reply": null,
		"channel_points_custom_reward_id": null,
		"source_broadcaster_user_id": ` + sourceField + `,
		"source_broadcaster_user_login": null,
		"source_broadcaster_user_name": null,
		"source_message_id": null,
		"source_badges": null
	}`
}

func translate(t *testing.T, subscriptionType string, subscriptionVersion string, eventJSON string) (trigger.Event, bool) {
	t.Helper()
	event, shouldTrigger, err := twitcheventsub.EventFromNotification(subscriptionType, subscriptionVersion, []byte(eventJSON))
	if err != nil {
		t.Fatalf("EventFromNotification returned an error: %v", err)
	}
	return event, shouldTrigger
}

func assertVariables(t *testing.T, event trigger.Event, wantedVariables map[string]string) {
	t.Helper()
	if !maps.Equal(event.Variables, wantedVariables) {
		t.Errorf("got variables %v\nwant %v", event.Variables, wantedVariables)
	}
}

func TestEventFromNotification_TranslatesRedeem(t *testing.T) {
	event, shouldTrigger := translate(t, redemptionType, "1", redemptionEventJSON)

	if !shouldTrigger || event.TriggerID != twitcheventsub.RedeemTriggerID {
		t.Fatalf("got trigger %q (should trigger: %v)", event.TriggerID, shouldTrigger)
	}
	assertVariables(t, event, map[string]string{
		twitcheventsub.RedemptionIDVariable:        "17fa2df1-ad76-4804-bfa5-a40ef63efe63",
		twitcheventsub.RedeemerUserIDVariable:      "9001",
		twitcheventsub.RedeemerLoginVariable:       "cooler_user",
		twitcheventsub.RedeemerDisplayNameVariable: "Cooler_User",
		twitcheventsub.UserInputVariable:           "pogchamp",
		twitcheventsub.RewardIDVariable:            "92af127c-7326-4483-a52b-b0da0be61c01",
		twitcheventsub.RewardTitleVariable:         "title",
		twitcheventsub.RewardCostVariable:          "100",
		twitcheventsub.RewardPromptVariable:        "reward prompt",
		twitcheventsub.RedeemedAtVariable:          "2020-07-15T17:16:03.17106713Z",
	})
}

func TestEventFromNotification_TranslatesChatMessageFromOwnChat(t *testing.T) {
	event, shouldTrigger := translate(t, chatMessageType, "1", chatMessageEventJSON(""))

	if !shouldTrigger || event.TriggerID != twitcheventsub.ChatMessageTriggerID {
		t.Fatalf("got trigger %q (should trigger: %v)", event.TriggerID, shouldTrigger)
	}
	assertVariables(t, event, map[string]string{
		twitcheventsub.MessageIDVariable:         "cc106a89-1814-919d-454c-f4f2f970aae7",
		twitcheventsub.SenderUserIDVariable:      "4145994",
		twitcheventsub.SenderLoginVariable:       "viptest",
		twitcheventsub.SenderDisplayNameVariable: "VIPTest",
		twitcheventsub.MessageTextVariable:       "!a1 please",
	})
}

func TestEventFromNotification_AcceptsOwnChatMessageDuringSharedChat(t *testing.T) {
	_, shouldTrigger := translate(t, chatMessageType, "1", chatMessageEventJSON(broadcasterUserID))

	if !shouldTrigger {
		t.Error("a message from the streamer's own chat during shared chat was ignored")
	}
}

func TestEventFromNotification_IgnoresChatMessageFromOtherSharedChatChannel(t *testing.T) {
	_, shouldTrigger := translate(t, chatMessageType, "1", chatMessageEventJSON(otherBroadcasterID))

	if shouldTrigger {
		t.Error("a message from another channel in shared chat would trigger automations")
	}
}

func TestEventFromNotification_TranslatesFollow(t *testing.T) {
	event, shouldTrigger := translate(t, followType, "2", followEventJSON)

	if !shouldTrigger || event.TriggerID != twitcheventsub.FollowTriggerID {
		t.Fatalf("got trigger %q (should trigger: %v)", event.TriggerID, shouldTrigger)
	}
	assertVariables(t, event, map[string]string{
		twitcheventsub.FollowerUserIDVariable:      "1234",
		twitcheventsub.FollowerLoginVariable:       "cool_user",
		twitcheventsub.FollowerDisplayNameVariable: "Cool_User",
		twitcheventsub.FollowedAtVariable:          "2020-07-15T18:16:11.17106713Z",
	})
}

func TestEventFromNotification_ReturnsErrorForUnknownSubscriptionType(t *testing.T) {
	_, _, err := twitcheventsub.EventFromNotification("channel.raid", "1", []byte(`{}`))

	if !errors.Is(err, twitcheventsub.ErrUnsupportedSubscription) {
		t.Errorf("got error %v, want ErrUnsupportedSubscription", err)
	}
}

func TestEventFromNotification_ReturnsErrorForUnsupportedVersion(t *testing.T) {
	_, _, err := twitcheventsub.EventFromNotification(followType, "1", []byte(followEventJSON))

	if !errors.Is(err, twitcheventsub.ErrUnsupportedSubscription) {
		t.Errorf("got error %v, want ErrUnsupportedSubscription", err)
	}
}

func TestEventFromNotification_ReturnsErrorForMalformedEvent(t *testing.T) {
	_, _, err := twitcheventsub.EventFromNotification(redemptionType, "1", []byte(`{"reward": "not an object"}`))

	if err == nil {
		t.Error("got no error for a malformed event")
	}
}

func TestTriggers_DeclareExactlyTheVariablesTheirEventsProvide(t *testing.T) {
	sampleEvents := map[string]trigger.Event{}
	for _, sample := range []struct {
		subscriptionType    string
		subscriptionVersion string
		eventJSON           string
	}{
		{redemptionType, "1", redemptionEventJSON},
		{chatMessageType, "1", chatMessageEventJSON("")},
		{followType, "2", followEventJSON},
	} {
		event, _ := translate(t, sample.subscriptionType, sample.subscriptionVersion, sample.eventJSON)
		sampleEvents[event.TriggerID] = event
	}

	for _, definition := range twitcheventsub.Triggers() {
		event, hasSample := sampleEvents[definition.ID]
		if !hasSample {
			t.Errorf("trigger %q has no sample event", definition.ID)
			continue
		}
		var declaredIDs []string
		for _, variable := range definition.Variables {
			declaredIDs = append(declaredIDs, variable.ID)
		}
		providedIDs := slices.Collect(maps.Keys(event.Variables))
		slices.Sort(declaredIDs)
		slices.Sort(providedIDs)
		if !slices.Equal(declaredIDs, providedIDs) {
			t.Errorf("trigger %q declares %v but its events provide %v", definition.ID, declaredIDs, providedIDs)
		}
	}
}
