package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

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
	// switchAutomations points every automation on one redeem at another,
	// for recreating a redeem. It returns the names of those it switched.
	switchAutomations func(oldRewardID string, newRewardID string) ([]string, error)
	// accountLogin gives the logged-in streamer's Twitch username, for
	// linking to their dashboard.
	accountLogin func() string
}

// rewardsDashboardURLFormat is where a streamer manages their redeems in the
// Twitch dashboard, given their username.
const rewardsDashboardURLFormat = "https://dashboard.twitch.tv/u/%s/viewer-rewards/channel-points/rewards"

// errOriginalStillExists means the streamer hasn't deleted the original
// redeem yet, which recreating needs, since two redeems can't share a name.
var errOriginalStillExists = errors.New("the original redeem is still in Twitch. Delete it in the Twitch dashboard, then try again")

// RecreateResult tells the Redeems page what recreating a redeem did.
type RecreateResult struct {
	Reward twitchrewards.Reward `json:"reward"`
	// SwitchedAutomations names the automations now on the new redeem.
	SwitchedAutomations []string `json:"switchedAutomations"`
	// SwitchProblem says why some automations couldn't be switched. The
	// redeem was still recreated, so the streamer switches those by hand.
	SwitchProblem string `json:"switchProblem"`
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

// OpenRewardsDashboard opens the streamer's redeems in the Twitch dashboard,
// where they delete the original while recreating a redeem in Whendo. Made-up
// redeems have no dashboard, so for those it acts out deleting the original
// instead.
func (service *RewardsService) OpenRewardsDashboard(originalRewardID string) error {
	actor, isActedOut := service.source.(twitchrewards.DashboardActor)
	if isActedOut {
		return actor.DeleteAsDashboard(originalRewardID)
	}
	login := service.accountLogin()
	if login == "" {
		return errors.New("not logged in to Twitch")
	}
	err := application.Get().Browser.OpenURL(fmt.Sprintf(rewardsDashboardURLFormat, url.PathEscape(login)))
	if err != nil {
		return fmt.Errorf("opening the Twitch dashboard: %w", err)
	}
	return nil
}

// RecreateReward makes a copy of a redeem made in the Twitch dashboard, once
// the streamer has deleted the original, so Whendo can change it. The
// original's settings come from the page, which kept them from the list it
// loaded, since Twitch no longer has them. Every automation on the original
// is switched to the copy. See
// docs/design/twitch-redeems.md#recreating-a-redeem-in-whendo.
func (service *RewardsService) RecreateReward(ctx context.Context, original twitchrewards.Reward) (RecreateResult, error) {
	err := service.checkOriginalIsGone(ctx, original.ID)
	if err != nil {
		return RecreateResult{}, err
	}
	copied, err := service.source.Create(ctx, original.Settings())
	if err != nil {
		return RecreateResult{}, err
	}
	result := RecreateResult{Reward: copied, SwitchedAutomations: []string{}}
	// Twitch only takes pausing on its own, after the redeem exists.
	if original.IsPaused {
		paused, pauseErr := service.source.SetPaused(ctx, copied.ID, true)
		if pauseErr != nil {
			result.SwitchProblem = fmt.Sprintf("The redeem was recreated, but couldn't be paused like the original: %v", pauseErr)
		} else {
			result.Reward = paused
		}
	}
	switched, switchErr := service.switchAutomations(original.ID, copied.ID)
	result.SwitchedAutomations = append(result.SwitchedAutomations, switched...)
	if switchErr != nil {
		result.SwitchProblem = strings.TrimSpace(result.SwitchProblem + " Some automations couldn't be switched to the new redeem, so pick it for them by hand: " + switchErr.Error())
	}
	return result, nil
}

// checkOriginalIsGone makes sure the streamer deleted the original before
// the copy is made under the same name.
func (service *RewardsService) checkOriginalIsGone(ctx context.Context, originalID string) error {
	rewards, err := service.source.List(ctx)
	if err != nil {
		return fmt.Errorf("checking that the original redeem was deleted: %w", err)
	}
	service.known.remember(rewards)
	isStillThere := slices.ContainsFunc(rewards, func(reward twitchrewards.Reward) bool { return reward.ID == originalID })
	if isStillThere {
		return errOriginalStillExists
	}
	return nil
}
