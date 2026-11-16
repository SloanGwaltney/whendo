package automation_test

import (
	"context"
	"encoding/json"
	"maps"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// The fakes stand in for real triggers and steps, so these tests check the
// automation rules without depending on any integration.
const (
	redeemTriggerID      = "test.redeem"
	rewardSettingID      = "reward"
	rewardPlaceholder    = "pick a reward"
	redeemerNameVariable = "redeemerName"

	sayStepType          = "test.say"
	messageSettingID     = "message"
	messagePlaceholder   = "write a message"
	maximumMessageLength = 40
	rollStepType         = "test.roll"
	rollVariable         = "roll"
	checkStepType        = "test.check"
	checkedSettingID     = "checked"
	yesOutcomeID         = "yes"
	noOutcomeID          = "no"
	waitStepType         = "test.waitForReply"
	repliedOutcomeID     = "replied"
	timedOutOutcomeID    = "timedOut"
	replyVariable        = "reply"

	testAutomationID = "test-automation"

	// notingTriggerID is a trigger that notes when its reward is a
	// placeholder, like the Twitch redeem trigger.
	notingTriggerID    = "test.notingRedeem"
	placeholderReward  = "placeholder"
	placeholderMessage = "This only runs when you test it."
)

type fakeTrigger struct {
	definition trigger.Definition
}

func (fake fakeTrigger) Definition() trigger.Definition {
	return fake.definition
}

func (fake fakeTrigger) Filter(event trigger.Event, settings automationsetting.Values) (map[string]string, bool, error) {
	return maps.Clone(event.Variables), event.TriggerID == fake.definition.ID, nil
}

func (fake fakeTrigger) TestValues(settings automationsetting.Values, streamer trigger.Streamer) map[string]string {
	return map[string]string{}
}

// notingTrigger notes a placeholder reward, so tests can check how the
// check reports a trigger's notes.
type notingTrigger struct {
	fakeTrigger
}

func (noting notingTrigger) NoteSettings(settings automationsetting.Values) []trigger.SettingsNote {
	reward, err := settings.Text(rewardSettingID)
	if err != nil || reward != placeholderReward {
		return nil
	}
	return []trigger.SettingsNote{{SettingID: rewardSettingID, Message: placeholderMessage}}
}

type fakeStep struct {
	definition step.Definition
	outcomes   []step.Outcome
}

func (fake fakeStep) Definition() step.Definition {
	return fake.definition
}

func (fake fakeStep) Outcomes(settings automationsetting.Values) ([]step.Outcome, error) {
	return fake.outcomes, nil
}

func (fake fakeStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	return step.Result{OutcomeID: fake.outcomes[0].ID}, nil
}

func newTestCatalog(t *testing.T) *automation.Catalog {
	t.Helper()
	triggers := []trigger.Trigger{
		fakeTrigger{definition: trigger.Definition{
			ID:       redeemTriggerID,
			Sentence: automationsetting.Sentence{automationsetting.Words("someone redeems"), automationsetting.Setting(rewardSettingID)},
			Settings: []automationsetting.Definition{{ID: rewardSettingID, Kind: automationsetting.KindText, IsRequired: true, Placeholder: rewardPlaceholder}},
			Variables: []trigger.VariableDefinition{
				{ID: redeemerNameVariable, DisplayName: "Redeemer's name", IsCommon: true},
			},
		}},
		notingTrigger{fakeTrigger{definition: trigger.Definition{
			ID:       notingTriggerID,
			Sentence: automationsetting.Sentence{automationsetting.Words("someone redeems"), automationsetting.Setting(rewardSettingID)},
			Settings: []automationsetting.Definition{{ID: rewardSettingID, Kind: automationsetting.KindText, IsRequired: true, Placeholder: rewardPlaceholder}},
		}}},
	}
	steps := []step.Step{
		fakeStep{
			definition: step.Definition{
				ID:          sayStepType,
				DisplayName: "Say",
				Sentence:    automationsetting.Sentence{automationsetting.Words("Say"), automationsetting.Setting(messageSettingID)},
				Settings: []automationsetting.Definition{
					{ID: messageSettingID, DisplayName: "Message", Kind: automationsetting.KindTemplate, IsRequired: true, Placeholder: messagePlaceholder, MaximumLength: maximumMessageLength},
				},
			},
			outcomes: []step.Outcome{{ID: step.DefaultOutcomeID}},
		},
		fakeStep{
			definition: step.Definition{ID: rollStepType, DisplayName: "Roll"},
			outcomes: []step.Outcome{{ID: step.DefaultOutcomeID, Variables: []trigger.VariableDefinition{
				{ID: rollVariable, DisplayName: "Roll"},
			}}},
		},
		fakeStep{
			definition: step.Definition{
				ID:          checkStepType,
				DisplayName: "Check",
				Sentence:    automationsetting.Sentence{automationsetting.Words("Check"), automationsetting.Setting(checkedSettingID)},
				Settings: []automationsetting.Definition{
					{ID: checkedSettingID, DisplayName: "Checked", Kind: automationsetting.KindVariable, IsRequired: true},
				},
			},
			outcomes: []step.Outcome{{ID: yesOutcomeID, DisplayName: "Yes"}, {ID: noOutcomeID, DisplayName: "No"}},
		},
		fakeStep{
			definition: step.Definition{ID: waitStepType, DisplayName: "Wait for a reply"},
			outcomes: []step.Outcome{
				{ID: repliedOutcomeID, DisplayName: "Replied", Variables: []trigger.VariableDefinition{{ID: replyVariable, DisplayName: "Reply"}}},
				{ID: timedOutOutcomeID, DisplayName: "Timed out"},
			},
		},
	}
	catalog, err := automation.NewCatalog(triggers, steps)
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	return catalog
}

func encode(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encodedValue, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %v: %v", value, err)
	}
	return encodedValue
}

