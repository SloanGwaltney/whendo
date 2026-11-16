// Package twitchsteps holds the steps that act on Twitch, such as sending a
// chat message or refunding a redemption. They act as the logged-in streamer, in the streamer's own
// channel. See docs/design/twitch-steps.md.
package twitchsteps

import (
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitchapi"
)

// chatGroup is the step picker's group for steps that act on chat.
const chatGroup = "Chat"

// Steps returns every Twitch step, acting through the given client. The
// refund and fulfill steps use rewards to point out redeems whose
// redemptions Twitch won't let Whendo change.
func Steps(twitch *twitchapi.Client, rewards RewardLookup) []step.Step {
	return []step.Step{
		sendChatMessageStep{twitch: twitch},
		redemptionStatusStep{twitch: twitch, rewards: rewards, wording: refundWording},
		redemptionStatusStep{twitch: twitch, rewards: rewards, wording: fulfillWording},
	}
}
