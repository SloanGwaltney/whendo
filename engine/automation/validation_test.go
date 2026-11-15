package automation_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

func assertValid(t *testing.T, testAutomation automation.Automation) {
	t.Helper()
	err := automation.Validate(testAutomation, newTestCatalog(t))
	if err != nil {
		t.Errorf("got error %v, want a valid automation", err)
	}
}

func assertInvalid(t *testing.T, testAutomation automation.Automation, wantedCause error) {
	t.Helper()
	err := automation.Validate(testAutomation, newTestCatalog(t))
	if !errors.Is(err, automation.ErrInvalidAutomation) {
		t.Fatalf("got error %v, want ErrInvalidAutomation", err)
	}
	if wantedCause != nil && !errors.Is(err, wantedCause) {
		t.Errorf("got error %v, want it caused by %v", err, wantedCause)
	}
}

func referenceTo(reference automationsetting.VariableReference) *automationsetting.VariableReference {
	return &reference
}

func TestValidate_AcceptsRollAndCheck(t *testing.T) {
	roll := stepVariable("roll", rollVariable)
	assertValid(t, newAutomation(t,
		rollStep("roll"),
		checkStep(t, "check", roll,
			branch(automation.AfterBranchEnd, sayStep(t, "sayNatural20", referenceTo(roll))),
			branch(automation.AfterBranchEnd, sayStep(t, "sayOther", referenceTo(triggerVariable(redeemerNameVariable)))),
		),
	))
}

func TestValidate_AllowsVariableAfterSplitWhenMissingOnlyFromBranchesThatEnd(t *testing.T) {
	assertValid(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchContinue),
			branch(automation.AfterBranchEnd),
		),
		sayStep(t, "sayChoice", referenceTo(stepVariable("wait", replyVariable))),
	))
}

func TestValidate_RejectsVariableAfterSplitWhenAContinuingBranchLacksIt(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchContinue),
			branch(automation.AfterBranchContinue),
		),
		sayStep(t, "sayChoice", referenceTo(stepVariable("wait", replyVariable))),
	), automationsetting.ErrVariableUnavailable)
}

func TestValidate_AllowsOutcomeVariableInsideItsBranch(t *testing.T) {
	assertValid(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd, sayStep(t, "sayChoice", referenceTo(stepVariable("wait", replyVariable)))),
			branch(automation.AfterBranchEnd),
		),
	))
}

func TestValidate_RejectsOutcomeVariableInAnotherBranch(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd),
			branch(automation.AfterBranchEnd, sayStep(t, "sayChoice", referenceTo(stepVariable("wait", replyVariable)))),
		),
	), automationsetting.ErrVariableUnavailable)
}

func TestValidate_RejectsVariableFromStepInsideOnlyOneContinuingBranch(t *testing.T) {
	roll := stepVariable("rollInYes", rollVariable)
	assertInvalid(t, newAutomation(t,
		checkStep(t, "check", triggerVariable(redeemerNameVariable),
			branch(automation.AfterBranchContinue, rollStep("rollInYes")),
			branch(automation.AfterBranchContinue),
		),
		sayStep(t, "sayRoll", referenceTo(roll)),
	), automationsetting.ErrVariableUnavailable)
}

func TestValidate_KeepsVariablesFromBeforeSplitAvailableAfterIt(t *testing.T) {
	roll := stepVariable("roll", rollVariable)
	assertValid(t, newAutomation(t,
		rollStep("roll"),
		checkStep(t, "check", roll,
			branch(automation.AfterBranchContinue),
			branch(automation.AfterBranchContinue),
		),
		sayStep(t, "sayRoll", referenceTo(roll)),
	))
}

