package main

import (
	"context"
	"errors"

	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
)

// RewardList is the account's channel point rewards, or that it has none to
// have. An account without channel points isn't an error, since the Redeems
// page and the redeem picker explain it rather than fail.
type RewardList struct {
	HasChannelPoints bool                   `json:"hasChannelPoints"`
	Rewards          []twitchrewards.Reward `json:"rewards"`
}

// RewardsService connects the Redeems page and the redeem picker to the
// streamer's channel point rewards. See docs/design/twitch-redeems.md.
type RewardsService struct {
	source twitchrewards.Source
}

// ListRewards returns every reward the account has, cheapest first.
func (service *RewardsService) ListRewards(ctx context.Context) (RewardList, error) {
	rewards, err := service.source.List(ctx)
	if errors.Is(err, twitchrewards.ErrNoChannelPoints) {
		return RewardList{HasChannelPoints: false, Rewards: []twitchrewards.Reward{}}, nil
	}
	if err != nil {
		return RewardList{}, err
	}
	return RewardList{HasChannelPoints: true, Rewards: rewards}, nil
}
