package automationsetting

import (
	"fmt"
	"strings"
)

// Validate checks each defined setting's value against its kind: required
// settings must have a value, and every value must be readable as its kind.
// Values with no definition are ignored rather than rejected, so an
// automation stays valid when a later version of Whendo drops a setting from
// a step. Kinds an integration adds are only checked for presence, since
// only the integration knows what they hold.
func (values Values) Validate(definitions []Definition) error {
	for _, definition := range definitions {
		err := values.validateSetting(definition)
		if err != nil {
			return err
		}
	}
	return nil
}

func (values Values) validateSetting(definition Definition) error {
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
		_, err := values.Duration(definition.ID)
		return err
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
	return nil
}

func (values Values) validateTemplate(definition Definition) error {
	template, err := values.Template(definition.ID)
	if err != nil {
		return err
	}
	if definition.IsRequired && len(template) == 0 {
		return fmt.Errorf("%w: %q is empty", ErrSettingMissing, definition.ID)
	}
	return nil
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
	for choiceIndex, choice := range choices {
		if choice.ID == "" {
			return fmt.Errorf("%w: choice %d of setting %q has no ID", ErrSettingInvalid, choiceIndex, definition.ID)
		}
		if seenChoiceIDs[choice.ID] {
			return fmt.Errorf("%w: setting %q has two choices with ID %q", ErrSettingInvalid, definition.ID, choice.ID)
		}
		seenChoiceIDs[choice.ID] = true
		if strings.TrimSpace(choice.Text) == "" {
			return fmt.Errorf("%w: choice %d of setting %q is blank", ErrSettingInvalid, choiceIndex, definition.ID)
		}
	}
	return nil
}

// VariableReferences returns every variable the defined settings refer to,
// through KindVariable settings and the variables inserted in KindTemplate
// settings. Settings without a value are skipped.
func (values Values) VariableReferences(definitions []Definition) ([]VariableReference, error) {
	var references []VariableReference
	for _, definition := range definitions {
		if !values.hasValue(definition.ID) {
			continue
		}
		switch definition.Kind {
		case KindVariable:
			reference, err := values.VariableReference(definition.ID)
			if err != nil {
				return nil, err
			}
			references = append(references, reference)
		case KindTemplate:
			template, err := values.Template(definition.ID)
			if err != nil {
				return nil, err
			}
			for _, part := range template {
				if part.Variable != nil {
					references = append(references, *part.Variable)
				}
			}
		}
	}
	return references, nil
}
