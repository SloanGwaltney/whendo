// Package twitcheventsub receives events from Twitch through EventSub and
// turns them into the Twitch integration's triggers. See
// docs/design/twitch-eventsub.md for the rules it follows.
package twitcheventsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// Trigger IDs are prefixed with the integration so they can't collide with
// another integration's triggers.
const (
	RedeemTriggerID      = "twitch.redeem"
	ChatMessageTriggerID = "twitch.chatMessage"
	FollowTriggerID      = "twitch.follow"
)

// Variable IDs for the redeem trigger.
const (
	RedemptionIDVariable        = "redemptionId"
	RedeemerUserIDVariable      = "redeemerUserId"
	RedeemerLoginVariable       = "redeemerLogin"
	RedeemerDisplayNameVariable = "redeemerDisplayName"
	UserInputVariable           = "userInput"
	RewardIDVariable            = "rewardId"
	RewardTitleVariable         = "rewardTitle"
	RewardCostVariable          = "rewardCost"
	RewardPromptVariable        = "rewardPrompt"
	RedeemedAtVariable          = "redeemedAt"
)

// Variable IDs for the chat message trigger.
const (
	MessageIDVariable         = "messageId"
	SenderUserIDVariable      = "senderUserId"
	SenderLoginVariable       = "senderLogin"
	SenderDisplayNameVariable = "senderDisplayName"
	MessageTextVariable       = "messageText"
)

// Variable IDs for the follow trigger.
const (
	FollowerUserIDVariable      = "followerUserId"
	FollowerLoginVariable       = "followerLogin"
	FollowerDisplayNameVariable = "followerDisplayName"
	FollowedAtVariable          = "followedAt"
)

// EventSub subscription types and the versions Whendo understands.
const (
	redemptionSubscriptionType     = "channel.channel_points_custom_reward_redemption.add"
	redemptionSubscriptionVersion  = "1"
	chatMessageSubscriptionType    = "channel.chat.message"
	chatMessageSubscriptionVersion = "1"
	followSubscriptionType         = "channel.follow"
	followSubscriptionVersion      = "2"
)

// ErrUnsupportedSubscription means a notification is for a subscription type
// or version Whendo doesn't understand.
var ErrUnsupportedSubscription = errors.New("unsupported EventSub subscription")

// RewardSettingKind is a channel point reward, stored as the reward's ID.
// The designer shows a picker of the user's rewards for it.
const RewardSettingKind automationsetting.Kind = "twitch.reward"

// Setting IDs for the Twitch triggers.
const (
	// RewardSetting is the reward a redeem trigger reacts to.
	RewardSetting = "reward"
	// StartsWithSetting is the text a chat message must start with to
	// trigger the automation, such as "!discord".
	StartsWithSetting = "startsWith"
)

// RestOfMessageVariable is the text of a matched chat message after the
// matched text, such as "coolstreamer" in "!so coolstreamer". The chat
// message trigger's filter adds it, since it depends on the automation's
// settings.
const RestOfMessageVariable = "restOfMessage"

// Triggers returns every trigger the Twitch integration offers.
func Triggers() []trigger.Trigger {
	return []trigger.Trigger{redeemTrigger{}, chatMessageTrigger{}, followTrigger{}}
}

type redeemTrigger struct{}

func (redeemTrigger) Definition() trigger.Definition {
	return trigger.Definition{
		ID:          RedeemTriggerID,
		DisplayName: "Channel point redeem",
		Description: "A viewer spends channel points on one of your redeems.",
		Sentence:    automationsetting.Sentence{automationsetting.Words("someone redeems"), automationsetting.Setting(RewardSetting)},
		Settings: []automationsetting.Definition{
			{ID: RewardSetting, DisplayName: "Reward", Kind: RewardSettingKind, IsRequired: true, Placeholder: "pick a redeem"},
		},
		Variables: []trigger.VariableDefinition{
			{ID: RedeemerDisplayNameVariable, DisplayName: "Redeemer's name", IsCommon: true},
			{ID: UserInputVariable, DisplayName: "Redeemer's message", IsCommon: true},
			{ID: RewardTitleVariable, DisplayName: "Reward name", IsCommon: true},
			{ID: RewardCostVariable, DisplayName: "Reward cost"},
			{ID: RewardPromptVariable, DisplayName: "Reward description"},
			{ID: RedeemedAtVariable, DisplayName: "Time of redeem"},
			{ID: RedeemerLoginVariable, DisplayName: "Redeemer's login name"},
			{ID: RedeemerUserIDVariable, DisplayName: "Redeemer's user ID"},
			{ID: RewardIDVariable, DisplayName: "Reward ID"},
			{ID: RedemptionIDVariable, DisplayName: "Redemption ID"},
		},
	}
}

