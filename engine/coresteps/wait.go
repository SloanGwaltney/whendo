package coresteps

import (
	"context"
	"fmt"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
)

const (
	// WaitStepID is the step that waits for a set amount of time.
	WaitStepID = "core.wait"
	// WaitDurationSetting is how long the wait step waits.
	WaitDurationSetting = "duration"
)

type waitStep struct{}

func (waitStep) Definition() step.Definition {
	return step.Definition{
		ID:          WaitStepID,
		DisplayName: "Wait",
		Description: "Pause for a while before the next step.",
		Group:       timingGroup,
		Sentence:    automationsetting.Sentence{automationsetting.Words("Wait"), automationsetting.Setting(WaitDurationSetting)},
		Settings: []automationsetting.Definition{
			{ID: WaitDurationSetting, DisplayName: "How long", Kind: automationsetting.KindDuration, IsRequired: true, Placeholder: "choose how long"},
		},
	}
}

func (waitStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return singleOutcome(), nil
}

// Run waits for the duration, or until Whendo shuts down.
func (waitStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	duration, err := input.Settings.Duration(WaitDurationSetting)
	if err != nil {
		return step.Result{}, fmt.Errorf("reading how long to wait: %w", err)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return step.Result{OutcomeID: step.DefaultOutcomeID}, nil
	case <-ctx.Done():
		return step.Result{}, ctx.Err()
	}
}