func triggerVariable(variableID string) automationsetting.VariableReference {
	return automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: variableID}
}

func stepVariable(stepID string, variableID string) automationsetting.VariableReference {
	return automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: stepID, ID: variableID}
}

func newAutomation(t *testing.T, steps ...automation.Step) automation.Automation {
	t.Helper()
	return automation.Automation{
		ID:        testAutomationID,
		Name:      "Test automation",
		IsEnabled: true,
		Trigger: automation.TriggerSelection{
			ID:       redeemTriggerID,
			Settings: automationsetting.Values{rewardSettingID: encode(t, "reward-1")},
		},
		Steps: steps,
	}
}

func sayStep(t *testing.T, stepID string, insertedVariable *automationsetting.VariableReference) automation.Step {
	t.Helper()
	template := automationsetting.Template{{Text: "Hello "}}
	if insertedVariable != nil {
		template = append(template, automationsetting.TemplatePart{Variable: insertedVariable})
	}
	return automation.Step{
		ID:       stepID,
		Type:     sayStepType,
		Settings: automationsetting.Values{messageSettingID: encode(t, template)},
	}
}

func rollStep(stepID string) automation.Step {
	return automation.Step{ID: stepID, Type: rollStepType}
}

func checkStep(t *testing.T, stepID string, checkedVariable automationsetting.VariableReference, yesBranch automation.Branch, noBranch automation.Branch) automation.Step {
	t.Helper()
	return automation.Step{
		ID:       stepID,
		Type:     checkStepType,
		Settings: automationsetting.Values{checkedSettingID: encode(t, checkedVariable)},
		Branches: map[string]automation.Branch{yesOutcomeID: yesBranch, noOutcomeID: noBranch},
	}
}

func waitStep(stepID string, repliedBranch automation.Branch, timedOutBranch automation.Branch) automation.Step {
	return automation.Step{
		ID:       stepID,
		Type:     waitStepType,
		Branches: map[string]automation.Branch{repliedOutcomeID: repliedBranch, timedOutOutcomeID: timedOutBranch},
	}
}

func branch(afterBranch automation.AfterBranch, steps ...automation.Step) automation.Branch {
	return automation.Branch{Steps: steps, AfterBranch: afterBranch}
}
