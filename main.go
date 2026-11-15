package main

import (
	"embed"
	"log"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/credentials"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
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
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// then runs the application and logs any error that might occur.
func main() {
	// The session reports state changes from the moment the app starts, so it
	// needs the app, which is created below.
	var app *application.App
	twitchSession := twitchauth.NewSession(twitchauth.SessionConfig{
		Client:          twitchauth.NewClient(twitchClientID(), twitchauth.DefaultIdentityBaseURL, &http.Client{Timeout: twitchRequestTimeout}),
		CredentialStore: credentials.NewKeyringStore(credentialServiceName),
	})
	twitchSession.AddStateListener(func(state twitchauth.LoginState) {
		app.Event.Emit(twitchLoginStateChangedEvent, state)
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
			application.NewService(&TwitchLoginService{session: twitchSession}),
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
	app.Window.NewWithOptions(application.WebviewWindowOptions{
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
