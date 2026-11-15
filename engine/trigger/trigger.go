// Package trigger defines the contract between integrations and the
// automation engine. The engine doesn't define any triggers itself: each
// integration, such as Twitch, declares the triggers it offers and delivers
// events for them in this generic shape. No event is guaranteed to exist on
// every platform, so nothing here is specific to one. See
// docs/design/twitch-eventsub.md#triggers-and-event-sources.
package trigger

// VariableDefinition describes one value a trigger provides, such as the
// redeemer's name. Steps and sentences refer to it by ID; users see its
// display name.
type VariableDefinition struct {
	ID          string
	DisplayName string
}

// Definition describes a trigger an integration offers. Its variables are
// declared up front so the designer and the test run form know what the
// trigger provides before any event arrives.
type Definition struct {
	ID          string
	DisplayName string
	Variables   []VariableDefinition
}

// Event is one occurrence of a trigger. Variables holds a value for each of
// the trigger's declared variables, keyed by variable ID.
type Event struct {
	TriggerID string
	Variables map[string]string
}

// Receiver accepts events from an integration. The automation engine
// implements it; until it exists, the app logs the events it receives.
type Receiver interface {
	ReceiveEvent(event Event)
}
