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
	// known remembers the last list that loaded, for missing redeems.
	known *knownRewards
}

// ListRewards returns every reward the account has, cheapest first.
func (service *RewardsService) ListRewards(ctx context.Context) (RewardList, error) {
	rewards, err := service.source.List(ctx)
	if errors.Is(err, twitchrewards.ErrNoChannelPoints) {
		service.known.forget()
		return RewardList{HasChannelPoints: false, Rewards: []twitchrewards.Reward{}}, nil
	}
	// Any other failure keeps the last list, since Twitch being unreachable
	// says nothing about which redeems exist.
	if err != nil {
		return RewardList{}, err
	}
	service.known.remember(rewards)
	return RewardList{HasChannelPoints: true, Rewards: rewards}, nil
}

// RewardLimits are Twitch's limits on a redeem, so the Redeems page can say
// what's allowed before asking Twitch.
type RewardLimits struct {
	MaximumTitleLength       int   `json:"maximumTitleLength"`
	MinimumCost              int64 `json:"minimumCost"`
	MaximumCost              int64 `json:"maximumCost"`
	MaximumRewardsPerChannel int   `json:"maximumRewardsPerChannel"`
}

// Limits returns Twitch's limits on a redeem.
func (service *RewardsService) Limits() RewardLimits {
	return RewardLimits{
		MaximumTitleLength:       twitchrewards.MaximumTitleLength,
		MinimumCost:              twitchrewards.MinimumCost,
		MaximumCost:              twitchrewards.MaximumCost,
		MaximumRewardsPerChannel: twitchrewards.MaximumRewardsPerChannel,
	}
}

// NewRewardSettings returns the settings a new redeem starts with.
func (service *RewardsService) NewRewardSettings() twitchrewards.Settings {
	return twitchrewards.NewSettings()
}

// CreateReward makes a new redeem in Twitch, which Whendo can then edit. The
// Redeems page reloads the list afterwards, which also updates what missing
// redeems are checked against.
func (service *RewardsService) CreateReward(ctx context.Context, settings twitchrewards.Settings) (twitchrewards.Reward, error) {
	return service.source.Create(ctx, settings)
}

// UpdateReward changes a redeem Whendo made.
func (service *RewardsService) UpdateReward(ctx context.Context, rewardID string, settings twitchrewards.Settings) (twitchrewards.Reward, error) {
	return service.source.Update(ctx, rewardID, settings)
}

// SetRewardPaused pauses or resumes a redeem Whendo made.
func (service *RewardsService) SetRewardPaused(ctx context.Context, rewardID string, isPaused bool) (twitchrewards.Reward, error) {
	return service.source.SetPaused(ctx, rewardID, isPaused)
}

// DeleteReward deletes a redeem Whendo made. Automations that use it are
// left as they are, and get a missing redeem problem.
func (service *RewardsService) DeleteReward(ctx context.Context, rewardID string) error {
	return service.source.Delete(ctx, rewardID)
}
