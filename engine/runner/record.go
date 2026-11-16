package runner

import (
	"time"

	"github.com/SloanGwaltney/whendo/engine/step"
)

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
	// TriggerID says which trigger set the run off, so run history can name
	// its variables.
	TriggerID string `json:"triggerId"`
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
	StepID   string `json:"stepId"`
	StepType string `json:"stepType"`
	// StepName is the step's display name when it ran, such as "Send a chat
	// message".
	StepName string `json:"stepName"`
	// BranchLabel names the branch the run took, as the designer labelled it
	// when the step ran, such as "If they type !a1". It's kept as text
	// because the label can depend on settings that are edited later, or on
	// an automation that's since been deleted. It's empty for steps that
	// don't split.
	BranchLabel string `json:"branchLabel,omitempty"`
	// Values are what the step provided to the steps after it, such as the
	// number it rolled, so a run that didn't do what was expected can be
	// understood.
	Values []RecordedValue `json:"values,omitempty"`
	// Details say what the step did, such as the message it sent, including
	// when it failed.
	Details       []step.Detail `json:"details,omitempty"`
	StartedAt     time.Time     `json:"startedAt"`
	EndedAt       time.Time     `json:"endedAt"`
	OutcomeID     string        `json:"outcomeId,omitempty"`
	FailureReason string        `json:"failureReason,omitempty"`
}

// RecordedValue is one value a step provided. Its name is kept as text, as
// the step named it when it ran.
type RecordedValue struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}
