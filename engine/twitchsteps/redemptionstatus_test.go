package twitchsteps_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

const (
	redemptionID          = "17fa2df1-ad76-4804-bfa5-a40ef63efe63"
	rewardID              = "92af127c-7326-4483-a52b-b0da0be61c01"
	updatedRedemptionBody = `{"data":[{"id":"17fa2df1-ad76-4804-bfa5-a40ef63efe63","status":"CANCELED"}]}`
)

// redemptionStatusSteps pairs each step with the status it sets.
var redemptionStatusSteps = map[string]string{
	twitchsteps.RefundRedemptionStepID:  "CANCELED",
	twitchsteps.FulfillRedemptionStepID: "FULFILLED",
}

func redemptionInput(redemptionIDValue string, rewardIDValue string) step.Input {
	return step.Input{
		Settings: automationsetting.Values{},
		Variables: automationsetting.VariableValues{
			{Source: automationsetting.VariableSourceTrigger, ID: twitcheventsub.RedemptionIDVariable}: redemptionIDValue,
			{Source: automationsetting.VariableSourceTrigger, ID: twitcheventsub.RewardIDVariable}:     rewardIDValue,
		},
	}
}

func redeemUsage(t *testing.T, rewardIDValue string) step.Usage {
	t.Helper()
	encodedRewardID, err := json.Marshal(rewardIDValue)
	if err != nil {
		t.Fatalf("encoding the reward ID: %v", err)
	}
	return step.Usage{
		TriggerID:       twitcheventsub.RedeemTriggerID,
		TriggerSettings: automationsetting.Values{twitcheventsub.RewardSetting: encodedRewardID},
	}
}

func noter(t *testing.T, stepID string, rewards fakeRewards) step.Noter {
	t.Helper()
	_, server := newFakeTwitch(t)
	statusStep := twitchStep(t, stepID, server, &fakeAccount{isLoggedIn: true}, rewards)
	stepNoter, canNote := statusStep.(step.Noter)
	if !canNote {
		t.Fatalf("%s doesn't note how it's used", stepID)
	}
	return stepNoter
}

func TestRedemptionStatusSteps_SetTheStatusOfTheRedemptionThatStartedTheAutomation(t *testing.T) {
	for stepID, wantStatus := range redemptionStatusSteps {
		fake, server := newFakeTwitch(t, fakeResponse{http.StatusOK, updatedRedemptionBody})

		result, err := twitchStep(t, stepID, server, &fakeAccount{isLoggedIn: true}, fakeRewards{}).Run(context.Background(), redemptionInput(redemptionID, rewardID))

		if err != nil || result.OutcomeID != step.DefaultOutcomeID {
			t.Fatalf("%s: got result %+v, error %v", stepID, result, err)
		}
		requests := fake.receivedRequests()
		if len(requests) != 1 {
			t.Fatalf("%s: got %d requests, want 1", stepID, len(requests))
		}
		request := requests[0]
		if request.method != http.MethodPatch || request.path != "/channel_points/custom_rewards/redemptions" {
			t.Errorf("%s: got %s %s", stepID, request.method, request.path)
		}
		if request.query.Get("broadcaster_id") != streamerUserID || request.query.Get("reward_id") != rewardID || request.query.Get("id") != redemptionID {
			t.Errorf("%s: got query %v, want the streamer, reward and redemption", stepID, request.query)
		}
		if request.body["status"] != wantStatus {
			t.Errorf("%s: got status %v, want %s", stepID, request.body["status"], wantStatus)
		}
	}
}

func TestRedemptionStatusSteps_NeedTheRedeemTriggerAndHaveNoSettings(t *testing.T) {
	for stepID := range redemptionStatusSteps {
		_, server := newFakeTwitch(t)

		definition := twitchStep(t, stepID, server, &fakeAccount{isLoggedIn: true}, fakeRewards{}).Definition()

		if definition.RequiredTriggerID != twitcheventsub.RedeemTriggerID || len(definition.Settings) != 0 {
			t.Errorf("%s: got %+v, want no settings and the redeem trigger required", stepID, definition)
		}
	}
}

func TestRedemptionStatusSteps_FailTestRunsWithoutCallingTwitch(t *testing.T) {
	for stepID := range redemptionStatusSteps {
		fake, server := newFakeTwitch(t)

		_, err := twitchStep(t, stepID, server, &fakeAccount{isLoggedIn: true}, fakeRewards{}).Run(context.Background(), redemptionInput(twitcheventsub.TestEventID, rewardID))

		if err == nil || !strings.Contains(err.Error(), "test run") || len(fake.receivedRequests()) != 0 {
			t.Errorf("%s: got error %v and %d requests, want a test run failure without calling Twitch", stepID, err, len(fake.receivedRequests()))
		}
	}
}

