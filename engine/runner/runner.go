// Package runner turns trigger events into runs. It receives events from
// integrations, hands them to steps that are waiting for events, matches
// them against each enabled automation's trigger, and runs each automation's
// queue one run at a time, with different automations in parallel. See
// docs/design/automation-engine.md#runs-and-queues.
package runner

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// MaximumQueuedRuns is how many runs can wait in one automation's queue.
// It's high enough that only a runaway loop or a flood of events reaches it,
// and keeps such a flood from using up memory.
const MaximumQueuedRuns = 2048

const queueFullReason = "Too many runs were waiting for this automation, so this one was skipped."

var (
	// ErrAutomationNotFound means the runner has no automation with the
	// given ID.
	ErrAutomationNotFound = errors.New("automation not found")
	// ErrTestValuesDontMatch means a test run's values wouldn't pass the
	// automation's trigger filter, so a real event with those values would
	// never start it.
	ErrTestValuesDontMatch = errors.New("test values don't match the automation's trigger")
	// ErrQueueFull means an automation already has MaximumQueuedRuns waiting.
	ErrQueueFull = errors.New("too many runs are waiting for this automation")
	// ErrShutDown means the runner has been shut down.
	ErrShutDown = errors.New("the runner has shut down")
)

// Config holds what a Runner needs.
type Config struct {
	Catalog *automation.Catalog
	// OnRunEnded, if set, is called with each run's record once the run has
	// ended, including runs that were cancelled or skipped because the queue
	// was full. It's called from many goroutines at once.
	OnRunEnded func(RunRecord)
	Logger     *slog.Logger
}

// Runner runs automations. It implements trigger.Receiver, so integrations
// deliver their events straight to it.
type Runner struct {
	catalog    *automation.Catalog
	onRunEnded func(RunRecord)
	logger     *slog.Logger

	// runContext is cancelled by Shutdown, which stops every run in
	// progress.
	runContext   context.Context
	cancelRuns   context.CancelFunc
	queueWorkers sync.WaitGroup

	// mutex guards queuesByAutomationID, isShutDown and every queue's
	// fields.
	mutex                sync.Mutex
	queuesByAutomationID map[string]*automationQueue
	isShutDown           bool

	subscriptions subscriptionList
}

// New returns a runner with no automations. Add them with SetAutomation.
func New(config Config) *Runner {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	onRunEnded := config.OnRunEnded
	if onRunEnded == nil {
		onRunEnded = func(RunRecord) {}
	}
	runContext, cancelRuns := context.WithCancel(context.Background())
	return &Runner{
		catalog:              config.Catalog,
		onRunEnded:           onRunEnded,
		logger:               logger,
		runContext:           runContext,
		cancelRuns:           cancelRuns,
		queuesByAutomationID: make(map[string]*automationQueue),
		subscriptions:        newSubscriptionList(logger),
	}
}

// SetAutomation adds an automation, or replaces the one with the same ID.
// Edits apply right away: each run uses the automation as it is when the run
// starts. Disabling an automation cancels its queued runs, but a run already
// in progress finishes. An invalid automation is refused, and the version
// already in the runner, if any, stays.
func (runner *Runner) SetAutomation(updatedAutomation automation.Automation) error {
	err := automation.Validate(updatedAutomation, runner.catalog)
	if err != nil {
		return err
	}
	runner.mutex.Lock()
	if runner.isShutDown {
		runner.mutex.Unlock()
		return ErrShutDown
	}
	queue, isExisting := runner.queuesByAutomationID[updatedAutomation.ID]
	if !isExisting {
		queue = newAutomationQueue(updatedAutomation)
		runner.queuesByAutomationID[updatedAutomation.ID] = queue
		runner.queueWorkers.Add(1)
		go runner.workThroughQueue(queue)
		runner.mutex.Unlock()
		return nil
	}
	wasEnabled := queue.automation.IsEnabled
	queue.automation = updatedAutomation
	var cancelledRuns []RunRecord
	if wasEnabled && !updatedAutomation.IsEnabled {
		cancelledRuns = queue.cancelPendingRuns()
	}
	runner.mutex.Unlock()
	runner.reportRuns(cancelledRuns)
	return nil
}

// RemoveAutomation removes an automation. Like disabling, it cancels the
// queued runs and lets a run in progress finish.
func (runner *Runner) RemoveAutomation(automationID string) error {
	runner.mutex.Lock()
	queue, isFound := runner.queuesByAutomationID[automationID]
	if !isFound {
		runner.mutex.Unlock()
		return fmt.Errorf("%w: %q", ErrAutomationNotFound, automationID)
	}
	delete(runner.queuesByAutomationID, automationID)
	queue.isRemoved = true
	cancelledRuns := queue.cancelPendingRuns()
	queue.signalWork()
	runner.mutex.Unlock()
	runner.reportRuns(cancelledRuns)
	return nil
}

