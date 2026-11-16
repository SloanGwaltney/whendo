package runner

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"strings"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
)

const interruptedReason = "Whendo shut down while this run was in progress."

// errRunInterrupted marks a step that stopped because Whendo shut down.
var errRunInterrupted = errors.New("run interrupted")

// runExecution is the state of one run while its steps execute. Only the
// queue's worker goroutine touches it.
type runExecution struct {
	runner    *Runner
	record    RunRecord
	variables automationsetting.VariableValues
}

// execute runs one queued run to its end and returns its record. A step
// that panics fails the run instead of crashing Whendo, so one broken step
// can't stop every other automation.
func (runner *Runner) execute(run pendingRun, automationAtStart automation.Automation) (record RunRecord) {
	execution := &runExecution{
		runner: runner,
		record: RunRecord{
			RunID:            run.runID,
			AutomationID:     automationAtStart.ID,
			AutomationName:   automationAtStart.Name,
			IsTest:           run.isTest,
			TriggerID:        automationAtStart.Trigger.ID,
			TriggerVariables: run.triggerVariables,
			QueuedAt:         run.queuedAt,
			StartedAt:        time.Now(),
		},
	}
	defer func() {
		panicValue := recover()
		if panicValue == nil {
			return
		}
		runner.logger.Error("a step panicked", "automation", automationAtStart.ID, "panic", panicValue, "stack", string(debug.Stack()))
		panicErr := fmt.Errorf("the step stopped unexpectedly: %v", panicValue)
		execution.markUnfinishedStepFailed(panicErr)
		record = execution.finish(panicErr)
	}()

	variables, err := runner.triggerVariableValues(automationAtStart, run.triggerVariables)
	if err != nil {
		return execution.finish(err)
	}
	execution.variables = variables
	_, err = execution.executeSequence(automationAtStart.Steps)
	return execution.finish(err)
}

// triggerVariableValues makes every variable the trigger declares available
// to the steps. A declared variable missing from the event, which can happen
// in a test run, is empty rather than unavailable, since validation already
// promised the steps it exists.
func (runner *Runner) triggerVariableValues(runAutomation automation.Automation, triggerVariables map[string]string) (automationsetting.VariableValues, error) {
	runTrigger, isKnown := runner.catalog.Trigger(runAutomation.Trigger.ID)
	if !isKnown {
		return nil, fmt.Errorf("there's no trigger %q", runAutomation.Trigger.ID)
	}
	variables := automationsetting.VariableValues{}
	for _, variable := range runTrigger.Definition().Variables {
		reference := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: variable.ID}
		variables[reference] = triggerVariables[variable.ID]
	}
	return variables, nil
}

// markUnfinishedStepFailed records a failure on the step that was running
// when the run stopped without that step returning. Steps that split end
// before their branches run, so only the step that stopped is unfinished.
func (execution *runExecution) markUnfinishedStepFailed(err error) {
	lastStepIndex := len(execution.record.Steps) - 1
	if lastStepIndex < 0 || !execution.record.Steps[lastStepIndex].EndedAt.IsZero() {
		return
	}
	execution.record.Steps[lastStepIndex].EndedAt = time.Now()
	execution.record.Steps[lastStepIndex].FailureReason = err.Error()
}

func (execution *runExecution) finish(err error) RunRecord {
	execution.record.EndedAt = time.Now()
	switch {
	case err == nil:
		execution.record.Status = RunStatusSucceeded
	case errors.Is(err, errRunInterrupted):
		execution.record.Status = RunStatusCancelled
		execution.record.FailureReason = interruptedReason
	default:
		execution.record.Status = RunStatusFailed
		execution.record.FailureReason = err.Error()
	}
	return execution.record
}

// executeSequence runs a sequence of steps in order. It returns false when a
// branch ended the automation, so the steps after it must not run.
func (execution *runExecution) executeSequence(steps []automation.Step) (bool, error) {
	for _, automationStep := range steps {
		shouldContinue, err := execution.executeStep(automationStep)
		if err != nil || !shouldContinue {
			return false, err
		}
	}
	return true, nil
}

