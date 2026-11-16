package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/runhistory"
	"github.com/SloanGwaltney/whendo/engine/runner"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// runEndedEvent carries a runner.RunRecord each time a run ends, so run
// history can update live.
const runEndedEvent = "run-ended"

// automationFileExtension lets an unreadable file be listed, and deleted, by
// the ID its name stands for.
const automationFileExtension = ".json"

// errNeedsFixing means an automation with must-fix problems was asked to
// turn on or run. See docs/design/mvp-ui.md#problems.
var errNeedsFixing = errors.New("this automation needs its problems fixed first")

// CatalogDefinitions lists every trigger and step the designer can offer, in
// the order to list them.
type CatalogDefinitions struct {
	Triggers []trigger.Definition `json:"triggers"`
	Steps    []step.Definition    `json:"steps"`
}

// SaveResult tells the designer what saving did beyond saving.
type SaveResult struct {
	// IsTurnedOffUntilFixed means the automation was on, but has must-fix
	// problems, so it was saved turned off.
	IsTurnedOffUntilFixed bool `json:"isTurnedOffUntilFixed"`
}

// TestRunStarted tells the designer what became of a test run it asked for.
type TestRunStarted struct {
	// RunID identifies the run's record when it ends. It's empty when the
	// run was refused.
	RunID string `json:"runId"`
	// MismatchReason says why the values were refused, because a real event
	// with them would never start the automation. It's empty when the run
	// started.
	MismatchReason string `json:"mismatchReason"`
}

// generalMismatchReason is for triggers that can't explain a mismatch
// themselves.
const generalMismatchReason = "A real event with these values wouldn't start this automation, so the test didn't run."

// AutomationSummary is one row of the automation list. Problem explains why
// an automation isn't running, such as a setting that's missing or a file
// that couldn't be read; it's empty for automations that are fine.
// IsUnreadable means the automation's file couldn't be read, so it can only
// be deleted; its name is then the file's name.
type AutomationSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	IsEnabled    bool   `json:"isEnabled"`
	TriggerID    string `json:"triggerId"`
	Problem      string `json:"problem"`
	IsUnreadable bool   `json:"isUnreadable"`
}

// AutomationsService connects the automation engine to the UI. It loads
// saved automations into the runner at startup, saves changes, and records
// every run in run history.
type AutomationsService struct {
	catalog         *automation.Catalog
	runner          *runner.Runner
	automationStore *automation.Store
	runHistoryStore *runhistory.Store
	logger          *slog.Logger
	// currentStreamer gives the logged-in account, whose details test runs
	// start with.
	currentStreamer func() (trigger.Streamer, error)
	// runHistoryRetentionDays gives how long run history is kept, which the
	// user can change while Whendo runs.
	runHistoryRetentionDays func() int

	// mutex guards the maps. They hold every saved automation, including
	// invalid ones the runner refused, so the UI can list them with their
	// problems.
	mutex                 sync.Mutex
	automationsByID       map[string]automation.Automation
	problemsByID          map[string]string
	unreadableFileReasons map[string]string
}

func newAutomationsService(catalog *automation.Catalog, automationStore *automation.Store, runHistoryStore *runhistory.Store, runHistoryRetentionDays func() int, currentStreamer func() (trigger.Streamer, error), logger *slog.Logger) *AutomationsService {
	service := &AutomationsService{
		catalog:                 catalog,
		currentStreamer:         currentStreamer,
		runHistoryRetentionDays: runHistoryRetentionDays,
		automationStore:         automationStore,
		runHistoryStore:         runHistoryStore,
		logger:                  logger,
		automationsByID:         make(map[string]automation.Automation),
		problemsByID:            make(map[string]string),
		unreadableFileReasons:   make(map[string]string),
	}
	service.runner = runner.New(runner.Config{Catalog: catalog, OnRunEnded: service.recordRun, Logger: logger})
	return service
}

// ServiceStartup loads every saved automation into the runner and keeps run
// history pruned until Whendo exits.
func (service *AutomationsService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	automations, failures, err := service.automationStore.LoadAll()
	if err != nil {
		return fmt.Errorf("loading automations: %w", err)
	}
	service.mutex.Lock()
	for _, loadedAutomation := range automations {
		service.loadAutomation(loadedAutomation)
	}
	for _, failure := range failures {
		service.logger.Warn("an automation file couldn't be read", "file", failure.FileName, "error", failure.Reason)
		service.unreadableFileReasons[failure.FileName] = failure.Reason.Error()
	}
	service.mutex.Unlock()
	// Wails cancels ctx right before shutdown, which stops pruning.
	go service.runHistoryStore.KeepPruned(ctx, service.runHistoryRetentionDays)
	return nil
}

