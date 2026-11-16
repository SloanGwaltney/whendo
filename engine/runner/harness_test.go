package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/runner"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// The fake trigger and steps let tests control exactly when events arrive
// and when steps finish.
const (
	eventTriggerID   = "test.event"
	wordSettingID    = "word"
	wordVariable     = "word"
	senderVariable   = "sender"
	sayStepType      = "test.say"
	messageSettingID = "message"
	blockStepType    = "test.block"
	chooseStepType   = "test.choose"
	outcomeSettingID = "outcome"
	firstOutcomeID   = "first"
	secondOutcomeID  = "second"
	chosenVariable   = "chosen"
	failStepType     = "test.fail"
	panicStepType    = "test.panic"
	waitStepType     = "test.waitForEvent"
	replierVariable  = "replier"
	replyWord        = "reply"
	badOutcomeType   = "test.badOutcome"

	failureMessage = "the light bulb burned out"
	waitTimeout    = 5 * time.Second
	// Long enough for a wrongly started run to show up, short enough to
	// keep the tests quick.
	quietPeriod = 50 * time.Millisecond
	bufferSize  = runner.MaximumQueuedRuns * 2
)

type eventTrigger struct{}

func (eventTrigger) Definition() trigger.Definition {
	return trigger.Definition{
		ID:       eventTriggerID,
		Sentence: automationsetting.Sentence{automationsetting.Setting(wordSettingID)},
		Settings: []automationsetting.Definition{{ID: wordSettingID, Kind: automationsetting.KindText, IsRequired: true}},
		Variables: []trigger.VariableDefinition{
			{ID: wordVariable, DisplayName: "Word"},
			{ID: senderVariable, DisplayName: "Sender"},
		},
	}
}

func (eventTrigger) Filter(event trigger.Event, settings automationsetting.Values) (map[string]string, bool, error) {
	word, err := settings.Text(wordSettingID)
	if err != nil {
		return nil, false, err
	}
	if event.TriggerID != eventTriggerID || event.Variables[wordVariable] != word {
		return nil, false, nil
	}
	return maps.Clone(event.Variables), true, nil
}

// fakeStep runs whatever function the harness gives it.
type fakeStep struct {
	definition step.Definition
	outcomes   []step.Outcome
	run        func(ctx context.Context, input step.Input) (step.Result, error)
}

func (fake fakeStep) Definition() step.Definition { return fake.definition }

func (fake fakeStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return fake.outcomes, nil
}

func (fake fakeStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	return fake.run(ctx, input)
}

func doneResult() step.Result {
	return step.Result{OutcomeID: step.DefaultOutcomeID}
}

type harness struct {
	t      *testing.T
	runner *runner.Runner
	// records receives every run record.
	records chan runner.RunRecord
	// said receives the message of every say step.
	said chan string
	// blockStarted receives a signal when a block step starts, and release
	// lets one block step finish.
	blockStarted chan struct{}
	release      chan struct{}
	// waitSubscribed receives a signal once a wait step has subscribed.
	waitSubscribed chan struct{}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	testHarness := &harness{
		t:              t,
		records:        make(chan runner.RunRecord, bufferSize),
		said:           make(chan string, bufferSize),
		blockStarted:   make(chan struct{}, bufferSize),
		release:        make(chan struct{}),
		waitSubscribed: make(chan struct{}, bufferSize),
	}
	catalog, err := automation.NewCatalog([]trigger.Trigger{eventTrigger{}}, testHarness.steps())
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	testHarness.runner = runner.New(runner.Config{
		Catalog:    catalog,
		OnRunEnded: func(record runner.RunRecord) { testHarness.records <- record },
	})
	t.Cleanup(testHarness.runner.Shutdown)
	return testHarness
}

func (testHarness *harness) steps() []step.Step {
	singleOutcome := []step.Outcome{{ID: step.DefaultOutcomeID}}
	return []step.Step{
		fakeStep{
			definition: step.Definition{ID: sayStepType, Sentence: automationsetting.Sentence{automationsetting.Setting(messageSettingID)}, Settings: []automationsetting.Definition{
				{ID: messageSettingID, Kind: automationsetting.KindTemplate, IsRequired: true},
			}},
			outcomes: singleOutcome,
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				template, err := input.Settings.Template(messageSettingID)
				if err != nil {
					return step.Result{}, err
				}
				message, err := template.Render(input.Variables)
				if err != nil {
					return step.Result{}, err
				}
				testHarness.said <- message
				return doneResult(), nil
			},
		},
		fakeStep{
			definition: step.Definition{ID: blockStepType},
			outcomes:   singleOutcome,
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				testHarness.blockStarted <- struct{}{}
				select {
				case <-testHarness.release:
					return doneResult(), nil
				case <-ctx.Done():
					return step.Result{}, ctx.Err()
				}
			},
		},
		fakeStep{
			definition: step.Definition{ID: chooseStepType, Sentence: automationsetting.Sentence{automationsetting.Setting(outcomeSettingID)}, Settings: []automationsetting.Definition{
				{ID: outcomeSettingID, Kind: automationsetting.KindOption, IsRequired: true, Options: []automationsetting.Option{
					{ID: firstOutcomeID}, {ID: secondOutcomeID},
				}},
			}},
			outcomes: []step.Outcome{
				{ID: firstOutcomeID, Variables: []trigger.VariableDefinition{{ID: chosenVariable}}},
				{ID: secondOutcomeID},
			},
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				outcomeID, err := input.Settings.Option(outcomeSettingID)
				if err != nil {
					return step.Result{}, err
				}
				return step.Result{OutcomeID: outcomeID, Variables: map[string]string{chosenVariable: "chosen-" + outcomeID}}, nil
			},
		},
		fakeStep{
			definition: step.Definition{ID: failStepType},
			outcomes:   singleOutcome,
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				return step.Result{}, errors.New(failureMessage)
			},
		},
		fakeStep{
			definition: step.Definition{ID: panicStepType},
			outcomes:   singleOutcome,
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				panic(failureMessage)
			},
		},
		fakeStep{
			definition: step.Definition{ID: waitStepType},
			outcomes:   []step.Outcome{{ID: step.DefaultOutcomeID, Variables: []trigger.VariableDefinition{{ID: replierVariable}}}},
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				subscription := input.Events.Subscribe()
				defer subscription.Unsubscribe()
				testHarness.waitSubscribed <- struct{}{}
				for {
					select {
					case event := <-subscription.Events():
						if event.Variables[wordVariable] == replyWord {
							return step.Result{OutcomeID: step.DefaultOutcomeID, Variables: map[string]string{replierVariable: event.Variables[senderVariable]}}, nil
						}
					case <-ctx.Done():
						return step.Result{}, ctx.Err()
					}
				}
			},
		},
		fakeStep{
			definition: step.Definition{ID: badOutcomeType},
			outcomes:   singleOutcome,
			run: func(ctx context.Context, input step.Input) (step.Result, error) {
				return step.Result{OutcomeID: "nonsense"}, nil
			},
		},
	}
}