func TestValidate_IgnoresBranchThatEndsInsideANestedSplit(t *testing.T) {
	// The "yes" branch is marked to continue, but every branch of the split
	// inside it ends the automation, so it never reaches the steps after the
	// outer split. Only the "no" branch does, and it provides the roll.
	roll := stepVariable("rollInNo", rollVariable)
	assertValid(t, newAutomation(t,
		checkStep(t, "check", triggerVariable(redeemerNameVariable),
			branch(automation.AfterBranchContinue, waitStep("wait",
				branch(automation.AfterBranchEnd),
				branch(automation.AfterBranchEnd),
			)),
			branch(automation.AfterBranchContinue, rollStep("rollInNo")),
		),
		sayStep(t, "sayRoll", referenceTo(roll)),
	))
}

func TestValidate_RejectsVariableFromLaterStep(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		sayStep(t, "sayRoll", referenceTo(stepVariable("roll", rollVariable))),
		rollStep("roll"),
	), automationsetting.ErrVariableUnavailable)
}

func TestValidate_RejectsVariableTheTriggerDoesntProvide(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		sayStep(t, "say", referenceTo(triggerVariable("weather"))),
	), automationsetting.ErrVariableUnavailable)
}

func TestValidate_RejectsStepsAfterSplitWhereEveryBranchEnds(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd),
			branch(automation.AfterBranchEnd),
		),
		sayStep(t, "neverRuns", nil),
	), nil)
}

func TestValidate_RejectsMissingRequiredSettings(t *testing.T) {
	missingTriggerSetting := newAutomation(t)
	missingTriggerSetting.Trigger.Settings = automationsetting.Values{}
	assertInvalid(t, missingTriggerSetting, automationsetting.ErrSettingMissing)

	stepWithoutMessage := automation.Step{ID: "say", Type: sayStepType}
	assertInvalid(t, newAutomation(t, stepWithoutMessage), automationsetting.ErrSettingMissing)
}

func TestValidate_RejectsUnknownTriggerAndStepTypes(t *testing.T) {
	unknownTrigger := newAutomation(t)
	unknownTrigger.Trigger.ID = "test.raid"
	assertInvalid(t, unknownTrigger, nil)

	assertInvalid(t, newAutomation(t, automation.Step{ID: "dance", Type: "test.dance"}), nil)
}

func TestValidate_RejectsDuplicateStepIDsAcrossBranches(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd, rollStep("roll")),
			branch(automation.AfterBranchEnd, rollStep("roll")),
		),
	), nil)
}

func TestValidate_RejectsStepWithoutID(t *testing.T) {
	assertInvalid(t, newAutomation(t, rollStep("")), nil)
}

func TestValidate_RejectsMissingAndExtraBranches(t *testing.T) {
	missingBranch := waitStep("wait", branch(automation.AfterBranchEnd), branch(automation.AfterBranchEnd))
	delete(missingBranch.Branches, timedOutOutcomeID)
	assertInvalid(t, newAutomation(t, missingBranch), nil)

	wrongBranch := waitStep("wait", branch(automation.AfterBranchEnd), branch(automation.AfterBranchEnd))
	delete(wrongBranch.Branches, timedOutOutcomeID)
	wrongBranch.Branches["gaveUp"] = branch(automation.AfterBranchEnd)
	assertInvalid(t, newAutomation(t, wrongBranch), nil)

	branchesOnSingleOutcomeStep := rollStep("roll")
	branchesOnSingleOutcomeStep.Branches = map[string]automation.Branch{step.DefaultOutcomeID: branch(automation.AfterBranchContinue)}
	assertInvalid(t, newAutomation(t, branchesOnSingleOutcomeStep), nil)
}

func TestValidate_RejectsUnknownBranchEnding(t *testing.T) {
	assertInvalid(t, newAutomation(t,
		waitStep("wait", branch("jumpToStart"), branch(automation.AfterBranchEnd)),
	), nil)
}

func TestValidate_RejectsIDsThatArentSafeFileNames(t *testing.T) {
	for _, unsafeID := range []string{"", "../outside", `folder\file`, "has space", "x.json"} {
		unsafeAutomation := newAutomation(t)
		unsafeAutomation.ID = unsafeID
		err := automation.Validate(unsafeAutomation, newTestCatalog(t))
		if !errors.Is(err, automation.ErrInvalidAutomation) {
			t.Errorf("ID %q: got error %v, want ErrInvalidAutomation", unsafeID, err)
		}
	}
}

