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
)

// runEndedEvent carries a runner.RunRecord each time a run ends, so run
// history can update live.
const runEndedEvent = "run-ended"

// automationFileExtension lets an unreadable file be listed, and deleted, by
// the ID its name stands for.
const automationFileExtension = ".json"

// AutomationSummary is one row of the automation list. Problem explains why
// an automation isn't running, such as a setting that's missing or a file
// that couldn't be read; it's empty for automations that are fine.
type AutomationSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsEnabled bool   `json:"isEnabled"`
	TriggerID string `json:"triggerId"`
	Problem   string `json:"problem"`
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

	// mutex guards the maps. They hold every saved automation, including
	// invalid ones the runner refused, so the UI can list them with their
	// problems.
	mutex                 sync.Mutex
	automationsByID       map[string]automation.Automation
	problemsByID          map[string]string
	unreadableFileReasons map[string]string
}

func newAutomationsService(catalog *automation.Catalog, automationStore *automation.Store, runHistoryStore *runhistory.Store, logger *slog.Logger) *AutomationsService {
	service := &AutomationsService{
		catalog:               catalog,
		automationStore:       automationStore,
		runHistoryStore:       runHistoryStore,
		logger:                logger,
		automationsByID:       make(map[string]automation.Automation),
		problemsByID:          make(map[string]string),
		unreadableFileReasons: make(map[string]string),
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
		service.automationsByID[loadedAutomation.ID] = loadedAutomation
		err := service.runner.SetAutomation(loadedAutomation)
		if err != nil {
			service.logger.Warn("an automation is invalid and won't run", "automation", loadedAutomation.ID, "error", err)
			service.problemsByID[loadedAutomation.ID] = err.Error()
		}
	}
	for _, failure := range failures {
		service.logger.Warn("an automation file couldn't be read", "file", failure.FileName, "error", failure.Reason)
		service.unreadableFileReasons[failure.FileName] = failure.Reason.Error()
	}
	service.mutex.Unlock()
	// Wails cancels ctx right before shutdown, which stops pruning.
	go service.runHistoryStore.KeepPruned(ctx, runhistory.DefaultRetentionDays)
	return nil
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
		summaries = append(summaries, AutomationSummary{ID: strings.TrimSuffix(fileName, automationFileExtension), Name: fileName, Problem: reason})
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

// SaveAutomation saves a new or changed automation and puts it to work right
// away. An invalid automation is refused, so nothing that can't run is ever
// saved from the designer.
func (service *AutomationsService) SaveAutomation(changedAutomation automation.Automation) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.saveAutomation(changedAutomation)
}

func (service *AutomationsService) saveAutomation(changedAutomation automation.Automation) error {
	err := automation.Validate(changedAutomation, service.catalog)
	if err != nil {
		return err
	}
	err = service.automationStore.Save(changedAutomation)
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

// SetAutomationEnabled turns an automation on or off. Turning it off cancels
// its queued runs; a run in progress finishes.
func (service *AutomationsService) SetAutomationEnabled(automationID string, isEnabled bool) error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	savedAutomation, isFound := service.automationsByID[automationID]
	if !isFound {
		return fmt.Errorf("%w: %q", runner.ErrAutomationNotFound, automationID)
	}
	savedAutomation.IsEnabled = isEnabled
	return service.saveAutomation(savedAutomation)
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

// TestRun runs an automation with made-up trigger values. The steps really
// happen, the same as for a real event.
func (service *AutomationsService) TestRun(automationID string, triggerVariables map[string]string) error {
	return service.runner.TestRun(automationID, triggerVariables)
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
