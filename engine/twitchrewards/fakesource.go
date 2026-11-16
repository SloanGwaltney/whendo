package twitchrewards

import (
	"context"
	"slices"
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
