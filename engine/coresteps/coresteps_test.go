package coresteps_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/coresteps"
	"github.com/SloanGwaltney/whendo/engine/step"
)

const (
	// A 20-sided die rolled this many times shows every face with near
	// certainty: the chance of missing one is about 20 × (19/20)^2000.
	rollCount = 2000
	dieSides  = 20

	shortWait        = 10 * time.Millisecond
	longWait         = time.Hour
	cancelDeadline   = time.Second
	checkedStepID    = "roll"
	checkedVariable  = "number"
	otherVariable    = "other"
	triggerVariable  = "userInput"
	missingVariable  = "missing"
	nonNumberMessage = "isn't one"
)

func findStep(t *testing.T, stepID string) step.Step {
	t.Helper()
	for _, coreStep := range coresteps.Steps() {
		if coreStep.Definition().ID == stepID {
			return coreStep
		}
	}
	t.Fatalf("no core step %q", stepID)
	return nil
}

func encode(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encodedValue, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %v: %v", value, err)
	}
	return encodedValue
}

func TestWait_FinishesAfterDuration(t *testing.T) {
	settings := automationsetting.Values{coresteps.WaitDurationSetting: encode(t, shortWait.Milliseconds())}
	startedAt := time.Now()

	result, err := findStep(t, coresteps.WaitStepID).Run(context.Background(), step.Input{Settings: settings})

	if err != nil || result.OutcomeID != step.DefaultOutcomeID {
		t.Fatalf("got result %+v, error %v", result, err)
	}
	if elapsed := time.Since(startedAt); elapsed < shortWait {
		t.Errorf("finished after %v, want at least %v", elapsed, shortWait)
	}
}

func TestWait_StopsWhenCancelled(t *testing.T) {
	settings := automationsetting.Values{coresteps.WaitDurationSetting: encode(t, longWait.Milliseconds())}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startedAt := time.Now()

	_, err := findStep(t, coresteps.WaitStepID).Run(ctx, step.Input{Settings: settings})

	if !errors.Is(err, context.Canceled) || time.Since(startedAt) > cancelDeadline {
		t.Errorf("got error %v after %v, want an immediate cancellation", err, time.Since(startedAt))
	}
}

func roll(t *testing.T, lowest int64, highest int64) (int64, error) {
	t.Helper()
	settings := automationsetting.Values{
		coresteps.LowestSetting:  encode(t, lowest),
		coresteps.HighestSetting: encode(t, highest),
	}
	result, err := findStep(t, coresteps.RandomNumberStepID).Run(context.Background(), step.Input{Settings: settings})
	if err != nil {
		return 0, err
	}
	rolled, err := strconv.ParseInt(result.Variables[coresteps.RandomNumberVariable], 10, 64)
	if err != nil {
		t.Fatalf("the roll %q isn't a whole number: %v", result.Variables[coresteps.RandomNumberVariable], err)
	}
	return rolled, nil
}

func TestRandomNumber_RollsEveryValueInRangeAndNothingOutside(t *testing.T) {
	seenRolls := map[int64]bool{}

	for range rollCount {
		rolled, err := roll(t, 1, dieSides)
		if err != nil {
			t.Fatalf("roll returned an error: %v", err)
		}
		if rolled < 1 || rolled > dieSides {
			t.Fatalf("rolled %d, outside 1 to %d", rolled, dieSides)
		}
		seenRolls[rolled] = true
	}

	if len(seenRolls) != dieSides {
		t.Errorf("saw %d different rolls, want all %d", len(seenRolls), dieSides)
	}
}

func TestRandomNumber_HandlesSingleValueNegativeAndHugeRanges(t *testing.T) {
	if rolled, err := roll(t, 7, 7); err != nil || rolled != 7 {
		t.Errorf("7 to 7: got %d, error %v", rolled, err)
	}
	if rolled, err := roll(t, -10, -5); err != nil || rolled < -10 || rolled > -5 {
		t.Errorf("-10 to -5: got %d, error %v", rolled, err)
	}
	if _, err := roll(t, -1<<63, 1<<63-1); err != nil {
		t.Errorf("the whole range: got error %v", err)
	}
}

func TestRandomNumber_FailsWhenLowestIsAboveHighest(t *testing.T) {
	_, err := roll(t, 20, 1)

	if err == nil {
		t.Error("got no error")
	}
}

func checkInput(t *testing.T, checked automationsetting.VariableReference, comparison string, comparedValue automationsetting.Template, variables automationsetting.VariableValues) step.Input {
	t.Helper()
	settings := automationsetting.Values{
		coresteps.CheckedVariableSetting: encode(t, checked),
		coresteps.ComparisonSetting:      encode(t, comparison),
	}
	if comparedValue != nil {
		settings[coresteps.ComparedValueSetting] = encode(t, comparedValue)
	}
	return step.Input{Settings: settings, Variables: variables}
}

