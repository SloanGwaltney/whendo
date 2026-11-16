package automation

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// IDs end up in file names, so they're limited to characters that are safe
// in a file name on every platform and can't climb out of the folder.
const maximumIDLength = 64

// ErrInvalidAutomation means an automation has a must-fix problem, which
// breaks one of the rules in docs/design/automation-engine.md#invalid-automations.
// Whendo never runs an invalid automation.
var ErrInvalidAutomation = errors.New("invalid automation")

// ProblemLevel says how much a problem matters.
type ProblemLevel string

const (
	// ProblemLevelMustFix means the automation is invalid: it can be saved,
	// but never runs or turns on until the problem is fixed.
	ProblemLevelMustFix ProblemLevel = "mustFix"
	// ProblemLevelGoodToKnow means the automation is valid, but the user
	// probably wants to know, such as steps that can never run.
	ProblemLevelGoodToKnow ProblemLevel = "goodToKnow"
)

// ProblemSubject says what part of an automation a problem is about.
type ProblemSubject string

// Problems are about the automation as a whole, such as its name, about its
// trigger, or about one of its steps.
const (
	ProblemSubjectAutomation ProblemSubject = "automation"
	ProblemSubjectTrigger    ProblemSubject = "trigger"
	ProblemSubjectStep       ProblemSubject = "step"
)

// Problem is one thing wrong with an automation, with a message written for
// the streamer. StepID is set for problems about a step, and SettingID for
// problems about one of its settings or one of the trigger's.
type Problem struct {
	Level     ProblemLevel   `json:"level"`
	Subject   ProblemSubject `json:"subject"`
	StepID    string         `json:"stepId,omitempty"`
	SettingID string         `json:"settingId,omitempty"`
	Message   string         `json:"message"`

	// position orders problems as the automation reads: the automation's
	// own first, then the trigger's, then each step's in order.
	position int
}

// AvailableVariable is a variable a step can use, with what the designer
// shows for it.
type AvailableVariable struct {
	Reference   automationsetting.VariableReference `json:"reference"`
	DisplayName string                              `json:"displayName"`
	IsCommon    bool                                `json:"isCommon"`
}

// StepReport is what the designer needs to know about one step: the
// variables it can use, in the order to list them, the outcomes it has with
// its current settings, and whether a run can reach it at all.
type StepReport struct {
	AvailableVariables []AvailableVariable `json:"availableVariables"`
	Outcomes           []step.Outcome      `json:"outcomes"`
	IsReachable        bool                `json:"isReachable"`
}

// Report is the result of checking an automation: every problem it has, in
// the order the automation reads, and a report for each step, keyed by step
// ID.
type Report struct {
	Problems []Problem             `json:"problems"`
	Steps    map[string]StepReport `json:"steps"`
}

// HasMustFixProblems reports whether the automation is invalid.
func (report Report) HasMustFixProblems() bool {
	_, isFound := report.FirstMustFixProblem()
	return isFound
}

// FirstMustFixProblem returns the first problem that makes the automation
// invalid, if there is one.
func (report Report) FirstMustFixProblem() (Problem, bool) {
	for _, problem := range report.Problems {
		if problem.Level == ProblemLevelMustFix {
			return problem, true
		}
	}
	return Problem{}, false
}

// Check checks an automation against every rule for a valid automation and
// reports all of its problems, not just the first, so the designer can show
// them while the automation is still being built. See
// docs/design/automation-engine.md#checking-automations.
func Check(automation Automation, catalog *Catalog) Report {
	checker := newChecker(catalog)
	checker.checkNameAndID(automation)
	triggerVariables, isTriggerKnown := checker.checkTrigger(automation.Trigger)
	checker.walkSequence(automation.Steps, triggerVariables, true)
	checker.checkStepSettings(isTriggerKnown)
	slices.SortStableFunc(checker.problems, func(first Problem, second Problem) int {
		return first.position - second.position
	})
	return Report{Problems: checker.problems, Steps: checker.stepReports}
}

// Validate returns ErrInvalidAutomation, with the first must-fix problem's
// message, if the automation is invalid.
func Validate(automation Automation, catalog *Catalog) error {
	problem, isFound := Check(automation, catalog).FirstMustFixProblem()
	if !isFound {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidAutomation, problem.Message)
}

// Positions of the problems that come before any step's.
const (
	automationProblemPosition = -2
	triggerProblemPosition    = -1
)

