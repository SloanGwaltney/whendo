// Package twitchsteps holds the steps that act on Twitch, such as sending a
// chat message. They act as the logged-in streamer, in the streamer's own
// channel. See docs/design/twitch-steps.md.
package twitchsteps

import (
	"context"
	"net/http"
	"strings"

	"github.com/SloanGwaltney/whendo/engine/step"
)

// DefaultHelixBaseURL is the base URL of Twitch's API.
const DefaultHelixBaseURL = "https://api.twitch.tv/helix"

// chatGroup is the step picker's group for steps that act on chat.
const chatGroup = "Chat"

// Account is the Twitch login the steps act as. twitchauth.Session
// implements it.
type Account interface {
	AccountUserID() (string, error)
	AccessToken(ctx context.Context) (string, error)
	// RenewAccessToken replaces a token Twitch rejected. It returns
	// twitchauth.ErrNotLoggedIn if the login can't be renewed.
	RenewAccessToken(ctx context.Context, rejectedAccessToken string) (string, error)
}

// Config holds what the Twitch steps need.
type Config struct {
	ClientID string
	Account  Account
	// HelixBaseURL defaults to Twitch's API. Tests point it at a fake.
	HelixBaseURL string
	HTTPClient   *http.Client
}

// Steps returns every Twitch step.
func Steps(config Config) []step.Step {
	helixBaseURL := config.HelixBaseURL
	if helixBaseURL == "" {
		helixBaseURL = DefaultHelixBaseURL
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	client := &helixClient{
		clientID:   config.ClientID,
		account:    config.Account,
		baseURL:    strings.TrimSuffix(helixBaseURL, "/"),
		httpClient: httpClient,
	}
	return []step.Step{sendChatMessageStep{helix: client}}
}
