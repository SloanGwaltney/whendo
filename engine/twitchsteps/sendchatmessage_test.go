package twitchsteps_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/SloanGwaltney/whendo/engine/automationsetting"
	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/twitchapi"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
	"github.com/SloanGwaltney/whendo/engine/twitchsteps"
)

const (
	testClientID       = "test-client-id"
	streamerUserID     = "1337"
	firstAccessToken   = "first-token"
	renewedAccessToken = "renewed-token"
	sentResponseBody   = `{"data":[{"message_id":"abc","is_sent":true,"drop_reason":null}]}`
	droppedSlowMode    = `{"data":[{"message_id":"","is_sent":false,"drop_reason":{"code":"slow_mode","message":"This room is in slow mode."}}]}`
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

// receivedRequest is one request the fake Twitch API received.
type receivedRequest struct {
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
	if request.Method != http.MethodPost || request.URL.Path != "/chat/messages" {
		fake.t.Errorf("got %s %s, want POST /chat/messages", request.Method, request.URL.Path)
	}
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

func sendChatMessageStep(t *testing.T, server *httptest.Server, account twitchapi.Account) step.Step {
	t.Helper()
	steps := twitchsteps.Steps(twitchapi.NewClient(twitchapi.Config{
		ClientID:   testClientID,
		Account:    account,
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	}))
	for _, twitchStep := range steps {
		if twitchStep.Definition().ID == twitchsteps.SendChatMessageStepID {
			return twitchStep
		}
	}
	t.Fatal("no send chat message step")
	return nil
}

func messageInput(t *testing.T, template automationsetting.Template, variables automationsetting.VariableValues) step.Input {
	t.Helper()
	encodedTemplate, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("encoding the template: %v", err)
	}
	return step.Input{
		Settings:  automationsetting.Values{twitchsteps.MessageSetting: encodedTemplate},
		Variables: variables,
	}
}

func plainMessage(t *testing.T, text string) step.Input {
	t.Helper()
	return messageInput(t, automationsetting.Template{{Text: text}}, automationsetting.VariableValues{})
}

func TestSendChatMessage_PostsMessageAsStreamerInOwnChannel(t *testing.T) {
	fake, server := newFakeTwitch(t, fakeResponse{http.StatusOK, sentResponseBody})
	redeemer := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: "redeemerDisplayName"}
	input := messageInput(t,
		automationsetting.Template{{Text: "Thanks for checking in, "}, {Variable: &redeemer}, {Text: "!"}},
		automationsetting.VariableValues{redeemer: "Cooler_User"},
	)

	result, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(), input)

	if err != nil || result.OutcomeID != step.DefaultOutcomeID {
		t.Fatalf("got result %+v, error %v", result, err)
	}
	wantDetails := []step.Detail{{Name: "Message", Value: "Thanks for checking in, Cooler_User!"}}
	if !slices.Equal(result.Details, wantDetails) {
		t.Errorf("got details %+v, want %+v", result.Details, wantDetails)
	}
	requests := fake.receivedRequests()
	if len(requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(requests))
	}
	request := requests[0]
	if request.authorization != "Bearer "+firstAccessToken || request.clientID != testClientID {
		t.Errorf("got Authorization %q and Client-Id %q", request.authorization, request.clientID)
	}
	if request.body["broadcaster_id"] != streamerUserID || request.body["sender_id"] != streamerUserID {
		t.Errorf("got body %v, want the streamer as broadcaster and sender", request.body)
	}
	if request.body["message"] != "Thanks for checking in, Cooler_User!" {
		t.Errorf("got message %q", request.body["message"])
	}
}

func TestSendChatMessage_RenewsRejectedTokenAndRetriesOnce(t *testing.T) {
	fake, server := newFakeTwitch(t,
		fakeResponse{http.StatusUnauthorized, `{"message":"Invalid OAuth token"}`},
		fakeResponse{http.StatusOK, sentResponseBody},
	)
	account := &fakeAccount{isLoggedIn: true}

	_, err := sendChatMessageStep(t, server, account).Run(context.Background(), plainMessage(t, "hello"))

	if err != nil {
		t.Fatalf("got error %v", err)
	}
	requests := fake.receivedRequests()
	if len(requests) != 2 || requests[1].authorization != "Bearer "+renewedAccessToken || account.renewedCount != 1 {
		t.Errorf("got %d requests and %d renewals, want a retry with the renewed token", len(requests), account.renewedCount)
	}
}

