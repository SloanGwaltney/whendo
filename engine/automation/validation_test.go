package automation_test

import (
	"errors"
	"slices"
	"strings"
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

// assertMustFix checks that the automation is invalid because of a must-fix
// problem about the given subject and step.
func assertMustFix(t *testing.T, testAutomation automation.Automation, subject automation.ProblemSubject, stepID string) {
	t.Helper()
	report := automation.Check(testAutomation, newTestCatalog(t))
	isFound := slices.ContainsFunc(report.Problems, func(problem automation.Problem) bool {
		return problem.Level == automation.ProblemLevelMustFix && problem.Subject == subject && problem.StepID == stepID
	})
	if !isFound {
		t.Errorf("got problems %+v, want a must-fix problem about %s %q", report.Problems, subject, stepID)
	}
	err := automation.Validate(testAutomation, newTestCatalog(t))
	if !errors.Is(err, automation.ErrInvalidAutomation) {
		t.Errorf("got error %v, want ErrInvalidAutomation", err)
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
	assertMustFix(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchContinue),
			branch(automation.AfterBranchContinue),
		),
		sayStep(t, "sayChoice", referenceTo(stepVariable("wait", replyVariable))),
	), automation.ProblemSubjectStep, "sayChoice")
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
	assertMustFix(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd),
			branch(automation.AfterBranchEnd, sayStep(t, "sayChoice", referenceTo(stepVariable("wait", replyVariable)))),
		),
	), automation.ProblemSubjectStep, "sayChoice")
}

func TestValidate_RejectsVariableFromStepInsideOnlyOneContinuingBranch(t *testing.T) {
	roll := stepVariable("rollInYes", rollVariable)
	assertMustFix(t, newAutomation(t,
		checkStep(t, "check", triggerVariable(redeemerNameVariable),
			branch(automation.AfterBranchContinue, rollStep("rollInYes")),
			branch(automation.AfterBranchContinue),
		),
		sayStep(t, "sayRoll", referenceTo(roll)),
	), automation.ProblemSubjectStep, "sayRoll")
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
	assertMustFix(t, newAutomation(t,
		sayStep(t, "sayRoll", referenceTo(stepVariable("roll", rollVariable))),
		rollStep("roll"),
	), automation.ProblemSubjectStep, "sayRoll")
}

func TestValidate_RejectsVariableTheTriggerDoesntProvide(t *testing.T) {
	assertMustFix(t, newAutomation(t,
		sayStep(t, "say", referenceTo(triggerVariable("weather"))),
	), automation.ProblemSubjectStep, "say")
}

func TestValidate_RejectsMissingRequiredSettings(t *testing.T) {
	missingTriggerSetting := newAutomation(t)
	missingTriggerSetting.Trigger.Settings = automationsetting.Values{}
	assertMustFix(t, missingTriggerSetting, automation.ProblemSubjectTrigger, "")

	stepWithoutMessage := automation.Step{ID: "say", Type: sayStepType}
	assertMustFix(t, newAutomation(t, stepWithoutMessage), automation.ProblemSubjectStep, "say")
}

func TestValidate_RejectsMissingAndUnknownTriggers(t *testing.T) {
	noTrigger := newAutomation(t)
	noTrigger.Trigger = automation.TriggerSelection{}
	assertMustFix(t, noTrigger, automation.ProblemSubjectTrigger, "")

	unknownTrigger := newAutomation(t)
	unknownTrigger.Trigger.ID = "test.raid"
	assertMustFix(t, unknownTrigger, automation.ProblemSubjectTrigger, "")
}

func TestValidate_RejectsUnknownStepTypes(t *testing.T) {
	assertMustFix(t, newAutomation(t, automation.Step{ID: "dance", Type: "test.dance"}), automation.ProblemSubjectStep, "dance")
}

func TestValidate_RejectsDuplicateStepIDsAcrossBranches(t *testing.T) {
	assertMustFix(t, newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd, rollStep("roll")),
			branch(automation.AfterBranchEnd, rollStep("roll")),
		),
	), automation.ProblemSubjectStep, "roll")
}

func TestValidate_RejectsStepWithoutID(t *testing.T) {
	assertMustFix(t, newAutomation(t, rollStep("")), automation.ProblemSubjectStep, "")
}

