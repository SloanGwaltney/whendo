package automation

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// IDs end up in file names, so they're limited to characters that are safe
// in a file name on every platform and can't climb out of the folder.
const maximumIDLength = 64

var (
	// ErrInvalidAutomation means an automation breaks one of the rules in
	// docs/design/automation-engine.md#invalid-automations. Whendo never
	// runs or saves an invalid automation.
	ErrInvalidAutomation = errors.New("invalid automation")
	// ErrStepNotFound means an automation has no step with the given ID.
	ErrStepNotFound = errors.New("step not found")
)

// AvailableVariable is a variable a step can use, with the display name the
// designer shows for it.
type AvailableVariable struct {
	Reference   automationsetting.VariableReference `json:"reference"`
	DisplayName string                              `json:"displayName"`
}

// Validate checks an automation against every rule for a valid automation:
// its ID and name, that its trigger and steps exist, their settings, the
// branches of steps that split, and that every variable it uses is available
// where it's used.
func Validate(automation Automation, catalog *Catalog) error {
	err := validate(automation, catalog)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAutomation, err)
	}
	return nil
}

func validate(automation Automation, catalog *Catalog) error {
	err := validateID(automation.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(automation.Name) == "" {
		return errors.New("the automation has no name")
	}
	triggerDefinition, err := findTriggerDefinition(automation.Trigger, catalog)
	if err != nil {
		return err
	}
	err = automation.Trigger.Settings.Validate(triggerDefinition.Settings)
	if err != nil {
		return fmt.Errorf("trigger %q: %w", triggerDefinition.ID, err)
	}
	walk := newAvailabilityWalk(catalog, true)
	_, _, err = walk.walkSequence(automation.Steps, triggerVariables(triggerDefinition))
	return err
}

// AvailableVariables returns the variables the step with the given ID can
// use, in the order the designer should list them: the trigger's first, then
// each earlier step's. It doesn't check settings, so the designer can ask
// while an automation is still being filled in.
func AvailableVariables(automation Automation, catalog *Catalog, stepID string) ([]AvailableVariable, error) {
	triggerDefinition, err := findTriggerDefinition(automation.Trigger, catalog)
	if err != nil {
		return nil, err
	}
	walk := newAvailabilityWalk(catalog, false)
	_, _, err = walk.walkSequence(automation.Steps, triggerVariables(triggerDefinition))
	if err != nil {
		return nil, err
	}
	available, isFound := walk.availableByStepID[stepID]
	if !isFound {
		return nil, fmt.Errorf("%w: %q", ErrStepNotFound, stepID)
	}
	return available, nil
}

func validateID(id string) error {
	if id == "" {
		return errors.New("the automation has no ID")
	}
	if len(id) > maximumIDLength {
		return fmt.Errorf("the automation's ID is longer than %d characters", maximumIDLength)
	}
	for _, character := range id {
		if !isSafeIDCharacter(character) {
			return fmt.Errorf("the automation's ID %q may only contain letters, digits, hyphens and underscores", id)
		}
	}
	return nil
}

func isSafeIDCharacter(character rune) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9') ||
		character == '-' || character == '_'
}

func findTriggerDefinition(selection TriggerSelection, catalog *Catalog) (trigger.Definition, error) {
	selectedTrigger, isFound := catalog.Trigger(selection.ID)
	if !isFound {
		return trigger.Definition{}, fmt.Errorf("there's no trigger %q", selection.ID)
	}
	return selectedTrigger.Definition(), nil
}

func triggerVariables(definition trigger.Definition) []AvailableVariable {
	variables := make([]AvailableVariable, 0, len(definition.Variables))
	for _, variable := range definition.Variables {
		variables = append(variables, AvailableVariable{
			Reference:   automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: variable.ID},
			DisplayName: variable.DisplayName,
		})
	}
	return variables
}

// availabilityWalk goes through an automation's steps in order, tracking
// which variables are available at each one. See
// docs/design/automation-engine.md#which-variables-are-available.
type availabilityWalk struct {
	catalog             *Catalog
	shouldCheckSettings bool
	availableByStepID   map[string][]AvailableVariable
}

func newAvailabilityWalk(catalog *Catalog, shouldCheckSettings bool) *availabilityWalk {
	return &availabilityWalk{
		catalog:             catalog,
		shouldCheckSettings: shouldCheckSettings,
		availableByStepID:   make(map[string][]AvailableVariable),
	}
}

// walkSequence walks a sequence of steps that starts with the given
// variables available. It returns the variables available after the last
// step, and whether a run can get there at all: it can't when a step splits
// and every branch ends the automation.
func (walk *availabilityWalk) walkSequence(steps []Step, available []AvailableVariable) ([]AvailableVariable, bool, error) {
	for stepIndex, automationStep := range steps {
		availableAfterStep, doesContinue, err := walk.walkStep(automationStep, available)
		if err != nil {
			return nil, false, fmt.Errorf("step %q: %w", automationStep.ID, err)
		}
		if doesContinue {
			available = availableAfterStep
			continue
		}
		isLastStep := stepIndex == len(steps)-1
		if !isLastStep {
			return nil, false, fmt.Errorf("the steps after step %q never run, because every branch of it ends the automation", automationStep.ID)
		}
		return nil, false, nil
	}
	return available, true, nil
}

