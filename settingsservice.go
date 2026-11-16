package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
	"github.com/SloanGwaltney/whendo/engine/runhistory"
)

// RetentionLimits are the run history retention periods the user can
// choose, in days, so the Settings page can say so.
type RetentionLimits struct {
	MinimumDays int `json:"minimumDays"`
	MaximumDays int `json:"maximumDays"`
}

// SettingsService connects the Settings page to the saved settings, and
// applies a change as soon as it's saved.
type SettingsService struct {
	settingsStore   *appsettings.Store
	runHistoryStore *runhistory.Store
	logger          *slog.Logger

	// mutex guards settings, which pruning reads on its own goroutine.
	mutex    sync.Mutex
	settings appsettings.Settings
}

// newSettingsService loads the saved settings. Settings that can't be read
// are logged and replaced by the defaults, rather than stopping Whendo from
// starting; the file is left alone until the user saves a change.
func newSettingsService(settingsStore *appsettings.Store, runHistoryStore *runhistory.Store, logger *slog.Logger) *SettingsService {
	settings, err := settingsStore.Load()
	if err != nil {
		logger.Error("couldn't read the settings, so the defaults are used", "error", err)
		settings = appsettings.Defaults()
	}
	return &SettingsService{settingsStore: settingsStore, runHistoryStore: runHistoryStore, logger: logger, settings: settings}
}

// GetSettings returns the current settings.
func (service *SettingsService) GetSettings() appsettings.Settings {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.settings
}

// RunHistoryRetentionLimits returns the retention periods the user can
// choose.
func (service *SettingsService) RunHistoryRetentionLimits() RetentionLimits {
	return RetentionLimits{
		MinimumDays: appsettings.MinimumRunHistoryRetentionDays,
		MaximumDays: appsettings.MaximumRunHistoryRetentionDays,
	}
}

// SetRunHistoryRetentionDays saves how long run history is kept, and deletes
// what's now too old straight away, so the change takes effect without
// waiting for the daily prune.
func (service *SettingsService) SetRunHistoryRetentionDays(retentionDays int) error {
	service.mutex.Lock()
	changedSettings := service.settings
	changedSettings.RunHistoryRetentionDays = retentionDays
	err := service.settingsStore.Save(changedSettings)
	if err == nil {
		service.settings = changedSettings
	}
	service.mutex.Unlock()
	if err != nil {
		return fmt.Errorf("saving how long run history is kept: %w", err)
	}
	err = service.runHistoryStore.Prune(retentionDays, time.Now())
	if err != nil {
		return fmt.Errorf("saved, but couldn't delete older run history: %w", err)
	}
	return nil
}

// runHistoryRetentionDays is how long run history is kept, for pruning.
func (service *SettingsService) runHistoryRetentionDays() int {
	return service.GetSettings().RunHistoryRetentionDays
}

// streamingSoftware is the program the streamer chose, for connecting to it.
func (service *SettingsService) streamingSoftware() appsettings.StreamingSoftware {
	return service.GetSettings().StreamingSoftware
}

// setStreamingSoftware saves the program the streamer chose. Connecting to
// it is up to the caller.
func (service *SettingsService) setStreamingSoftware(software appsettings.StreamingSoftware) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	changedSettings := service.settings
	changedSettings.StreamingSoftware = software
	err := service.settingsStore.Save(changedSettings)
	if err != nil {
		return fmt.Errorf("saving your streaming software: %w", err)
	}
	service.settings = changedSettings
	return nil
}