func rollReference() automationsetting.VariableReference {
	return automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: checkedStepID, ID: checkedVariable}
}

func check(t *testing.T, checkedValue string, comparison string, comparedText string) (string, error) {
	t.Helper()
	variables := automationsetting.VariableValues{rollReference(): checkedValue}
	input := checkInput(t, rollReference(), comparison, automationsetting.Template{{Text: comparedText}}, variables)
	result, err := findStep(t, coresteps.CheckConditionStepID).Run(context.Background(), input)
	return result.OutcomeID, err
}

func TestCheckCondition_ComparesTextAndNumbers(t *testing.T) {
	cases := []struct {
		checkedValue  string
		comparison    string
		comparedText  string
		wantedOutcome string
	}{
		{"20", coresteps.ComparisonIs, "20", coresteps.ConditionTrueOutcomeID},
		{"19", coresteps.ComparisonIs, "20", coresteps.ConditionFalseOutcomeID},
		{"Yes", coresteps.ComparisonIs, " yes ", coresteps.ConditionTrueOutcomeID},
		{"yes", coresteps.ComparisonIsNot, "YES", coresteps.ConditionFalseOutcomeID},
		{"no", coresteps.ComparisonIsNot, "yes", coresteps.ConditionTrueOutcomeID},
		{"20", coresteps.ComparisonIsMoreThan, "19.5", coresteps.ConditionTrueOutcomeID},
		{"9", coresteps.ComparisonIsMoreThan, "10", coresteps.ConditionFalseOutcomeID},
		{"-3", coresteps.ComparisonIsLessThan, "0", coresteps.ConditionTrueOutcomeID},
		{"5", coresteps.ComparisonIsLessThan, "5", coresteps.ConditionFalseOutcomeID},
	}

	for _, testCase := range cases {
		outcome, err := check(t, testCase.checkedValue, testCase.comparison, testCase.comparedText)
		if err != nil || outcome != testCase.wantedOutcome {
			t.Errorf("%q %s %q: got %q, error %v; want %q", testCase.checkedValue, testCase.comparison, testCase.comparedText, outcome, err, testCase.wantedOutcome)
		}
	}
}

func TestCheckCondition_FailsComparingNonNumbersByAmount(t *testing.T) {
	for _, nonNumber := range []string{"twenty", "NaN", "Inf", ""} {
		_, err := check(t, nonNumber, coresteps.ComparisonIsMoreThan, "1")
		if err == nil {
			t.Errorf("%q: got no error", nonNumber)
		}
	}
}

func TestCheckCondition_ComparesWithOtherVariables(t *testing.T) {
	other := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: otherVariable}
	variables := automationsetting.VariableValues{rollReference(): "12", other: "12"}
	input := checkInput(t, rollReference(), coresteps.ComparisonIs, automationsetting.Template{{Variable: &other}}, variables)

	result, err := findStep(t, coresteps.CheckConditionStepID).Run(context.Background(), input)

	if err != nil || result.OutcomeID != coresteps.ConditionTrueOutcomeID {
		t.Errorf("got %+v, error %v", result, err)
	}
}

func TestCheckCondition_WithoutValueChecksForEmpty(t *testing.T) {
	userInput := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: triggerVariable}
	emptyInput := checkInput(t, userInput, coresteps.ComparisonIs, nil, automationsetting.VariableValues{userInput: "  "})
	filledInput := checkInput(t, userInput, coresteps.ComparisonIs, nil, automationsetting.VariableValues{userInput: "hello"})
	checkStep := findStep(t, coresteps.CheckConditionStepID)

	emptyResult, emptyErr := checkStep.Run(context.Background(), emptyInput)
	filledResult, filledErr := checkStep.Run(context.Background(), filledInput)

	if emptyErr != nil || emptyResult.OutcomeID != coresteps.ConditionTrueOutcomeID {
		t.Errorf("empty: got %+v, error %v", emptyResult, emptyErr)
	}
	if filledErr != nil || filledResult.OutcomeID != coresteps.ConditionFalseOutcomeID {
		t.Errorf("filled: got %+v, error %v", filledResult, filledErr)
	}
}

func TestCheckCondition_FailsWhenCheckedVariableIsUnavailable(t *testing.T) {
	missing := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: missingVariable}
	input := checkInput(t, missing, coresteps.ComparisonIs, nil, automationsetting.VariableValues{})

	_, err := findStep(t, coresteps.CheckConditionStepID).Run(context.Background(), input)

	if !errors.Is(err, automationsetting.ErrVariableUnavailable) {
		t.Errorf("got error %v, want ErrVariableUnavailable", err)
	}
}