func (walk *availabilityWalk) walkStep(automationStep Step, available []AvailableVariable) ([]AvailableVariable, bool, error) {
	if automationStep.ID == "" {
		return nil, false, errors.New("a step has no ID")
	}
	if _, isSeen := walk.availableByStepID[automationStep.ID]; isSeen {
		return nil, false, errors.New("two steps have this ID")
	}
	walk.availableByStepID[automationStep.ID] = available

	stepType, isFound := walk.catalog.Step(automationStep.Type)
	if !isFound {
		return nil, false, fmt.Errorf("there's no step type %q", automationStep.Type)
	}
	if walk.shouldCheckSettings {
		err := checkStepSettings(automationStep.Settings, stepType.Definition().Settings, available)
		if err != nil {
			return nil, false, err
		}
	}
	outcomes, err := stepType.Outcomes(automationStep.Settings)
	if err != nil {
		return nil, false, fmt.Errorf("working out the step's outcomes: %w", err)
	}
	switch len(outcomes) {
	case 0:
		return nil, false, fmt.Errorf("step type %q has no outcomes", automationStep.Type)
	case 1:
		if len(automationStep.Branches) > 0 {
			return nil, false, errors.New("the step doesn't split, but has branches")
		}
		return withOutcomeVariables(available, automationStep.ID, outcomes[0]), true, nil
	default:
		return walk.walkBranches(automationStep, outcomes, available)
	}
}

// walkBranches walks each branch of a step that splits. After the split,
// only the variables every continuing branch provides remain available.
// Branches that end the automation never reach the steps after the split,
// so they don't count.
func (walk *availabilityWalk) walkBranches(automationStep Step, outcomes []step.Outcome, available []AvailableVariable) ([]AvailableVariable, bool, error) {
	if len(automationStep.Branches) != len(outcomes) {
		return nil, false, fmt.Errorf("the step has %d outcomes but %d branches", len(outcomes), len(automationStep.Branches))
	}
	var continuingBranchEnds [][]AvailableVariable
	for _, outcome := range outcomes {
		branch, isFound := automationStep.Branches[outcome.ID]
		if !isFound {
			return nil, false, fmt.Errorf("outcome %q has no branch", outcome.ID)
		}
		if branch.AfterBranch != AfterBranchEnd && branch.AfterBranch != AfterBranchContinue {
			return nil, false, fmt.Errorf("outcome %q's branch has unknown ending %q", outcome.ID, branch.AfterBranch)
		}
		branchEnd, doesReachEnd, err := walk.walkSequence(branch.Steps, withOutcomeVariables(available, automationStep.ID, outcome))
		if err != nil {
			return nil, false, fmt.Errorf("outcome %q: %w", outcome.ID, err)
		}
		if doesReachEnd && branch.AfterBranch == AfterBranchContinue {
			continuingBranchEnds = append(continuingBranchEnds, branchEnd)
		}
	}
	if len(continuingBranchEnds) == 0 {
		return nil, false, nil
	}
	return variablesInEvery(continuingBranchEnds), true, nil
}

func checkStepSettings(settings automationsetting.Values, definitions []automationsetting.Definition, available []AvailableVariable) error {
	err := settings.Validate(definitions)
	if err != nil {
		return err
	}
	references, err := settings.VariableReferences(definitions)
	if err != nil {
		return err
	}
	for _, reference := range references {
		if !containsReference(available, reference) {
			return fmt.Errorf("%w here: %+v", automationsetting.ErrVariableUnavailable, reference)
		}
	}
	return nil
}

func withOutcomeVariables(available []AvailableVariable, stepID string, outcome step.Outcome) []AvailableVariable {
	combined := slices.Clone(available)
	for _, variable := range outcome.Variables {
		combined = append(combined, AvailableVariable{
			Reference:   automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: stepID, ID: variable.ID},
			DisplayName: variable.DisplayName,
		})
	}
	return combined
}

// variablesInEvery returns the variables found in every one of the given
// lists, in the order of the first.
func variablesInEvery(variableLists [][]AvailableVariable) []AvailableVariable {
	var common []AvailableVariable
	for _, candidate := range variableLists[0] {
		isInEvery := true
		for _, otherList := range variableLists[1:] {
			if !containsReference(otherList, candidate.Reference) {
				isInEvery = false
				break
			}
		}
		if isInEvery {
			common = append(common, candidate)
		}
	}
	return common
}

func containsReference(variables []AvailableVariable, reference automationsetting.VariableReference) bool {
	return slices.ContainsFunc(variables, func(variable AvailableVariable) bool {
		return variable.Reference == reference
	})
}
