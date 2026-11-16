package automationsetting

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// VariableSource says where a referenced variable comes from.
type VariableSource string

const (
	// VariableSourceTrigger is a variable the automation's trigger provides.
	VariableSourceTrigger VariableSource = "trigger"
	// VariableSourceStep is a variable an earlier step provides.
	VariableSourceStep VariableSource = "step"
)

// VariableReference points to one variable by where it comes from and its
// ID, never by its display name, so renaming a display name never breaks an
// automation. StepID identifies the step for VariableSourceStep, so two steps
// of the same type don't conflict; it's empty for VariableSourceTrigger.
type VariableReference struct {
	Source VariableSource `json:"source"`
	StepID string         `json:"stepId,omitempty"`
	ID     string         `json:"id"`
}

// ErrVariableUnavailable means a referenced variable has no value at the
// point it's used.
var ErrVariableUnavailable = errors.New("variable is not available")

var errInvalidVariableReference = errors.New("invalid variable reference")

func (reference VariableReference) validate() error {
	if reference.ID == "" {
		return fmt.Errorf("%w: no variable ID", errInvalidVariableReference)
	}
	switch reference.Source {
	case VariableSourceTrigger:
		if reference.StepID != "" {
			return fmt.Errorf("%w: a trigger variable can't name a step", errInvalidVariableReference)
		}
		return nil
	case VariableSourceStep:
		if reference.StepID == "" {
			return fmt.Errorf("%w: a step variable must name its step", errInvalidVariableReference)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown source %q", errInvalidVariableReference, reference.Source)
	}
}

// VariableValues holds the value of every variable available at one point in
// a run.
type VariableValues map[VariableReference]string

// Value returns the value of the referenced variable.
func (variables VariableValues) Value(reference VariableReference) (string, error) {
	value, isAvailable := variables[reference]
	if !isAvailable {
		return "", fmt.Errorf("%w: %s", ErrVariableUnavailable, reference.describe())
	}
	return value, nil
}

func (reference VariableReference) describe() string {
	if reference.Source == VariableSourceStep {
		return fmt.Sprintf("%q from step %q", reference.ID, reference.StepID)
	}
	return fmt.Sprintf("%q from the %s", reference.ID, reference.Source)
}

// TemplatePart is one piece of a Template: either literal text or a
// variable, never both.
type TemplatePart struct {
	Text     string             `json:"text,omitempty"`
	Variable *VariableReference `json:"variable,omitempty"`
}

// Template is text with variables inserted, such as "Thanks for checking in,
// <redeemer's name>!". It's stored as parts rather than text with
// placeholders, so nothing a user types needs escaping.
type Template []TemplatePart

var errInvalidTemplatePart = errors.New("invalid template part")

func (template Template) validate() error {
	for partIndex, part := range template {
		if part.Variable == nil {
			continue
		}
		if part.Text != "" {
			return fmt.Errorf("%w: part %d has both text and a variable", errInvalidTemplatePart, partIndex)
		}
		err := part.Variable.validate()
		if err != nil {
			return fmt.Errorf("part %d: %w", partIndex, err)
		}
	}
	return nil
}

// isBlank reports whether the template has no variables and no text other
// than spaces, which is as empty to a user as no template at all.
func (template Template) isBlank() bool {
	for _, part := range template {
		if part.Variable != nil || strings.TrimSpace(part.Text) != "" {
			return false
		}
	}
	return true
}

// textLength counts the characters of the template's own text, leaving out
// its variables.
func (template Template) textLength() int {
	characterCount := 0
	for _, part := range template {
		characterCount += utf8.RuneCountInString(part.Text)
	}
	return characterCount
}

// Render returns the template's text with each variable's value inserted.
func (template Template) Render(variables VariableValues) (string, error) {
	var renderedText strings.Builder
	for _, part := range template {
		if part.Variable == nil {
			renderedText.WriteString(part.Text)
			continue
		}
		value, err := variables.Value(*part.Variable)
		if err != nil {
			return "", err
		}
		renderedText.WriteString(value)
	}
	return renderedText.String(), nil
}
