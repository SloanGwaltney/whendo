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
	redeemerNameVariable = "redeemerName"

	sayStepType       = "test.say"
	messageSettingID  = "message"
	rollStepType      = "test.roll"
	rollVariable      = "roll"
	checkStepType     = "test.check"
	checkedSettingID  = "checked"
	yesOutcomeID      = "yes"
	noOutcomeID       = "no"
	waitStepType      = "test.waitForReply"
	repliedOutcomeID  = "replied"
	timedOutOutcomeID = "timedOut"
	replyVariable     = "reply"

	testAutomationID = "test-automation"
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
			Settings: []automationsetting.Definition{{ID: rewardSettingID, Kind: automationsetting.KindText, IsRequired: true}},
			Variables: []trigger.VariableDefinition{
				{ID: redeemerNameVariable, DisplayName: "Redeemer's name"},
			},
		}},
	}
	steps := []step.Step{
		fakeStep{
			definition: step.Definition{ID: sayStepType, Settings: []automationsetting.Definition{
				{ID: messageSettingID, Kind: automationsetting.KindTemplate, IsRequired: true},
			}},
			outcomes: []step.Outcome{{ID: step.DefaultOutcomeID}},
		},
		fakeStep{
			definition: step.Definition{ID: rollStepType},
			outcomes: []step.Outcome{{ID: step.DefaultOutcomeID, Variables: []trigger.VariableDefinition{
				{ID: rollVariable, DisplayName: "Roll"},
			}}},
		},
		fakeStep{
			definition: step.Definition{ID: checkStepType, Settings: []automationsetting.Definition{
				{ID: checkedSettingID, Kind: automationsetting.KindVariable, IsRequired: true},
			}},
			outcomes: []step.Outcome{{ID: yesOutcomeID}, {ID: noOutcomeID}},
		},
		fakeStep{
			definition: step.Definition{ID: waitStepType},
			outcomes: []step.Outcome{
				{ID: repliedOutcomeID, Variables: []trigger.VariableDefinition{{ID: replyVariable, DisplayName: "Reply"}}},
				{ID: timedOutOutcomeID},
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
