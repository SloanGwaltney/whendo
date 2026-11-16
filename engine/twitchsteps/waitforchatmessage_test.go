package twitchsteps_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

const (
	redeemerUserID   = "4242"
	otherViewerID    = "5151"
	firstChoiceID    = "irish"
	secondChoiceID   = "scottish"
	longTimeLimit    = time.Minute
	shortTimeLimit   = 20 * time.Millisecond
	testEventTimeout = 5 * time.Second
)

// fakeEvents delivers the events a test gives it to the one step that
// subscribes, and remembers whether the step unsubscribed.
type fakeEvents struct {
	events         chan trigger.Event
	mutex          sync.Mutex
	isUnsubscribed bool
}

func newFakeEvents(events ...trigger.Event) *fakeEvents {
	fake := &fakeEvents{events: make(chan trigger.Event, len(events)+1)}
	for _, event := range events {
		fake.events <- event
	}
	return fake
}

func (fake *fakeEvents) Subscribe() step.EventSubscription { return fake }

func (fake *fakeEvents) Events() <-chan trigger.Event { return fake.events }

func (fake *fakeEvents) Unsubscribe() {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	fake.isUnsubscribed = true
}

func (fake *fakeEvents) wasUnsubscribed() bool {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.isUnsubscribed
}

func chatMessage(senderUserID string, text string) trigger.Event {
	return trigger.Event{
		TriggerID: twitcheventsub.ChatMessageTriggerID,
		Variables: map[string]string{
			twitcheventsub.MessageIDVariable:         "message-" + text,
			twitcheventsub.SenderUserIDVariable:      senderUserID,
			twitcheventsub.SenderLoginVariable:       "login-" + senderUserID,
			twitcheventsub.SenderDisplayNameVariable: "Name" + senderUserID,
			twitcheventsub.MessageTextVariable:       text,
		},
	}
}

func encodeValue(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encodedValue, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %v: %v", value, err)
	}
	return encodedValue
}

func waitSettings(t *testing.T, timeLimit time.Duration, from string, choices ...automationsetting.Choice) automationsetting.Values {
	t.Helper()
	return automationsetting.Values{
		twitchsteps.TimeLimitSetting: encodeValue(t, timeLimit.Milliseconds()),
		twitchsteps.FromSetting:      encodeValue(t, from),
		twitchsteps.ChoicesSetting:   encodeValue(t, choices),
	}
}

var voiceChoices = []automationsetting.Choice{{ID: firstChoiceID, Text: "!a1"}, {ID: secondChoiceID, Text: " !a2 "}}

func waitStep(t *testing.T) step.Step {
	t.Helper()
	_, server := newFakeTwitch(t)
	return twitchStep(t, twitchsteps.WaitForChatMessageStepID, server, &fakeAccount{isLoggedIn: true}, fakeRewards{})
}

// runWait runs the step and fails the test if it doesn't finish.
func runWait(t *testing.T, ctx context.Context, settings automationsetting.Values, events *fakeEvents) (step.Result, error) {
	t.Helper()
	type runOutput struct {
		result step.Result
		err    error
	}
	finished := make(chan runOutput, 1)
	stepToRun := waitStep(t)
	go func() {
		result, err := stepToRun.Run(ctx, step.Input{Settings: settings, Events: events, StartedByUserID: redeemerUserID})
		finished <- runOutput{result, err}
	}()
	select {
	case output := <-finished:
		return output.result, output.err
	case <-time.After(testEventTimeout):
		t.Fatal("the step never finished")
		return step.Result{}, nil
	}
}

