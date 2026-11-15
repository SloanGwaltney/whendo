package runner

import "time"

// RunStatus is how a run ended.
type RunStatus string

const (
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	// RunStatusCancelled covers runs removed from the queue when their
	// automation was disabled or deleted, and runs interrupted by Whendo
	// shutting down.
	RunStatusCancelled RunStatus = "cancelled"
)

// RunRecord is everything about one run once it has ended, as run history
// keeps it. See docs/design/mvp-ui.md#run-history.
type RunRecord struct {
	RunID        string `json:"runId"`
	AutomationID string `json:"automationId"`
	// AutomationName is the name when the run started, or when it ended for
	// a run that never started, so history still makes sense after the
	// automation is renamed or deleted.
	AutomationName string `json:"automationName"`
	IsTest         bool   `json:"isTest"`
	// TriggerVariables are the values the run started with, keyed by
	// variable ID, so the streamer can see who or what set it off.
	TriggerVariables map[string]string `json:"triggerVariables"`
	QueuedAt         time.Time         `json:"queuedAt"`
	// StartedAt is zero for runs that never left the queue.
	StartedAt     time.Time    `json:"startedAt"`
	EndedAt       time.Time    `json:"endedAt"`
	Status        RunStatus    `json:"status"`
	FailureReason string       `json:"failureReason,omitempty"`
	Steps         []StepRecord `json:"steps"`
}

// StepRecord is one step a run executed, in the order it ran. For steps
// that split, OutcomeID shows which branch the run took.
type StepRecord struct {
	StepID        string    `json:"stepId"`
	StepType      string    `json:"stepType"`
	StartedAt     time.Time `json:"startedAt"`
	EndedAt       time.Time `json:"endedAt"`
	OutcomeID     string    `json:"outcomeId,omitempty"`
	FailureReason string    `json:"failureReason,omitempty"`
}
