package coresteps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
)

const (
	// CheckConditionStepID is the step that splits on whether a variable
	// compares to a value, such as "the roll is 20".
	CheckConditionStepID = "core.checkCondition"
	// CheckedVariableSetting is the variable being checked.
	CheckedVariableSetting = "variable"
	// ComparisonSetting is how the variable is compared, one of the
	// Comparison values.
	ComparisonSetting = "comparison"
	// ComparedValueSetting is what the variable is compared with. It's a
	// template, so it can include other variables, and it may be empty to
	// check that a variable is empty.
	ComparedValueSetting = "value"

	// ConditionTrueOutcomeID and ConditionFalseOutcomeID are the outcomes of
	// the check.
	ConditionTrueOutcomeID  = "yes"
	ConditionFalseOutcomeID = "no"
)

// Comparison option IDs for ComparisonSetting.
const (
	ComparisonIs         = "is"
	ComparisonIsNot      = "isNot"
	ComparisonIsMoreThan = "isMoreThan"
	ComparisonIsLessThan = "isLessThan"
)

type checkConditionStep struct{}

func (checkConditionStep) Definition() step.Definition {
	return step.Definition{
		ID:          CheckConditionStepID,
		DisplayName: "Check a condition",
		Settings: []automationsetting.Definition{
			{ID: CheckedVariableSetting, DisplayName: "Check", Kind: automationsetting.KindVariable, IsRequired: true},
			{ID: ComparisonSetting, DisplayName: "Comparison", Kind: automationsetting.KindOption, IsRequired: true, Options: []automationsetting.Option{
				{ID: ComparisonIs, DisplayName: "is"},
				{ID: ComparisonIsNot, DisplayName: "is not"},
				{ID: ComparisonIsMoreThan, DisplayName: "is more than"},
				{ID: ComparisonIsLessThan, DisplayName: "is less than"},
			}},
			{ID: ComparedValueSetting, DisplayName: "Value", Kind: automationsetting.KindTemplate},
		},
	}
}

func (checkConditionStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return []step.Outcome{
		{ID: ConditionTrueOutcomeID, DisplayName: "Yes"},
		{ID: ConditionFalseOutcomeID, DisplayName: "No"},
	}, nil
}

// Run compares the variable with the value and takes the yes or no outcome.
func (checkConditionStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	checkedValue, err := readCheckedValue(input)
	if err != nil {
		return step.Result{}, err
	}
	comparison, err := input.Settings.Option(ComparisonSetting)
	if err != nil {
		return step.Result{}, fmt.Errorf("reading the comparison: %w", err)
	}
	comparedValue, err := readComparedValue(input)
	if err != nil {
		return step.Result{}, err
	}
	isConditionMet, err := compare(checkedValue, comparison, comparedValue)
	if err != nil {
		return step.Result{}, err
	}
	if isConditionMet {
		return step.Result{OutcomeID: ConditionTrueOutcomeID}, nil
	}
	return step.Result{OutcomeID: ConditionFalseOutcomeID}, nil
}

func readCheckedValue(input step.Input) (string, error) {
	reference, err := input.Settings.VariableReference(CheckedVariableSetting)
	if err != nil {
		return "", fmt.Errorf("reading which variable to check: %w", err)
	}
	return input.Variables.Value(reference)
}

// readComparedValue treats a missing value as empty, so "is" with no value
// checks that the variable is empty.
func readComparedValue(input step.Input) (string, error) {
	template, err := input.Settings.Template(ComparedValueSetting)
	if err != nil {
		if errors.Is(err, automationsetting.ErrSettingMissing) {
			return "", nil
		}
		return "", fmt.Errorf("reading the value to compare with: %w", err)
	}
	return template.Render(input.Variables)
}

// compare ignores spaces around both values, which users can't see, and
// capitalization, so "Yes" is "yes". "More than" and "less than" compare
// numbers, and fail if either side isn't one.
func compare(checkedValue string, comparison string, comparedValue string) (bool, error) {
	checkedValue = strings.TrimSpace(checkedValue)
	comparedValue = strings.TrimSpace(comparedValue)
	switch comparison {
	case ComparisonIs:
		return strings.EqualFold(checkedValue, comparedValue), nil
	case ComparisonIsNot:
		return !strings.EqualFold(checkedValue, comparedValue), nil
	case ComparisonIsMoreThan, ComparisonIsLessThan:
		return compareNumbers(checkedValue, comparison, comparedValue)
	default:
		return false, fmt.Errorf("unknown comparison %q", comparison)
	}
}

func compareNumbers(checkedValue string, comparison string, comparedValue string) (bool, error) {
	checkedNumber, err := parseNumber(checkedValue)
	if err != nil {
		return false, err
	}
	comparedNumber, err := parseNumber(comparedValue)
	if err != nil {
		return false, err
	}
	if comparison == ComparisonIsMoreThan {
		return checkedNumber > comparedNumber, nil
	}
	return checkedNumber < comparedNumber, nil
}

// parseNumber accepts only ordinary numbers. Go also reads "NaN" and "Inf"
// as numbers, but to a streamer those are words, and comparing them would
// give surprising answers.
func parseNumber(value string) (float64, error) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("can't compare %q as a number, because it isn't one", value)
	}
	return number, nil
}
