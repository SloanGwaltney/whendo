package twitchsteps_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
	"github.com/SloanGwaltney/whendo/engine/twitchrewards"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

const (
	testClientID       = "test-client-id"
	streamerUserID     = "1337"
	firstAccessToken   = "first-token"
	renewedAccessToken = "renewed-token"
)

// The real login must work as the steps' account.
var _ twitchapi.Account = (*twitchauth.Session)(nil)

// fakeAccount stands in for the Twitch login.
type fakeAccount struct {
	isLoggedIn   bool
	mutex        sync.Mutex
	renewedCount int
}

func (account *fakeAccount) AccountUserID() (string, error) {
	if !account.isLoggedIn {
		return "", twitchauth.ErrNotLoggedIn
	}
	return streamerUserID, nil
}

func (account *fakeAccount) AccessToken(context.Context) (string, error) {
	if !account.isLoggedIn {
		return "", twitchauth.ErrNotLoggedIn
	}
	return firstAccessToken, nil
}

func (account *fakeAccount) RenewAccessToken(ctx context.Context, rejectedAccessToken string) (string, error) {
	account.mutex.Lock()
	defer account.mutex.Unlock()
	account.renewedCount++
	return renewedAccessToken, nil
}

// fakeRewards stands in for the last list of rewards.
type fakeRewards map[string]twitchrewards.Reward

func (rewards fakeRewards) KnownReward(rewardID string) (twitchrewards.Reward, bool) {
	reward, isKnown := rewards[rewardID]
	return reward, isKnown
}

// receivedRequest is one request the fake Twitch API received.
type receivedRequest struct {
	method        string
	path          string
	query         url.Values
	authorization string
	clientID      string
	body          map[string]any
}

// fakeTwitch answers each request with the next response in its list and
// records what it received.
type fakeTwitch struct {
	t         *testing.T
	mutex     sync.Mutex
	responses []fakeResponse
	requests  []receivedRequest
}

type fakeResponse struct {
	statusCode int
	body       string
}

func newFakeTwitch(t *testing.T, responses ...fakeResponse) (*fakeTwitch, *httptest.Server) {
	t.Helper()
	fake := &fakeTwitch{t: t, responses: responses}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)
	return fake, server
}

func (fake *fakeTwitch) handle(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	encodedBody, err := io.ReadAll(request.Body)
	if err != nil {
		fake.t.Errorf("reading the request body: %v", err)
	}
	var body map[string]any
	err = json.Unmarshal(encodedBody, &body)
	if err != nil {
		fake.t.Errorf("decoding the request body: %v", err)
	}
	fake.requests = append(fake.requests, receivedRequest{
		method:        request.Method,
		path:          request.URL.Path,
		query:         request.URL.Query(),
		authorization: request.Header.Get("Authorization"),
		clientID:      request.Header.Get("Client-Id"),
		body:          body,
	})
	if len(fake.responses) == 0 {
		fake.t.Error("got more requests than expected")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	response := fake.responses[0]
	fake.responses = fake.responses[1:]
	writer.WriteHeader(response.statusCode)
	_, err = writer.Write([]byte(response.body))
	if err != nil {
		fake.t.Errorf("writing the response: %v", err)
	}
}

func (fake *fakeTwitch) receivedRequests() []receivedRequest {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return fake.requests
}

// twitchStep returns the Twitch step with the given ID, calling the fake
// server as the given account.
func twitchStep(t *testing.T, stepID string, server *httptest.Server, account twitchapi.Account, rewards twitchsteps.RewardLookup) step.Step {
	t.Helper()
	steps := twitchsteps.Steps(twitchapi.NewClient(twitchapi.Config{
		ClientID:   testClientID,
		Account:    account,
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}), rewards)
	for _, candidate := range steps {
		if candidate.Definition().ID == stepID {
			return candidate
		}
	}
	t.Fatalf("no step %q", stepID)
	return nil
}
