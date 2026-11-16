package twitchapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
)

const (
	testClientID       = "test-client-id"
	streamerUserID     = "1337"
	firstAccessToken   = "first-token"
	renewedAccessToken = "renewed-token"
)

// The real login must work as the client's account.
var _ twitchapi.Account = (*twitchauth.Session)(nil)

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

// receivedRequest is what the fake Twitch API saw of one request.
type receivedRequest struct {
	method        string
	path          string
	query         url.Values
	authorization string
	contentType   string
	body          string
}

type fakeResponse struct {
	statusCode int
	body       string
}

type fakeTwitch struct {
	t         *testing.T
	mutex     sync.Mutex
	responses []fakeResponse
	requests  []receivedRequest
}

func newClient(t *testing.T, account twitchapi.Account, responses ...fakeResponse) (*twitchapi.Client, *fakeTwitch) {
	t.Helper()
	fake := &fakeTwitch{t: t, responses: responses}
	server := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(server.Close)
	client := twitchapi.NewClient(twitchapi.Config{ClientID: testClientID, Account: account, BaseURL: server.URL, HTTPClient: server.Client()})
	return client, fake
}

func (fake *fakeTwitch) handle(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		fake.t.Errorf("reading the request body: %v", err)
	}
	fake.requests = append(fake.requests, receivedRequest{
		method:        request.Method,
		path:          request.URL.Path,
		query:         request.URL.Query(),
		authorization: request.Header.Get("Authorization"),
		contentType:   request.Header.Get("Content-Type"),
		body:          string(body),
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

func TestDo_SendsQueryWithoutBodyForGet(t *testing.T) {
	client, fake := newClient(t, &fakeAccount{isLoggedIn: true}, fakeResponse{http.StatusOK, `{"data":[]}`})

	response, err := client.Do(context.Background(), twitchapi.Request{
		Method: http.MethodGet,
		Path:   "/channel_points/custom_rewards",
		Query:  url.Values{"broadcaster_id": {streamerUserID}, "only_manageable_rewards": {"true"}},
	})

	if err != nil || response.StatusCode != http.StatusOK || string(response.Body) != `{"data":[]}` {
		t.Fatalf("got response %+v, error %v", response, err)
	}
	request := fake.receivedRequests()[0]
	if request.method != http.MethodGet || request.path != "/channel_points/custom_rewards" {
		t.Errorf("got %s %s", request.method, request.path)
	}
	if request.query.Get("broadcaster_id") != streamerUserID || request.query.Get("only_manageable_rewards") != "true" {
		t.Errorf("got query %v", request.query)
	}
	if request.body != "" || request.contentType != "" {
		t.Errorf("got body %q with Content-Type %q, want neither", request.body, request.contentType)
	}
}

func TestDo_SendsBodyAsJSON(t *testing.T) {
	client, fake := newClient(t, &fakeAccount{isLoggedIn: true}, fakeResponse{http.StatusOK, `{}`})

	_, err := client.Do(context.Background(), twitchapi.Request{Method: http.MethodPost, Path: "/chat/messages", Body: map[string]string{"message": "hi"}})

	if err != nil {
		t.Fatalf("got error %v", err)
	}
	request := fake.receivedRequests()[0]
	if request.body != `{"message":"hi"}` || request.contentType != "application/json" || request.authorization != "Bearer "+firstAccessToken {
		t.Errorf("got request %+v", request)
	}
}

func TestDo_RenewsRejectedTokenAndRetriesOnce(t *testing.T) {
	account := &fakeAccount{isLoggedIn: true}
	client, fake := newClient(t, account,
		fakeResponse{http.StatusUnauthorized, `{"message":"Invalid OAuth token"}`},
		fakeResponse{http.StatusOK, `{}`},
	)

	response, err := client.Do(context.Background(), twitchapi.Request{Method: http.MethodDelete, Path: "/channel_points/custom_rewards"})

	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("got response %+v, error %v", response, err)
	}
	requests := fake.receivedRequests()
	if len(requests) != 2 || requests[1].authorization != "Bearer "+renewedAccessToken || requests[1].method != http.MethodDelete || account.renewedCount != 1 {
		t.Errorf("got requests %+v and %d renewals, want one retry with the renewed token", requests, account.renewedCount)
	}
}

func TestDo_ReturnsNotLoggedInWithoutCallingTwitch(t *testing.T) {
	client, fake := newClient(t, &fakeAccount{isLoggedIn: false})

	_, err := client.Do(context.Background(), twitchapi.Request{Method: http.MethodGet, Path: "/anything"})

	if !errors.Is(err, twitchapi.ErrNotLoggedIn) {
		t.Errorf("got error %v, want ErrNotLoggedIn", err)
	}
	if len(fake.receivedRequests()) != 0 {
		t.Error("called Twitch without a login")
	}
}

func TestBroadcasterUserID_ReturnsNotLoggedIn(t *testing.T) {
	client, _ := newClient(t, &fakeAccount{isLoggedIn: false})

	_, err := client.BroadcasterUserID()

	if !errors.Is(err, twitchapi.ErrNotLoggedIn) {
		t.Errorf("got error %v, want ErrNotLoggedIn", err)
	}
}

func TestErrorMessage_UsesTwitchsMessageOrTheRawBody(t *testing.T) {
	withMessage := twitchapi.Response{Body: []byte(`{"error":"Forbidden","status":403,"message":"The broadcaster is not a partner or affiliate."}`)}
	unusual := twitchapi.Response{Body: []byte("upstream timed out")}

	if message := withMessage.ErrorMessage(); message != "The broadcaster is not a partner or affiliate." {
		t.Errorf("got %q for Twitch's usual error shape", message)
	}
	if message := unusual.ErrorMessage(); message != "upstream timed out" {
		t.Errorf("got %q for an unusual body", message)
	}
}
