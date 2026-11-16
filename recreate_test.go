package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

type recreateFixture struct {
	*serviceFixture
	source         *twitchrewards.FakeSource
	rewardsService *RewardsService
}

func newRecreateFixture(t *testing.T) recreateFixture {
	t.Helper()
	fixture := newServiceFixture(t, nil)
	source := twitchrewards.NewFakeSource()
	return recreateFixture{
		serviceFixture: fixture,
		source:         source,
		rewardsService: &RewardsService{
			source:            source,
			known:             fixture.service.knownRewards,
			switchAutomations: fixture.service.switchReward,
			accountLogin:      func() string { return testStreamer.Login },
		},
	}
}

// dashboardReward returns a made-up redeem Whendo didn't make.
func (fixture recreateFixture) dashboardReward(title string) twitchrewards.Reward {
	fixture.t.Helper()
	rewards, err := fixture.source.List(context.Background())
	if err != nil {
		fixture.t.Fatalf("List returned an error: %v", err)
	}
	index := slices.IndexFunc(rewards, func(reward twitchrewards.Reward) bool { return reward.Title == title })
	if index < 0 || rewards[index].IsMadeByWhendo {
		fixture.t.Fatalf("no dashboard redeem called %q", title)
	}
	return rewards[index]
}

func (fixture recreateFixture) rewardIDOf(automationID string) string {
	fixture.t.Helper()
	saved := fixture.loadFromFolder(automationID)
	rewardID, err := saved.Trigger.Settings.Text(twitcheventsub.RewardSetting)
	if err != nil {
		fixture.t.Fatalf("reading %q's redeem: %v", automationID, err)
	}
	return rewardID
}

func TestRecreateReward_RefusesWhileTheOriginalStillExists(t *testing.T) {
	fixture := newRecreateFixture(t)
	original := fixture.dashboardReward("D20 Mute Me")

	_, err := fixture.rewardsService.RecreateReward(context.Background(), original)

	if !errors.Is(err, errOriginalStillExists) {
		t.Errorf("got error %v, want errOriginalStillExists", err)
	}
}

func TestRecreateReward_CopiesTheRedeemAndSwitchesItsAutomations(t *testing.T) {
	fixture := newRecreateFixture(t)
	original := fixture.dashboardReward("D20 Mute Me")
	fixture.save(redeemAutomation(t, "dice", original.ID))
	fixture.save(redeemAutomation(t, "unrelated", existingRewardID))
	err := fixture.rewardsService.OpenRewardsDashboard(original.ID)
	if err != nil {
		t.Fatalf("acting out the dashboard deletion: %v", err)
	}

	result, err := fixture.rewardsService.RecreateReward(context.Background(), original)

	if err != nil {
		t.Fatalf("RecreateReward returned an error: %v", err)
	}
	copied := result.Reward
	if !copied.IsMadeByWhendo || copied.ID == original.ID || copied.Settings() != original.Settings() {
		t.Errorf("got copy %+v, want Whendo's copy of %+v with the same settings", copied, original)
	}
	if !slices.Equal(result.SwitchedAutomations, []string{"Redeem dice"}) || result.SwitchProblem != "" {
		t.Errorf("got switched %v with problem %q, want only the dice automation", result.SwitchedAutomations, result.SwitchProblem)
	}
	if rewardID := fixture.rewardIDOf("dice"); rewardID != copied.ID {
		t.Errorf("the dice automation uses %q, want the copy %q", rewardID, copied.ID)
	}
	if rewardID := fixture.rewardIDOf("unrelated"); rewardID != existingRewardID {
		t.Errorf("the unrelated automation was switched to %q", rewardID)
	}
}

func TestRecreateReward_KeepsTheOriginalPaused(t *testing.T) {
	fixture := newRecreateFixture(t)
	original := fixture.dashboardReward("Do a Voice")
	original.IsPaused = true
	err := fixture.rewardsService.OpenRewardsDashboard(original.ID)
	if err != nil {
		t.Fatalf("acting out the dashboard deletion: %v", err)
	}

	result, err := fixture.rewardsService.RecreateReward(context.Background(), original)

	if err != nil || !result.Reward.IsPaused {
		t.Errorf("got %+v, error %v; want the copy paused", result.Reward, err)
	}
}