func TestValidate_RejectsMissingAndExtraBranches(t *testing.T) {
	missingBranch := waitStep("wait", branch(automation.AfterBranchEnd), branch(automation.AfterBranchEnd))
	delete(missingBranch.Branches, timedOutOutcomeID)
	assertMustFix(t, newAutomation(t, missingBranch), automation.ProblemSubjectStep, "wait")

	wrongBranch := waitStep("wait", branch(automation.AfterBranchEnd), branch(automation.AfterBranchEnd))
	delete(wrongBranch.Branches, timedOutOutcomeID)
	wrongBranch.Branches["gaveUp"] = branch(automation.AfterBranchEnd)
	assertMustFix(t, newAutomation(t, wrongBranch), automation.ProblemSubjectStep, "wait")

	branchesOnSingleOutcomeStep := rollStep("roll")
	branchesOnSingleOutcomeStep.Branches = map[string]automation.Branch{step.DefaultOutcomeID: branch(automation.AfterBranchContinue)}
	assertMustFix(t, newAutomation(t, branchesOnSingleOutcomeStep), automation.ProblemSubjectStep, "roll")
}

func TestValidate_RejectsUnknownBranchEnding(t *testing.T) {
	assertMustFix(t, newAutomation(t,
		waitStep("wait", branch("jumpToStart"), branch(automation.AfterBranchEnd)),
	), automation.ProblemSubjectStep, "wait")
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
	assertMustFix(t, unnamedAutomation, automation.ProblemSubjectAutomation, "")
}

func TestValidate_AcceptsNewID(t *testing.T) {
	newIDAutomation := newAutomation(t)
	newIDAutomation.ID = automation.NewID()
	assertValid(t, newIDAutomation)
}

func TestValidate_ReturnsFirstProblemMessage(t *testing.T) {
	err := automation.Validate(newAutomation(t, automation.Step{ID: "say", Type: sayStepType}), newTestCatalog(t))

	if err == nil || !strings.Contains(err.Error(), messagePlaceholder) {
		t.Errorf("got error %v, want it to say the step needs you to %s", err, messagePlaceholder)
	}
}

func TestValidate_IgnoresSettingValuesWithoutDefinitions(t *testing.T) {
	stepWithDroppedSetting := sayStep(t, "say", nil)
	stepWithDroppedSetting.Settings["droppedSetting"] = encode(t, "anything")

	assertValid(t, newAutomation(t, stepWithDroppedSetting))
}

func TestCheck_ReportsEveryProblemNotJustTheFirst(t *testing.T) {
	testAutomation := newAutomation(t,
		automation.Step{ID: "firstSay", Type: sayStepType},
		automation.Step{ID: "secondSay", Type: sayStepType},
	)
	testAutomation.Name = ""

	report := automation.Check(testAutomation, newTestCatalog(t))

	if len(report.Problems) != 3 {
		t.Errorf("got problems %+v, want one for the name and one for each step", report.Problems)
	}
}

func TestCheck_OrdersProblemsAsTheAutomationReads(t *testing.T) {
	testAutomation := newAutomation(t,
		automation.Step{ID: "sayWithoutMessage", Type: sayStepType},
		automation.Step{ID: "dance", Type: "test.dance"},
	)
	testAutomation.Name = ""
	testAutomation.Trigger.Settings = automationsetting.Values{}

	report := automation.Check(testAutomation, newTestCatalog(t))

	var subjects []string
	for _, problem := range report.Problems {
		subjects = append(subjects, string(problem.Subject)+":"+problem.StepID)
	}
	wantedSubjects := []string{"automation:", "trigger:", "step:sayWithoutMessage", "step:dance"}
	if !slices.Equal(subjects, wantedSubjects) {
		t.Errorf("got problems about %v, want %v", subjects, wantedSubjects)
	}
}

func TestCheck_UsesPlaceholderForMissingSetting(t *testing.T) {
	testAutomation := newAutomation(t, automation.Step{ID: "say", Type: sayStepType})
	testAutomation.Trigger.Settings = automationsetting.Values{}

	report := automation.Check(testAutomation, newTestCatalog(t))

	wantedMessages := []string{"The trigger needs you to " + rewardPlaceholder + ".", "“Say” needs you to " + messagePlaceholder + "."}
	var messages []string
	for _, problem := range report.Problems {
		messages = append(messages, problem.Message)
	}
	if !slices.Equal(messages, wantedMessages) {
		t.Errorf("got messages %q, want %q", messages, wantedMessages)
	}
	if report.Problems[1].SettingID != messageSettingID {
		t.Errorf("got setting %q, want %q", report.Problems[1].SettingID, messageSettingID)
	}
}

