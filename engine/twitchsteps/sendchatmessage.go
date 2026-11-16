package twitchsteps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
)

const (
	// SendChatMessageStepID is the step that posts a message in the
	// streamer's chat as the streamer.
	SendChatMessageStepID = "twitch.sendChatMessage"
	// MessageSetting is the message to send, with variables inserted.
	MessageSetting = "message"
	// MaximumChatMessageLength is the longest message Twitch accepts, in
	// characters.
	MaximumChatMessageLength = 500

	chatMessagesPath = "/chat/messages"
	// messageDetailName labels the message in run history.
	messageDetailName = "Message"
)

type sendChatMessageStep struct {
	helix *helixClient
}

type sendChatMessageRequest struct {
	BroadcasterID string `json:"broadcaster_id"`
	SenderID      string `json:"sender_id"`
	Message       string `json:"message"`
}

type sendChatMessageResponse struct {
	Data []struct {
		IsSent     bool `json:"is_sent"`
		DropReason *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"drop_reason"`
	} `json:"data"`
}

func (sendChatMessageStep) Definition() step.Definition {
	return step.Definition{
		ID:          SendChatMessageStepID,
		DisplayName: "Send a chat message",
		Description: "Post a message in your chat, from your account.",
		Group:       chatGroup,
		Sentence:    automationsetting.Sentence{automationsetting.Words("Say in chat"), automationsetting.Setting(MessageSetting)},
		Settings: []automationsetting.Definition{
			{ID: MessageSetting, DisplayName: "Message", Kind: automationsetting.KindTemplate, IsRequired: true, Placeholder: "write a message", MaximumLength: MaximumChatMessageLength},
		},
	}
}

func (sendChatMessageStep) Outcomes(automationsetting.Values) ([]step.Outcome, error) {
	return []step.Outcome{{ID: step.DefaultOutcomeID, DisplayName: "Done"}}, nil
}

// Run posts the message in the streamer's chat as the streamer. A message
// Twitch would refuse, such as one too long after variables are inserted,
// fails the step with the reason rather than being cut short, since a
// truncated message could say something the streamer never meant. Once the
// message is filled in, it's in the result's details even if sending fails,
// so run history shows what Whendo tried to say.
func (chatStep sendChatMessageStep) Run(ctx context.Context, input step.Input) (step.Result, error) {
	message, err := renderMessage(input)
	if err != nil {
		return step.Result{}, err
	}
	result := step.Result{
		OutcomeID: step.DefaultOutcomeID,
		Details:   []step.Detail{{Name: messageDetailName, Value: message}},
	}
	failedResult := step.Result{Details: result.Details}
	err = checkMessage(message)
	if err != nil {
		return failedResult, err
	}
	broadcasterUserID, err := chatStep.helix.broadcasterUserID()
	if err != nil {
		return failedResult, err
	}
	response, err := chatStep.helix.postJSON(ctx, chatMessagesPath, sendChatMessageRequest{
		BroadcasterID: broadcasterUserID,
		SenderID:      broadcasterUserID,
		Message:       message,
	})
	if err != nil {
		return failedResult, err
	}
	err = checkSendResponse(response)
	if err != nil {
		return failedResult, err
	}
	return result, nil
}

func renderMessage(input step.Input) (string, error) {
	template, err := input.Settings.Template(MessageSetting)
	if err != nil {
		return "", fmt.Errorf("reading the message: %w", err)
	}
	message, err := template.Render(input.Variables)
	if err != nil {
		return "", fmt.Errorf("filling in the message: %w", err)
	}
	return message, nil
}

// checkMessage refuses messages Twitch would, before asking it.
func checkMessage(message string) error {
	if strings.TrimSpace(message) == "" {
		return errors.New("the message is empty, so there's nothing to send")
	}
	messageLength := utf8.RuneCountInString(message)
	if messageLength > MaximumChatMessageLength {
		return fmt.Errorf("the message is %d characters long, and Twitch allows at most %d", messageLength, MaximumChatMessageLength)
	}
	return nil
}

// checkSendResponse turns Twitch's answer into a failure the streamer can
// understand. A 200 response can still mean the message was dropped, for
// example by slow mode or AutoMod, so the body is checked too.
func checkSendResponse(response helixResponse) error {
	switch response.statusCode {
	case http.StatusOK:
		return checkMessageWasSent(response.body)
	case http.StatusUnauthorized:
		return fmt.Errorf("twitch didn't accept Whendo's login for sending chat messages: %s", response.errorMessage())
	case http.StatusForbidden:
		return fmt.Errorf("twitch doesn't allow your account to chat in your channel right now: %s", response.errorMessage())
	case http.StatusUnprocessableEntity:
		return fmt.Errorf("twitch says the message is too long: %s", response.errorMessage())
	case http.StatusTooManyRequests:
		return errors.New("twitch's limit on how fast you can send chat messages was reached, so this message wasn't sent")
	default:
		return fmt.Errorf("twitch couldn't send the message (HTTP %d): %s", response.statusCode, response.errorMessage())
	}
}

func checkMessageWasSent(body []byte) error {
	var sendResponse sendChatMessageResponse
	err := json.Unmarshal(body, &sendResponse)
	if err != nil {
		return fmt.Errorf("reading Twitch's response to sending the message: %w", err)
	}
	if len(sendResponse.Data) == 0 {
		return errors.New("twitch's response to sending the message was empty, so it may not have been sent")
	}
	result := sendResponse.Data[0]
	if result.IsSent {
		return nil
	}
	if result.DropReason == nil {
		return errors.New("twitch didn't send the message and didn't say why")
	}
	return fmt.Errorf("twitch didn't send the message: %s (%s)", result.DropReason.Message, result.DropReason.Code)
}
