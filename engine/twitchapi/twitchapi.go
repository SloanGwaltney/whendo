// Package twitchapi calls Twitch's Helix API as the logged-in streamer. It
// renews an access token Twitch rejects and tries once more, so the steps
// and redeems that call Twitch don't each handle expiring tokens. See
// docs/design/twitch-steps.md#acting-as-the-streamer.
package twitchapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/SloanGwaltney/whendo/engine/twitchauth"
)

// DefaultBaseURL is the base URL of Twitch's API.
const DefaultBaseURL = "https://api.twitch.tv/helix"

const (
	bytesPerMegabyte = 1 << 20
	// Twitch's responses are small. The limit only protects against a broken
	// or hostile response filling memory.
	maximumResponseBodyBytes = 1 * bytesPerMegabyte
)

// ErrNotLoggedIn means there's no Twitch login to act as. It's worded for
// run history and the UI, where the streamer reads it.
var ErrNotLoggedIn = errors.New("not logged in to Twitch")

// Account is the Twitch login requests act as. twitchauth.Session
// implements it.
type Account interface {
	AccountUserID() (string, error)
	AccessToken(ctx context.Context) (string, error)
	// RenewAccessToken replaces a token Twitch rejected. It returns
	// twitchauth.ErrNotLoggedIn if the login can't be renewed.
	RenewAccessToken(ctx context.Context, rejectedAccessToken string) (string, error)
}

// Config holds what a client needs.
type Config struct {
	ClientID string
	Account  Account
	// BaseURL defaults to Twitch's API. Tests point it at a fake.
	BaseURL    string
	HTTPClient *http.Client
}

// Client calls Twitch's API as the logged-in streamer. It's safe for
// concurrent use.
type Client struct {
	clientID   string
	account    Account
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a client for the given login.
func NewClient(config Config) *Client {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		clientID:   config.ClientID,
		account:    config.Account,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: httpClient,
	}
}

// Request is one call to Twitch's API.
type Request struct {
	// Method is an HTTP method, such as http.MethodGet.
	Method string
	// Path follows the base URL, such as "/chat/messages".
	Path  string
	Query url.Values
	// Body is sent as JSON. Nil sends no body.
	Body any
}

// Response is a response's status code and body. Twitch's errors are
// responses too, since what each one means depends on the request.
type Response struct {
	StatusCode int
	Body       []byte
}

// errorBody is the shape of Twitch's error responses.
type errorBody struct {
	Message string `json:"message"`
}

// ErrorMessage returns Twitch's explanation of a failed request, or the raw
// body if it isn't in the usual shape.
func (response Response) ErrorMessage() string {
	var decodedBody errorBody
	err := json.Unmarshal(response.Body, &decodedBody)
	if err != nil || decodedBody.Message == "" {
		return string(response.Body)
	}
	return decodedBody.Message
}

// BroadcasterUserID returns the logged-in streamer's user ID.
func (client *Client) BroadcasterUserID() (string, error) {
	userID, err := client.account.AccountUserID()
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return "", ErrNotLoggedIn
	}
	if err != nil {
		return "", fmt.Errorf("getting the Twitch account: %w", err)
	}
	return userID, nil
}

// Do sends a request. If Twitch rejects the access token, it renews the
// token and tries once more, since tokens expire every few hours and a
// streamer shouldn't see a failure for that.
func (client *Client) Do(ctx context.Context, request Request) (Response, error) {
	encodedBody, err := encodeBody(request)
	if err != nil {
		return Response{}, err
	}
	accessToken, err := client.account.AccessToken(ctx)
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return Response{}, ErrNotLoggedIn
	}
	if err != nil {
		return Response{}, fmt.Errorf("getting a Twitch access token: %w", err)
	}
	response, err := client.send(ctx, request, accessToken, encodedBody)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		return response, err
	}
	accessToken, err = client.account.RenewAccessToken(ctx, accessToken)
	if errors.Is(err, twitchauth.ErrNotLoggedIn) {
		return Response{}, ErrNotLoggedIn
	}
	if err != nil {
		return Response{}, fmt.Errorf("renewing the Twitch access token: %w", err)
	}
	return client.send(ctx, request, accessToken, encodedBody)
}

// encodeBody returns nil for a request without a body.
func encodeBody(request Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	encodedBody, err := json.Marshal(request.Body)
	if err != nil {
		return nil, fmt.Errorf("encoding the request to %s: %w", request.Path, err)
	}
	return encodedBody, nil
}

func (client *Client) send(ctx context.Context, request Request, accessToken string, encodedBody []byte) (Response, error) {
	requestURL := client.baseURL + request.Path
	if len(request.Query) > 0 {
		requestURL += "?" + request.Query.Encode()
	}
	var bodyReader io.Reader
	if encodedBody != nil {
		bodyReader = bytes.NewReader(encodedBody)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, requestURL, bodyReader)
	if err != nil {
		return Response{}, fmt.Errorf("creating the request to %s: %w", request.Path, err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+accessToken)
	httpRequest.Header.Set("Client-Id", client.clientID)
	if encodedBody != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	httpResponse, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("couldn't reach Twitch: %w", err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maximumResponseBodyBytes))
	if err != nil {
		return Response{}, fmt.Errorf("reading Twitch's response from %s: %w", request.Path, err)
	}
	return Response{StatusCode: httpResponse.StatusCode, Body: body}, nil
}
