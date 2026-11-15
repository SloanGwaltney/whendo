package main

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

// twitchConnectionStateChangedEvent carries the new
// twitcheventsub.ConnectionState each time the EventSub connection changes.
const twitchConnectionStateChangedEvent = "twitch-connection-state-changed"

// TwitchEventsService runs the EventSub listener while the user is logged in
// and lets the UI show its connection status.
type TwitchEventsService struct {
	listener *twitcheventsub.Listener
}

// ServiceStartup starts listening for Twitch events whenever the user is
// logged in, until Whendo exits.
func (service *TwitchEventsService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// Wails cancels ctx right before shutdown, which stops the listener.
	go service.listener.RunWhileLoggedIn(ctx)
	return nil
}

// GetConnectionState returns the current EventSub connection state.
func (service *TwitchEventsService) GetConnectionState() twitcheventsub.ConnectionState {
	return service.listener.State()
}

// loggingEventReceiver logs every trigger event it receives. It stands in
// for the automation engine until that exists, so events can be seen
// arriving while testing.
type loggingEventReceiver struct {
	logger *slog.Logger
}

func (receiver *loggingEventReceiver) ReceiveEvent(event trigger.Event) {
	receiver.logger.Info("received a Twitch event", "trigger", event.TriggerID, "variables", event.Variables)
}
