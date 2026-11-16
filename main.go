package main

import (
	"embed"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/coresteps"
	"github.com/SloanGwaltney/whendo/engine/credentials"
	"github.com/SloanGwaltney/whendo/engine/runhistory"
	"github.com/SloanGwaltney/whendo/engine/runner"
	"github.com/SloanGwaltney/whendo/engine/streamlabs"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Registering the event lets the binding generator give the frontend a
	// typed API for it.
	application.RegisterEvent[twitchauth.LoginState](twitchLoginStateChangedEvent)
	application.RegisterEvent[twitcheventsub.ConnectionState](twitchConnectionStateChangedEvent)
	application.RegisterEvent[runner.RunRecord](runEndedEvent)
	application.RegisterEvent[StreamingSoftwareState](streamingSoftwareStateChangedEvent)
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// then runs the application and logs any error that might occur.
func main() {
	// The session and the listener report state changes from the moment the
	// app starts, so they need the app, which is created below. The same goes
	// for the window, which a second launch brings forward.
	var app *application.App
	var mainWindow *application.WebviewWindow
	twitchHTTPClient := &http.Client{Timeout: twitchRequestTimeout}
	twitchSession := twitchauth.NewSession(twitchauth.SessionConfig{
		Client:          twitchauth.NewClient(twitchClientID(), twitchauth.DefaultIdentityBaseURL, twitchHTTPClient),
		CredentialStore: credentials.NewKeyringStore(credentialServiceName),
	})
	twitchSession.AddStateListener(func(state twitchauth.LoginState) {
		app.Event.Emit(twitchLoginStateChangedEvent, state)
	})
	folder, err := dataFolder()
	if err != nil {
		log.Fatal(err)
	}
	runHistoryStore := runhistory.NewStore(filepath.Join(folder, runHistoryFolderName), slog.Default())
	// Settings are read before any service starts, so the first prune
	// already keeps run history for as long as the user chose.
	settingsService := newSettingsService(appsettings.NewStore(filepath.Join(folder, settingsFileName)), runHistoryStore, slog.Default())
	twitchClient := twitchapi.NewClient(twitchapi.Config{
		ClientID:   twitchClientID(),
		Account:    twitchSession,
		HTTPClient: twitchHTTPClient,
	})
	// Both services share what the last list of redeems said, so automations
	// can be checked for redeems deleted in Twitch.
	knownRewards := &knownRewards{}
	automationsService, err := newAutomationsServiceForApp(folder, runHistoryStore, settingsService.runHistoryRetentionDays, knownRewards, twitchSession, twitchClient)
	if err != nil {
		log.Fatal(err)
	}
	twitchListener := twitcheventsub.NewListener(twitcheventsub.ListenerConfig{
		ClientID: twitchClientID(),
		Session:  twitchSession,
		Receiver: automationsService.runner,
		OnStateChange: func(state twitcheventsub.ConnectionState) {
			app.Event.Emit(twitchConnectionStateChangedEvent, state)
		},
		HTTPClient: twitchHTTPClient,
	})
	streamingSoftwareService := newStreamingSoftwareService(settingsService, streamlabs.Config{Logger: slog.Default()}, func(state StreamingSoftwareState) {
		app.Event.Emit(streamingSoftwareStateChangedEvent, state)
	})

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	app = application.New(application.Options{
		Name:        "whendo",
		Description: "A demo of using raw HTML & CSS",
		Services: []application.Service{
			// The login service is listed first so the stored login is
			// restored before the events service starts.
			application.NewService(&TwitchLoginService{session: twitchSession}),
			// Automations are loaded before events start arriving, so the
			// first events after startup aren't missed.
			application.NewService(automationsService),
			application.NewService(&TwitchEventsService{listener: twitchListener}),
			application.NewService(settingsService),
			application.NewService(streamingSoftwareService),
			application.NewService(&RewardsService{
				source:            newRewardSource(twitchClient),
				known:             knownRewards,
				switchAutomations: automationsService.switchReward,
				accountLogin:      func() string { return twitchSession.State().AccountLogin },
			}),
		},
		// Two copies would each run every automation and use up Twitch's
		// connection limit, so a second launch brings the first forward.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: singleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				mainWindow.UnMinimise()
				mainWindow.Show()
				mainWindow.Focus()
			},
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  windowTitle,
		Width:  defaultWindowWidth,
		Height: defaultWindowHeight,
		// The designer is laid out for at least an HD monitor; below the
		// minimum it would crowd too much. See docs/design/mvp-ui.md.
		MinWidth:  minimumWindowWidth,
		MinHeight: minimumWindowHeight,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		// Matches the default theme's background, so the window doesn't
		// flash a different colour before the page loads.
		BackgroundColour: application.NewRGB(12, 13, 18),
		URL:              "/",
	})

	// Run the application. This blocks until the application has been exited.
	err = app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}

// newAppCatalog returns every trigger and step Whendo offers. The refund and
// fulfill steps check redeems against the last list that loaded.
func newAppCatalog(twitchClient *twitchapi.Client, knownRewards *knownRewards) (*automation.Catalog, error) {
	steps := append(coresteps.Steps(), twitchsteps.Steps(twitchClient, knownRewards)...)
	catalog, err := automation.NewCatalog(twitcheventsub.Triggers(), steps)
	if err != nil {
		return nil, fmt.Errorf("building the catalog of triggers and steps: %w", err)
	}
	return catalog, nil
}

// newAutomationsServiceForApp builds the automation engine with every
// trigger and step Whendo offers, storing automations in Whendo's data
// folder.
func newAutomationsServiceForApp(folder string, runHistoryStore *runhistory.Store, runHistoryRetentionDays func() int, knownRewards *knownRewards, twitchSession *twitchauth.Session, twitchClient *twitchapi.Client) (*AutomationsService, error) {
	catalog, err := newAppCatalog(twitchClient, knownRewards)
	if err != nil {
		return nil, err
	}
	return newAutomationsService(
		catalog,
		automation.NewStore(filepath.Join(folder, automationsFolderName)),
		runHistoryStore,
		runHistoryRetentionDays,
		knownRewards,
		func() (trigger.Streamer, error) { return streamerFromSession(twitchSession) },
		slog.Default(),
	), nil
}

// streamerFromSession gives the logged-in Twitch account as the streamer
// test runs act as. The login only knows the account's login name, which is
// its display name apart from capitalization, so it stands in for both.
func streamerFromSession(session *twitchauth.Session) (trigger.Streamer, error) {
	userID, err := session.AccountUserID()
	if err != nil {
		return trigger.Streamer{}, err
	}
	login := session.State().AccountLogin
	return trigger.Streamer{UserID: userID, Login: login, DisplayName: login}, nil
}