// checkedStep is a step whose type is known, kept so its settings can be
// checked once the whole automation has been walked.
type checkedStep struct {
	automationStep Step
	definition     step.Definition
	available      []AvailableVariable
	position       int
	// outcomesErr is why the step's outcomes couldn't be worked out, if
	// they couldn't.
	outcomesErr error
}

// checker walks an automation once, collecting problems and step reports.
// See docs/design/automation-engine.md#which-variables-are-available.
type checker struct {
	catalog      *Catalog
	problems     []Problem
	stepReports  map[string]StepReport
	checkedSteps []checkedStep
	// stepCount gives each step its position in reading order.
	stepCount int
	// knownVariableNames holds the display name of every variable anything
	// in the automation provides, wherever it's available, so a problem can
	// name a variable used where it isn't available.
	knownVariableNames map[automationsetting.VariableReference]string
}

func newChecker(catalog *Catalog) *checker {
	return &checker{
		catalog:            catalog,
		stepReports:        make(map[string]StepReport),
		knownVariableNames: make(map[automationsetting.VariableReference]string),
	}
}

func (checker *checker) addProblem(problem Problem) {
	checker.problems = append(checker.problems, problem)
}

func (checker *checker) addMustFixStepProblem(stepID string, position int, message string) {
	checker.addProblem(Problem{Level: ProblemLevelMustFix, Subject: ProblemSubjectStep, StepID: stepID, Message: message, position: position})
}

func (checker *checker) checkNameAndID(automation Automation) {
	err := validateID(automation.ID)
	if err != nil {
		checker.addProblem(Problem{Level: ProblemLevelMustFix, Subject: ProblemSubjectAutomation, Message: asSentence(err.Error()), position: automationProblemPosition})
	}
	if strings.TrimSpace(automation.Name) == "" {
		checker.addProblem(Problem{Level: ProblemLevelMustFix, Subject: ProblemSubjectAutomation, Message: "Give the automation a name.", position: automationProblemPosition})
	}
}

// checkTrigger checks the trigger and its settings, and returns the
// variables it provides and whether the trigger is one Whendo has.
func (checker *checker) checkTrigger(selection TriggerSelection) ([]AvailableVariable, bool) {
	addTriggerProblem := func(settingID string, message string) {
		checker.addProblem(Problem{Level: ProblemLevelMustFix, Subject: ProblemSubjectTrigger, SettingID: settingID, Message: message, position: triggerProblemPosition})
	}
	if selection.ID == "" {
		addTriggerProblem("", "Pick what starts this automation.")
		return nil, false
	}
	selectedTrigger, isFound := checker.catalog.Trigger(selection.ID)
	if !isFound {
		addTriggerProblem("", fmt.Sprintf("This automation starts with a trigger Whendo doesn't have: %q. It may be from a newer version of Whendo.", selection.ID))
		return nil, false
	}
	definition := selectedTrigger.Definition()
	for _, settingDefinition := range definition.Settings {
		err := selection.Settings.ValidateSetting(settingDefinition)
		if err != nil {
			addTriggerProblem(settingDefinition.ID, settingProblemMessage("The trigger", settingDefinition, err))
		}
	}
	variables := make([]AvailableVariable, 0, len(definition.Variables))
	for _, variable := range definition.Variables {
		reference := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: variable.ID}
		variables = append(variables, checker.knownVariable(reference, variable))
	}
	return variables, true
}

func (checker *checker) knownVariable(reference automationsetting.VariableReference, variable trigger.VariableDefinition) AvailableVariable {
	checker.knownVariableNames[reference] = variable.DisplayName
	return AvailableVariable{Reference: reference, DisplayName: variable.DisplayName, IsCommon: variable.IsCommon}
}

// walkSequence walks a sequence of steps that starts with the given
// variables available. isReachable says whether a run can reach the start
// of the sequence. It returns the variables available after the last step,
// and whether the sequence continues past it: it doesn't when a step splits
// and no branch continues.
func (checker *checker) walkSequence(steps []Step, available []AvailableVariable, isReachable bool) ([]AvailableVariable, bool) {
	doesContinue := true
	for _, automationStep := range steps {
		if isReachable && !doesContinue {
			checker.reportNeverRuns(automationStep)
			// Report only the first step that never runs, so a long tail of
			// steps is one problem rather than many.
			isReachable = false
		}
		availableAfterStep, doesStepContinue := checker.walkStep(automationStep, available, isReachable && doesContinue)
		if doesContinue {
			available = availableAfterStep
		}
		doesContinue = doesContinue && doesStepContinue
	}
	return available, doesContinue
}

