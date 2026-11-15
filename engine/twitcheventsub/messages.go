package twitcheventsub

import "encoding/json"

// Message types Twitch sends over an EventSub WebSocket.
const (
	welcomeMessageType      = "session_welcome"
	keepaliveMessageType    = "session_keepalive"
	notificationMessageType = "notification"
	reconnectMessageType    = "session_reconnect"
	revocationMessageType   = "revocation"
)

// Reasons Twitch gives when it revokes a subscription.
const (
	authorizationRevokedReason = "authorization_revoked"
	userRemovedReason          = "user_removed"
	versionRemovedReason       = "version_removed"
)

type messageMetadata struct {
	MessageID   string `json:"message_id"`
	MessageType string `json:"message_type"`
}

type sessionPayload struct {
	ID                      string `json:"id"`
	KeepaliveTimeoutSeconds int    `json:"keepalive_timeout_seconds"`
	ReconnectURL            string `json:"reconnect_url"`
}

type subscriptionPayload struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Type    string `json:"type"`
	Version string `json:"version"`
}

type messagePayload struct {
	Session      sessionPayload      `json:"session"`
	Subscription subscriptionPayload `json:"subscription"`
	// Event is kept raw because its shape depends on the subscription type.
	Event json.RawMessage `json:"event"`
}

type incomingMessage struct {
	Metadata messageMetadata `json:"metadata"`
	Payload  messagePayload  `json:"payload"`
}