func TestRedemptionStatusSteps_FailWhenNotLoggedIn(t *testing.T) {
	for stepID := range redemptionStatusSteps {
		fake, server := newFakeTwitch(t)

		_, err := twitchStep(t, stepID, server, &fakeAccount{isLoggedIn: false}, fakeRewards{}).Run(context.Background(), redemptionInput(redemptionID, rewardID))

		if err == nil || len(fake.receivedRequests()) != 0 {
			t.Errorf("%s: got error %v and %d requests, want a failure without calling Twitch", stepID, err, len(fake.receivedRequests()))
		}
	}
}

func TestRedemptionStatusSteps_RenewRejectedTokenAndRetryOnce(t *testing.T) {
	fake, server := newFakeTwitch(t,
		fakeResponse{http.StatusUnauthorized, `{"message":"Invalid OAuth token"}`},
		fakeResponse{http.StatusOK, updatedRedemptionBody},
	)
	account := &fakeAccount{isLoggedIn: true}

	_, err := twitchStep(t, twitchsteps.RefundRedemptionStepID, server, account, fakeRewards{}).Run(context.Background(), redemptionInput(redemptionID, rewardID))

	if err != nil {
		t.Fatalf("got error %v", err)
	}
	requests := fake.receivedRequests()
	if len(requests) != 2 || requests[1].authorization != "Bearer "+renewedAccessToken || account.renewedCount != 1 {
		t.Errorf("got %d requests and %d renewals, want a retry with the renewed token", len(requests), account.renewedCount)
	}
}

func TestRedemptionStatusSteps_ExplainTwitchsRefusals(t *testing.T) {
	wantReasons := map[int]string{
		http.StatusForbidden:           "redeems Whendo made",
		http.StatusNotFound:            "no redemption waiting to be refunded",
		http.StatusBadRequest:          "Malformed query",
		http.StatusInternalServerError: "HTTP 500",
	}
	for statusCode, wantReason := range wantReasons {
		_, server := newFakeTwitch(t, fakeResponse{statusCode, `{"message":"Malformed query"}`})

		_, err := twitchStep(t, twitchsteps.RefundRedemptionStepID, server, &fakeAccount{isLoggedIn: true}, fakeRewards{}).Run(context.Background(), redemptionInput(redemptionID, rewardID))

		if err == nil || !strings.Contains(err.Error(), wantReason) {
			t.Errorf("HTTP %d: got error %v, want it to mention %q", statusCode, err, wantReason)
		}
	}
}

func TestRedemptionStatusSteps_NoteRedeemsWhendoDidntMake(t *testing.T) {
	rewards := fakeRewards{rewardID: {ID: rewardID, IsMadeByWhendo: false, CanBeRefunded: true}}
	for stepID := range redemptionStatusSteps {
		notes := noter(t, stepID, rewards).NoteStep(redeemUsage(t, rewardID))

		if len(notes) != 1 || !strings.Contains(notes[0].Message, "Whendo didn't make this redeem") {
			t.Errorf("%s: got notes %+v, want one about the redeem not being Whendo's", stepID, notes)
		}
	}
}

func TestRedemptionStatusSteps_NoteRedeemsThatCantBeRefunded(t *testing.T) {
	rewards := fakeRewards{rewardID: {ID: rewardID, IsMadeByWhendo: true, CanBeRefunded: false}}
	for stepID := range redemptionStatusSteps {
		notes := noter(t, stepID, rewards).NoteStep(redeemUsage(t, rewardID))

		if len(notes) != 1 || !strings.Contains(notes[0].Message, "“Viewers can be refunded”") {
			t.Errorf("%s: got notes %+v, want one about turning on refunds", stepID, notes)
		}
	}
}

func TestRedemptionStatusSteps_NoteNothingForRefundableWhendoRedeemsOrUnknownOnes(t *testing.T) {
	rewards := fakeRewards{rewardID: {ID: rewardID, IsMadeByWhendo: true, CanBeRefunded: true}}
	for _, usedRewardID := range []string{rewardID, twitcheventsub.PlaceholderRewardID, "unknown"} {
		notes := noter(t, twitchsteps.RefundRedemptionStepID, rewards).NoteStep(redeemUsage(t, usedRewardID))

		if len(notes) != 0 {
			t.Errorf("reward %q: got notes %+v, want none", usedRewardID, notes)
		}
	}
}