func (checker *checker) reportNeverRuns(automationStep Step) {
	checker.addProblem(Problem{
		Level:    ProblemLevelGoodToKnow,
		Subject:  ProblemSubjectStep,
		StepID:   automationStep.ID,
		Message:  fmt.Sprintf("%s never runs, because every path above it stops.", checker.stepName(automationStep)),
		position: checker.stepCount,
	})
}

// stepName names a step in a problem message by its type's display name, or
// its type ID when Whendo doesn't have that type.
func (checker *checker) stepName(automationStep Step) string {
	stepType, isFound := checker.catalog.Step(automationStep.Type)
	if !isFound {
		return fmt.Sprintf("“%s”", automationStep.Type)
	}
	return fmt.Sprintf("“%s”", stepType.Definition().DisplayName)
}

func (checker *checker) walkStep(automationStep Step, available []AvailableVariable, isReachable bool) ([]AvailableVariable, bool) {
	position := checker.stepCount
	checker.stepCount++
	if automationStep.ID == "" {
		checker.addMustFixStepProblem("", position, fmt.Sprintf("%s has no step ID.", checker.stepName(automationStep)))
		return available, true
	}
	if _, isSeen := checker.stepReports[automationStep.ID]; isSeen {
		checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("Two steps share the ID %q.", automationStep.ID))
		return available, true
	}
	stepType, isFound := checker.catalog.Step(automationStep.Type)
	if !isFound {
		checker.stepReports[automationStep.ID] = StepReport{AvailableVariables: available, IsReachable: isReachable}
		checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("This automation uses a step Whendo doesn't have: %q. It may be from a newer version of Whendo.", automationStep.Type))
		return available, true
	}
	outcomes, outcomesErr := stepType.Outcomes(automationStep.Settings)
	checker.checkedSteps = append(checker.checkedSteps, checkedStep{
		automationStep: automationStep,
		definition:     stepType.Definition(),
		available:      available,
		position:       position,
		outcomesErr:    outcomesErr,
	})
	checker.stepReports[automationStep.ID] = StepReport{AvailableVariables: available, Outcomes: outcomes, IsReachable: isReachable}
	if outcomesErr != nil {
		// Its branches can't be checked without outcomes. The problem is
		// reported with the step's settings, which are the usual cause.
		return available, true
	}
	switch len(outcomes) {
	case 0:
		checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("%s has no outcomes, which is a bug in Whendo.", checker.stepName(automationStep)))
		return available, true
	case 1:
		if len(automationStep.Branches) > 0 {
			checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("%s doesn't split, but has paths.", checker.stepName(automationStep)))
		}
		return checker.withOutcomeVariables(available, automationStep.ID, outcomes[0]), true
	default:
		return checker.walkBranches(automationStep, outcomes, available, isReachable, position)
	}
}

// walkBranches walks each branch of a step that splits. After the split,
// only the variables every continuing branch provides remain available.
// Branches that end the automation never reach the steps after the split,
// so they don't count.
func (checker *checker) walkBranches(automationStep Step, outcomes []step.Outcome, available []AvailableVariable, isReachable bool, position int) ([]AvailableVariable, bool) {
	name := checker.stepName(automationStep)
	outcomeIDs := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		outcomeIDs = append(outcomeIDs, outcome.ID)
	}
	for _, branchID := range slices.Sorted(maps.Keys(automationStep.Branches)) {
		if !slices.Contains(outcomeIDs, branchID) {
			checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("%s has a path for an outcome it doesn't have: %q.", name, branchID))
		}
	}
	var continuingBranchEnds [][]AvailableVariable
	for _, outcome := range outcomes {
		branch, isFound := automationStep.Branches[outcome.ID]
		if !isFound {
			checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("%s is missing its path for “%s”.", name, outcome.DisplayName))
			continue
		}
		isKnownEnding := branch.AfterBranch == AfterBranchEnd || branch.AfterBranch == AfterBranchContinue
		if !isKnownEnding {
			checker.addMustFixStepProblem(automationStep.ID, position, fmt.Sprintf("%s's path for “%s” ends in a way Whendo doesn't know: %q.", name, outcome.DisplayName, branch.AfterBranch))
		}
		branchEnd, doesBranchContinue := checker.walkSequence(branch.Steps, checker.withOutcomeVariables(available, automationStep.ID, outcome), isReachable)
		if doesBranchContinue && branch.AfterBranch == AfterBranchContinue {
			continuingBranchEnds = append(continuingBranchEnds, branchEnd)
		}
	}
	if len(continuingBranchEnds) == 0 {
		return available, false
	}
	return variablesInEvery(continuingBranchEnds), true
}

