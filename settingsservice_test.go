package main

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/appsettings"
	"github.com/SloanGwaltney/whendo/engine/runhistory"
	"github.com/SloanGwaltney/whendo/engine/runner"
)

type settingsFixture struct {
	service         *SettingsService
	settingsStore   *appsettings.Store
	runHistoryStore *runhistory.Store
}

func newSettingsFixture(t *testing.T) settingsFixture {
	t.Helper()
	folder := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	settingsStore := appsettings.NewStore(filepath.Join(folder, settingsFileName))
	runHistoryStore := runhistory.NewStore(filepath.Join(folder, runHistoryFolderName), logger)
	return settingsFixture{
		service:         newSettingsService(settingsStore, runHistoryStore, logger),
		settingsStore:   settingsStore,
		runHistoryStore: runHistoryStore,
	}
}

func (fixture settingsFixture) writeRunEndingDaysAgo(t *testing.T, runID string, daysAgo int) {
	t.Helper()
	err := fixture.runHistoryStore.Write(runner.RunRecord{RunID: runID, Status: runner.RunStatusSucceeded, EndedAt: time.Now().AddDate(0, 0, -daysAgo)})
	if err != nil {
		t.Fatalf("Write returned an error: %v", err)
	}
}

func TestSetRunHistoryRetentionDays_SavesAndDeletesRunsNowTooOld(t *testing.T) {
	fixture := newSettingsFixture(t)
	fixture.writeRunEndingDaysAgo(t, "recent", 2)
	fixture.writeRunEndingDaysAgo(t, "older", 10)

	err := fixture.service.SetRunHistoryRetentionDays(7)

	if err != nil {
		t.Fatalf("SetRunHistoryRetentionDays returned an error: %v", err)
	}
	if retentionDays := fixture.service.GetSettings().RunHistoryRetentionDays; retentionDays != 7 {
		t.Errorf("got %d days in the current settings, want 7", retentionDays)
	}
	savedSettings, err := fixture.settingsStore.Load()
	if err != nil || savedSettings.RunHistoryRetentionDays != 7 {
		t.Errorf("got saved settings %+v, error %v; want 7 days", savedSettings, err)
	}
	days, err := fixture.runHistoryStore.Days()
	wantDays := []string{time.Now().AddDate(0, 0, -2).Format(runhistory.DayFormat)}
	if err != nil || !slices.Equal(days, wantDays) {
		t.Errorf("got days %v, error %v; want only %v", days, err, wantDays)
	}
}

func TestSetRunHistoryRetentionDays_RefusesOutOfRangeAndKeepsCurrentSetting(t *testing.T) {
	fixture := newSettingsFixture(t)
	fixture.writeRunEndingDaysAgo(t, "older", 10)

	err := fixture.service.SetRunHistoryRetentionDays(0)

	if !errors.Is(err, appsettings.ErrRetentionOutOfRange) {
		t.Errorf("got error %v, want ErrRetentionOutOfRange", err)
	}
	if fixture.service.GetSettings() != appsettings.Defaults() {
		t.Errorf("got settings %+v, want the defaults kept", fixture.service.GetSettings())
	}
	if days, _ := fixture.runHistoryStore.Days(); len(days) != 1 {
		t.Errorf("got days %v, want the run kept", days)
	}
}