// loadAutomation puts a saved automation to work, unless it has must-fix
// problems. Those are listed with their first problem and never reach the
// runner.
func (service *AutomationsService) loadAutomation(loadedAutomation automation.Automation) {
	service.automationsByID[loadedAutomation.ID] = loadedAutomation
	problem, hasProblem := automation.Check(loadedAutomation, service.catalog).FirstMustFixProblem()
	if hasProblem {
		service.logger.Warn("an automation has problems and won't run", "automation", loadedAutomation.ID, "problem", problem.Message)
		service.problemsByID[loadedAutomation.ID] = problem.Message
		return
	}
	err := service.runner.SetAutomation(loadedAutomation)
	if err != nil {
		service.logger.Error("an automation couldn't be started", "automation", loadedAutomation.ID, "error", err)
		service.problemsByID[loadedAutomation.ID] = err.Error()
	}
}

// ServiceShutdown stops runs in progress and records them, and the queued
// runs, as cancelled.
func (service *AutomationsService) ServiceShutdown() error {
	service.runner.Shutdown()
	return nil
}

// recordRun writes a finished run to run history and tells the UI. A run
// that can't be written is logged rather than retried, since the run itself
// has already happened.
func (service *AutomationsService) recordRun(record runner.RunRecord) {
	err := service.runHistoryStore.Write(record)
	if err != nil {
		service.logger.Error("couldn't write a run to run history", "run", record.RunID, "automation", record.AutomationID, "error", err)
	}
	application.Get().Event.Emit(runEndedEvent, record)
}

// ListAutomations returns every saved automation, including ones with
// problems and files that couldn't be read, sorted by name.
func (service *AutomationsService) ListAutomations() []AutomationSummary {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	summaries := make([]AutomationSummary, 0, len(service.automationsByID)+len(service.unreadableFileReasons))
	for _, savedAutomation := range service.automationsByID {
		summaries = append(summaries, AutomationSummary{
			ID:        savedAutomation.ID,
			Name:      savedAutomation.Name,
			IsEnabled: savedAutomation.IsEnabled,
			TriggerID: savedAutomation.Trigger.ID,
			Problem:   service.problemsByID[savedAutomation.ID],
		})
	}
	for fileName, reason := range service.unreadableFileReasons {
		summaries = append(summaries, AutomationSummary{ID: strings.TrimSuffix(fileName, automationFileExtension), Name: fileName, Problem: reason, IsUnreadable: true})
	}
	slices.SortFunc(summaries, func(first AutomationSummary, second AutomationSummary) int {
		return cmp.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return summaries
}

// GetAutomation returns one saved automation.
func (service *AutomationsService) GetAutomation(automationID string) (automation.Automation, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	savedAutomation, isFound := service.automationsByID[automationID]
	if !isFound {
		return automation.Automation{}, fmt.Errorf("%w: %q", runner.ErrAutomationNotFound, automationID)
	}
	return savedAutomation, nil
}

// NewAutomationID returns an ID for a new automation.
func (service *AutomationsService) NewAutomationID() string {
	return automation.NewID()
}

// Catalog returns every trigger and step, for the designer to offer.
func (service *AutomationsService) Catalog() CatalogDefinitions {
	return CatalogDefinitions{
		Triggers: service.catalog.TriggerDefinitions(),
		Steps:    service.catalog.StepDefinitions(),
	}
}

// CheckAutomation checks a draft from the designer without saving it, and
// reports its problems and what each step can use. The designer calls it as
// the user edits, so the rules live only in the engine.
func (service *AutomationsService) CheckAutomation(draft automation.Automation) automation.Report {
	return automation.Check(draft, service.catalog)
}

// SaveAutomation saves a new or changed automation. A valid one is put to
// work right away. One with must-fix problems is still saved, so no work in
// the designer is lost, but it's saved turned off and stays off until it's
// fixed. See docs/design/automation-engine.md#invalid-automations.
func (service *AutomationsService) SaveAutomation(changedAutomation automation.Automation) (SaveResult, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	problem, hasProblem := automation.Check(changedAutomation, service.catalog).FirstMustFixProblem()
	if hasProblem {
		return service.saveBrokenAutomation(changedAutomation, problem.Message)
	}
	return SaveResult{}, service.saveValidAutomation(changedAutomation)
}

func (service *AutomationsService) saveValidAutomation(changedAutomation automation.Automation) error {
	err := service.automationStore.Save(changedAutomation)
	if err != nil {
		return err
	}
	err = service.runner.SetAutomation(changedAutomation)
	if err != nil {
		return fmt.Errorf("saved automation %q, but couldn't start it: %w", changedAutomation.ID, err)
	}
	service.automationsByID[changedAutomation.ID] = changedAutomation
	delete(service.problemsByID, changedAutomation.ID)
	return nil
}

// saveBrokenAutomation saves an automation turned off. If the version before
// it was valid, the runner keeps that version, turned off, rather than
// dropping it: that cancels its queued runs and lets a run in progress
// finish, so a fixed version saved later can't start a second run alongside
// it.
func (service *AutomationsService) saveBrokenAutomation(brokenAutomation automation.Automation, problemMessage string) (SaveResult, error) {
	wasTurnedOn := brokenAutomation.IsEnabled
	brokenAutomation.IsEnabled = false
	err := service.automationStore.Save(brokenAutomation)
	if err != nil {
		return SaveResult{}, err
	}
	previousAutomation, hasPrevious := service.automationsByID[brokenAutomation.ID]
	_, wasPreviousBroken := service.problemsByID[brokenAutomation.ID]
	service.automationsByID[brokenAutomation.ID] = brokenAutomation
	service.problemsByID[brokenAutomation.ID] = problemMessage
	if hasPrevious && !wasPreviousBroken && previousAutomation.IsEnabled {
		previousAutomation.IsEnabled = false
		err = service.runner.SetAutomation(previousAutomation)
		if err != nil {
			return SaveResult{}, fmt.Errorf("saved automation %q turned off, but couldn't stop its runs: %w", brokenAutomation.ID, err)
		}
	}
	return SaveResult{IsTurnedOffUntilFixed: wasTurnedOn}, nil
}

// SetAutomationEnabled turns an automation on or off. Turning it off cancels
// its queued runs; a run in progress finishes. An automation with must-fix
// problems can't be turned on.
func (service *AutomationsService) SetAutomationEnabled(automationID string, isEnabled bool) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	savedAutomation, isFound := service.automationsByID[automationID]
	if !isFound {
		return fmt.Errorf("%w: %q", runner.ErrAutomationNotFound, automationID)
	}
	problemMessage, isBroken := service.problemsByID[automationID]
	if !isBroken {
		savedAutomation.IsEnabled = isEnabled
		return service.saveValidAutomation(savedAutomation)
	}
	if isEnabled {
		return fmt.Errorf("%w: %s", errNeedsFixing, problemMessage)
	}
	// It's already off unless its file was edited by hand; saving makes sure.
	_, err := service.saveBrokenAutomation(savedAutomation, problemMessage)
	return err
}

// DeleteAutomation removes an automation and its file, or the file of one
// that couldn't be read.
func (service *AutomationsService) DeleteAutomation(automationID string) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	err := service.runner.RemoveAutomation(automationID)
	// Invalid automations and unreadable files were never in the runner.
	if err != nil && !errors.Is(err, runner.ErrAutomationNotFound) {
		return err
	}
	err = service.automationStore.Delete(automationID)
	if err != nil {
		return err
	}
	delete(service.automationsByID, automationID)
	delete(service.problemsByID, automationID)
	delete(service.unreadableFileReasons, automationID+automationFileExtension)
	return nil
}

