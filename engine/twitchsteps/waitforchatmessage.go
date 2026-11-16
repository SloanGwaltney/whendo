package twitchsteps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

const (
	// WaitForChatMessageStepID is the step that waits for a chat message
	// starting with one of a list of choices, such as "!a1".
	WaitForChatMessageStepID = "twitch.waitForChatMessage"
	// TimeLimitSetting is how long the step waits for an answer.
	TimeLimitSetting = "timeLimit"
	// FromSetting is who may answer: one of the From options.
	FromSetting = "from"
	// ChoicesSetting is the list of texts an answer may start with.
	ChoicesSetting = "choices"

	// FromPersonWhoStartedIt accepts answers only from the person behind
	// the trigger's event, such as the redeemer.
	FromPersonWhoStartedIt = "personWhoStartedIt"
	// FromAnyone accepts answers from anyone in chat.
	FromAnyone = "anyone"

	// TimedOutOutcomeID is the outcome for no answer in time. Each choice's
	// outcome has the choice's ID, so no choice may have this one.
	TimedOutOutcomeID = "timedOut"

	// MaximumChatMessageWait is the longest the step may wait. An
	// automation runs one run at a time, so a long wait holds up every
	// later run. See docs/design/twitch-steps.md#time-limit.
	MaximumChatMessageWait = 5 * time.Minute
)

// ErrTimedOutChoiceID means a choice has the ID reserved for timing out.
var ErrTimedOutChoiceID = errors.New("a choice can't have the ID " + TimedOutOutcomeID)

type waitForChatMessageStep struct{}

// fromPersonWhoStartedItJSON is the "from" setting's initial value. The
// option ID needs no escaping, so it's written as JSON directly.
var fromPersonWhoStartedItJSON = json.RawMessage(`"` + FromPersonWhoStartedIt + `"`)

func (waitForChatMessageStep) Definition() step.Definition {
	return step.Definition{
		ID:          WaitForChatMessageStepID,
		DisplayName: "Wait for a chat message",
		Description: "Wait for someone to type one of your choices in chat, like !a1, and do something different for each.",
		Group:       chatGroup,
		Sentence: automationsetting.Sentence{
			automationsetting.Words("Wait up to"), automationsetting.Setting(TimeLimitSetting),
			automationsetting.Words("for"), automationsetting.Setting(FromSetting),
			automationsetting.Words("to type"), automationsetting.Setting(ChoicesSetting),
		},
		Settings: []automationsetting.Definition{
			{ID: TimeLimitSetting, DisplayName: "How long", Kind: automationsetting.KindDuration, IsRequired: true, Placeholder: "choose how long", MaximumMilliseconds: MaximumChatMessageWait.Milliseconds()},
			{ID: FromSetting, DisplayName: "From", Kind: automationsetting.KindOption, IsRequired: true, Placeholder: "choose who can answer", InitialValue: fromPersonWhoStartedItJSON, Options: []automationsetting.Option{
				{ID: FromPersonWhoStartedIt, DisplayName: "the person who started this"},
				{ID: FromAnyone, DisplayName: "anyone"},
			}},
			{ID: ChoicesSetting, DisplayName: "Choices", Kind: automationsetting.KindChoiceList, IsRequired: true, Placeholder: "add the choices"},
		},
	}
}

// answerVariables are what each choice's path provides about the answer.
// They share the chat message trigger's IDs and names, so the same value is
// called the same thing wherever it comes from.
var answerVariables = []trigger.VariableDefinition{
	{ID: twitcheventsub.SenderDisplayNameVariable, DisplayName: "Sender's name", IsCommon: true},
	{ID: twitcheventsub.MessageTextVariable, DisplayName: "Message", IsCommon: true},
	{ID: twitcheventsub.RestOfMessageVariable, DisplayName: "Rest of the message", IsCommon: true},
	{ID: twitcheventsub.SenderLoginVariable, DisplayName: "Sender's login name"},
	{ID: twitcheventsub.SenderUserIDVariable, DisplayName: "Sender's user ID"},
	{ID: twitcheventsub.MessageIDVariable, DisplayName: "Message ID"},
}

