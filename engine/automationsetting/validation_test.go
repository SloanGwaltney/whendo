package automationsetting_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
)

const integrationKind automationsetting.Kind = "test.reward"

func definitionOf(kind automationsetting.Kind, isRequired bool) automationsetting.Definition {
	return automationsetting.Definition{ID: settingID, DisplayName: "Test setting", Kind: kind, IsRequired: isRequired}
}

func validate(rawJSON string, definition automationsetting.Definition) error {
	values := automationsetting.Values{}
	if rawJSON != "" {
		values[settingID] = json.RawMessage(rawJSON)
	}
	return values.ValidateSetting(definition)
}

func TestValidateSetting_AcceptsValidValueOfEachKind(t *testing.T) {
	comparisonDefinition := definitionOf(automationsetting.KindOption, true)
	comparisonDefinition.Options = []automationsetting.Option{{ID: "is", DisplayName: "is"}}
	validValues := []struct {
		rawJSON    string
		definition automationsetting.Definition
	}{
		{`"!discord"`, definitionOf(automationsetting.KindText, true)},
		{`[{"text":"hi "},{"variable":{"source":"trigger","id":"redeemerDisplayName"}}]`, definitionOf(automationsetting.KindTemplate, true)},
		{`20`, definitionOf(automationsetting.KindWholeNumber, true)},
		{`30000`, definitionOf(automationsetting.KindDuration, true)},
		{`{"source":"trigger","id":"redeemerUserId"}`, definitionOf(automationsetting.KindVariable, true)},
		{`"is"`, comparisonDefinition},
		{`[{"id":"c1","text":"!a1"}]`, definitionOf(automationsetting.KindChoiceList, true)},
		{`"any-reward-id"`, definitionOf(integrationKind, true)},
	}

	for _, valid := range validValues {
		err := validate(valid.rawJSON, valid.definition)
		if err != nil {
			t.Errorf("kind %q with %s: got error %v", valid.definition.Kind, valid.rawJSON, err)
		}
	}
}

func TestValidateSetting_ReturnsMissingForAbsentRequiredSetting(t *testing.T) {
	err := validate("", definitionOf(integrationKind, true))

	if !errors.Is(err, automationsetting.ErrSettingMissing) {
		t.Errorf("got error %v, want ErrSettingMissing", err)
	}
}

func TestValidateSetting_AcceptsAbsentOptionalSetting(t *testing.T) {
	err := validate("", definitionOf(automationsetting.KindVariable, false))

	if err != nil {
		t.Errorf("got error %v", err)
	}
}

func TestValidateSetting_ReturnsMissingForEmptyRequiredValues(t *testing.T) {
	emptyValues := map[automationsetting.Kind]string{
		automationsetting.KindText:       `"   "`,
		automationsetting.KindTemplate:   `[]`,
		automationsetting.KindChoiceList: `[]`,
	}

	for kind, rawJSON := range emptyValues {
		err := validate(rawJSON, definitionOf(kind, true))
		if !errors.Is(err, automationsetting.ErrSettingMissing) {
			t.Errorf("kind %q with %s: got error %v, want ErrSettingMissing", kind, rawJSON, err)
		}
	}
}

func TestValidateSetting_ReturnsInvalidForOptionNotInList(t *testing.T) {
	definition := definitionOf(automationsetting.KindOption, true)
	definition.Options = []automationsetting.Option{{ID: "is", DisplayName: "is"}}

	err := validate(`"isProbably"`, definition)

	if !errors.Is(err, automationsetting.ErrSettingInvalid) {
		t.Errorf("got error %v, want ErrSettingInvalid", err)
	}
}

func TestValidateSetting_ReturnsInvalidForBadChoices(t *testing.T) {
	badChoiceLists := map[string]string{
		"duplicate IDs": `[{"id":"c1","text":"!a1"},{"id":"c1","text":"!a2"}]`,
		"missing ID":    `[{"text":"!a1"}]`,
		"blank text":    `[{"id":"c1","text":"  "}]`,
		"repeated text": `[{"id":"c1","text":"!a1"},{"id":"c2","text":"!A1 "}]`,
	}

	for description, rawJSON := range badChoiceLists {
		err := validate(rawJSON, definitionOf(automationsetting.KindChoiceList, true))
		if !errors.Is(err, automationsetting.ErrSettingInvalid) {
			t.Errorf("%s: got error %v, want ErrSettingInvalid", description, err)
		}
	}
}

