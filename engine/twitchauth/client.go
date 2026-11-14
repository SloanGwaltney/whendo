// Package twitchauth logs users in to Twitch with the Device Code Grant flow
// and keeps their tokens refreshed and validated. See
// docs/design/twitch-login.md for the rules it follows.
package twitchauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultIdentityBaseURL is the base URL of Twitch's OAuth server.
const DefaultIdentityBaseURL = "https://id.twitch.tv"

const (
	deviceCodePath = "/oauth2/device"
	tokenPath      = "/oauth2/token"
	validatePath   = "/oauth2/validate"
	revokePath     = "/oauth2/revoke"

	deviceCodeGrantType   = "urn:ietf:params:oauth:grant-type:device_code"
	refreshTokenGrantType = "refresh_token"

	authorizationPendingMessage = "authorization_pending"
	slowDownMessage             = "slow_down"

	// RFC 8628 says to poll every 5 seconds when the server doesn't say, and
	// to add 5 seconds each time it asks us to slow down.
	defaultPollInterval      = 5 * time.Second
	slowDownIntervalIncrease = 5 * time.Second

	bytesPerMegabyte = 1024 * 1024

	// Twitch's responses are small. The limit only protects against a broken
	// or hostile response filling memory.
	maximumResponseBodyBytes = 1 * bytesPerMegabyte
)

var (
	// ErrDeviceCodeExpired means the user didn't approve the login before the
	// device code expired. The user has to start the login again.
	ErrDeviceCodeExpired = errors.New("the login code expired before it was approved")
	// ErrTokenRejected means Twitch no longer accepts a token, for example
	// because the user changed their password or disconnected Whendo.
	ErrTokenRejected = errors.New("twitch rejected the token")
)

// DeviceAuthorization is a pending login that the user approves in their
// browser.
type DeviceAuthorization struct {
	DeviceCode string
	// UserCode is the code the user confirms on Twitch's activation page.
	UserCode string
	// VerificationURI is Twitch's activation page, with the code filled in.
	VerificationURI string
	ExpiresAt       time.Time
	PollInterval    time.Duration
}

// Token is a Twitch access token and the refresh token that renews it.
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// TokenValidation is what Twitch reports about a valid access token.
type TokenValidation struct {
	Login     string
	UserID    string
	Scopes    []string
	ExpiresIn time.Duration
}

// Client calls Twitch's OAuth endpoints for a public client, which has no
// client secret.
type Client struct {
	clientID        string
	identityBaseURL string
	httpClient      *http.Client
}

// NewClient returns a Client for the Twitch application clientID. Pass
// DefaultIdentityBaseURL as identityBaseURL outside of tests.
func NewClient(clientID string, identityBaseURL string, httpClient *http.Client) *Client {
	return &Client{
		clientID:        clientID,
		identityBaseURL: strings.TrimSuffix(identityBaseURL, "/"),
		httpClient:      httpClient,
	}
}

type deviceCodeResponse struct {
	DeviceCode       string `json:"device_code"`
	UserCode         string `json:"user_code"`
	VerificationURI  string `json:"verification_uri"`
	ExpiresInSeconds int    `json:"expires_in"`
	IntervalSeconds  int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresInSeconds int    `json:"expires_in"`
}

type validateResponse struct {
	Login            string   `json:"login"`
	UserID           string   `json:"user_id"`
	Scopes           []string `json:"scopes"`
	ExpiresInSeconds int      `json:"expires_in"`
}

type errorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// RequestDeviceCode starts a login that requests scopes.
func (client *Client) RequestDeviceCode(ctx context.Context, scopes []string) (DeviceAuthorization, error) {
	form := url.Values{
		"client_id": {client.clientID},
		"scopes":    {strings.Join(scopes, " ")},
	}
	var response deviceCodeResponse
	statusCode, body, err := client.postForm(ctx, deviceCodePath, form)
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("requesting a device code: %w", err)
	}
	if statusCode != http.StatusOK {
		return DeviceAuthorization{}, fmt.Errorf("requesting a device code: %w", describeErrorResponse(statusCode, body))
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return DeviceAuthorization{}, fmt.Errorf("reading the device code response: %w", err)
	}

	pollInterval := time.Duration(response.IntervalSeconds) * time.Second
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	return DeviceAuthorization{
		DeviceCode:      response.DeviceCode,
		UserCode:        response.UserCode,
		VerificationURI: response.VerificationURI,
		ExpiresAt:       time.Now().Add(time.Duration(response.ExpiresInSeconds) * time.Second),
		PollInterval:    pollInterval,
	}, nil
}

