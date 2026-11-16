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
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// Description is one line for the step picker.
	Description string `json:"description"`
	// Group is what the step acts on, such as "Chat" or "Timing". The step
	// picker groups steps by it.
	Group string `json:"group"`
	// Sentence is how the designer shows the step, such as "Say in chat"
	// followed by the message setting.
	Sentence automationsetting.Sentence     `json:"sentence"`
	Settings []automationsetting.Definition `json:"settings"`
	// RequiredTriggerID is the trigger an automation must start with for
	// the step to work, because the step acts on the event that started it,
	// such as refunding a redemption. Empty means any trigger.
	RequiredTriggerID string `json:"requiredTriggerId"`
}

// Outcome is one way a step can finish. Steps with more than one outcome
// split the automation, and each outcome gets its own branch. Variables are
// the values the step provides when it finishes this way, such as the reply
// to "wait for a chat message".
type Outcome struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// Sentence labels the outcome's branch in the designer, such as "If
	// so". Steps that don't split leave it empty, since they have no
	// branches.
	Sentence  automationsetting.Sentence   `json:"sentence"`
	Variables []trigger.VariableDefinition `json:"variables"`
}

// Input is everything a step gets when it runs.
type Input struct {
	Settings automationsetting.Values
	// Variables holds every variable available to the step, so it can
	// resolve the templates and variable references in its settings.
	Variables automationsetting.VariableValues
	// Events lets a step wait for events, such as a reply in chat.
	Events EventSource
	// StartedByUserID is the user ID of the person who started the run,
	// such as the redeemer, or empty when the trigger has no person. See
	// trigger.Definition.PersonUserIDVariable.
	StartedByUserID string
}

// Result is how a step finished: the outcome it took, and the value of each
// variable that outcome provides, keyed by variable ID.
type Result struct {
	OutcomeID string
	Variables map[string]string
	// Details say what the step did, for run history. They're kept even when
	// Run returns an error, so a failure can show what the step tried, such
	// as the message Twitch refused.
	Details []Detail
}

// Detail is something a step did, such as the message it sent, shown in run
// history to explain a run. Unlike an outcome's variables, later steps can't
// use it, so a step can report what it did without offering the designer
// another value.
type Detail struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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

// Noter is a step that has something the streamer should know about how
// it's used, even though the automation is fine, such as refunding
// redemptions of a redeem that can't be refunded. Notes are never must-fix:
// rules that make settings invalid belong in the setting definitions.
type Noter interface {
	NoteStep(usage Usage) []Note
}

// UsageChecker is a step whose settings only work with some triggers, such
// as waiting for "the person who started this" after a trigger that has no
// person. Its problems are must-fix, unlike a Noter's notes.
type UsageChecker interface {
	CheckUsage(usage Usage) []Note
}

// Usage is where a step is used: its own settings and the trigger the
// automation starts with.
type Usage struct {
	Settings        automationsetting.Values
	TriggerID       string
	TriggerSettings automationsetting.Values
	// TriggerName is the trigger's display name, for messages.
	TriggerName string
	// HasTriggerPerson says whether the trigger's events are caused by a
	// person, so steps can act on "the person who started this".
	HasTriggerPerson bool
}

// Note is one thing worth knowing about how a step is used, written for the
// streamer.
type Note struct {
	// SettingID is the step's setting the note is about, or empty when it's
	// about the step as a whole.
	SettingID string
	Message   string
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