func TestCheck_NamesVariableThatComesFromALaterStep(t *testing.T) {
	report := automation.Check(newAutomation(t,
		sayStep(t, "sayRoll", referenceTo(stepVariable("roll", rollVariable))),
		rollStep("roll"),
	), newTestCatalog(t))

	if len(report.Problems) != 1 || !strings.Contains(report.Problems[0].Message, "“Roll”") {
		t.Errorf("got problems %+v, want one naming the roll", report.Problems)
	}
}

func TestCheck_ReportsVariableFromDeletedStepAsNoLongerExisting(t *testing.T) {
	report := automation.Check(newAutomation(t,
		sayStep(t, "sayRoll", referenceTo(stepVariable("deletedRoll", rollVariable))),
	), newTestCatalog(t))

	if len(report.Problems) != 1 || !strings.Contains(report.Problems[0].Message, "no longer exists") {
		t.Errorf("got problems %+v, want one saying the value no longer exists", report.Problems)
	}
}

func TestCheck_ReportsMessageWhoseOwnTextIsTooLong(t *testing.T) {
	tooLongMessage := automationsetting.Template{{Text: strings.Repeat("a", maximumMessageLength+1)}}
	tooLongStep := automation.Step{ID: "say", Type: sayStepType, Settings: automationsetting.Values{messageSettingID: encode(t, tooLongMessage)}}

	assertMustFix(t, newAutomation(t, tooLongStep), automation.ProblemSubjectStep, "say")
}

func TestCheck_ReportsDurationOverItsLimitInMinutes(t *testing.T) {
	tooLongPause := automation.Step{ID: "pause", Type: pauseStepType, Settings: automationsetting.Values{pauseSettingID: encode(t, maximumPause+1)}}

	report := automation.Check(newAutomation(t, tooLongPause), newTestCatalog(t))

	want := automation.Problem{Level: automation.ProblemLevelMustFix, Subject: automation.ProblemSubjectStep, StepID: "pause", SettingID: pauseSettingID, Message: "“Pause” is over its 5-minute limit."}
	if len(report.Problems) != 1 || !problemsMatch(report.Problems[0], want) {
		t.Errorf("got problems %+v, want only %+v", report.Problems, want)
	}
}

func TestCheck_ReportsStepsAfterSplitWhereEveryBranchEndsAsGoodToKnow(t *testing.T) {
	testAutomation := newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd),
			branch(automation.AfterBranchEnd),
		),
		sayStep(t, "neverRuns", nil),
		rollStep("alsoNeverRuns"),
	)

	report := automation.Check(testAutomation, newTestCatalog(t))

	wantedProblem := automation.Problem{
		Level:   automation.ProblemLevelGoodToKnow,
		Subject: automation.ProblemSubjectStep,
		StepID:  "neverRuns",
		Message: "“Say” never runs, because every path above it stops.",
	}
	if len(report.Problems) != 1 || report.Problems[0].Message != wantedProblem.Message || report.Problems[0].StepID != wantedProblem.StepID || report.Problems[0].Level != wantedProblem.Level {
		t.Errorf("got problems %+v, want only %+v", report.Problems, wantedProblem)
	}
	if !report.Steps["wait"].IsReachable || report.Steps["neverRuns"].IsReachable || report.Steps["alsoNeverRuns"].IsReachable {
		t.Errorf("got step reports %+v, want only the wait step reachable", report.Steps)
	}
	assertValid(t, testAutomation)
}

func TestCheck_MarksStepsInsideBranchesOfUnreachableStepsUnreachable(t *testing.T) {
	report := automation.Check(newAutomation(t,
		waitStep("wait", branch(automation.AfterBranchEnd), branch(automation.AfterBranchEnd)),
		waitStep("neverRuns", branch(automation.AfterBranchEnd, rollStep("insideNeverRuns")), branch(automation.AfterBranchEnd)),
	), newTestCatalog(t))

	if report.Steps["insideNeverRuns"].IsReachable {
		t.Error("got the step inside an unreachable step reachable")
	}
	if len(report.Problems) != 1 {
		t.Errorf("got problems %+v, want one for the first step that never runs", report.Problems)
	}
}

