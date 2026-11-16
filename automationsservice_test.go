package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/coresteps"
	"github.com/SloanGwaltney/whendo/engine/runhistory"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

const waitMilliseconds = 10

var testStreamer = trigger.Streamer{UserID: "1234", Login: "teststreamer", DisplayName: "teststreamer"}

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
	service := newAutomationsService(catalog, automation.NewStore(automationFolder), runhistory.NewStore(filepath.Join(folder, runHistoryFolderName), logger), func() int { return runhistory.DefaultRetentionDays }, func() (trigger.Streamer, error) { return testStreamer, nil }, logger)
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
	if summaries["invalid"].IsUnreadable || !summaries["broken"].IsUnreadable {
		t.Errorf("got %+v and %+v, want only the file that couldn't be read marked unreadable", summaries["invalid"], summaries["broken"])
	}
}

func (fixture *serviceFixture) save(savedAutomation automation.Automation) SaveResult {
	fixture.t.Helper()
	result, err := fixture.service.SaveAutomation(savedAutomation)
	if err != nil {
		fixture.t.Fatalf("SaveAutomation returned an error: %v", err)
	}
	return result
}

func (fixture *serviceFixture) loadFromFolder(automationID string) automation.Automation {
	fixture.t.Helper()
	loaded, _, err := automation.NewStore(fixture.automationFolder).LoadAll()
	if err != nil {
		fixture.t.Fatalf("LoadAll returned an error: %v", err)
	}
	for _, loadedAutomation := range loaded {
		if loadedAutomation.ID == automationID {
			return loadedAutomation
		}
	}
	fixture.t.Fatalf("automation %q isn't in the folder", automationID)
	return automation.Automation{}
}

func brokenChatAutomation(t *testing.T, automationID string) automation.Automation {
	t.Helper()
	broken := chatAutomation(t, automationID, "Broken")
	broken.Trigger.Settings = automationsetting.Values{}
	return broken
}

func TestAutomationsService_SavesBrokenAutomationTurnedOff(t *testing.T) {
	fixture := newServiceFixture(t, nil)

	result := fixture.save(brokenChatAutomation(t, "broken"))

	if !result.IsTurnedOffUntilFixed {
		t.Error("got IsTurnedOffUntilFixed false, want true for an automation that was on")
	}
	if fixture.loadFromFolder("broken").IsEnabled {
		t.Error("the broken automation was saved turned on")
	}
	summary := summariesByID(fixture.service.ListAutomations())["broken"]
	if summary.IsEnabled || summary.Problem == "" {
		t.Errorf("got summary %+v, want it off with its problem", summary)
	}
}

func TestAutomationsService_SavingBrokenAutomationThatWasOffDoesntSayItTurnedOff(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	broken := brokenChatAutomation(t, "broken")
	broken.IsEnabled = false

	result := fixture.save(broken)

	if result.IsTurnedOffUntilFixed {
		t.Error("got IsTurnedOffUntilFixed true for an automation that was already off")
	}
}

func TestAutomationsService_RefusesToTurnOnBrokenAutomation(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(brokenChatAutomation(t, "broken"))

	turnOnErr := fixture.service.SetAutomationEnabled("broken", true)
	turnOffErr := fixture.service.SetAutomationEnabled("broken", false)

	if !errors.Is(turnOnErr, errNeedsFixing) || turnOffErr != nil {
		t.Errorf("got errors %v and %v, want only turning on refused", turnOnErr, turnOffErr)
	}
	if fixture.loadFromFolder("broken").IsEnabled {
		t.Error("the broken automation was turned on")
	}
}

func TestAutomationsService_RefusesToTestRunAutomationThatWasValidThenBroken(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(chatAutomation(t, "edited", "Edited"))
	fixture.save(brokenChatAutomation(t, "edited"))

	_, err := fixture.service.TestRun("edited", map[string]string{twitcheventsub.MessageTextVariable: "!test"})

	if !errors.Is(err, errNeedsFixing) {
		t.Errorf("got error %v, want errNeedsFixing", err)
	}
}

