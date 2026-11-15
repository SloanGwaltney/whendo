// Package trigger defines the contract between integrations and the
// automation engine. The engine doesn't define any triggers itself: each
// integration, such as Twitch, declares the triggers it offers and delivers
// events for them in this generic shape. No event is guaranteed to exist on
// every platform, so nothing here is specific to one. See
// docs/design/twitch-eventsub.md#triggers-and-event-sources and
// docs/design/automation-engine.md#triggers.
package trigger

import "github.com/SloanGwaltney/whendo/engine/automationsetting"

// VariableDefinition describes one value a trigger or step provides, such as
// the redeemer's name. Steps and sentences refer to it by ID; users see its
// display name.
type VariableDefinition struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// IsAddedByFilter means the trigger's filter works out the value from the
	// event and the automation's settings, such as the rest of a chat message
	// after the matched text. A test run form doesn't ask for these, because
	// the filter fills them in.
	IsAddedByFilter bool `json:"isAddedByFilter"`
}

// Definition describes a trigger an integration offers. Its settings and
// variables are declared up front so the designer and the test run form know
// what the trigger takes and provides before any event arrives.
type Definition struct {
	ID          string                         `json:"id"`
	DisplayName string                         `json:"displayName"`
	Settings    []automationsetting.Definition `json:"settings"`
	Variables   []VariableDefinition           `json:"variables"`
}

// Trigger is a trigger an integration offers, with the filter that decides
// which of its events an automation reacts to.
type Trigger interface {
	Definition() Definition
	// Filter checks an event against an automation's settings for this
	// trigger. When the event matches, it returns the event's variables
	// along with any the filter adds. It returns an error when the settings
	// themselves are invalid, such as a missing required setting.
	Filter(event Event, settings automationsetting.Values) (variables map[string]string, isMatch bool, err error)
}

// Event is one occurrence of a trigger. Variables holds a value for each of
// the trigger's declared variables that its events provide, keyed by
// variable ID.
type Event struct {
	TriggerID string
	Variables map[string]string
}

// Receiver accepts events from an integration. The automation engine's
// runner implements it.
type Receiver interface {
	ReceiveEvent(event Event)
}
