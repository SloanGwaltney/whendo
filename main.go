package main

import (
	"embed"
	"log"
	"log/slog"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/credentials"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
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
	twitchListener := twitcheventsub.NewListener(twitcheventsub.ListenerConfig{
		ClientID: twitchClientID(),
		Session:  twitchSession,
		Receiver: &loggingEventReceiver{logger: slog.Default()},
		OnStateChange: func(state twitcheventsub.ConnectionState) {
			app.Event.Emit(twitchConnectionStateChangedEvent, state)
		},
		HTTPClient: twitchHTTPClient,
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
			application.NewService(&TwitchEventsService{listener: twitchListener}),
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

	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Window 1",
		// Window sized to the golden ratio (1000 / 618 ≈ 1.618).
		Width:  1000,
		Height: 618,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	// Run the application. This blocks until the application has been exited.
	err := app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
