package runner_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automation"
	"github.com/SloanGwaltney/whendo/engine/runner"
	"github.com/SloanGwaltney/whendo/engine/step"
)

func TestReceiveEvent_RunsMatchingAutomationWithTriggerVariables(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("greet", "hello",
		testHarness.sayStep("say", "Hi ", triggerVariable(senderVariable)),
	))

	testHarness.sendEvent("hello", "Cooler_User")

	if said := testHarness.awaitSaid(); said != "Hi Cooler_User" {
		t.Errorf("said %q, want %q", said, "Hi Cooler_User")
	}
	record := testHarness.awaitRecord()
	if record.Status != runner.RunStatusSucceeded || record.AutomationID != "greet" || record.IsTest {
		t.Errorf("got record %+v, want a succeeded real run of greet", record)
	}
	if len(record.Steps) != 1 || record.Steps[0].StepID != "say" || record.Steps[0].OutcomeID != step.DefaultOutcomeID {
		t.Errorf("got step records %+v", record.Steps)
	}
	if record.TriggerVariables[senderVariable] != "Cooler_User" {
		t.Errorf("got trigger variables %v", record.TriggerVariables)
	}
}

func TestReceiveEvent_IgnoresNonMatchingAndDisabledAutomations(t *testing.T) {
	testHarness := newHarness(t)
	otherWord := testHarness.newAutomation("otherWord", "goodbye", testHarness.sayStep("say", "wrong word"))
	disabled := testHarness.newAutomation("disabled", "hello", testHarness.sayStep("say", "disabled"))
	disabled.IsEnabled = false
	testHarness.addAutomation(otherWord)
	testHarness.addAutomation(disabled)

	testHarness.sendEvent("hello", "Cooler_User")
	testHarness.runner.Shutdown()

	assertNothingArrives(t, testHarness.said, "say step")
	assertNothingArrives(t, testHarness.records, "run record")
}

func TestReceiveEvent_RunsOneRunAtATimeInOrder(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("queued", "hello",
		simpleStep("block", blockStepType),
		testHarness.sayStep("say", "", triggerVariable(senderVariable)),
	))

	for _, sender := range []string{"first", "second", "third"} {
		testHarness.sendEvent("hello", sender)
	}

	for _, wantedSender := range []string{"first", "second", "third"} {
		testHarness.awaitBlockStarted()
		assertNothingArrives(t, testHarness.blockStarted, "second block step running at once")
		testHarness.releaseBlock()
		if said := testHarness.awaitSaid(); said != wantedSender {
			t.Errorf("said %q, want %q", said, wantedSender)
		}
	}
}

func TestReceiveEvent_RunsDifferentAutomationsInParallel(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("slow", "hello", simpleStep("block", blockStepType)))
	testHarness.addAutomation(testHarness.newAutomation("fast", "hello", testHarness.sayStep("say", "fast")))

	testHarness.sendEvent("hello", "Cooler_User")

	testHarness.awaitBlockStarted()
	if said := testHarness.awaitSaid(); said != "fast" {
		t.Errorf("said %q, want %q", said, "fast")
	}
	testHarness.releaseBlock()
}

func TestReceiveEvent_RecordsRunAsFailedWhenQueueIsFull(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("busy", "hello", simpleStep("block", blockStepType)))
	testHarness.sendEvent("hello", "running")
	testHarness.awaitBlockStarted()
	for range runner.MaximumQueuedRuns {
		testHarness.sendEvent("hello", "queued")
	}

	testHarness.sendEvent("hello", "one too many")

	record := testHarness.awaitRecord()
	if record.Status != runner.RunStatusFailed || record.TriggerVariables[senderVariable] != "one too many" || !record.StartedAt.IsZero() {
		t.Errorf("got record %+v, want the extra run failed without starting", record)
	}
}

