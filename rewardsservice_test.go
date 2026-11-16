package main

import (
	"context"
	"errors"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

// fixedRewardSource lists the same answer every time. Tests that change
// rewards use the fake source instead; the embedded Source is nil, so
// calling anything else fails the test loudly.
type fixedRewardSource struct {
	twitchrewards.Source
	rewards []twitchrewards.Reward
	err     error
}

func (source fixedRewardSource) List(context.Context) ([]twitchrewards.Reward, error) {
	return source.rewards, source.err
}

func TestListRewards_ExplainsAnAccountWithoutChannelPointsRatherThanFailing(t *testing.T) {
	service := &RewardsService{known: &knownRewards{}, source: fixedRewardSource{err: twitchrewards.ErrNoChannelPoints}}

	rewardList, err := service.ListRewards(context.Background())

	if err != nil || rewardList.HasChannelPoints || rewardList.Rewards == nil || len(rewardList.Rewards) != 0 {
		t.Errorf("got %+v, error %v; want an empty list without channel points", rewardList, err)
	}
}

func TestListRewards_ReturnsTheRewards(t *testing.T) {
	rewards := []twitchrewards.Reward{{ID: "hydrate", Title: "Hydrate"}}
	service := &RewardsService{known: &knownRewards{}, source: fixedRewardSource{rewards: rewards}}

	rewardList, err := service.ListRewards(context.Background())

	if err != nil || !rewardList.HasChannelPoints || len(rewardList.Rewards) != 1 || rewardList.Rewards[0].ID != "hydrate" {
		t.Errorf("got %+v, error %v; want the reward", rewardList, err)
	}
}

func TestListRewards_ReturnsOtherErrors(t *testing.T) {
	twitchDown := errors.New("twitch is down")
	service := &RewardsService{known: &knownRewards{}, source: fixedRewardSource{err: twitchDown}}

	_, err := service.ListRewards(context.Background())

	if !errors.Is(err, twitchDown) {
		t.Errorf("got error %v, want Twitch's", err)
	}
}