func TestCheck_SkipsTriggerVariablesWhileThereIsNoTrigger(t *testing.T) {
	testAutomation := newAutomation(t, sayStep(t, "say", referenceTo(triggerVariable(redeemerNameVariable))))
	testAutomation.Trigger = automation.TriggerSelection{}

	report := automation.Check(testAutomation, newTestCatalog(t))

	if len(report.Problems) != 1 || report.Problems[0].Subject != automation.ProblemSubjectTrigger {
		t.Errorf("got problems %+v, want only the missing trigger", report.Problems)
	}
}

func availableReferences(variables []automation.AvailableVariable) []automationsetting.VariableReference {
	var references []automationsetting.VariableReference
	for _, variable := range variables {
		references = append(references, variable.Reference)
	}
	return references
}

func TestCheck_ListsTriggerVariablesThenEarlierSteps(t *testing.T) {
	testAutomation := newAutomation(t,
		rollStep("firstRoll"),
		rollStep("secondRoll"),
		sayStep(t, "say", nil),
	)

	variables := automation.Check(testAutomation, newTestCatalog(t)).Steps["say"].AvailableVariables

	wantedReferences := []automationsetting.VariableReference{
		triggerVariable(redeemerNameVariable),
		stepVariable("firstRoll", rollVariable),
		stepVariable("secondRoll", rollVariable),
	}
	if !slices.Equal(availableReferences(variables), wantedReferences) {
		t.Fatalf("got %v, want %v", variables, wantedReferences)
	}
	if variables[0].DisplayName != "Redeemer's name" || !variables[0].IsCommon || variables[1].DisplayName != "Roll" {
		t.Errorf("got variables %+v", variables)
	}
}

func TestCheck_IncludesOutcomeVariablesInsideBranch(t *testing.T) {
	testAutomation := newAutomation(t,
		waitStep("wait",
			branch(automation.AfterBranchEnd, sayStep(t, "sayChoice", nil)),
			branch(automation.AfterBranchEnd),
		),
	)

	variables := automation.Check(testAutomation, newTestCatalog(t)).Steps["sayChoice"].AvailableVariables

	if !slices.Contains(availableReferences(variables), stepVariable("wait", replyVariable)) {
		t.Errorf("got %v, want the reply included", variables)
	}
}

func TestCheck_ReportsEachStepsOutcomes(t *testing.T) {
	report := automation.Check(newAutomation(t,
		waitStep("wait", branch(automation.AfterBranchEnd), branch(automation.AfterBranchEnd)),
	), newTestCatalog(t))

	var outcomeIDs []string
	for _, outcome := range report.Steps["wait"].Outcomes {
		outcomeIDs = append(outcomeIDs, outcome.ID)
	}
	if !slices.Equal(outcomeIDs, []string{repliedOutcomeID, timedOutOutcomeID}) {
		t.Errorf("got outcomes %v", outcomeIDs)
	}
}

