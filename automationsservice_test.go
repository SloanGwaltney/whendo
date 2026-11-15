package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/coresteps"
	"github.com/SloanGwaltney/whendo/engine/runhistory"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

const waitMilliseconds = 10

type serviceFixture struct {
	t                *testing.T
	service          *AutomationsService
	automationFolder string
}

// newServiceFixture starts a service over temporary folders. It uses the
// core steps only, so no test needs a Twitch login.
func newServiceFixture(t *testing.T, prepareFolder func(automationFolder string)) *serviceFixture {
	t.Helper()
	folder := t.TempDir()
	automationFolder := filepath.Join(folder, automationsFolderName)
	if prepareFolder != nil {
		prepareFolder(automationFolder)
	}
	catalog, err := automation.NewCatalog(twitcheventsub.Triggers(), coresteps.Steps())
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := newAutomationsService(catalog, automation.NewStore(automationFolder), runhistory.NewStore(filepath.Join(folder, runHistoryFolderName), logger), logger)
	ctx, cancel := context.WithCancel(context.Background())
	err = service.ServiceStartup(ctx, application.ServiceOptions{})
	if err != nil {
		t.Fatalf("ServiceStartup returned an error: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		err := service.ServiceShutdown()
		if err != nil {
			t.Errorf("ServiceShutdown returned an error: %v", err)
		}
	})
	return &serviceFixture{t: t, service: service, automationFolder: automationFolder}
}

func encodeSetting(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encodedValue, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %v: %v", value, err)
	}
	return encodedValue
}

func chatAutomation(t *testing.T, automationID string, name string) automation.Automation {
	t.Helper()
	return automation.Automation{
		ID:        automationID,
		Name:      name,
		IsEnabled: true,
		Trigger: automation.TriggerSelection{
			ID:       twitcheventsub.ChatMessageTriggerID,
			Settings: automationsetting.Values{twitcheventsub.StartsWithSetting: encodeSetting(t, "!test")},
		},
		Steps: []automation.Step{{
			ID:       "wait",
			Type:     coresteps.WaitStepID,
			Settings: automationsetting.Values{coresteps.WaitDurationSetting: encodeSetting(t, waitMilliseconds)},
		}},
	}
}

func saveToFolder(t *testing.T, folder string, savedAutomation automation.Automation) {
	t.Helper()
	err := automation.NewStore(folder).Save(savedAutomation)
	if err != nil {
		t.Fatalf("saving %q: %v", savedAutomation.ID, err)
	}
}

func summariesByID(summaries []AutomationSummary) map[string]AutomationSummary {
	byID := map[string]AutomationSummary{}
	for _, summary := range summaries {
		byID[summary.ID] = summary
	}
	return byID
}

func TestAutomationsService_ListsSavedAutomationsWithTheirProblems(t *testing.T) {
	fixture := newServiceFixture(t, func(automationFolder string) {
		saveToFolder(t, automationFolder, chatAutomation(t, "valid", "Valid"))
		invalid := chatAutomation(t, "invalid", "Invalid")
		invalid.Trigger.Settings = automationsetting.Values{}
		saveToFolder(t, automationFolder, invalid)
		err := os.WriteFile(filepath.Join(automationFolder, "broken.json"), []byte("{"), 0o600)
		if err != nil {
			t.Fatalf("writing broken.json: %v", err)
		}
	})

	summaries := summariesByID(fixture.service.ListAutomations())

	if len(summaries) != 3 {
		t.Fatalf("got %d automations, want 3: %v", len(summaries), summaries)
	}
	if summaries["valid"].Problem != "" {
		t.Errorf("valid automation has problem %q", summaries["valid"].Problem)
	}
	if summaries["invalid"].Problem == "" || summaries["broken"].Problem == "" {
		t.Errorf("got %+v and %+v, want both to explain their problem", summaries["invalid"], summaries["broken"])
	}
}

func TestAutomationsService_SaveRefusesInvalidAndKeepsValid(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	invalid := chatAutomation(t, "invalid", "Invalid")
	invalid.Name = ""

	invalidErr := fixture.service.SaveAutomation(invalid)
	validErr := fixture.service.SaveAutomation(chatAutomation(t, "saved", "Saved"))

	if invalidErr == nil || validErr != nil {
		t.Fatalf("got errors %v and %v, want only the invalid one refused", invalidErr, validErr)
	}
	if _, err := os.Stat(filepath.Join(fixture.automationFolder, "invalid.json")); !os.IsNotExist(err) {
		t.Error("the invalid automation was written to disk")
	}
	saved, err := fixture.service.GetAutomation("saved")
	if err != nil || saved.Name != "Saved" {
		t.Errorf("got %+v, error %v", saved, err)
	}
}

func TestAutomationsService_SetAutomationEnabledSavesTheChange(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	err := fixture.service.SaveAutomation(chatAutomation(t, "toggled", "Toggled"))
	if err != nil {
		t.Fatalf("SaveAutomation returned an error: %v", err)
	}

	err = fixture.service.SetAutomationEnabled("toggled", false)

	if err != nil {
		t.Fatalf("SetAutomationEnabled returned an error: %v", err)
	}
	loaded, _, err := automation.NewStore(fixture.automationFolder).LoadAll()
	if err != nil || len(loaded) != 1 || loaded[0].IsEnabled {
		t.Errorf("got %+v, error %v; want it saved as disabled", loaded, err)
	}
}

func TestAutomationsService_DeleteRemovesAutomationAndUnreadableFiles(t *testing.T) {
	fixture := newServiceFixture(t, func(automationFolder string) {
		saveToFolder(t, automationFolder, chatAutomation(t, "deleted", "Deleted"))
		err := os.WriteFile(filepath.Join(automationFolder, "broken.json"), []byte("{"), 0o600)
		if err != nil {
			t.Fatalf("writing broken.json: %v", err)
		}
	})

	deletedErr := fixture.service.DeleteAutomation("deleted")
	brokenErr := fixture.service.DeleteAutomation("broken")

	if deletedErr != nil || brokenErr != nil {
		t.Fatalf("got errors %v and %v", deletedErr, brokenErr)
	}
	if summaries := fixture.service.ListAutomations(); len(summaries) != 0 {
		t.Errorf("got %v, want nothing left", summaries)
	}
	entries, err := os.ReadDir(fixture.automationFolder)
	if err != nil || len(entries) != 0 {
		t.Errorf("got %d files, error %v; want an empty folder", len(entries), err)
	}
}
