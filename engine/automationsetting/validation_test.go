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
	return values.Validate([]automationsetting.Definition{definition})
}

func TestValidate_AcceptsValidValueOfEachKind(t *testing.T) {
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

func TestValidate_ReturnsMissingForAbsentRequiredSetting(t *testing.T) {
	err := validate("", definitionOf(integrationKind, true))

	if !errors.Is(err, automationsetting.ErrSettingMissing) {
		t.Errorf("got error %v, want ErrSettingMissing", err)
	}
}

func TestValidate_AcceptsAbsentOptionalSetting(t *testing.T) {
	err := validate("", definitionOf(automationsetting.KindVariable, false))

	if err != nil {
		t.Errorf("got error %v", err)
	}
}

func TestValidate_ReturnsMissingForEmptyRequiredValues(t *testing.T) {
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

func TestValidate_ReturnsInvalidForOptionNotInList(t *testing.T) {
	definition := definitionOf(automationsetting.KindOption, true)
	definition.Options = []automationsetting.Option{{ID: "is", DisplayName: "is"}}

	err := validate(`"isProbably"`, definition)

	if !errors.Is(err, automationsetting.ErrSettingInvalid) {
		t.Errorf("got error %v, want ErrSettingInvalid", err)
	}
}

func TestValidate_ReturnsInvalidForBadChoices(t *testing.T) {
	badChoiceLists := map[string]string{
		"duplicate IDs": `[{"id":"c1","text":"!a1"},{"id":"c1","text":"!a2"}]`,
		"missing ID":    `[{"text":"!a1"}]`,
		"blank text":    `[{"id":"c1","text":"  "}]`,
	}

	for description, rawJSON := range badChoiceLists {
		err := validate(rawJSON, definitionOf(automationsetting.KindChoiceList, true))
		if !errors.Is(err, automationsetting.ErrSettingInvalid) {
			t.Errorf("%s: got error %v, want ErrSettingInvalid", description, err)
		}
	}
}

func TestValidate_IgnoresValuesWithoutDefinitions(t *testing.T) {
	values := automationsetting.Values{"droppedSetting": json.RawMessage(`"anything"`)}

	err := values.Validate(nil)

	if err != nil {
		t.Errorf("got error %v", err)
	}
}

func TestVariableReferences_CollectsFromVariableAndTemplateSettings(t *testing.T) {
	values := automationsetting.Values{
		"onlyFrom": json.RawMessage(`{"source":"trigger","id":"redeemerUserId"}`),
		"message":  json.RawMessage(`[{"text":"You rolled "},{"variable":{"source":"step","stepId":"s1","id":"roll"}}]`),
		"note":     json.RawMessage(`"plain text"`),
	}
	definitions := []automationsetting.Definition{
		{ID: "onlyFrom", Kind: automationsetting.KindVariable},
		{ID: "message", Kind: automationsetting.KindTemplate},
		{ID: "note", Kind: automationsetting.KindText},
		{ID: "unset", Kind: automationsetting.KindVariable},
	}

	references, err := values.VariableReferences(definitions)

	wantedReferences := []automationsetting.VariableReference{
		{Source: automationsetting.VariableSourceTrigger, ID: "redeemerUserId"},
		{Source: automationsetting.VariableSourceStep, StepID: "s1", ID: "roll"},
	}
	if err != nil || !slices.Equal(references, wantedReferences) {
		t.Errorf("got %v, error %v; want %v", references, err, wantedReferences)
	}
}