func (execution *runExecution) executeStep(automationStep automation.Step) (bool, error) {
	stepIndex := len(execution.record.Steps)
	execution.record.Steps = append(execution.record.Steps, StepRecord{
		StepID:    automationStep.ID,
		StepType:  automationStep.Type,
		StepName:  execution.stepName(automationStep.Type),
		StartedAt: time.Now(),
	})
	outcome, details, err := execution.performStep(automationStep)
	stepRecord := &execution.record.Steps[stepIndex]
	stepRecord.EndedAt = time.Now()
	stepRecord.Details = details
	if err != nil {
		stepRecord.FailureReason = err.Error()
		if errors.Is(err, errRunInterrupted) {
			stepRecord.FailureReason = interruptedReason
		}
		return false, err
	}
	stepRecord.OutcomeID = outcome.ID
	stepRecord.BranchLabel = branchLabel(outcome.Sentence)
	stepRecord.Values = execution.providedValues(automationStep.ID, outcome)

	branch, hasBranches := automationStep.Branches[outcome.ID]
	if !hasBranches {
		return true, nil
	}
	hasReachedBranchEnd, err := execution.executeSequence(branch.Steps)
	if err != nil || !hasReachedBranchEnd {
		return false, err
	}
	return branch.AfterBranch == automation.AfterBranchContinue, nil
}

// stepName returns a step type's display name. A type Whendo doesn't have
// fails when it runs, so its record falls back to the type's ID.
func (execution *runExecution) stepName(stepType string) string {
	definedStep, isKnown := execution.runner.catalog.Step(stepType)
	if !isKnown {
		return stepType
	}
	return definedStep.Definition().DisplayName
}

// providedValues returns the values a step's outcome provided, in the order
// the outcome declares them.
func (execution *runExecution) providedValues(stepID string, outcome step.Outcome) []RecordedValue {
	var values []RecordedValue
	for _, variable := range outcome.Variables {
		reference := automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: stepID, ID: variable.ID}
		values = append(values, RecordedValue{ID: variable.ID, Name: variable.DisplayName, Value: execution.variables[reference]})
	}
	return values
}

// branchLabel writes an outcome's sentence as plain text. Outcome sentences
// hold only fixed words and text the user wrote, never settings.
func branchLabel(outcomeSentence automationsetting.Sentence) string {
	var labelParts []string
	for _, part := range outcomeSentence {
		labelParts = append(labelParts, cmp.Or(part.Words, part.UserText))
	}
	return strings.Join(labelParts, " ")
}

// performStep runs one step and makes the variables its outcome provides
// available to the steps after it. It returns the step's details even when
// the step fails.
func (execution *runExecution) performStep(automationStep automation.Step) (step.Outcome, []step.Detail, error) {
	stepType, isKnown := execution.runner.catalog.Step(automationStep.Type)
	if !isKnown {
		return step.Outcome{}, nil, fmt.Errorf("there's no step type %q", automationStep.Type)
	}
	outcomes, err := stepType.Outcomes(automationStep.Settings)
	if err != nil {
		return step.Outcome{}, nil, fmt.Errorf("working out the step's outcomes: %w", err)
	}
	result, err := stepType.Run(execution.runner.runContext, step.Input{
		Settings:  automationStep.Settings,
		Variables: maps.Clone(execution.variables),
		Events:    &execution.runner.subscriptions,
	})
	if execution.runner.runContext.Err() != nil {
		return step.Outcome{}, result.Details, errRunInterrupted
	}
	if err != nil {
		return step.Outcome{}, result.Details, err
	}
	outcome, err := findOutcome(outcomes, result.OutcomeID)
	if err != nil {
		return step.Outcome{}, result.Details, err
	}
	// Only declared variables are kept, so a run provides exactly what
	// validation promised. A declared variable the step didn't return is
	// empty, as with trigger variables.
	for _, variable := range outcome.Variables {
		reference := automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: automationStep.ID, ID: variable.ID}
		execution.variables[reference] = result.Variables[variable.ID]
	}
	return outcome, result.Details, nil
}

func findOutcome(outcomes []step.Outcome, outcomeID string) (step.Outcome, error) {
	for _, outcome := range outcomes {
		if outcome.ID == outcomeID {
			return outcome, nil
		}
	}
	return step.Outcome{}, fmt.Errorf("the step finished with outcome %q, which it doesn't declare", outcomeID)
}
