// Package coresteps holds the steps that need no integration: waiting,
// rolling a random number and checking a condition. See
// docs/design/automation-engine.md#mvp-core-steps.
package coresteps

import "github.com/SloanGwaltney/whendo/engine/step"

// Steps returns every core step.
func Steps() []step.Step {
	return []step.Step{waitStep{}, randomNumberStep{}, checkConditionStep{}}
}

func singleOutcome() []step.Outcome {
	return []step.Outcome{{ID: step.DefaultOutcomeID, DisplayName: "Done"}}
}