func TestWaitForChatMessage_TakesTheChoiceThePersonWhoStartedItTypes(t *testing.T) {
	events := newFakeEvents(chatMessage(redeemerUserID, "!A2 please"))

	result, err := runWait(t, context.Background(), waitSettings(t, longTimeLimit, twitchsteps.FromPersonWhoStartedIt, voiceChoices...), events)

	if err != nil || result.OutcomeID != secondChoiceID {
		t.Fatalf("got result %+v, error %v, want the second choice", result, err)
	}
	wantVariables := map[string]string{
		twitcheventsub.SenderDisplayNameVariable: "Name" + redeemerUserID,
		twitcheventsub.MessageTextVariable:       "!A2 please",
		twitcheventsub.RestOfMessageVariable:     "please",
		twitcheventsub.SenderLoginVariable:       "login-" + redeemerUserID,
		twitcheventsub.SenderUserIDVariable:      redeemerUserID,
		twitcheventsub.MessageIDVariable:         "message-!A2 please",
	}
	for variableID, wantValue := range wantVariables {
		if result.Variables[variableID] != wantValue {
			t.Errorf("got %s %q, want %q", variableID, result.Variables[variableID], wantValue)
		}
	}
	if !events.wasUnsubscribed() {
		t.Error("the step didn't unsubscribe")
	}
}

func TestWaitForChatMessage_IgnoresOtherPeopleAndMessagesThatMatchNoChoice(t *testing.T) {
	events := newFakeEvents(
		chatMessage(otherViewerID, "!a1"),
		chatMessage(redeemerUserID, "!a10"),
		trigger.Event{TriggerID: twitcheventsub.FollowTriggerID, Variables: map[string]string{}},
		chatMessage(redeemerUserID, "!a1"),
	)

	result, err := runWait(t, context.Background(), waitSettings(t, longTimeLimit, twitchsteps.FromPersonWhoStartedIt, voiceChoices...), events)

	if err != nil || result.OutcomeID != firstChoiceID || result.Variables[twitcheventsub.SenderUserIDVariable] != redeemerUserID {
		t.Errorf("got result %+v, error %v, want the redeemer's !a1", result, err)
	}
}

func TestWaitForChatMessage_AcceptsAnyoneWhenFromAnyone(t *testing.T) {
	events := newFakeEvents(chatMessage(otherViewerID, "!a1"))

	result, err := runWait(t, context.Background(), waitSettings(t, longTimeLimit, twitchsteps.FromAnyone, voiceChoices...), events)

	if err != nil || result.OutcomeID != firstChoiceID {
		t.Errorf("got result %+v, error %v, want another viewer's answer accepted", result, err)
	}
}

func TestWaitForChatMessage_EarlierChoiceWinsWhenTwoMatch(t *testing.T) {
	overlapping := []automationsetting.Choice{{ID: firstChoiceID, Text: "!a"}, {ID: secondChoiceID, Text: "!a 1"}}
	events := newFakeEvents(chatMessage(redeemerUserID, "!a 1"))

	result, err := runWait(t, context.Background(), waitSettings(t, longTimeLimit, twitchsteps.FromAnyone, overlapping...), events)

	if err != nil || result.OutcomeID != firstChoiceID {
		t.Errorf("got result %+v, error %v, want the earlier choice", result, err)
	}
}

func TestWaitForChatMessage_TimesOutWithoutAnAnswer(t *testing.T) {
	events := newFakeEvents(chatMessage(otherViewerID, "!a1"))

	result, err := runWait(t, context.Background(), waitSettings(t, shortTimeLimit, twitchsteps.FromPersonWhoStartedIt, voiceChoices...), events)

	if err != nil || result.OutcomeID != twitchsteps.TimedOutOutcomeID || len(result.Variables) != 0 {
		t.Errorf("got result %+v, error %v, want timed out with no variables", result, err)
	}
	if !events.wasUnsubscribed() {
		t.Error("the step didn't unsubscribe")
	}
}

func TestWaitForChatMessage_StopsWhenWhendoShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runWait(t, ctx, waitSettings(t, longTimeLimit, twitchsteps.FromPersonWhoStartedIt, voiceChoices...), newFakeEvents())

	if !errors.Is(err, context.Canceled) {
		t.Errorf("got error %v, want the cancellation", err)
	}
}

