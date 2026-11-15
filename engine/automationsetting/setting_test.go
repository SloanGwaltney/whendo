package automationsetting_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
)

const settingID = "testSetting"

func valuesWith(rawJSON string) automationsetting.Values {
	return automationsetting.Values{settingID: json.RawMessage(rawJSON)}
}

func TestValues_ReadsEachKind(t *testing.T) {
	text, err := valuesWith(`"!discord"`).Text(settingID)
	if err != nil || text != "!discord" {
		t.Errorf("Text: got %q, error %v", text, err)
	}

	wholeNumber, err := valuesWith(`20`).WholeNumber(settingID)
	if err != nil || wholeNumber != 20 {
		t.Errorf("WholeNumber: got %d, error %v", wholeNumber, err)
	}

	duration, err := valuesWith(`1500`).Duration(settingID)
	if err != nil || duration != 1500*time.Millisecond {
		t.Errorf("Duration: got %v, error %v", duration, err)
	}

	option, err := valuesWith(`"is"`).Option(settingID)
	if err != nil || option != "is" {
		t.Errorf("Option: got %q, error %v", option, err)
	}

	choices, err := valuesWith(`[{"id":"c1","text":"!a1"},{"id":"c2","text":"!a2"}]`).ChoiceList(settingID)
	wantedChoices := []automationsetting.Choice{{ID: "c1", Text: "!a1"}, {ID: "c2", Text: "!a2"}}
	if err != nil || !slices.Equal(choices, wantedChoices) {
		t.Errorf("ChoiceList: got %v, error %v", choices, err)
	}

	reference, err := valuesWith(`{"source":"step","stepId":"s1","id":"roll"}`).VariableReference(settingID)
	wantedReference := automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: "s1", ID: "roll"}
	if err != nil || reference != wantedReference {
		t.Errorf("VariableReference: got %v, error %v", reference, err)
	}
}

func TestValues_ReturnsMissingForAbsentOrNullSetting(t *testing.T) {
	_, absentErr := automationsetting.Values{}.Text(settingID)
	_, nullErr := valuesWith(`null`).Text(settingID)

	if !errors.Is(absentErr, automationsetting.ErrSettingMissing) || !errors.Is(nullErr, automationsetting.ErrSettingMissing) {
		t.Errorf("got errors %v and %v, want ErrSettingMissing", absentErr, nullErr)
	}
}

func TestValues_ReturnsInvalidForWrongType(t *testing.T) {
	_, err := valuesWith(`"twenty"`).WholeNumber(settingID)

	if !errors.Is(err, automationsetting.ErrSettingInvalid) {
		t.Errorf("got error %v, want ErrSettingInvalid", err)
	}
}

func TestValues_ReturnsInvalidForFractionalWholeNumber(t *testing.T) {
	_, err := valuesWith(`1.5`).WholeNumber(settingID)

	if !errors.Is(err, automationsetting.ErrSettingInvalid) {
		t.Errorf("got error %v, want ErrSettingInvalid", err)
	}
}

func TestValues_ReturnsInvalidForNegativeDuration(t *testing.T) {
	_, err := valuesWith(`-1`).Duration(settingID)

	if !errors.Is(err, automationsetting.ErrSettingInvalid) {
		t.Errorf("got error %v, want ErrSettingInvalid", err)
	}
}

func TestValues_ReturnsInvalidForMalformedVariableReferences(t *testing.T) {
	malformedReferences := map[string]string{
		"no ID":                    `{"source":"trigger"}`,
		"unknown source":           `{"source":"weather","id":"rain"}`,
		"step source without step": `{"source":"step","id":"roll"}`,
		"trigger source with step": `{"source":"trigger","stepId":"s1","id":"redeemerDisplayName"}`,
	}

	for description, rawJSON := range malformedReferences {
		_, err := valuesWith(rawJSON).VariableReference(settingID)
		if !errors.Is(err, automationsetting.ErrSettingInvalid) {
			t.Errorf("%s: got error %v, want ErrSettingInvalid", description, err)
		}
	}
}

func TestValues_ReturnsInvalidForTemplatePartWithTextAndVariable(t *testing.T) {
	_, err := valuesWith(`[{"text":"hi","variable":{"source":"trigger","id":"redeemerDisplayName"}}]`).Template(settingID)

	if !errors.Is(err, automationsetting.ErrSettingInvalid) {
		t.Errorf("got error %v, want ErrSettingInvalid", err)
	}
}

func TestTemplate_RendersTextWithVariablesInserted(t *testing.T) {
	template, err := valuesWith(`[
		{"text": "Thanks for checking in, "},
		{"variable": {"source": "trigger", "id": "redeemerDisplayName"}},
		{"text": "! You rolled "},
		{"variable": {"source": "step", "stepId": "s1", "id": "roll"}}
	]`).Template(settingID)
	if err != nil {
		t.Fatalf("reading template: %v", err)
	}
	variables := automationsetting.VariableValues{
		{Source: automationsetting.VariableSourceTrigger, ID: "redeemerDisplayName"}: "Cooler_User",
		{Source: automationsetting.VariableSourceStep, StepID: "s1", ID: "roll"}:     "20",
	}

	renderedText, err := template.Render(variables)

	wantedText := "Thanks for checking in, Cooler_User! You rolled 20"
	if err != nil || renderedText != wantedText {
		t.Errorf("got %q, error %v; want %q", renderedText, err, wantedText)
	}
}

func TestTemplate_KeepsPlaceholderLikeTextAsTyped(t *testing.T) {
	template := automationsetting.Template{{Text: "use {redeemerDisplayName} or $roll"}}

	renderedText, err := template.Render(automationsetting.VariableValues{})

	if err != nil || renderedText != "use {redeemerDisplayName} or $roll" {
		t.Errorf("got %q, error %v", renderedText, err)
	}
}

func TestTemplate_ReturnsErrorForUnavailableVariable(t *testing.T) {
	template := automationsetting.Template{{Variable: &automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: "s1", ID: "roll"}}}

	_, err := template.Render(automationsetting.VariableValues{})

	if !errors.Is(err, automationsetting.ErrVariableUnavailable) {
		t.Errorf("got error %v, want ErrVariableUnavailable", err)
	}
}
