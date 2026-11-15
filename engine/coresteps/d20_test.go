package coresteps_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/coresteps"
	"github.com/SloanGwaltney/whendo/engine/runner"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

// This test builds the MVP user's D20 Mute Me redeem as test data, the way
// the designer will, and runs it through the real redeem trigger, the core
// steps and the runner. A fake step stands in for sending a chat message
// until that step exists.
const (
	fakeSayStepID = "test.say"
	sayMessage    = "message"
	d20RewardID   = "d20-reward"
	// Enough redeems that both branches run: the chance of never rolling a
	// 20 is (19/20)^200, about 1 in 30,000.
	d20Rolls       = 200
	recordDeadline = 5 * time.Second
	natural20Text  = "NATURAL 20! "
	otherRollText  = "Rolled "
)

type fakeSayStep struct {
	said chan string
}

func (fakeSayStep) Definition() step.Definition {
	return step.Definition{ID: fakeSayStepID, Settings: []automationsetting.Definition{
		{ID: sayMessage, Kind: automationsetting.KindTemplate, IsRequired: true},
	}}
}

func (fakeSayStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return []step.Outcome{{ID: step.DefaultOutcomeID}}, nil
}

func (fake fakeSayStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	template, err := input.Settings.Template(sayMessage)
	if err != nil {
		return step.Result{}, err
	}
	message, err := template.Render(input.Variables)
	if err != nil {
		return step.Result{}, err
	}
	fake.said <- message
	return step.Result{OutcomeID: step.DefaultOutcomeID}, nil
}

func d20MuteMe(t *testing.T) automation.Automation {
	t.Helper()
	roll := automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: "roll", ID: coresteps.RandomNumberVariable}
	redeemer := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: twitcheventsub.RedeemerDisplayNameVariable}
	sayStep := func(stepID string, text string) automation.Step {
		template := automationsetting.Template{{Text: text}, {Variable: &roll}, {Text: " for "}, {Variable: &redeemer}}
		return automation.Step{ID: stepID, Type: fakeSayStepID, Settings: automationsetting.Values{sayMessage: encode(t, template)}}
	}
	return automation.Automation{
		ID:        "d20-mute-me",
		Name:      "D20 Mute Me",
		IsEnabled: true,
		Trigger: automation.TriggerSelection{
			ID:       twitcheventsub.RedeemTriggerID,
			Settings: automationsetting.Values{twitcheventsub.RewardSetting: encode(t, d20RewardID)},
		},
		Steps: []automation.Step{
			{ID: "roll", Type: coresteps.RandomNumberStepID, Settings: automationsetting.Values{
				coresteps.LowestSetting:  encode(t, 1),
				coresteps.HighestSetting: encode(t, dieSides),
			}},
			{
				ID:   "isNatural20",
				Type: coresteps.CheckConditionStepID,
				Settings: automationsetting.Values{
					coresteps.CheckedVariableSetting: encode(t, roll),
					coresteps.ComparisonSetting:      encode(t, coresteps.ComparisonIs),
					coresteps.ComparedValueSetting:   encode(t, automationsetting.Template{{Text: "20"}}),
				},
				Branches: map[string]automation.Branch{
					coresteps.ConditionTrueOutcomeID:  {Steps: []automation.Step{sayStep("sayNatural20", natural20Text)}, AfterBranch: automation.AfterBranchEnd},
					coresteps.ConditionFalseOutcomeID: {Steps: []automation.Step{sayStep("sayOther", otherRollText)}, AfterBranch: automation.AfterBranchEnd},
				},
			},
		},
	}
}

func TestD20MuteMe_RunsThroughTheRunner(t *testing.T) {
	said := make(chan string, d20Rolls)
	records := make(chan runner.RunRecord, d20Rolls)
	steps := append(coresteps.Steps(), fakeSayStep{said: said})
	catalog, err := automation.NewCatalog(twitcheventsub.Triggers(), steps)
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	automationRunner := runner.New(runner.Config{Catalog: catalog, OnRunEnded: func(record runner.RunRecord) { records <- record }})
	defer automationRunner.Shutdown()
	err = automationRunner.SetAutomation(d20MuteMe(t))
	if err != nil {
		t.Fatalf("SetAutomation returned an error: %v", err)
	}

	for range d20Rolls {
		automationRunner.ReceiveEvent(trigger.Event{
			TriggerID: twitcheventsub.RedeemTriggerID,
			Variables: map[string]string{
				twitcheventsub.RewardIDVariable:            d20RewardID,
				twitcheventsub.RedeemerDisplayNameVariable: "Cooler_User",
			},
		})
	}

	sawNatural20, sawOtherRoll := false, false
	for range d20Rolls {
		select {
		case record := <-records:
			if record.Status != runner.RunStatusSucceeded {
				t.Fatalf("got record %+v, want success", record)
			}
		case <-time.After(recordDeadline):
			t.Fatal("timed out waiting for a run to end")
		}
		message := <-said
		switch {
		case message == natural20Text+"20 for Cooler_User":
			sawNatural20 = true
		case strings.HasPrefix(message, otherRollText) && strings.HasSuffix(message, " for Cooler_User") && message != otherRollText+"20 for Cooler_User":
			sawOtherRoll = true
		default:
			t.Errorf("said %q, which neither branch should", message)
		}
	}
	if !sawNatural20 || !sawOtherRoll {
		t.Errorf("saw a natural 20: %v, saw another roll: %v; want both branches taken", sawNatural20, sawOtherRoll)
	}
}
