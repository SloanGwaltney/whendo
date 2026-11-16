package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// defaultTwitchClientID identifies Whendo's official Twitch application,
// which every release build uses. It's a public client, so the ID isn't a
// secret and there's no client secret.
const defaultTwitchClientID = "b9sfy6khyzesqkcyjmykaylirq4h1o"

// twitchClientIDEnvironmentVariable lets a developer log in through their
// own Twitch application instead of the official one.
const twitchClientIDEnvironmentVariable = "WHENDO_TWITCH_CLIENT_ID"

// twitchRequestTimeout stops a request to Twitch that hangs from blocking a
// login or refresh forever.
const twitchRequestTimeout = 30 * time.Second

// singleInstanceID identifies Whendo to the operating system so a second
// launch can find the running copy. It matches the product identifier in
// build/config.yml.
const singleInstanceID = "io.github.sloangwaltney.whendo"

// The main window's title and sizes, in pixels.
const (
	windowTitle         = "Whendo"
	defaultWindowWidth  = 1280
	defaultWindowHeight = 800
	minimumWindowWidth  = 1024
	minimumWindowHeight = 640
)

// credentialServiceName groups Whendo's secrets in the operating system's
// credential manager.
const credentialServiceName = "Whendo"

// twitchClientID returns the Twitch application to log in through: the one
// named by WHENDO_TWITCH_CLIENT_ID if it's set, otherwise the official one.
func twitchClientID() string {
	overrideClientID := os.Getenv(twitchClientIDEnvironmentVariable)
	if overrideClientID == "" {
		return defaultTwitchClientID
	}
	return overrideClientID
}

// Whendo keeps its files in the operating system's per-user config folder,
// which is %AppData% on Windows.
const (
	dataFolderName        = "whendo"
	automationsFolderName = "automations"
	runHistoryFolderName  = "run-history"
)

// dataFolder returns the folder Whendo keeps its files in.
func dataFolder() (string, error) {
	userConfigFolder, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding the user config folder: %w", err)
	}
	return filepath.Join(userConfigFolder, dataFolderName), nil
}
