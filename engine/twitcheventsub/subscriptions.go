package twitcheventsub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/SloanGwaltney/whendo/engine/twitchauth"
)

const (
	subscriptionsPath      = "/eventsub/subscriptions"
	webSocketTransportName = "websocket"
	// Twitch's responses are small. The limit only protects against a broken
	// or hostile response filling memory.
	maximumResponseBodyBytes = 1 * bytesPerMegabyte
)

// Condition field names used by the subscriptions Whendo creates.
const (
	broadcasterUserIDCondition = "broadcaster_user_id"
	userIDCondition            = "user_id"
	moderatorUserIDCondition   = "moderator_user_id"
)

type subscriptionTransport struct {
	Method    string `json:"method"`
	SessionID string `json:"session_id"`
}

type subscriptionRequest struct {
	Type      string                `json:"type"`
	Version   string                `json:"version"`
	Condition map[string]string     `json:"condition"`
	Transport subscriptionTransport `json:"transport"`
}

type helixErrorResponse struct {
	Message string `json:"message"`
}

// subscriptionRequests returns the subscriptions Whendo always creates. The
// streamer is both the channel and the user each subscription acts as.
func subscriptionRequests(broadcasterUserID string, sessionID string) []subscriptionRequest {
	transport := subscriptionTransport{Method: webSocketTransportName, SessionID: sessionID}
	return []subscriptionRequest{
		{
			Type:      redemptionSubscriptionType,
			Version:   redemptionSubscriptionVersion,
			Condition: map[string]string{broadcasterUserIDCondition: broadcasterUserID},
			Transport: transport,
		},
		{
			Type:    chatMessageSubscriptionType,
			Version: chatMessageSubscriptionVersion,
			Condition: map[string]string{
				broadcasterUserIDCondition: broadcasterUserID,
				userIDCondition:            broadcasterUserID,
			},
			Transport: transport,
		},
		{
			Type:    followSubscriptionType,
			Version: followSubscriptionVersion,
			Condition: map[string]string{
				broadcasterUserIDCondition: broadcasterUserID,
				moderatorUserIDCondition:   broadcasterUserID,
			},
			Transport: transport,
		},
	}
}

// subscribeToAll creates every subscription for the session. A subscription
// Twitch refuses for reasons other than authorization is reported as a
// problem and the rest still go ahead, since the others are still useful.
func (listener *Listener) subscribeToAll(ctx context.Context, sessionID string) ([]string, error) {
	broadcasterUserID, err := listener.authorization.AccountUserID()
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return nil, ErrAuthorizationLost
	}
	if err != nil {
		return nil, fmt.Errorf("getting the Twitch account to subscribe for: %w", err)
	}
	accessToken, err := listener.authorization.AccessToken(ctx)
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return nil, ErrAuthorizationLost
	}
	if err != nil {
		return nil, fmt.Errorf("getting a token to subscribe with: %w", err)
	}

	var problems []string
	for _, request := range subscriptionRequests(broadcasterUserID, sessionID) {
		var problem string
		accessToken, problem, err = listener.subscribe(ctx, accessToken, request)
		if err != nil {
			return nil, err
		}
		if problem != "" {
			listener.logger.Error("Twitch refused an EventSub subscription", "type", request.Type, "version", request.Version, "problem", problem)
			problems = append(problems, problem)
		}
	}
	return problems, nil
}

// subscribe creates one subscription, renewing the token once if Twitch
// rejects it. It returns the token to use for the next subscription, and a
// problem description if Twitch refused the subscription for another reason.
func (listener *Listener) subscribe(ctx context.Context, accessToken string, request subscriptionRequest) (string, string, error) {
	statusCode, message, err := listener.createSubscription(ctx, accessToken, request)
	if err != nil {
		return accessToken, "", err
	}
	if statusCode == http.StatusUnauthorized {
		accessToken, err = listener.authorization.RenewAccessToken(ctx, accessToken)
		if errors.Is(err, twitchauth.ErrNotLoggedIn) {
			return accessToken, "", ErrAuthorizationLost
		}
		if err != nil {
			return accessToken, "", fmt.Errorf("renewing the token Twitch rejected: %w", err)
		}
		statusCode, message, err = listener.createSubscription(ctx, accessToken, request)
		if err != nil {
			return accessToken, "", err
		}
	}

	switch statusCode {
	case http.StatusAccepted, http.StatusConflict:
		// Conflict means the subscription already exists, which is the
		// result we wanted.
		return accessToken, "", nil
	case http.StatusUnauthorized:
		return accessToken, "", fmt.Errorf("twitch rejected a renewed token while subscribing to %s: %s", request.Type, message)
	case http.StatusForbidden:
		// The login lacks a scope this version of Whendo needs. Logging in
		// again requests the current scopes.
		listener.logger.Warn("the Twitch login is missing a scope; logging out so the user can log in again", "type", request.Type, "message", message)
		listener.authorization.EndSessionRevokedByTwitch()
		return accessToken, "", ErrAuthorizationLost
	default:
		return accessToken, fmt.Sprintf("couldn't subscribe to %s version %s: HTTP %d: %s", request.Type, request.Version, statusCode, message), nil
	}
}

// createSubscription sends one subscription request and returns Twitch's
// status code and error message, if any.
func (listener *Listener) createSubscription(ctx context.Context, accessToken string, request subscriptionRequest) (int, string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return 0, "", fmt.Errorf("encoding the %s subscription: %w", request.Type, err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, listener.helixBaseURL+subscriptionsPath, bytes.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("creating the %s subscription request: %w", request.Type, err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)
	httpRequest.Header.Set("Client-Id", listener.clientID)
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := listener.httpClient.Do(httpRequest)
	if err != nil {
		return 0, "", fmt.Errorf("subscribing to %s: %w", request.Type, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBodyBytes))
	if err != nil {
		return 0, "", fmt.Errorf("reading the %s subscription response: %w", request.Type, err)
	}
	return response.StatusCode, errorMessageFromResponse(responseBody), nil
}

// errorMessageFromResponse returns the message from a Helix error response,
// or an empty string if the body isn't one.
func errorMessageFromResponse(body []byte) string {
	var errorResponse helixErrorResponse
	err := json.Unmarshal(body, &errorResponse)
	if err != nil {
		return ""
	}
	return errorResponse.Message
}
