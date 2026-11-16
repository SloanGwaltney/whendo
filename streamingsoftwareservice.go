package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
	"github.com/SloanGwaltney/whendo/engine/scenes"
	"github.com/SloanGwaltney/whendo/engine/streamlabs"
)

// streamingSoftwareStateChangedEvent carries the new StreamingSoftwareState
// each time the chosen program or its connection status changes.
const streamingSoftwareStateChangedEvent = "streaming-software-state-changed"

// errNoStreamingSoftwareChosen means the streamer hasn't chosen a program,
// so there are no scenes to list.
var errNoStreamingSoftwareChosen = errors.New("no streaming software is chosen, so choose it in Settings")

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
	// knownScenes is the last list of scenes, which the show and hide
	// steps use to point out a source that's gone. It's refreshed when
	// the program connects and when the picker lists scenes, and dropped
	// when the program disconnects.
	knownScenes *scenes.KnownScenes
	logger      *slog.Logger

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
	logger := connectionConfig.Logger
	if logger == nil {
		logger = slog.Default()
	}
	service := &StreamingSoftwareService{settings: settings, onStateChange: onStateChange, knownScenes: &scenes.KnownScenes{}, logger: logger}
	connectionConfig.OnStatusChange = service.handleConnectionStatusChange
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

// ListScenes returns every scene in the chosen program, for the source
// picker, and remembers them for pointing out sources that are gone.
func (service *StreamingSoftwareService) ListScenes(ctx context.Context) ([]scenes.Scene, error) {
	chosen, isChosen := service.program().Chosen()
	if !isChosen {
		return nil, errNoStreamingSoftwareChosen
	}
	sceneList, err := chosen.Connection.Scenes(ctx)
	if err != nil {
		return nil, err
	}
	service.knownScenes.Remember(sceneList)
	return sceneList, nil
}

// program returns the chosen program as the show and hide steps see it.
func (service *StreamingSoftwareService) program() scenes.Program {
	return chosenStreamingSoftware{service: service}
}

// handleConnectionStatusChange refreshes the known scenes when Streamlabs
// connects and drops them when it disconnects, then tells the UI.
func (service *StreamingSoftwareService) handleConnectionStatusChange(status streamlabs.ConnectionStatus) {
	if status == streamlabs.ConnectionStatusConnected {
		// The connection reports its status from its own goroutine, which
		// shouldn't wait for a request.
		go service.rememberScenes()
	} else {
		service.knownScenes.Forget()
	}
	service.reportStatusChange()
}

// rememberScenes asks for every scene, so sources that are gone can be
// pointed out straight away. Failing only means none are pointed out yet.
func (service *StreamingSoftwareService) rememberScenes() {
	service.choiceMutex.Lock()
	ctx := service.applicationCtx
	service.choiceMutex.Unlock()
	if ctx == nil {
		return
	}
	_, err := service.ListScenes(ctx)
	if err != nil {
		service.logger.Warn("couldn't list the scenes after connecting to the streaming software", "error", err)
		return
	}
	// The connection may have dropped while the list loaded, and its
	// scenes are old news once it has.
	if service.connection.Status() != streamlabs.ConnectionStatusConnected {
		service.knownScenes.Forget()
	}
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
	service.knownScenes.Forget()
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

// chosenStreamingSoftware is the program the streamer chose in Settings, as
// the show and hide steps see it.
type chosenStreamingSoftware struct {
	service *StreamingSoftwareService
}

func (chosen chosenStreamingSoftware) Chosen() (scenes.ChosenProgram, bool) {
	if chosen.service.settings.streamingSoftware() != appsettings.StreamingSoftwareStreamlabsDesktop {
		return scenes.ChosenProgram{}, false
	}
	return scenes.ChosenProgram{Name: streamlabs.ProgramName, Connection: streamlabs.NewProgram(chosen.service.connection)}, true
}
