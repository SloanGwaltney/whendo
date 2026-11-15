package twitchsteps

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
	bytesPerMegabyte = 1 << 20
	// Twitch's responses are small. The limit only protects against a broken
	// or hostile response filling memory.
	maximumResponseBodyBytes = 1 * bytesPerMegabyte
)

// errNotLoggedIn is worded for run history, where the streamer reads it.
var errNotLoggedIn = errors.New("not logged in to Twitch")

// helixClient calls Twitch's API as the logged-in streamer.
type helixClient struct {
	clientID   string
	account    Account
	baseURL    string
	httpClient *http.Client
}

// helixResponse is a response's status code and body.
type helixResponse struct {
	statusCode int
	body       []byte
}

// helixErrorBody is the shape of Twitch's error responses.
type helixErrorBody struct {
	Message string `json:"message"`
}

// errorMessage returns Twitch's explanation of a failed request, or the raw
// body if it isn't in the usual shape.
func (response helixResponse) errorMessage() string {
	var errorBody helixErrorBody
	err := json.Unmarshal(response.body, &errorBody)
	if err != nil || errorBody.Message == "" {
		return string(response.body)
	}
	return errorBody.Message
}

// broadcasterUserID returns the logged-in streamer's user ID.
func (client *helixClient) broadcasterUserID() (string, error) {
	userID, err := client.account.AccountUserID()
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return "", errNotLoggedIn
	}
	if err != nil {
		return "", fmt.Errorf("getting the Twitch account: %w", err)
	}
	return userID, nil
}

// postJSON sends a request body as JSON. If Twitch rejects the access token,
// it renews the token and tries once more, since tokens expire every few
// hours and a streamer shouldn't see a failure for that.
func (client *helixClient) postJSON(ctx context.Context, path string, requestBody any) (helixResponse, error) {
	encodedBody, err := json.Marshal(requestBody)
	if err != nil {
		return helixResponse{}, fmt.Errorf("encoding the request to %s: %w", path, err)
	}
	accessToken, err := client.account.AccessToken(ctx)
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return helixResponse{}, errNotLoggedIn
	}
	if err != nil {
		return helixResponse{}, fmt.Errorf("getting a Twitch access token: %w", err)
	}
	response, err := client.send(ctx, path, accessToken, encodedBody)
	if err != nil || response.statusCode != http.StatusUnauthorized {
		return response, err
	}
	accessToken, err = client.account.RenewAccessToken(ctx, accessToken)
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return helixResponse{}, errNotLoggedIn
	}
	if err != nil {
		return helixResponse{}, fmt.Errorf("renewing the Twitch access token: %w", err)
	}
	return client.send(ctx, path, accessToken, encodedBody)
}

func (client *helixClient) send(ctx context.Context, path string, accessToken string, encodedBody []byte) (helixResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(encodedBody))
	if err != nil {
		return helixResponse{}, fmt.Errorf("creating the request to %s: %w", path, err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Client-Id", client.clientID)
	request.Header.Set("Content-Type", "application/json")
	httpResponse, err := client.httpClient.Do(request)
	if err != nil {
		return helixResponse{}, fmt.Errorf("couldn't reach Twitch: %w", err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maximumResponseBodyBytes))
	if err != nil {
		return helixResponse{}, fmt.Errorf("reading Twitch's response from %s: %w", path, err)
	}
	return helixResponse{statusCode: httpResponse.StatusCode, body: body}, nil
}