// ReceiveEvent hands an event to every step waiting for events, then queues
// a run for every enabled automation whose trigger it matches. It never
// waits on a run, because integrations call it from the goroutine that
// reads their connection.
func (runner *Runner) ReceiveEvent(event trigger.Event) {
	runner.subscriptions.deliver(event)
	skippedRuns := runner.queueMatchingRuns(event)
	runner.reportRuns(skippedRuns)
}

func (runner *Runner) queueMatchingRuns(event trigger.Event) []RunRecord {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	if runner.isShutDown {
		return nil
	}
	eventTrigger, isKnown := runner.catalog.Trigger(event.TriggerID)
	if !isKnown {
		runner.logger.Warn("received an event for a trigger the catalog doesn't have", "trigger", event.TriggerID)
		return nil
	}
	var skippedRuns []RunRecord
	for _, queue := range runner.queuesByAutomationID {
		queuedAutomation := queue.automation
		if !queuedAutomation.IsEnabled || queuedAutomation.Trigger.ID != event.TriggerID {
			continue
		}
		variables, isMatch, err := eventTrigger.Filter(event, queuedAutomation.Trigger.Settings)
		if err != nil {
			// Settings are validated before an automation is added, so
			// this points to a bug in the trigger or in validation.
			runner.logger.Error("a trigger filter rejected an automation's settings", "automation", queuedAutomation.ID, "trigger", event.TriggerID, "error", err)
			continue
		}
		if !isMatch {
			continue
		}
		run := newPendingRun(variables, false)
		if !queue.addPendingRun(run) {
			skippedRuns = append(skippedRuns, run.recordAsSkipped(queuedAutomation, RunStatusFailed, queueFullReason))
		}
	}
	return skippedRuns
}

// TestRun queues a run of one automation, enabled or not, with made-up
// trigger values instead of a real event. The values still go through the
// trigger's filter, so it can add its own variables; values a real event
// could never match are refused with ErrTestValuesDontMatch. See
// docs/design/mvp-ui.md#test-runs.
func (runner *Runner) TestRun(automationID string, triggerVariables map[string]string) error {
	runner.mutex.Lock()
	defer runner.mutex.Unlock()
	if runner.isShutDown {
		return ErrShutDown
	}
	queue, isFound := runner.queuesByAutomationID[automationID]
	if !isFound {
		return fmt.Errorf("%w: %q", ErrAutomationNotFound, automationID)
	}
	testedTrigger, isKnown := runner.catalog.Trigger(queue.automation.Trigger.ID)
	if !isKnown {
		return fmt.Errorf("automation %q uses trigger %q, which the catalog doesn't have", automationID, queue.automation.Trigger.ID)
	}
	testEvent := trigger.Event{TriggerID: queue.automation.Trigger.ID, Variables: triggerVariables}
	variables, isMatch, err := testedTrigger.Filter(testEvent, queue.automation.Trigger.Settings)
	if err != nil {
		return fmt.Errorf("checking the test values against automation %q's trigger: %w", automationID, err)
	}
	if !isMatch {
		return ErrTestValuesDontMatch
	}
	if !queue.addPendingRun(newPendingRun(variables, true)) {
		return ErrQueueFull
	}
	return nil
}

// Shutdown stops every run in progress, cancels every queued run, and waits
// until all of them are recorded. After it returns, the runner ignores
// events and refuses changes.
func (runner *Runner) Shutdown() {
	runner.mutex.Lock()
	runner.isShutDown = true
	runner.mutex.Unlock()
	runner.cancelRuns()
	runner.queueWorkers.Wait()
}

func (runner *Runner) reportRuns(records []RunRecord) {
	for _, record := range records {
		runner.onRunEnded(record)
	}
}

func newRunID() string {
	return rand.Text()
}

// pendingRun is a run waiting in an automation's queue.
type pendingRun struct {
	runID            string
	isTest           bool
	triggerVariables map[string]string
	queuedAt         time.Time
}

func newPendingRun(triggerVariables map[string]string, isTest bool) pendingRun {
	return pendingRun{
		runID:            newRunID(),
		isTest:           isTest,
		triggerVariables: triggerVariables,
		queuedAt:         time.Now(),
	}
}

// recordAsSkipped records a run that ended without ever starting.
func (run pendingRun) recordAsSkipped(skippedAutomation automation.Automation, status RunStatus, reason string) RunRecord {
	return RunRecord{
		RunID:            run.runID,
		AutomationID:     skippedAutomation.ID,
		AutomationName:   skippedAutomation.Name,
		IsTest:           run.isTest,
		TriggerVariables: run.triggerVariables,
		QueuedAt:         run.queuedAt,
		EndedAt:          time.Now(),
		Status:           status,
		FailureReason:    reason,
	}
}
