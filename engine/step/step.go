// Package step defines the contract for automation steps, such as "wait" or
// "send a chat message". It mirrors the trigger contract: each step declares
// its settings and outcomes up front, and the engine runs it. Steps that need
// no integration live in the engine; the others are defined by their
// integration. See docs/design/automation-engine.md#steps.
package step

import (
	"context"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// DefaultOutcomeID is the ID of the only outcome of a step that doesn't
// split, so steps like "wait" don't each invent their own.
const DefaultOutcomeID = "done"

// Definition describes a step. Its ID is prefixed by where the step comes
// from, such as "core.wait" or "twitch.sendChatMessage".
type Definition struct {
	ID          string                         `json:"id"`
	DisplayName string                         `json:"displayName"`
	Settings    []automationsetting.Definition `json:"settings"`
}

// Outcome is one way a step can finish. Steps with more than one outcome
// split the automation, and each outcome gets its own branch. Variables are
// the values the step provides when it finishes this way, such as the reply
// to "wait for a chat message".
type Outcome struct {
	ID          string                       `json:"id"`
	DisplayName string                       `json:"displayName"`
	Variables   []trigger.VariableDefinition `json:"variables"`
}

// Input is everything a step gets when it runs.
type Input struct {
	Settings automationsetting.Values
	// Variables holds every variable available to the step, so it can
	// resolve the templates and variable references in its settings.
	Variables automationsetting.VariableValues
	// Events lets a step wait for events, such as a reply in chat.
	Events EventSource
}

// Result is how a step finished: the outcome it took, and the value of each
// variable that outcome provides, keyed by variable ID.
type Result struct {
	OutcomeID string
	Variables map[string]string
}

// Step is a step the engine can run.
type Step interface {
	Definition() Definition
	// Outcomes returns the ways the step can finish with the given settings.
	// They depend on the settings for steps like "wait for a chat message",
	// which has one outcome per choice.
	Outcomes(settings automationsetting.Values) ([]Outcome, error)
	// Run performs the step. An error fails the run. The context is cancelled
	// when Whendo shuts down, so a step that waits must stop when it is.
	Run(ctx context.Context, input Input) (Result, error)
}

// EventSource lets a running step receive the events integrations deliver,
// the same events triggers receive.
type EventSource interface {
	// Subscribe starts delivering events to the returned subscription. The
	// step must call Unsubscribe when it no longer needs them.
	Subscribe() EventSubscription
}

// EventSubscription delivers events to one step until it unsubscribes.
type EventSubscription interface {
	Events() <-chan trigger.Event
	Unsubscribe()
}
