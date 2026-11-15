// Package automation defines automations as users build them in the
// designer: a trigger and a sequence of steps, with branches for steps that
// split. It checks that an automation is valid, works out which variables
// each step can use, and saves automations as files. Whendo's code never
// defines an automation itself; users create every one. See
// docs/design/automation-engine.md.
package automation

import (
	"crypto/rand"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
)

// AfterBranch says what happens when a branch finishes.
type AfterBranch string

const (
	// AfterBranchEnd ends the automation.
	AfterBranchEnd AfterBranch = "end"
	// AfterBranchContinue continues with the step after the split.
	AfterBranchContinue AfterBranch = "continue"
)

// Automation is one automation: when its trigger fires, do its steps.
type Automation struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	IsEnabled bool             `json:"isEnabled"`
	Trigger   TriggerSelection `json:"trigger"`
	Steps     []Step           `json:"steps"`
}

// TriggerSelection is the trigger an automation uses and the automation's
// settings for it, which act as the trigger's filter.
type TriggerSelection struct {
	ID       string                   `json:"id"`
	Settings automationsetting.Values `json:"settings"`
}

// Step is one step in an automation. Type is the step's definition ID, such
// as "core.wait". ID identifies this particular step, so variables from two
// steps of the same type don't conflict.
type Step struct {
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Settings automationsetting.Values `json:"settings"`
	// Branches holds the branch for each of the step's outcomes, keyed by
	// outcome ID. Steps with a single outcome have none. The file calls them
	// outcomes, since that's how users see them.
	Branches map[string]Branch `json:"outcomes,omitempty"`
}

// Branch is the sequence of steps for one outcome of a step that splits.
type Branch struct {
	Steps       []Step      `json:"steps"`
	AfterBranch AfterBranch `json:"afterBranch"`
}

// NewID returns a new random ID for an automation. It's safe to use as a
// file name.
func NewID() string {
	return rand.Text()
}