func TestSetAutomation_DisablingCancelsQueuedRunsButFinishesCurrentOne(t *testing.T) {
	testHarness := newHarness(t)
	disabledLater := testHarness.newAutomation("disabledLater", "hello", simpleStep("block", blockStepType))
	testHarness.addAutomation(disabledLater)
	testHarness.sendEvent("hello", "running")
	testHarness.awaitBlockStarted()
	testHarness.sendEvent("hello", "queued 1")
	testHarness.sendEvent("hello", "queued 2")

	disabledLater.IsEnabled = false
	testHarness.addAutomation(disabledLater)

	for range 2 {
		record := testHarness.awaitRecord()
		if record.Status != runner.RunStatusCancelled || !strings.HasPrefix(record.TriggerVariables[senderVariable], "queued") {
			t.Errorf("got record %+v, want a queued run cancelled", record)
		}
	}
	testHarness.releaseBlock()
	if record := testHarness.awaitRecord(); record.Status != runner.RunStatusSucceeded {
		t.Errorf("got record %+v, want the running run to finish", record)
	}
}

func TestSetAutomation_EditsApplyToRunsThatHaventStarted(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("edited", "hello",
		simpleStep("block", blockStepType),
		testHarness.sayStep("say", "before the edit"),
	))
	testHarness.sendEvent("hello", "running")
	testHarness.awaitBlockStarted()
	testHarness.sendEvent("hello", "queued")

	testHarness.addAutomation(testHarness.newAutomation("edited", "hello",
		simpleStep("block", blockStepType),
		testHarness.sayStep("say", "after the edit"),
	))

	testHarness.releaseBlock()
	if said := testHarness.awaitSaid(); said != "before the edit" {
		t.Errorf("the run in progress said %q, want %q", said, "before the edit")
	}
	testHarness.awaitBlockStarted()
	testHarness.releaseBlock()
	if said := testHarness.awaitSaid(); said != "after the edit" {
		t.Errorf("the queued run said %q, want %q", said, "after the edit")
	}
}

func TestSetAutomation_RefusesInvalidAutomationAndKeepsPreviousVersion(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("kept", "hello", testHarness.sayStep("say", "previous version")))
	invalid := testHarness.newAutomation("kept", "hello", simpleStep("dance", "test.dance"))

	err := testHarness.runner.SetAutomation(invalid)

	if !errors.Is(err, automation.ErrInvalidAutomation) {
		t.Fatalf("got error %v, want ErrInvalidAutomation", err)
	}
	testHarness.sendEvent("hello", "Cooler_User")
	if said := testHarness.awaitSaid(); said != "previous version" {
		t.Errorf("said %q, want the previous version to run", said)
	}
}

func TestRemoveAutomation_CancelsQueuedRunsAndStopsMatching(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("removed", "hello", simpleStep("block", blockStepType)))
	testHarness.sendEvent("hello", "running")
	testHarness.awaitBlockStarted()
	testHarness.sendEvent("hello", "queued")

	err := testHarness.runner.RemoveAutomation("removed")

	if err != nil {
		t.Fatalf("RemoveAutomation returned an error: %v", err)
	}
	if record := testHarness.awaitRecord(); record.Status != runner.RunStatusCancelled {
		t.Errorf("got record %+v, want the queued run cancelled", record)
	}
	testHarness.releaseBlock()
	if record := testHarness.awaitRecord(); record.Status != runner.RunStatusSucceeded {
		t.Errorf("got record %+v, want the running run to finish", record)
	}
	testHarness.sendEvent("hello", "after removal")
	assertNothingArrives(t, testHarness.blockStarted, "block step after removal")
}

func TestRemoveAutomation_ReturnsErrorForUnknownAutomation(t *testing.T) {
	err := newHarness(t).runner.RemoveAutomation("missing")

	if !errors.Is(err, runner.ErrAutomationNotFound) {
		t.Errorf("got error %v, want ErrAutomationNotFound", err)
	}
}