func TestValidate_RejectsBlankName(t *testing.T) {
	unnamedAutomation := newAutomation(t)
	unnamedAutomation.Name = "  "
	assertInvalid(t, unnamedAutomation, nil)
}

func TestValidate_AcceptsNewID(t *testing.T) {
	newIDAutomation := newAutomation(t)
	newIDAutomation.ID = automation.NewID()
	assertValid(t, newIDAutomation)
}

func availableReferences(variables []automation.AvailableVariable) []automationsetting.VariableReference {
	var references []automationsetting.VariableReference
	for _, variable := range variables {
		references = append(references, variable.Reference)
	}
	return references
}

func TestAvailableVariables_ListsTriggerVariablesThenEarlierSteps(t *testing.T) {
	testAutomation := newAutomation(t,
		rollStep("firstRoll"),
		rollStep("secondRoll"),
		sayStep(t, "say", nil),
	)

	variables, err := automation.AvailableVariables(testAutomation, newTestCatalog(t), "say")

	wantedReferences := []automationsetting.VariableReference{
		triggerVariable(redeemerNameVariable),
		stepVariable("firstRoll", rollVariable),
		stepVariable("secondRoll", rollVariable),
	}
	if err != nil || !slices.Equal(availableReferences(variables), wantedReferences) {
		t.Errorf("got %v, error %v; want %v", variables, err, wantedReferences)
	}
	if variables[0].DisplayName != "Redeemer's name" || variables[1].DisplayName != "Roll" {
		t.Errorf("got display names %q and %q", variables[0].DisplayName, variables[1].DisplayName)
	}
}

func TestAvailableVariables_IncludesOutcomeVariablesInsideBranch(t *testing.T) {
	testAutomation := newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd, sayStep(t, "sayChoice", nil)),
			branch(automation.AfterBranchEnd),
		),
	)

	variables, err := automation.AvailableVariables(testAutomation, newTestCatalog(t), "sayChoice")

	if err != nil || !slices.Contains(availableReferences(variables), stepVariable("wait", replyVariable)) {
		t.Errorf("got %v, error %v; want the reply included", variables, err)
	}
}

func TestAvailableVariables_WorksWhileSettingsAreUnfinished(t *testing.T) {
	testAutomation := newAutomation(t, automation.Step{ID: "say", Type: sayStepType})

	variables, err := automation.AvailableVariables(testAutomation, newTestCatalog(t), "say")

	if err != nil || len(variables) != 1 {
		t.Errorf("got %v, error %v; want the trigger's variable", variables, err)
	}
}

func TestAvailableVariables_ReturnsErrorForUnknownStep(t *testing.T) {
	_, err := automation.AvailableVariables(newAutomation(t, rollStep("roll")), newTestCatalog(t), "missing")

	if !errors.Is(err, automation.ErrStepNotFound) {
		t.Errorf("got error %v, want ErrStepNotFound", err)
	}
}

func TestNewCatalog_ReturnsErrorForDuplicateIDs(t *testing.T) {
	duplicateTriggers := []trigger.Trigger{
		fakeTrigger{definition: trigger.Definition{ID: redeemTriggerID}},
		fakeTrigger{definition: trigger.Definition{ID: redeemTriggerID}},
	}
	_, triggerErr := automation.NewCatalog(duplicateTriggers, nil)

	duplicateSteps := []step.Step{
		fakeStep{definition: step.Definition{ID: rollStepType}},
		fakeStep{definition: step.Definition{ID: rollStepType}},
	}
	_, stepErr := automation.NewCatalog(nil, duplicateSteps)

	if !errors.Is(triggerErr, automation.ErrDuplicateID) || !errors.Is(stepErr, automation.ErrDuplicateID) {
		t.Errorf("got errors %v and %v, want ErrDuplicateID", triggerErr, stepErr)
	}
}
