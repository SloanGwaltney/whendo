package coresteps

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

const (
	// RandomNumberStepID is the step that rolls a random number.
	RandomNumberStepID = "core.randomNumber"
	// LowestSetting and HighestSetting are the range of the roll. Both are
	// included, so 1 to 20 rolls a 20-sided die.
	LowestSetting  = "lowest"
	HighestSetting = "highest"
	// RandomNumberVariable is the number rolled.
	RandomNumberVariable = "number"
)

type randomNumberStep struct{}

func (randomNumberStep) Definition() step.Definition {
	return step.Definition{
		ID:          RandomNumberStepID,
		DisplayName: "Roll a random number",
		Description: "Pick a number in a range, like rolling a die.",
		Group:       numbersAndChoicesGroup,
		Sentence: automationsetting.Sentence{
			automationsetting.Words("Roll a random number from"),
			automationsetting.Setting(LowestSetting),
			automationsetting.Words("to"),
			automationsetting.Setting(HighestSetting),
		},
		Settings: []automationsetting.Definition{
			{ID: LowestSetting, DisplayName: "Lowest", Kind: automationsetting.KindWholeNumber, IsRequired: true, Placeholder: "choose the lowest number"},
			{ID: HighestSetting, DisplayName: "Highest", Kind: automationsetting.KindWholeNumber, IsRequired: true, Placeholder: "choose the highest number"},
		},
	}
}

func (randomNumberStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return []step.Outcome{{
		ID:          step.DefaultOutcomeID,
		DisplayName: "Done",
		Variables:   []trigger.VariableDefinition{{ID: RandomNumberVariable, DisplayName: "Random number", IsCommon: true}},
	}}, nil
}

// Run rolls a whole number between the lowest and highest, both included.
// A lowest above the highest fails the step rather than being swapped,
// since it's most likely a typo the streamer should see.
func (randomNumberStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	lowest, err := input.Settings.WholeNumber(LowestSetting)
	if err != nil {
		return step.Result{}, fmt.Errorf("reading the lowest number: %w", err)
	}
	highest, err := input.Settings.WholeNumber(HighestSetting)
	if err != nil {
		return step.Result{}, fmt.Errorf("reading the highest number: %w", err)
	}
	if lowest > highest {
		return step.Result{}, fmt.Errorf("the lowest number, %d, is higher than the highest, %d", lowest, highest)
	}
	roll := rollBetween(lowest, highest)
	return step.Result{
		OutcomeID: step.DefaultOutcomeID,
		Variables: map[string]string{RandomNumberVariable: strconv.FormatInt(roll, 10)},
	}, nil
}

// rollBetween works with the range as an unsigned number, because the
// distance between the lowest and highest whole numbers is too large for a
// signed one, and two's complement arithmetic brings the roll back in range.
func rollBetween(lowest int64, highest int64) int64 {
	span := uint64(highest) - uint64(lowest)
	if span == math.MaxUint64 {
		return int64(rand.Uint64())
	}
	return lowest + int64(rand.Uint64N(span+1))
}
