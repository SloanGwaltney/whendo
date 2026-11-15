package runner

import "github.com/SloanGwaltney/whendo/engine/automation"

const (
	disabledReason = "The automation was disabled or deleted before this run started."
	shutdownReason = "Whendo shut down before this run started."
)

// automationQueue holds one automation and the runs waiting for it. Its
// fields are guarded by the runner's mutex.
type automationQueue struct {
	automation  automation.Automation
	pendingRuns []pendingRun
	isRemoved   bool
	// hasWork wakes the queue's worker. It holds at most one signal, since
	// the worker checks everything each time it wakes.
	hasWork chan struct{}
}

func newAutomationQueue(queuedAutomation automation.Automation) *automationQueue {
	return &automationQueue{
		automation: queuedAutomation,
		hasWork:    make(chan struct{}, 1),
	}
}

// addPendingRun adds a run to the end of the queue. It returns false when
// the queue is full.
func (queue *automationQueue) addPendingRun(run pendingRun) bool {
	if len(queue.pendingRuns) >= MaximumQueuedRuns {
		return false
	}
	queue.pendingRuns = append(queue.pendingRuns, run)
	queue.signalWork()
	return true
}

func (queue *automationQueue) signalWork() {
	select {
	case queue.hasWork <- struct{}{}:
	default:
	}
}

func (queue *automationQueue) cancelPendingRuns() []RunRecord {
	return queue.skipPendingRuns(disabledReason)
}

func (queue *automationQueue) skipPendingRuns(reason string) []RunRecord {
	records := make([]RunRecord, 0, len(queue.pendingRuns))
	for _, run := range queue.pendingRuns {
		records = append(records, run.recordAsSkipped(queue.automation, RunStatusCancelled, reason))
	}
	queue.pendingRuns = nil
	return records
}

// workThroughQueue runs the queue's runs one at a time until the automation
// is removed or the runner shuts down. Each run takes the automation as it
// is when the run starts, so edits apply to queued runs.
func (runner *Runner) workThroughQueue(queue *automationQueue) {
	defer runner.queueWorkers.Done()
	for {
		runner.mutex.Lock()
		if runner.runContext.Err() != nil {
			cancelledRuns := queue.skipPendingRuns(shutdownReason)
			runner.mutex.Unlock()
			runner.reportRuns(cancelledRuns)
			return
		}
		if len(queue.pendingRuns) > 0 {
			nextRun := queue.pendingRuns[0]
			queue.pendingRuns = queue.pendingRuns[1:]
			automationAtStart := queue.automation
			runner.mutex.Unlock()
			runner.onRunEnded(runner.execute(nextRun, automationAtStart))
			continue
		}
		if queue.isRemoved {
			runner.mutex.Unlock()
			return
		}
		runner.mutex.Unlock()
		select {
		case <-queue.hasWork:
		case <-runner.runContext.Done():
		}
	}
}
