package twitchrewards

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// FakeSource keeps made-up rewards in memory, for developing the Redeems
// page and the redeem picker on an account without channel points. Some are
// marked as made by Whendo and some aren't, so both kinds can be worked on.
// It's for developers only and never shown to users. See
// docs/design/twitch-redeems.md#developing-without-channel-points.
type FakeSource struct {
	mutex   sync.Mutex
	rewards []Reward
	// createdCount numbers the IDs of rewards made while Whendo runs.
	createdCount int
}

// Costs and cooldowns of the made-up rewards, loosely based on our first
// user's redeems.
const (
	fakeHydrateCost         = 100
	fakeCheckInCost         = 50
	fakeDiceCost            = 500
	fakeVoiceCost           = 1000
	fakeDiceCooldownSeconds = 300
	fakeCheckInPerStream    = 1
)

// NewFakeSource returns a source with a few made-up rewards. Changes to them
// last until Whendo restarts.
func NewFakeSource() *FakeSource {
	return &FakeSource{rewards: []Reward{
		{
			ID: "fake-check-in", Title: "Check In", Prompt: "Say hi to chat.", Cost: fakeCheckInCost,
			BackgroundColor: "#4C7BD9", IsShownToViewers: true, MaximumPerViewerPerStream: fakeCheckInPerStream, IsMadeByWhendo: true,
		},
		{
			ID: "fake-hydrate", Title: "Hydrate", Prompt: "Make the streamer drink some water.", Cost: fakeHydrateCost,
			BackgroundColor: "#3FB6C8", IsShownToViewers: true, IsMadeByWhendo: true,
		},
		{
			ID: "fake-d20-mute-me", Title: "D20 Mute Me", Prompt: "Roll a 20 and the streamer mutes you.", Cost: fakeDiceCost,
			BackgroundColor: "#D9534C", IsShownToViewers: true, CooldownSeconds: fakeDiceCooldownSeconds,
		},
		{
			ID: "fake-do-a-voice", Title: "Do a Voice", Prompt: "Pick a voice for the streamer.", Cost: fakeVoiceCost,
			BackgroundColor: "#9147FF", IsShownToViewers: false, IsUserInputRequired: true, CanBeRefunded: true,
		},
	}}
}

// List returns the made-up rewards, cheapest first.
func (source *FakeSource) List(ctx context.Context) ([]Reward, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	rewards := slices.Clone(source.rewards)
	sortForShowing(rewards)
	return rewards, nil
}

// Create makes a made-up reward, following Twitch's rules on names and the
// number of rewards, so the Redeems page behaves as it would with Twitch.
func (source *FakeSource) Create(ctx context.Context, settings Settings) (Reward, error) {
	err := settings.Validate()
	if err != nil {
		return Reward{}, err
	}
	source.mutex.Lock()
	defer source.mutex.Unlock()
	if len(source.rewards) >= MaximumRewardsPerChannel {
		return Reward{}, ErrTooManyRewards
	}
	if source.isTitleTaken(settings.Title, "") {
		return Reward{}, ErrTitleTaken
	}
	source.createdCount++
	reward := Reward{ID: fmt.Sprintf("fake-created-%d", source.createdCount), IsMadeByWhendo: true}.withSettings(settings)
	source.rewards = append(source.rewards, reward)
	return reward, nil
}

// Update replaces a made-up reward's settings.
func (source *FakeSource) Update(ctx context.Context, rewardID string, settings Settings) (Reward, error) {
	err := settings.Validate()
	if err != nil {
		return Reward{}, err
	}
	source.mutex.Lock()
	defer source.mutex.Unlock()
	if source.isTitleTaken(settings.Title, rewardID) {
		return Reward{}, ErrTitleTaken
	}
	return source.changeReward(rewardID, func(reward Reward) Reward { return reward.withSettings(settings) })
}

// SetPaused pauses or resumes a made-up reward.
func (source *FakeSource) SetPaused(ctx context.Context, rewardID string, isPaused bool) (Reward, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	return source.changeReward(rewardID, func(reward Reward) Reward {
		reward.IsPaused = isPaused
		return reward
	})
}

// Delete removes a made-up reward.
func (source *FakeSource) Delete(ctx context.Context, rewardID string) error {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	index, err := source.changeableIndex(rewardID)
	if err != nil {
		return err
	}
	source.rewards = slices.Delete(source.rewards, index, index+1)
	return nil
}

// changeReward applies a change to a reward Whendo made. The mutex must be
// held.
func (source *FakeSource) changeReward(rewardID string, change func(Reward) Reward) (Reward, error) {
	index, err := source.changeableIndex(rewardID)
	if err != nil {
		return Reward{}, err
	}
	source.rewards[index] = change(source.rewards[index])
	return source.rewards[index], nil
}

// changeableIndex finds a reward Whendo may change, refusing others as
// Twitch would. The mutex must be held.
func (source *FakeSource) changeableIndex(rewardID string) (int, error) {
	index := slices.IndexFunc(source.rewards, func(reward Reward) bool { return reward.ID == rewardID })
	if index < 0 {
		return 0, ErrRewardNotFound
	}
	if !source.rewards[index].IsMadeByWhendo {
		return 0, ErrNotMadeByWhendo
	}
	return index, nil
}

// isTitleTaken checks names ignoring capitalization, as Twitch does. The
// reward being renamed doesn't count. The mutex must be held.
func (source *FakeSource) isTitleTaken(title string, renamedRewardID string) bool {
	trimmedTitle := strings.TrimSpace(title)
	return slices.ContainsFunc(source.rewards, func(reward Reward) bool {
		return reward.ID != renamedRewardID && strings.EqualFold(reward.Title, trimmedTitle)
	})
}
