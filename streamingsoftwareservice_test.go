package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
	"github.com/SloanGwaltney/whendo/engine/streamlabs"
)

const (
	streamingSoftwareWaitTimeout  = 5 * time.Second
	streamingSoftwarePollInterval = 5 * time.Millisecond
	testStreamlabsRetryDelay      = 20 * time.Millisecond
	testStreamlabsRequestTimeout  = 300 * time.Millisecond
)

// fakeStreamlabsServer answers every request with an empty result, which
// is enough for the connection to count as connected. It counts the
// clients that are connected right now.
type fakeStreamlabsServer struct {
	listener         net.Listener
	connectedClients atomic.Int32
	waitGroup        sync.WaitGroup
}

func startFakeStreamlabsServer(t *testing.T) *fakeStreamlabsServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("starting the fake Streamlabs: %v", err)
	}
	server := &fakeStreamlabsServer{listener: listener}
	server.waitGroup.Go(func() { server.acceptClients(t) })
	t.Cleanup(func() {
		closeErr := listener.Close()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			t.Errorf("stopping the fake Streamlabs: %v", closeErr)
		}
		server.waitGroup.Wait()
	})
	return server
}

func (server *fakeStreamlabsServer) acceptClients(t *testing.T) {
	for {
		client, err := server.listener.Accept()
		if err != nil {
			return
		}
		server.waitGroup.Go(func() { server.answerClient(t, client) })
	}
}

func (server *fakeStreamlabsServer) answerClient(t *testing.T, client net.Conn) {
	server.connectedClients.Add(1)
	defer server.connectedClients.Add(-1)
	defer func() {
		closeErr := client.Close()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			t.Errorf("closing a client: %v", closeErr)
		}
	}()
	reader := bufio.NewReader(client)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request struct {
			ID int64 `json:"id"`
		}
		err = json.Unmarshal(line, &request)
		if err != nil {
			t.Errorf("the fake Streamlabs got a line that isn't JSON: %q", line)
			return
		}
		_, err = fmt.Fprintf(client, "{\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":\"scene_1\"}\n", request.ID)
		if err != nil {
			return
		}
	}
}

type streamingSoftwareFixture struct {
	service       *StreamingSoftwareService
	settingsStore *appsettings.Store
	stateChanges  chan StreamingSoftwareState
}

func newStreamingSoftwareFixture(t *testing.T, server *fakeStreamlabsServer) streamingSoftwareFixture {
	t.Helper()
	settings := newSettingsFixture(t)
	return newStreamingSoftwareFixtureWithSettings(t, server, settings)
}