func (testHarness *harness) encode(value any) json.RawMessage {
	testHarness.t.Helper()
	encodedValue, err := json.Marshal(value)
	if err != nil {
		testHarness.t.Fatalf("encoding %v: %v", value, err)
	}
	return encodedValue
}

func (testHarness *harness) newAutomation(automationID string, word string, steps ...automation.Step) automation.Automation {
	return automation.Automation{
		ID:        automationID,
		Name:      "Automation " + automationID,
		IsEnabled: true,
		Trigger: automation.TriggerSelection{
			ID:       eventTriggerID,
			Settings: automationsetting.Values{wordSettingID: testHarness.encode(word)},
		},
		Steps: steps,
	}
}

func (testHarness *harness) addAutomation(addedAutomation automation.Automation) {
	testHarness.t.Helper()
	err := testHarness.runner.SetAutomation(addedAutomation)
	if err != nil {
		testHarness.t.Fatalf("SetAutomation returned an error: %v", err)
	}
}

// sayStep says text followed by the values of the given variables.
func (testHarness *harness) sayStep(stepID string, text string, insertedVariables ...automationsetting.VariableReference) automation.Step {
	template := automationsetting.Template{{Text: text}}
	for _, variable := range insertedVariables {
		template = append(template, automationsetting.TemplatePart{Variable: &variable})
	}
	return automation.Step{ID: stepID, Type: sayStepType, Settings: automationsetting.Values{messageSettingID: testHarness.encode(template)}}
}

func (testHarness *harness) chooseStep(stepID string, outcomeID string, firstBranch automation.Branch, secondBranch automation.Branch) automation.Step {
	return automation.Step{
		ID:       stepID,
		Type:     chooseStepType,
		Settings: automationsetting.Values{outcomeSettingID: testHarness.encode(outcomeID)},
		Branches: map[string]automation.Branch{firstOutcomeID: firstBranch, secondOutcomeID: secondBranch},
	}
}

func simpleStep(stepID string, stepType string) automation.Step {
	return automation.Step{ID: stepID, Type: stepType}
}

func branch(afterBranch automation.AfterBranch, steps ...automation.Step) automation.Branch {
	return automation.Branch{Steps: steps, AfterBranch: afterBranch}
}

func triggerVariable(variableID string) automationsetting.VariableReference {
	return automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: variableID}
}

func stepVariable(stepID string, variableID string) automationsetting.VariableReference {
	return automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: stepID, ID: variableID}
}

func (testHarness *harness) sendEvent(word string, sender string) {
	testHarness.runner.ReceiveEvent(trigger.Event{
		TriggerID: eventTriggerID,
		Variables: map[string]string{wordVariable: word, senderVariable: sender},
	})
}

func (testHarness *harness) releaseBlock() {
	testHarness.t.Helper()
	select {
	case testHarness.release <- struct{}{}:
	case <-time.After(waitTimeout):
		testHarness.t.Fatal("no block step was waiting to be released")
	}
}

func awaitValue[Value any](t *testing.T, values chan Value, description string) Value {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(waitTimeout):
		t.Fatalf("timed out waiting for %s", description)
		var zeroValue Value
		return zeroValue
	}
}

func (testHarness *harness) awaitRecord() runner.RunRecord {
	testHarness.t.Helper()
	return awaitValue(testHarness.t, testHarness.records, "a run record")
}

func (testHarness *harness) awaitSaid() string {
	testHarness.t.Helper()
	return awaitValue(testHarness.t, testHarness.said, "a say step")
}

func (testHarness *harness) awaitBlockStarted() {
	testHarness.t.Helper()
	awaitValue(testHarness.t, testHarness.blockStarted, "a block step to start")
}

func assertNothingArrives[Value any](t *testing.T, values chan Value, description string) {
	t.Helper()
	select {
	case value := <-values:
		t.Errorf("got %s %v, want none", description, value)
	case <-time.After(quietPeriod):
	}
}