func (checker *checker) withOutcomeVariables(available []AvailableVariable, stepID string, outcome step.Outcome) []AvailableVariable {
	combined := slices.Clone(available)
	for _, variable := range outcome.Variables {
		reference := automationsetting.VariableReference{Source: automationsetting.VariableSourceStep, StepID: stepID, ID: variable.ID}
		combined = append(combined, checker.knownVariable(reference, variable))
	}
	return combined
}

// checkStepSettings runs after the whole automation has been walked, so a
// problem can name a variable even when it comes from a later step.
// isTriggerKnown is false when the trigger is missing or unknown; then every
// trigger variable would be a problem and none would help, so they're left
// unchecked.
func (checker *checker) checkStepSettings(isTriggerKnown bool) {
	for _, checked := range checker.checkedSteps {
		name := fmt.Sprintf("“%s”", checked.definition.DisplayName)
		hasSettingProblem := false
		addSettingProblem := func(settingID string, message string) {
			hasSettingProblem = true
			checker.addProblem(Problem{Level: ProblemLevelMustFix, Subject: ProblemSubjectStep, StepID: checked.automationStep.ID, SettingID: settingID, Message: message, position: checked.position})
		}
		for _, settingDefinition := range checked.definition.Settings {
			err := checked.automationStep.Settings.ValidateSetting(settingDefinition)
			if err != nil {
				addSettingProblem(settingDefinition.ID, settingProblemMessage(name, settingDefinition, err))
				continue
			}
			references, err := checked.automationStep.Settings.VariableReferences(settingDefinition)
			if err != nil {
				addSettingProblem(settingDefinition.ID, settingProblemMessage(name, settingDefinition, err))
				continue
			}
			for _, reference := range references {
				if reference.Source == automationsetting.VariableSourceTrigger && !isTriggerKnown {
					continue
				}
				if !containsReference(checked.available, reference) {
					addSettingProblem(settingDefinition.ID, checker.unavailableVariableMessage(name, reference))
				}
			}
		}
		// A setting problem already explains why the outcomes couldn't be
		// worked out; this covers any other reason, so the step is never
		// passed as valid without them.
		if checked.outcomesErr != nil && !hasSettingProblem {
			checker.addMustFixStepProblem(checked.automationStep.ID, checked.position, fmt.Sprintf("%s couldn't work out its paths: %v.", name, checked.outcomesErr))
		}
	}
}

func (checker *checker) unavailableVariableMessage(name string, reference automationsetting.VariableReference) string {
	displayName, isKnown := checker.knownVariableNames[reference]
	if !isKnown {
		return fmt.Sprintf("%s uses a value that no longer exists.", name)
	}
	return fmt.Sprintf("%s uses “%s”, which isn't available at this point.", name, displayName)
}

// settingProblemMessage words a setting's problem for the streamer. name is
// what the setting belongs to, such as “Send a chat message”.
func settingProblemMessage(name string, definition automationsetting.Definition, err error) string {
	switch {
	case errors.Is(err, automationsetting.ErrSettingMissing) && definition.Placeholder != "":
		return fmt.Sprintf("%s needs you to %s.", name, definition.Placeholder)
	case errors.Is(err, automationsetting.ErrSettingMissing):
		return fmt.Sprintf("%s needs a value for “%s”.", name, definition.DisplayName)
	case errors.Is(err, automationsetting.ErrSettingTooLong):
		return fmt.Sprintf("%s is over its %d-character limit.", name, definition.MaximumLength)
	case errors.Is(err, automationsetting.ErrBlankChoice):
		return fmt.Sprintf("%s has a blank choice.", name)
	case errors.Is(err, automationsetting.ErrRepeatedChoice):
		return fmt.Sprintf("%s has the same choice twice.", name)
	default:
		return fmt.Sprintf("%s has a value for “%s” that Whendo can't read.", name, definition.DisplayName)
	}
}

// asSentence turns an error message into a sentence for the streamer.
func asSentence(message string) string {
	firstRune, size := utf8.DecodeRuneInString(message)
	return string(unicode.ToUpper(firstRune)) + message[size:] + "."
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