// Filter matches redeems of the automation's reward. Twitch delivers every
// redeem through one subscription, so this is where redeems are told apart;
// see docs/design/twitch-eventsub.md#subscriptions.
func (redeemTrigger) Filter(event trigger.Event, settings automationsetting.Values) (map[string]string, bool, error) {
	rewardID, err := settings.Text(RewardSetting)
	if err != nil {
		return nil, false, fmt.Errorf("reading the redeem trigger's settings: %w", err)
	}
	if event.TriggerID != RedeemTriggerID || event.Variables[RewardIDVariable] != rewardID {
		return nil, false, nil
	}
	return maps.Clone(event.Variables), true, nil
}

type chatMessageTrigger struct{}

func (chatMessageTrigger) Definition() trigger.Definition {
	return trigger.Definition{
		ID:          ChatMessageTriggerID,
		DisplayName: "Chat message",
		// Whendo has no separate idea of a command, so the description says
		// this is how to make one, for users who look for "commands".
		Description: "Someone types a chat message that starts with your text, like !discord. This is how to make a chat command.",
		Sentence:    automationsetting.Sentence{automationsetting.Words("a chat message starts with"), automationsetting.Setting(StartsWithSetting)},
		Settings: []automationsetting.Definition{
			{ID: StartsWithSetting, DisplayName: "Starts with", Kind: automationsetting.KindText, IsRequired: true, Placeholder: "type some text, like !discord"},
		},
		Variables: []trigger.VariableDefinition{
			{ID: SenderDisplayNameVariable, DisplayName: "Sender's name", IsCommon: true},
			{ID: MessageTextVariable, DisplayName: "Message", IsCommon: true},
			{ID: RestOfMessageVariable, DisplayName: "Rest of the message", IsAddedByFilter: true, IsCommon: true},
			{ID: SenderLoginVariable, DisplayName: "Sender's login name"},
			{ID: SenderUserIDVariable, DisplayName: "Sender's user ID"},
			{ID: MessageIDVariable, DisplayName: "Message ID"},
		},
	}
}

// Filter matches messages that start with the automation's text, following
// docs/design/twitch-eventsub.md#chat-messages, and adds the rest of the
// message.
func (chatMessageTrigger) Filter(event trigger.Event, settings automationsetting.Values) (map[string]string, bool, error) {
	startText, err := settings.Text(StartsWithSetting)
	if err != nil {
		return nil, false, fmt.Errorf("reading the chat message trigger's settings: %w", err)
	}
	startText = strings.TrimSpace(startText)
	if startText == "" {
		// An empty start would match every message, which no one means.
		return nil, false, fmt.Errorf("reading the chat message trigger's settings: %w: %q is blank", automationsetting.ErrSettingMissing, StartsWithSetting)
	}
	if event.TriggerID != ChatMessageTriggerID {
		return nil, false, nil
	}
	restOfMessage, isMatch := matchStartOfMessage(event.Variables[MessageTextVariable], startText)
	if !isMatch {
		return nil, false, nil
	}
	variables := maps.Clone(event.Variables)
	variables[RestOfMessageVariable] = restOfMessage
	return variables, true, nil
}

// matchStartOfMessage reports whether message starts with startText,
// ignoring capitalization, as a whole word: "!a1" matches "!a1 please" but
// not "!a10". It compares rune by rune rather than slicing message to
// startText's length, because a letter and its other case can take a
// different number of bytes.
func matchStartOfMessage(message string, startText string) (restOfMessage string, isMatch bool) {
	remainingMessage := message
	for _, startRune := range startText {
		messageRune, runeSize := utf8.DecodeRuneInString(remainingMessage)
		if runeSize == 0 || !strings.EqualFold(string(startRune), string(messageRune)) {
			return "", false
		}
		remainingMessage = remainingMessage[runeSize:]
	}
	nextRune, nextRuneSize := utf8.DecodeRuneInString(remainingMessage)
	if nextRuneSize > 0 && !unicode.IsSpace(nextRune) {
		return "", false
	}
	return strings.TrimLeftFunc(remainingMessage, unicode.IsSpace), true
}

type followTrigger struct{}

func (followTrigger) Definition() trigger.Definition {
	return trigger.Definition{
		ID:          FollowTriggerID,
		DisplayName: "Follow",
		Description: "Someone follows your channel.",
		Sentence:    automationsetting.Sentence{automationsetting.Words("someone follows you")},
		Variables: []trigger.VariableDefinition{
			{ID: FollowerDisplayNameVariable, DisplayName: "Follower's name", IsCommon: true},
			{ID: FollowedAtVariable, DisplayName: "Time of follow"},
			{ID: FollowerLoginVariable, DisplayName: "Follower's login name"},
			{ID: FollowerUserIDVariable, DisplayName: "Follower's user ID"},
		},
	}
}

// Filter matches every follow, since the follow trigger has no settings.
func (followTrigger) Filter(event trigger.Event, settings automationsetting.Values) (map[string]string, bool, error) {
	if event.TriggerID != FollowTriggerID {
		return nil, false, nil
	}
	return maps.Clone(event.Variables), true, nil
}