func TestSendChatMessage_FailsWhenRenewedTokenIsAlsoRejected(t *testing.T) {
	_, server := newFakeTwitch(t,
		fakeResponse{http.StatusUnauthorized, `{"message":"Invalid OAuth token"}`},
		fakeResponse{http.StatusUnauthorized, `{"message":"Missing scope: user:write:chat"}`},
	)

	_, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(), plainMessage(t, "hello"))

	if err == nil || !strings.Contains(err.Error(), "Missing scope") {
		t.Errorf("got error %v, want Twitch's reason", err)
	}
}

func TestSendChatMessage_FailsWithTwitchsReasonWhenMessageIsDropped(t *testing.T) {
	_, server := newFakeTwitch(t, fakeResponse{http.StatusOK, droppedSlowMode})

	result, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(), plainMessage(t, "hello"))

	if err == nil || !strings.Contains(err.Error(), "slow mode") {
		t.Errorf("got error %v, want the drop reason", err)
	}
	wantDetails := []step.Detail{{Name: "Message", Value: "hello"}}
	if !slices.Equal(result.Details, wantDetails) {
		t.Errorf("got details %+v, want the message Twitch dropped", result.Details)
	}
}

func TestSendChatMessage_FailsForErrorResponses(t *testing.T) {
	for _, statusCode := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusInternalServerError} {
		_, server := newFakeTwitch(t, fakeResponse{statusCode, `{"message":"no"}`})

		_, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(), plainMessage(t, "hello"))

		if err == nil {
			t.Errorf("HTTP %d: got no error", statusCode)
		}
	}
}

func TestSendChatMessage_RefusesTooLongOrEmptyMessagesWithoutCallingTwitch(t *testing.T) {
	tooLong := strings.Repeat("é", twitchsteps.MaximumChatMessageLength+1)
	for description, text := range map[string]string{"too long": tooLong, "blank": "   "} {
		fake, server := newFakeTwitch(t)

		_, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(), plainMessage(t, text))

		if err == nil || len(fake.receivedRequests()) != 0 {
			t.Errorf("%s: got error %v and %d requests, want a failure without calling Twitch", description, err, len(fake.receivedRequests()))
		}
	}
}

func TestSendChatMessage_AllowsMessageAtTheLimitCountingCharactersNotBytes(t *testing.T) {
	_, server := newFakeTwitch(t, fakeResponse{http.StatusOK, sentResponseBody})
	atLimit := strings.Repeat("é", twitchsteps.MaximumChatMessageLength)

	_, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(), plainMessage(t, atLimit))

	if err != nil {
		t.Errorf("got error %v", err)
	}
}

func TestSendChatMessage_FailsWhenNotLoggedIn(t *testing.T) {
	fake, server := newFakeTwitch(t)

	_, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: false}).Run(context.Background(), plainMessage(t, "hello"))

	if err == nil || len(fake.receivedRequests()) != 0 {
		t.Errorf("got error %v and %d requests, want a failure without calling Twitch", err, len(fake.receivedRequests()))
	}
}

func TestSendChatMessage_FailsWhenAVariableIsUnavailable(t *testing.T) {
	_, server := newFakeTwitch(t)
	missing := automationsetting.VariableReference{Source: automationsetting.VariableSourceTrigger, ID: "missing"}

	_, err := sendChatMessageStep(t, server, &fakeAccount{isLoggedIn: true}).Run(context.Background(),
		messageInput(t, automationsetting.Template{{Variable: &missing}}, automationsetting.VariableValues{}))

	if !errors.Is(err, automationsetting.ErrVariableUnavailable) {
		t.Errorf("got error %v, want ErrVariableUnavailable", err)
	}
}