func TestWaitForChatMessage_FailsWhenThereIsNoPersonToWaitFor(t *testing.T) {
	input := step.Input{Settings: waitSettings(t, longTimeLimit, twitchsteps.FromPersonWhoStartedIt, voiceChoices...), Events: newFakeEvents(), StartedByUserID: ""}

	_, err := waitStep(t).Run(context.Background(), input)

	if err == nil {
		t.Error("got no error, want a failure for no person to wait for")
	}
}

func TestWaitForChatMessage_HasAnOutcomePerChoiceThenTimedOut(t *testing.T) {
	cases := map[string][]string{
		twitchsteps.FromPersonWhoStartedIt: {"If they type !a1", "If they type !a2", "If they don't answer in time"},
		twitchsteps.FromAnyone:             {"If someone types !a1", "If someone types !a2", "If no one answers in time"},
	}
	for from, wantSentences := range cases {
		outcomes, err := waitStep(t).Outcomes(waitSettings(t, longTimeLimit, from, voiceChoices...))
		if err != nil {
			t.Fatalf("%s: got error %v", from, err)
		}

		var outcomeIDs, sentences []string
		for _, outcome := range outcomes {
			outcomeIDs = append(outcomeIDs, outcome.ID)
			sentences = append(sentences, sentenceText(outcome.Sentence))
		}
		if !slices.Equal(outcomeIDs, []string{firstChoiceID, secondChoiceID, twitchsteps.TimedOutOutcomeID}) {
			t.Errorf("%s: got outcomes %v", from, outcomeIDs)
		}
		if !slices.Equal(sentences, wantSentences) {
			t.Errorf("%s: got sentences %q, want %q", from, sentences, wantSentences)
		}
		if len(outcomes[0].Variables) == 0 || len(outcomes[2].Variables) != 0 {
			t.Errorf("%s: want the answer in each choice's path and nothing when timed out", from)
		}
	}
}

func sentenceText(sentence automationsetting.Sentence) string {
	var text string
	for partIndex, part := range sentence {
		if partIndex > 0 {
			text += " "
		}
		text += part.Words + part.UserText
	}
	return text
}

func TestWaitForChatMessage_RefusesAChoiceWithTheTimedOutID(t *testing.T) {
	clashing := automationsetting.Choice{ID: twitchsteps.TimedOutOutcomeID, Text: "!late"}

	_, err := waitStep(t).Outcomes(waitSettings(t, longTimeLimit, twitchsteps.FromAnyone, clashing))

	if !errors.Is(err, twitchsteps.ErrTimedOutChoiceID) {
		t.Errorf("got error %v, want ErrTimedOutChoiceID", err)
	}
}

func TestWaitForChatMessage_CantWaitForThePersonAfterATriggerWithoutOne(t *testing.T) {
	checker, canCheck := waitStep(t).(step.UsageChecker)
	if !canCheck {
		t.Fatal("the step doesn't check how it's used")
	}
	personSettings := waitSettings(t, longTimeLimit, twitchsteps.FromPersonWhoStartedIt, voiceChoices...)
	anyoneSettings := waitSettings(t, longTimeLimit, twitchsteps.FromAnyone, voiceChoices...)

	withoutPerson := checker.CheckUsage(step.Usage{Settings: personSettings, TriggerName: "Timer", HasTriggerPerson: false})
	withPerson := checker.CheckUsage(step.Usage{Settings: personSettings, HasTriggerPerson: true})
	fromAnyone := checker.CheckUsage(step.Usage{Settings: anyoneSettings, HasTriggerPerson: false})

	if len(withoutPerson) != 1 || withoutPerson[0].SettingID != twitchsteps.FromSetting {
		t.Errorf("got %+v, want a problem on who can answer", withoutPerson)
	}
	if len(withPerson) != 0 || len(fromAnyone) != 0 {
		t.Errorf("got %+v and %+v, want no problems", withPerson, fromAnyone)
	}
}
