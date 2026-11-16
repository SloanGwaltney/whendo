package main

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
	"github.com/SloanGwaltney/whendo/engine/streamlabs"
)

// streamingSoftwareStateChangedEvent carries the new StreamingSoftwareState
// each time the chosen program or its connection status changes.
const streamingSoftwareStateChangedEvent = "streaming-software-state-changed"

// StreamingSoftwareState is everything the UI needs to show the streaming
// software and its connection status.
type StreamingSoftwareState struct {
	Software appsettings.StreamingSoftware `json:"software"`
	// Status means nothing while Software is none.
	Status streamlabs.ConnectionStatus `json:"status"`
}

// StreamingSoftwareService connects to the streaming software the streamer
// chose in Settings, and lets the UI change the choice and show the
// connection status. See docs/design/streaming-software.md.
type StreamingSoftwareService struct {
	settings   *SettingsService
	connection *streamlabs.Connection
	// onStateChange is called with the new state each time it changes.
	onStateChange func(StreamingSoftwareState)

	// choiceMutex lets one choice be applied at a time, and guards the
	// fields below. The connection reports its status without taking it,
	// since a choice waits for the connection to stop while holding it.
	choiceMutex    sync.Mutex
	applicationCtx context.Context
	stopConnection func()
}

// newStreamingSoftwareService returns a service that connects to Streamlabs
// Desktop as connectionConfig says. Its OnStatusChange is replaced, so the
// service can report status changes itself.
func newStreamingSoftwareService(settings *SettingsService, connectionConfig streamlabs.Config, onStateChange func(StreamingSoftwareState)) *StreamingSoftwareService {
	service := &StreamingSoftwareService{settings: settings, onStateChange: onStateChange}
	connectionConfig.OnStatusChange = func(streamlabs.ConnectionStatus) { service.reportStatusChange() }
	service.connection = streamlabs.NewConnection(connectionConfig)
	return service
}

// ServiceStartup connects to the program chosen last time, if any.
func (service *StreamingSoftwareService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	service.choiceMutex.Lock()
	defer service.choiceMutex.Unlock()
	// Wails cancels ctx right before shutdown, which stops the connection.
	service.applicationCtx = ctx
	service.applyChoice(service.settings.streamingSoftware())
	return nil
}

// ServiceShutdown waits for the connection to stop.
func (service *StreamingSoftwareService) ServiceShutdown() error {
	service.choiceMutex.Lock()
	defer service.choiceMutex.Unlock()
	service.stopRunningConnection()
	return nil
}

// GetState returns the chosen program and its connection status.
func (service *StreamingSoftwareService) GetState() StreamingSoftwareState {
	return StreamingSoftwareState{
		Software: service.settings.streamingSoftware(),
		Status:   service.connection.Status(),
	}
}

// ChooseStreamingSoftware saves the program the streamer uses, and connects
// to it straight away, or disconnects for none.
func (service *StreamingSoftwareService) ChooseStreamingSoftware(software appsettings.StreamingSoftware) error {
	service.choiceMutex.Lock()
	defer service.choiceMutex.Unlock()
	if software == service.settings.streamingSoftware() {
		return nil
	}
	err := service.settings.setStreamingSoftware(software)
	if err != nil {
		return err
	}
	service.applyChoice(software)
	service.reportStatusChange()
	return nil
}

// reportStatusChange tells the UI about the current state.
func (service *StreamingSoftwareService) reportStatusChange() {
	if service.onStateChange != nil {
		service.onStateChange(service.GetState())
	}
}

// applyChoice stops any connection and starts the chosen program's.
// choiceMutex must be held.
func (service *StreamingSoftwareService) applyChoice(software appsettings.StreamingSoftware) {
	service.stopRunningConnection()
	if software != appsettings.StreamingSoftwareStreamlabsDesktop || service.applicationCtx == nil {
		return
	}
	connectionCtx, cancel := context.WithCancel(service.applicationCtx)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		service.connection.Run(connectionCtx)
	}()
	service.stopConnection = func() {
		cancel()
		<-stopped
	}
}

// stopRunningConnection stops the connection, if it's running, and waits
// for it, so Run is never called twice at once. choiceMutex must be held.
func (service *StreamingSoftwareService) stopRunningConnection() {
	if service.stopConnection == nil {
		return
	}
	service.stopConnection()
	service.stopConnection = nil
}
