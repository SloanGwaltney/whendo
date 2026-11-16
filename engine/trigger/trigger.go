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
	// IsSetBySettings means the automation's settings decide the value, such
	// as the ID of the reward a redeem trigger watches for. A test run form
	// fills these in from the trigger's test values rather than asking,
	// since any other value would never match.
	IsSetBySettings bool `json:"isSetBySettings"`
	// IsCommon means the designer lists the variable first, before a "more"
	// link, because most automations use it, like the redeemer's name.
	IsCommon bool `json:"isCommon"`
}

// Definition describes a trigger an integration offers. Its settings and
// variables are declared up front so the designer and the test run form know
// what the trigger takes and provides before any event arrives.
type Definition struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// Description is one line for the trigger picker.
	Description string `json:"description"`
	// Sentence is how the designer shows the trigger after "When", such as
	// "someone redeems" followed by the reward setting.
	Sentence  automationsetting.Sentence     `json:"sentence"`
	Settings  []automationsetting.Definition `json:"settings"`
	Variables []VariableDefinition           `json:"variables"`
	// PersonUserIDVariable is the variable holding the user ID of the
	// person behind the event, such as the redeemer, so a step can act on
	// "the person who started this" without knowing which trigger it
	// follows. Empty means the trigger's events aren't caused by a person.
	PersonUserIDVariable string `json:"personUserIdVariable"`
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
	// TestValues returns starting values for a test run, which pass Filter
	// with these settings, so a quick test is one click. Values about a
	// person are the streamer's own. Only the trigger knows its filter, so
	// it's the one place that can always offer values that match. See
	// docs/design/mvp-ui.md#test-runs.
	TestValues(settings automationsetting.Values, streamer Streamer) map[string]string
}

// MismatchExplainer is a trigger that can say, in words for the streamer,
// why test values don't pass its filter, such as "The message has to start
// with “!d20”." Triggers that can't are still testable; the test form then
// gives a general reason.
type MismatchExplainer interface {
	ExplainMismatch(variables map[string]string, settings automationsetting.Values) string
}

// SettingsNoter is a trigger that has something the streamer should know
// about its settings, even though the automation is fine, such as a
// placeholder redeem that only runs when tested. Notes are never must-fix:
// rules that make settings invalid belong in the setting definitions.
type SettingsNoter interface {
	NoteSettings(settings automationsetting.Values) []SettingsNote
}

// SettingsNote is one thing worth knowing about a trigger's settings,
// written for the streamer.
type SettingsNote struct {
	// SettingID is the setting the note is about, or empty when it's about
	// the trigger as a whole.
	SettingID string
	Message   string
}

// Streamer is the account an automation runs for, whose details a test
// run's starting values use.
type Streamer struct {
	UserID string
	Login  string
	// DisplayName is the name shown in chat. It can differ from Login in
	// capitalization, or for names in other alphabets.
	DisplayName string
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