func TestAutomationsService_FixingBrokenAutomationPutsItToWork(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(brokenChatAutomation(t, "fixed"))

	result := fixture.save(chatAutomation(t, "fixed", "Fixed"))

	summary := summariesByID(fixture.service.ListAutomations())["fixed"]
	if result.IsTurnedOffUntilFixed || summary.Problem != "" || !summary.IsEnabled {
		t.Errorf("got result %+v and summary %+v, want it on with no problem", result, summary)
	}
	if err := fixture.service.SetAutomationEnabled("fixed", true); err != nil {
		t.Errorf("SetAutomationEnabled returned an error: %v", err)
	}
}

func TestAutomationsService_NeverSavesAutomationWithUnsafeID(t *testing.T) {
	fixture := newServiceFixture(t, nil)

	_, err := fixture.service.SaveAutomation(chatAutomation(t, "../outside", "Outside"))

	if err == nil {
		t.Fatal("got no error, want the unsafe ID refused")
	}
	if _, statErr := os.Stat(filepath.Join(fixture.automationFolder, "..", "outside.json")); !os.IsNotExist(statErr) {
		t.Error("a file was written outside the automations folder")
	}
}

func TestAutomationsService_CheckAutomationReportsProblemsWithoutSaving(t *testing.T) {
	fixture := newServiceFixture(t, nil)

	report := fixture.service.CheckAutomation(brokenChatAutomation(t, "draft"))

	if !report.HasMustFixProblems() {
		t.Errorf("got report %+v, want a must-fix problem", report)
	}
	if _, isListed := summariesByID(fixture.service.ListAutomations())["draft"]; isListed {
		t.Error("checking a draft saved it")
	}
}

func TestAutomationsService_CatalogListsTriggersAndSteps(t *testing.T) {
	fixture := newServiceFixture(t, nil)

	catalog := fixture.service.Catalog()

	if len(catalog.Triggers) != len(twitcheventsub.Triggers()) || len(catalog.Steps) != len(coresteps.Steps()) {
		t.Errorf("got %d triggers and %d steps", len(catalog.Triggers), len(catalog.Steps))
	}
}

func TestNewAppCatalog_AcceptsEveryTriggerAndStepDefinition(t *testing.T) {
	// Building the catalog checks every definition the designer will show,
	// such as each sentence placing each setting once.
	_, err := newAppCatalog(twitchsteps.Config{})

	if err != nil {
		t.Errorf("newAppCatalog returned an error: %v", err)
	}
}

func TestAutomationsService_SetAutomationEnabledSavesTheChange(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(chatAutomation(t, "toggled", "Toggled"))

	err := fixture.service.SetAutomationEnabled("toggled", false)

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

func TestAutomationsService_TestValuesComeFromTheTriggerAndStreamer(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(chatAutomation(t, "chat", "Chat"))

	testValues, err := fixture.service.TestValues("chat")

	if err != nil {
		t.Fatalf("TestValues returned an error: %v", err)
	}
	if testValues[twitcheventsub.MessageTextVariable] != "!test" || testValues[twitcheventsub.SenderLoginVariable] != testStreamer.Login {
		t.Errorf("got test values %v, want the start text sent by the streamer", testValues)
	}
}

func TestAutomationsService_TestRunExplainsValuesThatWouldNeverMatch(t *testing.T) {
	fixture := newServiceFixture(t, nil)
	fixture.save(chatAutomation(t, "chat", "Chat"))

	started, err := fixture.service.TestRun("chat", map[string]string{twitcheventsub.MessageTextVariable: "hello"})

	if err != nil {
		t.Fatalf("TestRun returned an error: %v", err)
	}
	if started.RunID != "" || !strings.Contains(started.MismatchReason, "“!test”") {
		t.Errorf("got %+v, want no run and a reason naming !test", started)
	}
}