func TestRun_FollowsBranchAndContinuesAfterSplit(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("branching", "hello",
		testHarness.chooseStep("choose", firstOutcomeID,
			branch(automation.AfterBranchContinue, testHarness.sayStep("inFirst", "first got ", stepVariable("choose", chosenVariable))),
			branch(automation.AfterBranchContinue, testHarness.sayStep("inSecond", "second")),
		),
		testHarness.sayStep("after", "after the split"),
	))

	testHarness.sendEvent("hello", "Cooler_User")

	if said := testHarness.awaitSaid(); said != "first got chosen-first" {
		t.Errorf("said %q in the branch", said)
	}
	if said := testHarness.awaitSaid(); said != "after the split" {
		t.Errorf("said %q after the split", said)
	}
	record := testHarness.awaitRecord()
	var stepIDs []string
	for _, stepRecord := range record.Steps {
		stepIDs = append(stepIDs, stepRecord.StepID)
	}
	if strings.Join(stepIDs, ",") != "choose,inFirst,after" || record.Steps[0].OutcomeID != firstOutcomeID {
		t.Errorf("got steps %v with first outcome %q", stepIDs, record.Steps[0].OutcomeID)
	}
}

func TestRun_BranchThatEndsSkipsStepsAfterSplit(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("ending", "hello",
		testHarness.chooseStep("choose", secondOutcomeID,
			branch(automation.AfterBranchContinue),
			branch(automation.AfterBranchEnd, testHarness.sayStep("inSecond", "second")),
		),
		testHarness.sayStep("after", "after the split"),
	))

	testHarness.sendEvent("hello", "Cooler_User")

	if said := testHarness.awaitSaid(); said != "second" {
		t.Errorf("said %q, want %q", said, "second")
	}
	if record := testHarness.awaitRecord(); record.Status != runner.RunStatusSucceeded || len(record.Steps) != 2 {
		t.Errorf("got record %+v, want success after two steps", record)
	}
	assertNothingArrives(t, testHarness.said, "say step after an ending branch")
}

func TestRun_FailingStepEndsRunAndRecordsWhichStep(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("failing", "hello",
		testHarness.sayStep("before", "before"),
		simpleStep("broken", failStepType),
		testHarness.sayStep("after", "after"),
	))

	testHarness.sendEvent("hello", "Cooler_User")

	record := testHarness.awaitRecord()
	if record.Status != runner.RunStatusFailed || !strings.Contains(record.FailureReason, failureMessage) {
		t.Errorf("got record %+v, want failed with the step's reason", record)
	}
	if len(record.Steps) != 2 || record.Steps[1].StepID != "broken" || record.Steps[1].FailureReason == "" {
		t.Errorf("got steps %+v, want the broken step recorded as failed", record.Steps)
	}
	testHarness.awaitSaid()
	assertNothingArrives(t, testHarness.said, "say step after a failure")
}

func TestRun_PanickingStepFailsRunAndRunnerKeepsWorking(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("panicking", "hello", simpleStep("panic", panicStepType)))
	testHarness.addAutomation(testHarness.newAutomation("healthy", "later", testHarness.sayStep("say", "still working")))

	testHarness.sendEvent("hello", "Cooler_User")

	record := testHarness.awaitRecord()
	if record.Status != runner.RunStatusFailed || record.Steps[0].FailureReason == "" {
		t.Errorf("got record %+v, want the panic recorded as a failed step", record)
	}
	testHarness.sendEvent("later", "Cooler_User")
	if said := testHarness.awaitSaid(); said != "still working" {
		t.Errorf("said %q, want %q", said, "still working")
	}
}

func TestRun_FailsWhenStepReturnsUndeclaredOutcome(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("confused", "hello", simpleStep("bad", badOutcomeType)))

	testHarness.sendEvent("hello", "Cooler_User")

	if record := testHarness.awaitRecord(); record.Status != runner.RunStatusFailed {
		t.Errorf("got record %+v, want failed", record)
	}
}

