package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/coresteps"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

const (
	existingRewardID = "c7a2-existing"
	deletedRewardID  = "9f1b-deleted"
)

func redeemAutomation(t *testing.T, automationID string, rewardID string) automation.Automation {
	t.Helper()
	return automation.Automation{
		ID:        automationID,
		Name:      "Redeem " + automationID,
		IsEnabled: false,
		Trigger: automation.TriggerSelection{
			ID:       twitcheventsub.RedeemTriggerID,
			Settings: automationsetting.Values{twitcheventsub.RewardSetting: encodeSetting(t, rewardID)},
		},
		Steps: []automation.Step{{
			ID:       "wait",
			Type:     coresteps.WaitStepID,
			Settings: automationsetting.Values{coresteps.WaitDurationSetting: encodeSetting(t, waitMilliseconds)},
		}},
	}
}

// listRewards loads a list of redeems through the rewards service, which the
// automations service shares, the way the Redeems page or picker would.
func (fixture *serviceFixture) listRewards(source twitchrewards.Source) {
	fixture.t.Helper()
	rewardsService := &RewardsService{source: source, known: fixture.service.knownRewards}
	_, err := rewardsService.ListRewards(context.Background())
	if err != nil {
		fixture.t.Fatalf("ListRewards returned an error: %v", err)
	}
}

func hasMissingRedeemProblem(report automation.Report) bool {
	for _, problem := range report.Problems {
		if problem.Message == missingRedeemMessage {
			return problem.Level == automation.ProblemLevelGoodToKnow && problem.SettingID == twitcheventsub.RewardSetting
		}
	}
	return false
}

var existingRewardList = fixedRewardSource{rewards: []twitchrewards.Reward{{ID: existingRewardID, Title: "Hydrate"}}}

func TestCheckAutomation_PointsOutARedeemTheLastListDidntHave(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.listRewards(existingRewardList)

	report := fixture.service.CheckAutomation(redeemAutomation(t, "deleted", deletedRewardID))

	if !hasMissingRedeemProblem(report) {
		t.Errorf("got problems %+v, want the missing redeem as good to know", report.Problems)
	}
	if _, hasMustFix := report.FirstMustFixProblem(); hasMustFix {
		t.Error("a missing redeem made the automation must-fix")
	}
}

func TestCheckAutomation_LeavesExistingRedeemsAlone(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.listRewards(existingRewardList)

	report := fixture.service.CheckAutomation(redeemAutomation(t, "existing", existingRewardID))

	if hasMissingRedeemProblem(report) {
		t.Errorf("got problems %+v for a redeem that exists", report.Problems)
	}
}

func TestCheckAutomation_NeverCallsARedeemMissingBeforeAListLoads(t *testing.T) {
	fixture := newServiceFixture(t, nil)

	report := fixture.service.CheckAutomation(redeemAutomation(t, "unknown", deletedRewardID))

	if hasMissingRedeemProblem(report) {
		t.Errorf("got problems %+v without a list of redeems", report.Problems)
	}
}

func TestCheckAutomation_ForgetsTheListWhenTheAccountHasNoChannelPoints(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.listRewards(existingRewardList)
	fixture.listRewards(fixedRewardSource{err: twitchrewards.ErrNoChannelPoints})

	report := fixture.service.CheckAutomation(redeemAutomation(t, "deleted", deletedRewardID))

	if hasMissingRedeemProblem(report) {
		t.Errorf("got problems %+v for an account without channel points", report.Problems)
	}
}

func TestCheckAutomation_KeepsTheListWhenTwitchCantBeReached(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.listRewards(existingRewardList)
	rewardsService := &RewardsService{source: fixedRewardSource{err: errors.New("twitch is down")}, known: fixture.service.knownRewards}
	_, err := rewardsService.ListRewards(context.Background())
	if err == nil {
		t.Fatal("got no error from a source that failed")
	}

	report := fixture.service.CheckAutomation(redeemAutomation(t, "deleted", deletedRewardID))

	if !hasMissingRedeemProblem(report) {
		t.Errorf("got problems %+v, want the last list that loaded still used", report.Problems)
	}
}

func TestCheckAutomation_NeverCallsThePlaceholderMissing(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.listRewards(existingRewardList)

	report := fixture.service.CheckAutomation(redeemAutomation(t, "placeholder", twitcheventsub.PlaceholderRewardID))

	if hasMissingRedeemProblem(report) {
		t.Errorf("got problems %+v for the placeholder", report.Problems)
	}
}

func TestListAutomations_NotesPlaceholderAndMissingRedeems(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(redeemAutomation(t, "deleted", deletedRewardID))
	fixture.save(redeemAutomation(t, "placeholder", twitcheventsub.PlaceholderRewardID))
	fixture.save(redeemAutomation(t, "existing", existingRewardID))
	fixture.listRewards(existingRewardList)

	summaries := summariesByID(fixture.service.ListAutomations())

	if summaries["deleted"].TriggerNote != missingRedeemMessage {
		t.Errorf("got note %q for the deleted redeem", summaries["deleted"].TriggerNote)
	}
	if summaries["placeholder"].TriggerNote == "" {
		t.Error("got no note for the placeholder")
	}
	if summaries["existing"].TriggerNote != "" {
		t.Errorf("got note %q for a redeem that exists", summaries["existing"].TriggerNote)
	}
}

func TestAutomationsUsingReward_NamesOnlyAutomationsOnThatRedeem(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(redeemAutomation(t, "second", existingRewardID))
	fixture.save(redeemAutomation(t, "first", existingRewardID))
	fixture.save(redeemAutomation(t, "other", deletedRewardID))
	fixture.save(chatAutomation(t, "chat", "Chat"))

	names := fixture.service.AutomationsUsingReward(existingRewardID)

	if !slices.Equal(names, []string{"Redeem first", "Redeem second"}) {
		t.Errorf("got %v, want the two automations on the redeem, sorted", names)
	}
}