// Outcomes has one outcome per choice, in order, and then timing out.
func (waitForChatMessageStep) Outcomes(settings automationsetting.Values) ([]step.Outcome, error) {
	choices, err := settings.ChoiceList(ChoicesSetting)
	if err != nil {
		return nil, fmt.Errorf("reading the choices: %w", err)
	}
	isFromPersonWhoStartedIt := isWaitingForPersonWhoStartedIt(settings)
	choiceWords, timedOutWords := "If someone types", "If no one answers in time"
	if isFromPersonWhoStartedIt {
		choiceWords, timedOutWords = "If they type", "If they don't answer in time"
	}
	outcomes := make([]step.Outcome, 0, len(choices)+1)
	for _, choice := range choices {
		if choice.ID == TimedOutOutcomeID {
			return nil, ErrTimedOutChoiceID
		}
		choiceText := strings.TrimSpace(choice.Text)
		outcomes = append(outcomes, step.Outcome{
			ID:          choice.ID,
			DisplayName: choiceText,
			Sentence:    automationsetting.Sentence{automationsetting.Words(choiceWords), automationsetting.UserText(choiceText)},
			Variables:   answerVariables,
		})
	}
	outcomes = append(outcomes, step.Outcome{
		ID:          TimedOutOutcomeID,
		DisplayName: "Timed out",
		Sentence:    automationsetting.Sentence{automationsetting.Words(timedOutWords)},
	})
	return outcomes, nil
}

// isWaitingForPersonWhoStartedIt reads the "from" setting. While it's
// unreadable, which the check reports as a problem of its own, the outcomes
// are worded for the person who started it, the setting's initial value.
func isWaitingForPersonWhoStartedIt(settings automationsetting.Values) bool {
	from, err := settings.Option(FromSetting)
	return err != nil || from != FromAnyone
}

// CheckUsage refuses waiting for the person who started the automation when
// its trigger has no person.
func (waitForChatMessageStep) CheckUsage(usage step.Usage) []step.Note {
	if usage.HasTriggerPerson || !isWaitingForPersonWhoStartedIt(usage.Settings) {
		return nil
	}
	return []step.Note{{
		SettingID: FromSetting,
		Message:   fmt.Sprintf("“Wait for a chat message” can't wait for the person who started this, because “%s” isn't started by a person.", usage.TriggerName),
	}}
}

// waitSettings are the step's settings, read once when it starts.
type waitSettings struct {
	timeLimit time.Duration
	// onlyFromUserID is the user ID answers must come from, or empty for
	// anyone.
	onlyFromUserID string
	choices        []automationsetting.Choice
}

func readWaitSettings(input step.Input) (waitSettings, error) {
	timeLimit, err := input.Settings.Duration(TimeLimitSetting)
	if err != nil {
		return waitSettings{}, fmt.Errorf("reading how long to wait: %w", err)
	}
	from, err := input.Settings.Option(FromSetting)
	if err != nil {
		return waitSettings{}, fmt.Errorf("reading who can answer: %w", err)
	}
	choices, err := input.Settings.ChoiceList(ChoicesSetting)
	if err != nil {
		return waitSettings{}, fmt.Errorf("reading the choices: %w", err)
	}
	settings := waitSettings{timeLimit: timeLimit, choices: choices}
	if from == FromAnyone {
		return settings, nil
	}
	if input.StartedByUserID == "" {
		return waitSettings{}, errors.New("there's no person who started this run to wait for")
	}
	settings.onlyFromUserID = input.StartedByUserID
	return settings, nil
}

// Run waits for a chat message that answers one of the choices, and takes
// that choice's outcome, or times out. Only messages sent after it starts
// count, since it subscribes to them when it starts.
func (waitForChatMessageStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	settings, err := readWaitSettings(input)
	if err != nil {
		return step.Result{}, err
	}
	subscription := input.Events.Subscribe()
	defer subscription.Unsubscribe()
	timer := time.NewTimer(settings.timeLimit)
	defer timer.Stop()
	for {
		select {
		case event := <-subscription.Events():
			result, isAnswer := settings.answer(event)
			if isAnswer {
				return result, nil
			}
		case <-timer.C:
			return step.Result{OutcomeID: TimedOutOutcomeID}, nil
		case <-ctx.Done():
			return step.Result{}, ctx.Err()
		}
	}
}

// answer checks whether an event answers the step, and returns the
// outcome it takes if it does. The first choice the message matches wins.
func (settings waitSettings) answer(event trigger.Event) (step.Result, bool) {
	if event.TriggerID != twitcheventsub.ChatMessageTriggerID {
		return step.Result{}, false
	}
	if settings.onlyFromUserID != "" && event.Variables[twitcheventsub.SenderUserIDVariable] != settings.onlyFromUserID {
		return step.Result{}, false
	}
	message := event.Variables[twitcheventsub.MessageTextVariable]
	for _, choice := range settings.choices {
		restOfMessage, isMatch := twitcheventsub.MatchStartOfMessage(message, strings.TrimSpace(choice.Text))
		if !isMatch {
			continue
		}
		variables := make(map[string]string, len(answerVariables))
		for _, variable := range answerVariables {
			variables[variable.ID] = event.Variables[variable.ID]
		}
		variables[twitcheventsub.RestOfMessageVariable] = restOfMessage
		return step.Result{OutcomeID: choice.ID, Variables: variables}, true
	}
	return step.Result{}, false
}
