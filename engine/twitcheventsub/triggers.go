// Package twitcheventsub receives events from Twitch through EventSub and
// turns them into the Twitch integration's triggers. See
// docs/design/twitch-eventsub.md for the rules it follows.
package twitcheventsub

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

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

// Triggers returns the definitions of every trigger the Twitch integration
// offers.
func Triggers() []trigger.Definition {
	return []trigger.Definition{
		{
			ID:          RedeemTriggerID,
			DisplayName: "Channel point redeem",
			Variables: []trigger.VariableDefinition{
				{ID: RedeemerDisplayNameVariable, DisplayName: "Redeemer's name"},
				{ID: UserInputVariable, DisplayName: "Redeemer's message"},
				{ID: RewardTitleVariable, DisplayName: "Reward name"},
				{ID: RewardCostVariable, DisplayName: "Reward cost"},
				{ID: RewardPromptVariable, DisplayName: "Reward description"},
				{ID: RedeemedAtVariable, DisplayName: "Time of redeem"},
				{ID: RedeemerLoginVariable, DisplayName: "Redeemer's login name"},
				{ID: RedeemerUserIDVariable, DisplayName: "Redeemer's user ID"},
				{ID: RewardIDVariable, DisplayName: "Reward ID"},
				{ID: RedemptionIDVariable, DisplayName: "Redemption ID"},
			},
		},
		{
			ID:          ChatMessageTriggerID,
			DisplayName: "Chat message",
			Variables: []trigger.VariableDefinition{
				{ID: SenderDisplayNameVariable, DisplayName: "Sender's name"},
				{ID: MessageTextVariable, DisplayName: "Message"},
				{ID: SenderLoginVariable, DisplayName: "Sender's login name"},
				{ID: SenderUserIDVariable, DisplayName: "Sender's user ID"},
				{ID: MessageIDVariable, DisplayName: "Message ID"},
			},
		},
		{
			ID:          FollowTriggerID,
			DisplayName: "Follow",
			Variables: []trigger.VariableDefinition{
				{ID: FollowerDisplayNameVariable, DisplayName: "Follower's name"},
				{ID: FollowedAtVariable, DisplayName: "Time of follow"},
				{ID: FollowerLoginVariable, DisplayName: "Follower's login name"},
				{ID: FollowerUserIDVariable, DisplayName: "Follower's user ID"},
			},
		},
	}
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
