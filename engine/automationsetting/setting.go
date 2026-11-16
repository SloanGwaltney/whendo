// Package automationsetting defines the settings contract that triggers and steps
// share, such as which reward a redeem trigger uses or how long a wait step
// waits, along with the variable references and templates that settings can
// hold. Sharing one contract lets the designer render trigger and step
// settings the same way. See docs/design/automation-engine.md#settings.
package automationsetting

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Kind tells the designer which control to show for a setting and the
// engine how to read its value. Integrations can add their own kinds, named
// with their prefix like trigger IDs, so they can't collide with the
// engine's.
type Kind string

const (
	// KindText is plain text.
	KindText Kind = "text"
	// KindTemplate is text with variables inserted. Its value is a Template.
	KindTemplate Kind = "template"
	// KindWholeNumber is an integer.
	KindWholeNumber Kind = "wholeNumber"
	// KindDuration is a length of time, stored in milliseconds.
	KindDuration Kind = "duration"
	// KindVariable is a reference to one variable. Its value is a
	// VariableReference.
	KindVariable Kind = "variable"
	// KindOption is the ID of one of the Options the setting's definition
	// lists.
	KindOption Kind = "option"
	// KindChoiceList is a list of Choices.
	KindChoiceList Kind = "choiceList"
)

// Option is one of the fixed values a KindOption setting allows.
type Option struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// Definition describes one setting a trigger or step takes.
type Definition struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Kind        Kind   `json:"kind"`
	IsRequired  bool   `json:"isRequired"`
	// Options lists the allowed values of a KindOption setting. Other kinds
	// leave it empty.
	Options []Option `json:"options"`
	// Placeholder is what the designer shows while the setting is empty.
	// For a required setting it's an instruction such as "pick a redeem",
	// so problems can reuse it: "needs you to pick a redeem". An optional
	// setting's says what empty means instead, such as "empty".
	Placeholder string `json:"placeholder"`
	// MaximumLength is the most characters a KindText or KindTemplate
	// setting may hold, or 0 for no limit. For a template it counts only
	// the template's own text, since variables' values aren't known until
	// it runs.
	MaximumLength int `json:"maximumLength"`
}

// Choice is one entry in a KindChoiceList setting. Its ID stays the same
// when the user edits its text, so anything tied to the choice, such as the
// branch for its outcome, survives the edit.
type Choice struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

var (
	// ErrSettingMissing means a setting has no value.
	ErrSettingMissing = errors.New("setting has no value")
	// ErrSettingInvalid means a setting's value isn't valid for its kind.
	ErrSettingInvalid = errors.New("setting value is invalid")
	// ErrSettingTooLong means a setting holds more characters than its
	// definition's MaximumLength.
	ErrSettingTooLong = errors.New("setting value is too long")
	// ErrBlankChoice and ErrRepeatedChoice are the ways a choice list can be
	// invalid that a user can cause. Both are also ErrSettingInvalid.
	ErrBlankChoice    = fmt.Errorf("%w: a choice is blank", ErrSettingInvalid)
	ErrRepeatedChoice = fmt.Errorf("%w: a choice is repeated", ErrSettingInvalid)
)

// Values holds the values of a trigger's or step's settings, keyed by
// setting ID, exactly as they're saved in an automation's file. The
// accessors read a value as a particular kind.
type Values map[string]json.RawMessage

// Text reads a KindText setting.
func (values Values) Text(settingID string) (string, error) {
	return readValue[string](values, settingID)
}

// WholeNumber reads a KindWholeNumber setting.
func (values Values) WholeNumber(settingID string) (int64, error) {
	return readValue[int64](values, settingID)
}

// Duration reads a KindDuration setting.
func (values Values) Duration(settingID string) (time.Duration, error) {
	milliseconds, err := readValue[int64](values, settingID)
	if err != nil {
		return 0, err
	}
	if milliseconds < 0 {
		return 0, fmt.Errorf("%w: setting %q is a negative duration", ErrSettingInvalid, settingID)
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}

// VariableReference reads a KindVariable setting.
func (values Values) VariableReference(settingID string) (VariableReference, error) {
	reference, err := readValue[VariableReference](values, settingID)
	if err != nil {
		return VariableReference{}, err
	}
	err = reference.validate()
	if err != nil {
		return VariableReference{}, fmt.Errorf("%w: setting %q: %w", ErrSettingInvalid, settingID, err)
	}
	return reference, nil
}

// Template reads a KindTemplate setting.
func (values Values) Template(settingID string) (Template, error) {
	template, err := readValue[Template](values, settingID)
	if err != nil {
		return nil, err
	}
	err = template.validate()
	if err != nil {
		return nil, fmt.Errorf("%w: setting %q: %w", ErrSettingInvalid, settingID, err)
	}
	return template, nil
}

// Option reads a KindOption setting, returning the chosen option's ID.
func (values Values) Option(settingID string) (string, error) {
	return readValue[string](values, settingID)
}

// ChoiceList reads a KindChoiceList setting.
func (values Values) ChoiceList(settingID string) ([]Choice, error) {
	return readValue[[]Choice](values, settingID)
}

// readValue decodes one setting. A JSON null counts as missing, the same as
// an absent setting, because the designer may save a cleared field as null.
func readValue[Value any](values Values, settingID string) (Value, error) {
	var value Value
	rawValue, isPresent := values[settingID]
	if !isPresent || string(rawValue) == "null" {
		return value, fmt.Errorf("%w: %q", ErrSettingMissing, settingID)
	}
	err := json.Unmarshal(rawValue, &value)
	if err != nil {
		return value, fmt.Errorf("%w: setting %q: %w", ErrSettingInvalid, settingID, err)
	}
	return value, nil
}