type redemptionEvent struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	UserLogin  string `json:"user_login"`
	UserName   string `json:"user_name"`
	UserInput  string `json:"user_input"`
	RedeemedAt string `json:"redeemed_at"`
	Reward     struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Cost   int    `json:"cost"`
		Prompt string `json:"prompt"`
	} `json:"reward"`
}

type chatMessageEvent struct {
	BroadcasterUserID       string  `json:"broadcaster_user_id"`
	SourceBroadcasterUserID *string `json:"source_broadcaster_user_id"`
	ChatterUserID           string  `json:"chatter_user_id"`
	ChatterUserLogin        string  `json:"chatter_user_login"`
	ChatterUserName         string  `json:"chatter_user_name"`
	MessageID               string  `json:"message_id"`
	Message                 struct {
		Text string `json:"text"`
	} `json:"message"`
}

type followEvent struct {
	UserID     string `json:"user_id"`
	UserLogin  string `json:"user_login"`
	UserName   string `json:"user_name"`
	FollowedAt string `json:"followed_at"`
}

// EventFromNotification translates the event object of an EventSub
// notification into a trigger event. It returns false when the notification
// is valid but shouldn't trigger anything, such as a chat message from
// another channel in a shared chat session.
func EventFromNotification(subscriptionType string, subscriptionVersion string, eventJSON []byte) (trigger.Event, bool, error) {
	switch {
	case subscriptionType == redemptionSubscriptionType && subscriptionVersion == redemptionSubscriptionVersion:
		return redeemEventFromJSON(eventJSON)
	case subscriptionType == chatMessageSubscriptionType && subscriptionVersion == chatMessageSubscriptionVersion:
		return chatMessageEventFromJSON(eventJSON)
	case subscriptionType == followSubscriptionType && subscriptionVersion == followSubscriptionVersion:
		return followEventFromJSON(eventJSON)
	default:
		return trigger.Event{}, false, fmt.Errorf("%w: %s version %s", ErrUnsupportedSubscription, subscriptionType, subscriptionVersion)
	}
}

func redeemEventFromJSON(eventJSON []byte) (trigger.Event, bool, error) {
	var redemption redemptionEvent
	err := json.Unmarshal(eventJSON, &redemption)
	if err != nil {
		return trigger.Event{}, false, fmt.Errorf("reading a redeem event: %w", err)
	}
	return trigger.Event{
		TriggerID: RedeemTriggerID,
		Variables: map[string]string{
			RedemptionIDVariable:        redemption.ID,
			RedeemerUserIDVariable:      redemption.UserID,
			RedeemerLoginVariable:       redemption.UserLogin,
			RedeemerDisplayNameVariable: redemption.UserName,
			UserInputVariable:           redemption.UserInput,
			RewardIDVariable:            redemption.Reward.ID,
			RewardTitleVariable:         redemption.Reward.Title,
			RewardCostVariable:          strconv.Itoa(redemption.Reward.Cost),
			RewardPromptVariable:        redemption.Reward.Prompt,
			RedeemedAtVariable:          redemption.RedeemedAt,
		},
	}, true, nil
}

func chatMessageEventFromJSON(eventJSON []byte) (trigger.Event, bool, error) {
	var chatMessage chatMessageEvent
	err := json.Unmarshal(eventJSON, &chatMessage)
	if err != nil {
		return trigger.Event{}, false, fmt.Errorf("reading a chat message event: %w", err)
	}
	if !isFromBroadcastersOwnChat(chatMessage) {
		return trigger.Event{}, false, nil
	}
	return trigger.Event{
		TriggerID: ChatMessageTriggerID,
		Variables: map[string]string{
			MessageIDVariable:         chatMessage.MessageID,
			SenderUserIDVariable:      chatMessage.ChatterUserID,
			SenderLoginVariable:       chatMessage.ChatterUserLogin,
			SenderDisplayNameVariable: chatMessage.ChatterUserName,
			MessageTextVariable:       chatMessage.Message.Text,
		},
	}, true, nil
}

// isFromBroadcastersOwnChat reports whether a message was sent in the
// streamer's own chat. During a shared chat session, Twitch also delivers
// messages from the other channels, with the source fields naming the channel
// they came from. Reacting to those would answer other streamers' viewers.
func isFromBroadcastersOwnChat(chatMessage chatMessageEvent) bool {
	if chatMessage.SourceBroadcasterUserID == nil {
		return true
	}
	return *chatMessage.SourceBroadcasterUserID == chatMessage.BroadcasterUserID
}

func followEventFromJSON(eventJSON []byte) (trigger.Event, bool, error) {
	var follow followEvent
	err := json.Unmarshal(eventJSON, &follow)
	if err != nil {
		return trigger.Event{}, false, fmt.Errorf("reading a follow event: %w", err)
	}
	return trigger.Event{
		TriggerID: FollowTriggerID,
		Variables: map[string]string{
			FollowerUserIDVariable:      follow.UserID,
			FollowerLoginVariable:       follow.UserLogin,
			FollowerDisplayNameVariable: follow.UserName,
			FollowedAtVariable:          follow.FollowedAt,
		},
	}, true, nil
}
