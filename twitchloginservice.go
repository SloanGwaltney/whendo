package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/twitchauth"
)

// twitchLoginStateChangedEvent carries the new twitchauth.LoginState each
// time the user's Twitch login changes.
const twitchLoginStateChangedEvent = "twitch-login-state-changed"

// TwitchLoginService connects the engine's Twitch login to the UI. The login
// itself lives in the engine; this service only opens the browser and relays
// state changes.
type TwitchLoginService struct {
	session *twitchauth.Session
}

// ServiceStartup restores the login from an earlier run and keeps it
// validated until Whendo exits.
func (service *TwitchLoginService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	err := service.session.Restore()
	if err != nil {
		return fmt.Errorf("restoring the Twitch login: %w", err)
	}
	// Wails cancels ctx right before shutdown, which stops validation.
	go service.session.KeepValidated(ctx)
	return nil
}

// GetLoginState returns the current Twitch login state.
func (service *TwitchLoginService) GetLoginState() twitchauth.LoginState {
	return service.session.State()
}

// StartLogin starts a Twitch login and opens Twitch's activation page in the
// user's browser.
func (service *TwitchLoginService) StartLogin(ctx context.Context) (twitchauth.LoginState, error) {
	state, err := service.session.StartLogin(ctx)
	if err != nil {
		return twitchauth.LoginState{}, err
	}
	err = application.Get().Browser.OpenURL(state.VerificationURI)
	if err != nil {
		// The UI shows the link and a button to try again, so the user can
		// still finish logging in.
		application.Get().Logger.Warn("couldn't open the Twitch activation page", "error", err)
	}
	return state, nil
}

// OpenVerificationPage opens Twitch's activation page again, for when the
// user closed it.
func (service *TwitchLoginService) OpenVerificationPage() error {
	state := service.session.State()
	if state.Status != twitchauth.LoginStatusWaitingForApproval {
		return fmt.Errorf("no Twitch login is waiting for approval")
	}
	err := application.Get().Browser.OpenURL(state.VerificationURI)
	if err != nil {
		return fmt.Errorf("opening the Twitch activation page: %w", err)
	}
	return nil
}

// CancelLogin stops waiting for the user to approve a login.
func (service *TwitchLoginService) CancelLogin() {
	service.session.CancelLogin()
}

// Logout logs the user out of Twitch.
func (service *TwitchLoginService) Logout(ctx context.Context) error {
	return service.session.Logout(ctx)
}
