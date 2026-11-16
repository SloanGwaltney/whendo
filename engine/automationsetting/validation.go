package automationsetting

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// ValidateSetting checks one defined setting's value against its kind:
// a required setting must have a value, and a value must be readable as its
// kind. Kinds an integration adds are only checked for presence, since only
// the integration knows what they hold. Values that have no definition are
// never checked, so an automation stays valid when a later version of Whendo
// drops a setting from a step.
func (values Values) ValidateSetting(definition Definition) error {
	if !values.hasValue(definition.ID) {
		if definition.IsRequired {
			return fmt.Errorf("%w: %q", ErrSettingMissing, definition.ID)
		}
		return nil
	}
	switch definition.Kind {
	case KindText:
		return values.validateText(definition)
	case KindTemplate:
		return values.validateTemplate(definition)
	case KindWholeNumber:
		_, err := values.WholeNumber(definition.ID)
		return err
	case KindDuration:
		return values.validateDuration(definition)
	case KindVariable:
		_, err := values.VariableReference(definition.ID)
		return err
	case KindOption:
		return values.validateOption(definition)
	case KindChoiceList:
		return values.validateChoiceList(definition)
	default:
		return nil
	}
}

func (values Values) hasValue(settingID string) bool {
	rawValue, isPresent := values[settingID]
	return isPresent && string(rawValue) != "null"
}

// validateText treats blank text in a required setting as missing, since a
// field of spaces is as empty to a user as no field at all.
func (values Values) validateText(definition Definition) error {
	text, err := values.Text(definition.ID)
	if err != nil {
		return err
	}
	if definition.IsRequired && strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: %q is blank", ErrSettingMissing, definition.ID)
	}
	return checkLength(definition, utf8.RuneCountInString(text))
}

// checkLength counts characters rather than bytes, because limits such as
// Twitch's are in characters.
func checkLength(definition Definition, characterCount int) error {
	if definition.MaximumLength > 0 && characterCount > definition.MaximumLength {
		return fmt.Errorf("%w: %q has %d characters, more than %d", ErrSettingTooLong, definition.ID, characterCount, definition.MaximumLength)
	}
	return nil
}

func (values Values) validateDuration(definition Definition) error {
	duration, err := values.Duration(definition.ID)
	if err != nil {
		return err
	}
	maximum := time.Duration(definition.MaximumMilliseconds) * time.Millisecond
	if maximum > 0 && duration > maximum {
		return fmt.Errorf("%w: %q is %s, more than %s", ErrDurationTooLong, definition.ID, duration, maximum)
	}
	return nil
}

func (values Values) validateTemplate(definition Definition) error {
	template, err := values.Template(definition.ID)
	if err != nil {
		return err
	}
	if definition.IsRequired && template.isBlank() {
		return fmt.Errorf("%w: %q is empty", ErrSettingMissing, definition.ID)
	}
	// Only the template's own text counts, since variables' values aren't
	// known until it runs. Text that's already too long can never fit.
	return checkLength(definition, template.textLength())
}

func (values Values) validateOption(definition Definition) error {
	optionID, err := values.Option(definition.ID)
	if err != nil {
		return err
	}
	for _, option := range definition.Options {
		if option.ID == optionID {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is not one of setting %q's options", ErrSettingInvalid, optionID, definition.ID)
}

func (values Values) validateChoiceList(definition Definition) error {
	choices, err := values.ChoiceList(definition.ID)
	if err != nil {
		return err
	}
	if definition.IsRequired && len(choices) == 0 {
		return fmt.Errorf("%w: %q has no choices", ErrSettingMissing, definition.ID)
	}
	seenChoiceIDs := make(map[string]bool, len(choices))
	seenChoiceTexts := make(map[string]bool, len(choices))
	for choiceIndex, choice := range choices {
		if choice.ID == "" {
			return fmt.Errorf("%w: choice %d of setting %q has no ID", ErrSettingInvalid, choiceIndex, definition.ID)
		}
		if seenChoiceIDs[choice.ID] {
			return fmt.Errorf("%w: setting %q has two choices with ID %q", ErrSettingInvalid, definition.ID, choice.ID)
		}
		seenChoiceIDs[choice.ID] = true
		trimmedText := strings.TrimSpace(choice.Text)
		if trimmedText == "" {
			return fmt.Errorf("%w: choice %d of setting %q", ErrBlankChoice, choiceIndex, definition.ID)
		}
		// Choices are matched ignoring capitalization, so "!A1" repeats
		// "!a1", and the second could never be chosen.
		comparableText := strings.ToLower(trimmedText)
		if seenChoiceTexts[comparableText] {
			return fmt.Errorf("%w: %q in setting %q", ErrRepeatedChoice, trimmedText, definition.ID)
		}
		seenChoiceTexts[comparableText] = true
	}
	return nil
}

// VariableReferences returns every variable one setting refers to: the
// variable of a KindVariable setting, or the variables inserted in a
// KindTemplate setting. A setting without a value refers to none.
func (values Values) VariableReferences(definition Definition) ([]VariableReference, error) {
	if !values.hasValue(definition.ID) {
		return nil, nil
	}
	switch definition.Kind {
	case KindVariable:
		reference, err := values.VariableReference(definition.ID)
		if err != nil {
			return nil, err
		}
		return []VariableReference{reference}, nil
	case KindTemplate:
		template, err := values.Template(definition.ID)
		if err != nil {
			return nil, err
		}
		var references []VariableReference
		for _, part := range template {
			if part.Variable != nil {
				references = append(references, *part.Variable)
			}
		}
		return references, nil
	default:
		return nil, nil
	}
}