func TestValidateSetting_TellsBlankAndRepeatedChoicesApart(t *testing.T) {
	definition := definitionOf(automationsetting.KindChoiceList, true)

	blankErr := validate(`[{"id":"c1","text":" "}]`, definition)
	repeatedErr := validate(`[{"id":"c1","text":"!a1"},{"id":"c2","text":"!A1"}]`, definition)

	if !errors.Is(blankErr, automationsetting.ErrBlankChoice) || !errors.Is(repeatedErr, automationsetting.ErrRepeatedChoice) {
		t.Errorf("got errors %v and %v, want ErrBlankChoice and ErrRepeatedChoice", blankErr, repeatedErr)
	}
}

func TestValidateSetting_ReturnsMissingForTemplateOfOnlySpaces(t *testing.T) {
	err := validate(`[{"text":"   "}]`, definitionOf(automationsetting.KindTemplate, true))

	if !errors.Is(err, automationsetting.ErrSettingMissing) {
		t.Errorf("got error %v, want ErrSettingMissing", err)
	}
}

func TestValidateSetting_ReturnsTooLongPastMaximumLength(t *testing.T) {
	textDefinition := definitionOf(automationsetting.KindText, true)
	textDefinition.MaximumLength = 3
	templateDefinition := definitionOf(automationsetting.KindTemplate, true)
	templateDefinition.MaximumLength = 3

	textErr := validate(`"abcd"`, textDefinition)
	templateErr := validate(`[{"text":"ab"},{"text":"cd"}]`, templateDefinition)

	if !errors.Is(textErr, automationsetting.ErrSettingTooLong) || !errors.Is(templateErr, automationsetting.ErrSettingTooLong) {
		t.Errorf("got errors %v and %v, want ErrSettingTooLong", textErr, templateErr)
	}
}

func TestValidateSetting_ReturnsDurationTooLongPastMaximumOnly(t *testing.T) {
	definition := definitionOf(automationsetting.KindDuration, true)
	definition.MaximumMilliseconds = 300000

	atMaximumErr := validate(`300000`, definition)
	pastMaximumErr := validate(`300001`, definition)

	if atMaximumErr != nil {
		t.Errorf("got error %v at the maximum, want none", atMaximumErr)
	}
	if !errors.Is(pastMaximumErr, automationsetting.ErrDurationTooLong) {
		t.Errorf("got error %v past the maximum, want ErrDurationTooLong", pastMaximumErr)
	}
}

func TestValidateSetting_CountsCharactersAndLeavesOutVariables(t *testing.T) {
	definition := definitionOf(automationsetting.KindTemplate, true)
	definition.MaximumLength = 3

	// Each "é" is two bytes but one character, and the variable's value
	// isn't known yet, so this is three characters.
	err := validate(`[{"text":"éé"},{"variable":{"source":"trigger","id":"redeemerDisplayName"}},{"text":"é"}]`, definition)

	if err != nil {
		t.Errorf("got error %v, want the template to fit", err)
	}
}

func TestVariableReferences_CollectsFromVariableAndTemplateSettings(t *testing.T) {
	values := automationsetting.Values{
		"onlyFrom": json.RawMessage(`{"source":"trigger","id":"redeemerUserId"}`),
		"message":  json.RawMessage(`[{"text":"You rolled "},{"variable":{"source":"step","stepId":"s1","id":"roll"}}]`),
		"note":     json.RawMessage(`"plain text"`),
	}
	wantedReferences := map[string][]automationsetting.VariableReference{
		"onlyFrom": {{Source: automationsetting.VariableSourceTrigger, ID: "redeemerUserId"}},
		"message":  {{Source: automationsetting.VariableSourceStep, StepID: "s1", ID: "roll"}},
		"note":     nil,
		"unset":    nil,
	}
	kinds := map[string]automationsetting.Kind{
		"onlyFrom": automationsetting.KindVariable,
		"message":  automationsetting.KindTemplate,
		"note":     automationsetting.KindText,
		"unset":    automationsetting.KindVariable,
	}

	for settingID, wanted := range wantedReferences {
		references, err := values.VariableReferences(automationsetting.Definition{ID: settingID, Kind: kinds[settingID]})
		if err != nil || !slices.Equal(references, wanted) {
			t.Errorf("setting %q: got %v, error %v; want %v", settingID, references, err, wanted)
		}
	}
}