func newStreamingSoftwareFixtureWithSettings(t *testing.T, server *fakeStreamlabsServer, settings settingsFixture) streamingSoftwareFixture {
	t.Helper()
	stateChanges := make(chan StreamingSoftwareState, 64)
	service := newStreamingSoftwareService(settings.service, streamlabs.Config{
		Address:        server.listener.Addr().String(),
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		RetryDelay:     testStreamlabsRetryDelay,
		RequestTimeout: testStreamlabsRequestTimeout,
	}, func(state StreamingSoftwareState) {
		// A full buffer only means the test has stopped reading changes,
		// so dropping one is fine.
		select {
		case stateChanges <- state:
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	err := service.ServiceStartup(ctx, application.ServiceOptions{})
	if err != nil {
		t.Fatalf("ServiceStartup: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		shutdownErr := service.ServiceShutdown()
		if shutdownErr != nil {
			t.Errorf("ServiceShutdown: %v", shutdownErr)
		}
	})
	return streamingSoftwareFixture{service: service, settingsStore: settings.settingsStore, stateChanges: stateChanges}
}

func waitUntil(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(streamingSoftwareWaitTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", description)
		}
		time.Sleep(streamingSoftwarePollInterval)
	}
}

func TestStreamingSoftwareService_StartsWithNothingChosenAndDoesNotConnect(t *testing.T) {
	server := startFakeStreamlabsServer(t)
	fixture := newStreamingSoftwareFixture(t, server)

	time.Sleep(5 * testStreamlabsRetryDelay)

	state := fixture.service.GetState()
	if state.Software != appsettings.StreamingSoftwareNone || state.Status != streamlabs.ConnectionStatusWaiting {
		t.Errorf("state = %+v, want none and waiting", state)
	}
	if server.connectedClients.Load() != 0 {
		t.Errorf("%d clients connected, want none", server.connectedClients.Load())
	}
}

func TestChooseStreamingSoftware_ConnectsToStreamlabsAndSavesTheChoice(t *testing.T) {
	server := startFakeStreamlabsServer(t)
	fixture := newStreamingSoftwareFixture(t, server)

	err := fixture.service.ChooseStreamingSoftware(appsettings.StreamingSoftwareStreamlabsDesktop)

	if err != nil {
		t.Fatalf("ChooseStreamingSoftware: %v", err)
	}
	waitUntil(t, "Streamlabs is connected", func() bool {
		return fixture.service.GetState().Status == streamlabs.ConnectionStatusConnected
	})
	savedSettings, err := fixture.settingsStore.Load()
	if err != nil || savedSettings.StreamingSoftware != appsettings.StreamingSoftwareStreamlabsDesktop {
		t.Errorf("saved settings = %+v, error %v; want Streamlabs Desktop", savedSettings, err)
	}
	waitUntil(t, "the UI hears that Streamlabs is connected", func() bool {
		select {
		case state := <-fixture.stateChanges:
			return state.Software == appsettings.StreamingSoftwareStreamlabsDesktop && state.Status == streamlabs.ConnectionStatusConnected
		default:
			return false
		}
	})
}

func TestChooseStreamingSoftware_DisconnectsWhenNoneIsChosen(t *testing.T) {
	server := startFakeStreamlabsServer(t)
	fixture := newStreamingSoftwareFixture(t, server)
	err := fixture.service.ChooseStreamingSoftware(appsettings.StreamingSoftwareStreamlabsDesktop)
	if err != nil {
		t.Fatalf("choosing Streamlabs: %v", err)
	}
	waitUntil(t, "Streamlabs is connected", func() bool {
		return fixture.service.GetState().Status == streamlabs.ConnectionStatusConnected
	})

	err = fixture.service.ChooseStreamingSoftware(appsettings.StreamingSoftwareNone)

	if err != nil {
		t.Fatalf("choosing none: %v", err)
	}
	state := fixture.service.GetState()
	if state.Software != appsettings.StreamingSoftwareNone || state.Status != streamlabs.ConnectionStatusWaiting {
		t.Errorf("state = %+v, want none and waiting", state)
	}
	waitUntil(t, "the fake Streamlabs sees the client leave", func() bool {
		return server.connectedClients.Load() == 0
	})
}

func TestStreamingSoftwareService_ConnectsAtStartupToThePreviousChoice(t *testing.T) {
	server := startFakeStreamlabsServer(t)
	settings := newSettingsFixture(t)
	err := settings.service.setStreamingSoftware(appsettings.StreamingSoftwareStreamlabsDesktop)
	if err != nil {
		t.Fatalf("saving the choice: %v", err)
	}

	fixture := newStreamingSoftwareFixtureWithSettings(t, server, settings)

	waitUntil(t, "Streamlabs is connected", func() bool {
		return fixture.service.GetState().Status == streamlabs.ConnectionStatusConnected
	})
}

func TestChooseStreamingSoftware_RefusesUnknownSoftware(t *testing.T) {
	server := startFakeStreamlabsServer(t)
	fixture := newStreamingSoftwareFixture(t, server)

	err := fixture.service.ChooseStreamingSoftware("someOtherProgram")

	if !errors.Is(err, appsettings.ErrUnknownStreamingSoftware) {
		t.Fatalf("error = %v, want ErrUnknownStreamingSoftware", err)
	}
	if software := fixture.service.GetState().Software; software != appsettings.StreamingSoftwareNone {
		t.Errorf("software = %q after a refused choice, want none", software)
	}
}