// TestValues returns the values a test run of an automation starts with,
// which its trigger works out so they'd start it for real. See
// docs/design/mvp-ui.md#test-runs.
func (service *AutomationsService) TestValues(automationID string) (map[string]string, error) {
	service.mutex.Lock()
	savedAutomation, isFound := service.automationsByID[automationID]
	service.mutex.Unlock()
	if !isFound {
		return nil, fmt.Errorf("%w: %q", runner.ErrAutomationNotFound, automationID)
	}
	testedTrigger, isKnown := service.catalog.Trigger(savedAutomation.Trigger.ID)
	if !isKnown {
		return nil, fmt.Errorf("automation %q uses trigger %q, which Whendo doesn't have", automationID, savedAutomation.Trigger.ID)
	}
	streamer, err := service.currentStreamer()
	if err != nil {
		return nil, fmt.Errorf("finding the account to test with: %w", err)
	}
	return testedTrigger.TestValues(savedAutomation.Trigger.Settings, streamer), nil
}

// TestRun queues a run of the saved automation with made-up trigger values.
// The steps really happen, the same as for a real event. Values a real event
// could never match aren't an error: the result says why, for the test form
// to show. An automation with must-fix problems can't be test run: the
// runner may still hold its last valid version, and testing that would
// mislead.
func (service *AutomationsService) TestRun(automationID string, triggerVariables map[string]string) (TestRunStarted, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	problemMessage, isBroken := service.problemsByID[automationID]
	if isBroken {
		return TestRunStarted{}, fmt.Errorf("%w: %s", errNeedsFixing, problemMessage)
	}
	runID, err := service.runner.TestRun(automationID, triggerVariables)
	var mismatch *runner.TestValuesMismatchError
	if errors.As(err, &mismatch) {
		return TestRunStarted{MismatchReason: cmp.Or(mismatch.Reason, generalMismatchReason)}, nil
	}
	if err != nil {
		return TestRunStarted{}, err
	}
	return TestRunStarted{RunID: runID}, nil
}

// RunHistoryDays returns every day that has run history, newest first.
func (service *AutomationsService) RunHistoryDays() ([]string, error) {
	return service.runHistoryStore.Days()
}

// RunHistory returns the runs that ended on a day, newest first. The day is
// written as YYYY-MM-DD.
func (service *AutomationsService) RunHistory(day string) ([]runner.RunRecord, error) {
	return service.runHistoryStore.ReadDay(day)
}
