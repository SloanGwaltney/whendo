// Package twitchsteps holds the steps that act on Twitch, such as sending a
// chat message. They act as the logged-in streamer, in the streamer's own
// channel. See docs/design/twitch-steps.md.
package twitchsteps

import (
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitchapi"
)

// chatGroup is the step picker's group for steps that act on chat.
const chatGroup = "Chat"

// Steps returns every Twitch step, acting through the given client.
func Steps(twitch *twitchapi.Client) []step.Step {
	return []step.Step{sendChatMessageStep{twitch: twitch}}
}