// PollForToken waits until the user approves the login, then returns its
// token. It returns ErrDeviceCodeExpired if the user doesn't approve in time,
// and the context's error if ctx is cancelled first.
func (client *Client) PollForToken(ctx context.Context, authorization DeviceAuthorization, scopes []string) (Token, error) {
	form := url.Values{
		"client_id":   {client.clientID},
		"scopes":      {strings.Join(scopes, " ")},
		"device_code": {authorization.DeviceCode},
		"grant_type":  {deviceCodeGrantType},
	}
	pollInterval := authorization.PollInterval
	for {
		err := waitFor(ctx, pollInterval)
		if err != nil {
			return Token{}, err
		}
		if time.Now().After(authorization.ExpiresAt) {
			return Token{}, ErrDeviceCodeExpired
		}

		statusCode, body, err := client.postForm(ctx, tokenPath, form)
		if err != nil {
			return Token{}, fmt.Errorf("polling for the login token: %w", err)
		}
		if statusCode == http.StatusOK {
			return parseTokenResponse(body)
		}

		message := errorMessage(body)
		if message == authorizationPendingMessage {
			continue
		}
		if message == slowDownMessage {
			pollInterval += slowDownIntervalIncrease
			continue
		}
		// Twitch reports an expired code as a generic error, so check the
		// deadline before reporting it as a failure.
		if time.Now().After(authorization.ExpiresAt) {
			return Token{}, ErrDeviceCodeExpired
		}
		return Token{}, fmt.Errorf("polling for the login token: %w", describeErrorResponse(statusCode, body))
	}
}

// RefreshToken exchanges refreshToken for a new token. Each refresh token
// works only once, so the returned token's RefreshToken must replace it. It
// returns ErrTokenRejected if Twitch no longer accepts refreshToken.
func (client *Client) RefreshToken(ctx context.Context, refreshToken string) (Token, error) {
	form := url.Values{
		"client_id":     {client.clientID},
		"grant_type":    {refreshTokenGrantType},
		"refresh_token": {refreshToken},
	}
	statusCode, body, err := client.postForm(ctx, tokenPath, form)
	if err != nil {
		return Token{}, fmt.Errorf("refreshing the token: %w", err)
	}
	if statusCode == http.StatusBadRequest || statusCode == http.StatusUnauthorized {
		return Token{}, fmt.Errorf("refreshing the token: %w: %w", ErrTokenRejected, describeErrorResponse(statusCode, body))
	}
	if statusCode != http.StatusOK {
		return Token{}, fmt.Errorf("refreshing the token: %w", describeErrorResponse(statusCode, body))
	}
	return parseTokenResponse(body)
}

// ValidateToken asks Twitch whether accessToken is still valid, and who it
// belongs to. It returns ErrTokenRejected if the token is invalid.
func (client *Client) ValidateToken(ctx context.Context, accessToken string) (TokenValidation, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.identityBaseURL+validatePath, nil)
	if err != nil {
		return TokenValidation{}, fmt.Errorf("creating the validation request: %w", err)
	}
	request.Header.Set("Authorization", "OAuth "+accessToken)

	statusCode, body, err := client.send(request)
	if err != nil {
		return TokenValidation{}, fmt.Errorf("validating the token: %w", err)
	}
	if statusCode == http.StatusUnauthorized {
		return TokenValidation{}, fmt.Errorf("validating the token: %w", ErrTokenRejected)
	}
	if statusCode != http.StatusOK {
		return TokenValidation{}, fmt.Errorf("validating the token: %w", describeErrorResponse(statusCode, body))
	}

	var response validateResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return TokenValidation{}, fmt.Errorf("reading the validation response: %w", err)
	}
	return TokenValidation{
		Login:     response.Login,
		UserID:    response.UserID,
		Scopes:    response.Scopes,
		ExpiresIn: time.Duration(response.ExpiresInSeconds) * time.Second,
	}, nil
}

// RevokeToken tells Twitch to stop accepting accessToken, which disconnects
// Whendo from the user's account.
func (client *Client) RevokeToken(ctx context.Context, accessToken string) error {
	form := url.Values{
		"client_id": {client.clientID},
		"token":     {accessToken},
	}
	statusCode, body, err := client.postForm(ctx, revokePath, form)
	if err != nil {
		return fmt.Errorf("revoking the token: %w", err)
	}
	if statusCode != http.StatusOK {
		return fmt.Errorf("revoking the token: %w", describeErrorResponse(statusCode, body))
	}
	return nil
}

func (client *Client) postForm(ctx context.Context, path string, form url.Values) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.identityBaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, fmt.Errorf("creating the request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.send(request)
}

func (client *Client) send(request *http.Request) (int, []byte, error) {
	response, err := client.httpClient.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBodyBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("reading the response: %w", err)
	}
	return response.StatusCode, body, nil
}

func parseTokenResponse(body []byte) (Token, error) {
	var response tokenResponse
	err := json.Unmarshal(body, &response)
	if err != nil {
		return Token{}, fmt.Errorf("reading the token response: %w", err)
	}
	return Token{
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(response.ExpiresInSeconds) * time.Second),
	}, nil
}

// errorMessage returns the message from a Twitch error response, or an empty
// string if the body isn't one.
func errorMessage(body []byte) string {
	var response errorResponse
	err := json.Unmarshal(body, &response)
	if err != nil {
		return ""
	}
	return response.Message
}

func describeErrorResponse(statusCode int, body []byte) error {
	message := errorMessage(body)
	if message == "" {
		return fmt.Errorf("twitch responded with HTTP %d", statusCode)
	}
	return fmt.Errorf("twitch responded with HTTP %d: %s", statusCode, message)
}

func waitFor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