func TestShutdown_CancelsRunInProgressAndQueuedRuns(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("interrupted", "hello", simpleStep("block", blockStepType)))
	testHarness.sendEvent("hello", "running")
	testHarness.awaitBlockStarted()
	testHarness.sendEvent("hello", "queued")

	testHarness.runner.Shutdown()

	for range 2 {
		if record := testHarness.awaitRecord(); record.Status != runner.RunStatusCancelled {
			t.Errorf("got record %+v, want cancelled", record)
		}
	}
	if err := testHarness.runner.TestRun("interrupted", nil); !errors.Is(err, runner.ErrShutDown) {
		t.Errorf("TestRun after shutdown: got error %v, want ErrShutDown", err)
	}
}

func TestTestRun_RunsDisabledAutomationAndMarksItAsTest(t *testing.T) {
	testHarness := newHarness(t)
	disabled := testHarness.newAutomation("disabled", "hello", testHarness.sayStep("say", "tested by ", triggerVariable(senderVariable)))
	disabled.IsEnabled = false
	testHarness.addAutomation(disabled)

	err := testHarness.runner.TestRun("disabled", map[string]string{wordVariable: "hello", senderVariable: "Streamer"})

	if err != nil {
		t.Fatalf("TestRun returned an error: %v", err)
	}
	if said := testHarness.awaitSaid(); said != "tested by Streamer" {
		t.Errorf("said %q", said)
	}
	if record := testHarness.awaitRecord(); !record.IsTest {
		t.Errorf("got record %+v, want it marked as a test", record)
	}
}

func TestTestRun_GivesMissingTriggerVariablesEmptyValues(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("partial", "hello", testHarness.sayStep("say", "sender:", triggerVariable(senderVariable))))

	err := testHarness.runner.TestRun("partial", map[string]string{wordVariable: "hello"})

	if err != nil {
		t.Fatalf("TestRun returned an error: %v", err)
	}
	if said := testHarness.awaitSaid(); said != "sender:" {
		t.Errorf("said %q, want an empty sender", said)
	}
}

func TestTestRun_RefusesValuesTheTriggerWouldNeverMatch(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("picky", "hello", testHarness.sayStep("say", "ran")))

	err := testHarness.runner.TestRun("picky", map[string]string{wordVariable: "goodbye"})

	if !errors.Is(err, runner.ErrTestValuesDontMatch) {
		t.Errorf("got error %v, want ErrTestValuesDontMatch", err)
	}
}

func TestTestRun_ReturnsErrorForUnknownAutomation(t *testing.T) {
	err := newHarness(t).runner.TestRun("missing", nil)

	if !errors.Is(err, runner.ErrAutomationNotFound) {
		t.Errorf("got error %v, want ErrAutomationNotFound", err)
	}
}

func TestSubscriptions_WaitingStepReceivesEventsThatAlsoTriggerAutomations(t *testing.T) {
	testHarness := newHarness(t)
	testHarness.addAutomation(testHarness.newAutomation("waiting", "hello",
		automation.Step{ID: "wait", Type: waitStepType},
		testHarness.sayStep("say", "reply from ", stepVariable("wait", replierVariable)),
	))
	testHarness.addAutomation(testHarness.newAutomation("onReply", replyWord, testHarness.sayStep("say", "reply automation ran")))
	testHarness.sendEvent("hello", "Cooler_User")
	awaitValue(t, testHarness.waitSubscribed, "the wait step to subscribe")

	testHarness.sendEvent(replyWord, "VIPTest")

	saidMessages := map[string]bool{testHarness.awaitSaid(): true, testHarness.awaitSaid(): true}
	if !saidMessages["reply from VIPTest"] || !saidMessages["reply automation ran"] {
		t.Errorf("said %v, want both the waiting step and the reply automation to react", saidMessages)
	}
}