func TestCheck_ReportsStepsWhileSettingsAreUnfinished(t *testing.T) {
	report := automation.Check(newAutomation(t, automation.Step{ID: "say", Type: sayStepType}), newTestCatalog(t))

	stepReport, isFound := report.Steps["say"]
	if !isFound || len(stepReport.AvailableVariables) != 1 || !stepReport.IsReachable {
		t.Errorf("got report %+v, want the trigger's variable and the step reachable", stepReport)
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

func TestNewCatalog_ReturnsErrorWhenSentenceDoesntPlaceEachSettingOnce(t *testing.T) {
	message := automationsetting.Definition{ID: messageSettingID, Kind: automationsetting.KindTemplate}
	mismatchedSentences := map[string]automationsetting.Sentence{
		"missing setting":   {automationsetting.Words("Say")},
		"repeated setting":  {automationsetting.Setting(messageSettingID), automationsetting.Setting(messageSettingID)},
		"undefined setting": {automationsetting.Setting(messageSettingID), automationsetting.Setting("volume")},
	}

	for description, sentence := range mismatchedSentences {
		steps := []step.Step{fakeStep{definition: step.Definition{ID: sayStepType, Sentence: sentence, Settings: []automationsetting.Definition{message}}}}
		_, err := automation.NewCatalog(nil, steps)
		if !errors.Is(err, automation.ErrSentenceMismatch) {
			t.Errorf("%s: got error %v, want ErrSentenceMismatch", description, err)
		}
	}
}

func TestNewCatalog_ReturnsErrorWhenTriggersPersonIsntOneOfItsVariables(t *testing.T) {
	withUnknownPerson := fakeTrigger{definition: trigger.Definition{ID: "test.unknownPerson", PersonUserIDVariable: "nobody"}}

	_, err := automation.NewCatalog([]trigger.Trigger{withUnknownPerson}, nil)

	if !errors.Is(err, automation.ErrUnknownPersonVariable) {
		t.Errorf("got error %v, want ErrUnknownPersonVariable", err)
	}
}

func TestNewCatalog_ReturnsErrorWhenASettingStartsWithAValueItRefuses(t *testing.T) {
	brokenStart := fakeStep{definition: step.Definition{
		ID:       "test.brokenStart",
		Sentence: automationsetting.Sentence{automationsetting.Setting(pauseSettingID)},
		Settings: []automationsetting.Definition{{ID: pauseSettingID, Kind: automationsetting.KindDuration, MaximumMilliseconds: maximumPause, InitialValue: encode(t, maximumPause+1)}},
	}}

	_, err := automation.NewCatalog(nil, []step.Step{brokenStart})

	if !errors.Is(err, automation.ErrInvalidInitialValue) {
		t.Errorf("got error %v, want ErrInvalidInitialValue", err)
	}
}

func TestCheck_ReportsStepUsageProblemsAsMustFix(t *testing.T) {
	greetWithoutPerson := newAutomation(t, automation.Step{ID: "greet", Type: greetStepType})
	greetWithoutPerson.Trigger = automation.TriggerSelection{ID: notingTriggerID, Settings: automationsetting.Values{rewardSettingID: encode(t, "reward-1")}}
	greetWithPerson := newAutomation(t, automation.Step{ID: "greet", Type: greetStepType})

	withoutPersonReport := automation.Check(greetWithoutPerson, newTestCatalog(t))
	withPersonReport := automation.Check(greetWithPerson, newTestCatalog(t))

	want := automation.Problem{Level: automation.ProblemLevelMustFix, Subject: automation.ProblemSubjectStep, StepID: "greet", Message: noPersonMessage}
	if len(withoutPersonReport.Problems) != 1 || !problemsMatch(withoutPersonReport.Problems[0], want) {
		t.Errorf("got problems %+v, want only %+v", withoutPersonReport.Problems, want)
	}
	if len(withPersonReport.Problems) != 0 {
		t.Errorf("got problems %+v with a person, want none", withPersonReport.Problems)
	}
}

func TestCatalog_ListsDefinitionsInTheOrderGiven(t *testing.T) {
	catalog := newTestCatalog(t)

	var stepIDs []string
	for _, definition := range catalog.StepDefinitions() {
		stepIDs = append(stepIDs, definition.ID)
	}
	var triggerIDs []string
	for _, definition := range catalog.TriggerDefinitions() {
		triggerIDs = append(triggerIDs, definition.ID)
	}

	if !slices.Equal(stepIDs, []string{sayStepType, rollStepType, checkStepType, waitStepType, pauseStepType, greetStepType, refundStepType}) {
		t.Errorf("got steps %v", stepIDs)
	}
	if !slices.Equal(triggerIDs, []string{redeemTriggerID, notingTriggerID}) {
		t.Errorf("got triggers %v", triggerIDs)
	}
}

func TestCheck_ReportsTriggerNotesAsGoodToKnow(t *testing.T) {
	onPlaceholder := newAutomation(t)
	onPlaceholder.Trigger = automation.TriggerSelection{ID: notingTriggerID, Settings: automationsetting.Values{rewardSettingID: encode(t, placeholderReward)}}

	report := automation.Check(onPlaceholder, newTestCatalog(t))

	want := automation.Problem{Level: automation.ProblemLevelGoodToKnow, Subject: automation.ProblemSubjectTrigger, SettingID: rewardSettingID, Message: placeholderMessage}
	if len(report.Problems) != 1 || !problemsMatch(report.Problems[0], want) {
		t.Errorf("got problems %+v, want only the note", report.Problems)
	}
	if _, hasMustFix := report.FirstMustFixProblem(); hasMustFix {
		t.Error("a note made the automation must-fix")
	}
}

func TestCheck_LeavesOutTriggerNotesWhileItsSettingsHaveProblems(t *testing.T) {
	missingReward := newAutomation(t)
	missingReward.Trigger = automation.TriggerSelection{ID: notingTriggerID, Settings: automationsetting.Values{}}

	report := automation.Check(missingReward, newTestCatalog(t))

	for _, problem := range report.Problems {
		if problem.Level == automation.ProblemLevelGoodToKnow {
			t.Errorf("got note %+v alongside the missing reward", problem)
		}
	}
}

func TestCheck_RejectsStepThatNeedsAnotherTrigger(t *testing.T) {
	onOtherTrigger := newAutomation(t, automation.Step{ID: "refund", Type: refundStepType})
	onOtherTrigger.Trigger = automation.TriggerSelection{ID: notingTriggerID, Settings: automationsetting.Values{rewardSettingID: encode(t, "reward-1")}}

	report := automation.Check(onOtherTrigger, newTestCatalog(t))

	want := automation.Problem{Level: automation.ProblemLevelMustFix, Subject: automation.ProblemSubjectStep, StepID: "refund", Message: "“Refund” only works in automations started by “Redeem”."}
	if len(report.Problems) != 1 || !problemsMatch(report.Problems[0], want) {
		t.Errorf("got problems %+v, want only %+v", report.Problems, want)
	}
}

func TestCheck_AcceptsStepStartedByTheTriggerItNeeds(t *testing.T) {
	report := automation.Check(newAutomation(t, automation.Step{ID: "refund", Type: refundStepType}), newTestCatalog(t))

	if len(report.Problems) != 0 {
		t.Errorf("got problems %+v, want none", report.Problems)
	}
}

func TestCheck_SkipsRequiredTriggerWhileThereIsNoTrigger(t *testing.T) {
	withoutTrigger := newAutomation(t, automation.Step{ID: "refund", Type: refundStepType})
	withoutTrigger.Trigger = automation.TriggerSelection{}

	report := automation.Check(withoutTrigger, newTestCatalog(t))

	for _, problem := range report.Problems {
		if problem.Subject == automation.ProblemSubjectStep {
			t.Errorf("got step problem %+v while there's no trigger", problem)
		}
	}
}

func TestCheck_ReportsStepNotesAsGoodToKnowOnTheStep(t *testing.T) {
	unrefundable := newAutomation(t, rollStep("roll"), automation.Step{ID: "refund", Type: refundStepType})
	unrefundable.Trigger.Settings = automationsetting.Values{rewardSettingID: encode(t, unrefundableReward)}

	report := automation.Check(unrefundable, newTestCatalog(t))

	want := automation.Problem{Level: automation.ProblemLevelGoodToKnow, Subject: automation.ProblemSubjectStep, StepID: "refund", Message: unrefundableMessage}
	if len(report.Problems) != 1 || !problemsMatch(report.Problems[0], want) {
		t.Errorf("got problems %+v, want only the note", report.Problems)
	}
}

func TestCheck_LeavesOutStepNotesWhenTheStepNeedsAnotherTrigger(t *testing.T) {
	onOtherTrigger := newAutomation(t, automation.Step{ID: "refund", Type: refundStepType})
	onOtherTrigger.Trigger = automation.TriggerSelection{ID: notingTriggerID, Settings: automationsetting.Values{rewardSettingID: encode(t, unrefundableReward)}}

	report := automation.Check(onOtherTrigger, newTestCatalog(t))

	for _, problem := range report.Problems {
		if problem.Level == automation.ProblemLevelGoodToKnow {
			t.Errorf("got note %+v alongside the wrong trigger", problem)
		}
	}
}

// problemsMatch compares the fields a problem shows, since its position is
// internal.
func problemsMatch(got automation.Problem, want automation.Problem) bool {
	return got.Level == want.Level && got.Subject == want.Subject && got.StepID == want.StepID && got.SettingID == want.SettingID && got.Message == want.Message
}
