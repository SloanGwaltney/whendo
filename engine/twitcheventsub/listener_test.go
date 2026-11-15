package twitcheventsub_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
	"github.com/SloanGwaltney/whendo/engine/twitcheventsub"
)

const (
	testClientID              = "test-client-id"
	testAccessToken           = "test-access-token"
	renewedAccessToken        = "renewed-access-token"
	largeKeepaliveSeconds     = 600
	shortKeepaliveSeconds     = 1
	waitTimeout               = 15 * time.Second
	expectedSubscriptionCount = 3
)

// fakeEventSub imitates Twitch's EventSub WebSocket server and its
// subscription API. Set its fields before the listener connects.
type fakeEventSub struct {
	t      *testing.T
	server *httptest.Server

	keepaliveSeconds int
	rejectedTokens   map[string]bool
	isScopeMissing   bool
	acceptedSockets  chan *fakeSocket
	mutex            sync.Mutex
	sessionCount     int
	subscriptions    []recordedSubscription
}

type recordedSubscription struct {
	Type      string            `json:"type"`
	Version   string            `json:"version"`
	Condition map[string]string `json:"condition"`
	Transport struct {
		Method    string `json:"method"`
		SessionID string `json:"session_id"`
	} `json:"transport"`
	accessToken string
	clientID    string
}

type fakeSocket struct {
	webSocket      *websocket.Conn
	sessionID      string
	closedByClient context.Context
}

func newFakeEventSub(t *testing.T) *fakeEventSub {
	t.Helper()
	fake := &fakeEventSub{
		t:                t,
		keepaliveSeconds: largeKeepaliveSeconds,
		rejectedTokens:   map[string]bool{},
		acceptedSockets:  make(chan *fakeSocket, 8),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", fake.handleWebSocket)
	mux.HandleFunc("POST /helix/eventsub/subscriptions", fake.handleSubscribe)
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (fake *fakeEventSub) webSocketURL() string {
	return "ws" + strings.TrimPrefix(fake.server.URL, "http") + "/ws"
}

func (fake *fakeEventSub) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	webSocket, err := websocket.Accept(writer, request, nil)
	if err != nil {
		fake.t.Errorf("accepting a WebSocket: %v", err)
		return
	}
	fake.mutex.Lock()
	fake.sessionCount++
	sessionID := fmt.Sprintf("session-%d", fake.sessionCount)
	keepaliveSeconds := fake.keepaliveSeconds
	fake.mutex.Unlock()

	// CloseRead keeps reading in the background, so the server notices when
	// the client closes the connection.
	closedByClient := webSocket.CloseRead(context.Background())
	socket := &fakeSocket{webSocket: webSocket, sessionID: sessionID, closedByClient: closedByClient}
	socket.send(fake.t, fmt.Sprintf(`{
		"metadata": {"message_id": "welcome-%s", "message_type": "session_welcome", "message_timestamp": "2023-07-19T14:56:51.634234626Z"},
		"payload": {"session": {"id": %q, "status": "connected", "connected_at": "2023-07-19T14:56:51.616329898Z", "keepalive_timeout_seconds": %d, "reconnect_url": null}}
	}`, sessionID, sessionID, keepaliveSeconds))
	fake.acceptedSockets <- socket
	<-closedByClient.Done()
}

func (fake *fakeEventSub) handleSubscribe(writer http.ResponseWriter, request *http.Request) {
	accessToken := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
	fake.mutex.Lock()
	isRejected := fake.rejectedTokens[accessToken]
	isScopeMissing := fake.isScopeMissing
	fake.mutex.Unlock()
	if isRejected {
		writeHelixError(writer, http.StatusUnauthorized, "Invalid OAuth token")
		return
	}
	if isScopeMissing {
		writeHelixError(writer, http.StatusForbidden, "subscription missing proper authorization")
		return
	}

	var subscription recordedSubscription
	err := json.NewDecoder(request.Body).Decode(&subscription)
	if err != nil {
		writeHelixError(writer, http.StatusBadRequest, "invalid body")
		return
	}
	subscription.accessToken = accessToken
	subscription.clientID = request.Header.Get("Client-Id")
	fake.mutex.Lock()
	fake.subscriptions = append(fake.subscriptions, subscription)
	fake.mutex.Unlock()
	writer.WriteHeader(http.StatusAccepted)
}

func writeHelixError(writer http.ResponseWriter, statusCode int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	err := json.NewEncoder(writer).Encode(map[string]any{"status": statusCode, "message": message})
	if err != nil {
		panic(err)
	}
}

func (fake *fakeEventSub) recordedSubscriptions() []recordedSubscription {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]recordedSubscription(nil), fake.subscriptions...)
}

func (fake *fakeEventSub) awaitSocket(t *testing.T) *fakeSocket {
	t.Helper()
	select {
	case socket := <-fake.acceptedSockets:
		return socket
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for the listener to connect")
		return nil
	}
}

func (socket *fakeSocket) send(t *testing.T, message string) {
	t.Helper()
	err := socket.webSocket.Write(context.Background(), websocket.MessageText, []byte(message))
	if err != nil {
		t.Errorf("sending a message: %v", err)
	}
}

func notificationMessage(messageID string, subscriptionType string, subscriptionVersion string, eventJSON string) string {
	return fmt.Sprintf(`{
		"metadata": {"message_id": %q, "message_type": "notification", "message_timestamp": "2023-07-19T10:11:12.634234626Z", "subscription_type": %q, "subscription_version": %q},
		"payload": {"subscription": {"id": "subscription-id", "status": "enabled", "type": %q, "version": %q, "cost": 0, "condition": {}, "transport": {"method": "websocket", "session_id": "session"}, "created_at": "2023-07-19T14:56:51.634234626Z"}, "event": %s}
	}`, messageID, subscriptionType, subscriptionVersion, subscriptionType, subscriptionVersion, eventJSON)
}

func revocationMessage(reason string, subscriptionType string, subscriptionVersion string) string {
	return fmt.Sprintf(`{
		"metadata": {"message_id": "revocation", "message_type": "revocation", "message_timestamp": "2023-07-19T10:11:12.634234626Z", "subscription_type": %q, "subscription_version": %q},
		"payload": {"subscription": {"id": "subscription-id", "status": %q, "type": %q, "version": %q, "cost": 0, "condition": {}, "transport": {"method": "websocket", "session_id": "session"}, "created_at": "2023-07-19T14:56:51.634234626Z"}}
	}`, subscriptionType, subscriptionVersion, reason, subscriptionType, subscriptionVersion)
}

func reconnectMessage(reconnectURL string) string {
	return fmt.Sprintf(`{
		"metadata": {"message_id": "reconnect", "message_type": "session_reconnect", "message_timestamp": "2023-07-19T10:11:12.634234626Z"},
		"payload": {"session": {"id": "session", "status": "reconnecting", "keepalive_timeout_seconds": null, "reconnect_url": %q, "connected_at": "2023-07-19T14:56:51.616329898Z"}}
	}`, reconnectURL)
}

func chatNotification(messageID string) string {
	return notificationMessage(messageID, chatMessageType, "1", chatMessageEventJSON(""))
}

type fakeAuthorization struct {
	mutex            sync.Mutex
	accessToken      string
	renewCount       int
	isLoggedIn       bool
	sessionEndedOnce sync.Once
	sessionEnded     chan struct{}
}

func newFakeAuthorization() *fakeAuthorization {
	return &fakeAuthorization{accessToken: testAccessToken, isLoggedIn: true, sessionEnded: make(chan struct{})}
}

func (authorization *fakeAuthorization) AccessToken(ctx context.Context) (string, error) {
	authorization.mutex.Lock()
	defer authorization.mutex.Unlock()
	if !authorization.isLoggedIn {
		return "", twitchauth.ErrNotLoggedIn
	}
	return authorization.accessToken, nil
}

func (authorization *fakeAuthorization) RenewAccessToken(ctx context.Context, rejectedAccessToken string) (string, error) {
	authorization.mutex.Lock()
	defer authorization.mutex.Unlock()
	authorization.renewCount++
	authorization.accessToken = renewedAccessToken
	return authorization.accessToken, nil
}

func (authorization *fakeAuthorization) AccountUserID() (string, error) {
	return broadcasterUserID, nil
}

func (authorization *fakeAuthorization) EndSessionRevokedByTwitch() {
	authorization.mutex.Lock()
	authorization.isLoggedIn = false
	authorization.mutex.Unlock()
	authorization.sessionEndedOnce.Do(func() { close(authorization.sessionEnded) })
}

type eventCollector struct {
	events chan trigger.Event
}

func (collector *eventCollector) ReceiveEvent(event trigger.Event) {
	collector.events <- event
}

type listenerUnderTest struct {
	listener      *twitcheventsub.Listener
	authorization *fakeAuthorization
	events        chan trigger.Event
	states        chan twitcheventsub.ConnectionState
	runResult     chan error
	cancel        context.CancelFunc
}

func startListener(t *testing.T, fake *fakeEventSub, authorization *fakeAuthorization) listenerUnderTest {
	t.Helper()
	events := make(chan trigger.Event, 16)
	states := make(chan twitcheventsub.ConnectionState, 64)
	listener := twitcheventsub.NewListener(twitcheventsub.ListenerConfig{
		ClientID:      testClientID,
		Authorization: authorization,
		Receiver:      &eventCollector{events: events},
		OnStateChange: func(state twitcheventsub.ConnectionState) { states <- state },
		WebSocketURL:  fake.webSocketURL(),
		HelixBaseURL:  fake.server.URL + "/helix",
		HTTPClient:    fake.server.Client(),
	})
	ctx, cancel := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	runFinished := make(chan struct{})
	go func() {
		runResult <- listener.Run(ctx)
		close(runFinished)
	}()
	subject := listenerUnderTest{listener: listener, authorization: authorization, events: events, states: states, runResult: runResult, cancel: cancel}
	t.Cleanup(func() {
		cancel()
		select {
		case <-runFinished:
		case <-time.After(waitTimeout):
			t.Error("Run didn't stop after its context was cancelled")
		}
	})
	return subject
}

func (subject listenerUnderTest) awaitStatus(t *testing.T, wantedStatus twitcheventsub.ConnectionStatus) twitcheventsub.ConnectionState {
	t.Helper()
	timeout := time.After(waitTimeout)
	for {
		select {
		case state := <-subject.states:
			if state.Status == wantedStatus {
				return state
			}
		case <-timeout:
			t.Fatalf("timed out waiting for connection status %q", wantedStatus)
		}
	}
}

func (subject listenerUnderTest) awaitEvent(t *testing.T) trigger.Event {
	t.Helper()
	select {
	case event := <-subject.events:
		return event
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for an event")
		return trigger.Event{}
	}
}

func (subject listenerUnderTest) awaitRunResult(t *testing.T) error {
	t.Helper()
	select {
	case err := <-subject.runResult:
		return err
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for Run to return")
		return nil
	}
}

func TestListenerRun_SubscribesToEveryEventForTheSession(t *testing.T) {
	fake := newFakeEventSub(t)
	subject := startListener(t, fake, newFakeAuthorization())
	socket := fake.awaitSocket(t)

	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	subscriptions := fake.recordedSubscriptions()
	if len(subscriptions) != expectedSubscriptionCount {
		t.Fatalf("got %d subscriptions, want %d", len(subscriptions), expectedSubscriptionCount)
	}
	wantedVersions := map[string]string{redemptionType: "1", chatMessageType: "1", followType: "2"}
	for _, subscription := range subscriptions {
		if subscription.Version != wantedVersions[subscription.Type] {
			t.Errorf("subscribed to %s version %q, want %q", subscription.Type, subscription.Version, wantedVersions[subscription.Type])
		}
		if subscription.Transport.Method != "websocket" || subscription.Transport.SessionID != socket.sessionID {
			t.Errorf("subscribed to %s with transport %+v, want this session's WebSocket", subscription.Type, subscription.Transport)
		}
		if subscription.Condition["broadcaster_user_id"] != broadcasterUserID {
			t.Errorf("subscribed to %s with condition %v", subscription.Type, subscription.Condition)
		}
		if subscription.accessToken != testAccessToken || subscription.clientID != testClientID {
			t.Errorf("subscribed to %s with token %q and client ID %q", subscription.Type, subscription.accessToken, subscription.clientID)
		}
	}
}

func TestListenerRun_DeliversNotificationsAsTriggerEvents(t *testing.T) {
	fake := newFakeEventSub(t)
	subject := startListener(t, fake, newFakeAuthorization())
	socket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	socket.send(t, notificationMessage("redeem-1", redemptionType, "1", redemptionEventJSON))

	event := subject.awaitEvent(t)
	if event.TriggerID != twitcheventsub.RedeemTriggerID || event.Variables[twitcheventsub.UserInputVariable] != "pogchamp" {
		t.Errorf("got event %+v", event)
	}
}

func TestListenerRun_IgnoresDuplicateNotifications(t *testing.T) {
	fake := newFakeEventSub(t)
	subject := startListener(t, fake, newFakeAuthorization())
	socket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	socket.send(t, chatNotification("message-1"))
	socket.send(t, chatNotification("message-1"))
	socket.send(t, notificationMessage("message-2", followType, "2", followEventJSON))

	if event := subject.awaitEvent(t); event.TriggerID != twitcheventsub.ChatMessageTriggerID {
		t.Errorf("got first event for %q, want the chat message", event.TriggerID)
	}
	if event := subject.awaitEvent(t); event.TriggerID != twitcheventsub.FollowTriggerID {
		t.Errorf("got second event for %q, want the follow; the duplicate wasn't ignored", event.TriggerID)
	}
}

func TestListenerRun_ReconnectsAndResubscribesAfterConnectionDrops(t *testing.T) {
	fake := newFakeEventSub(t)
	subject := startListener(t, fake, newFakeAuthorization())
	firstSocket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	err := firstSocket.webSocket.Close(websocket.StatusCode(4006), "network error")
	if err != nil {
		t.Fatalf("closing the connection: %v", err)
	}

	secondSocket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)
	subscriptions := fake.recordedSubscriptions()
	if len(subscriptions) != 2*expectedSubscriptionCount {
		t.Fatalf("got %d subscriptions in total, want %d", len(subscriptions), 2*expectedSubscriptionCount)
	}
	for _, subscription := range subscriptions[expectedSubscriptionCount:] {
		if subscription.Transport.SessionID != secondSocket.sessionID {
			t.Errorf("resubscribed to %s for session %q, want %q", subscription.Type, subscription.Transport.SessionID, secondSocket.sessionID)
		}
	}
}

func TestListenerRun_ReconnectsWhenKeepalivesStop(t *testing.T) {
	fake := newFakeEventSub(t)
	fake.keepaliveSeconds = shortKeepaliveSeconds
	startListener(t, fake, newFakeAuthorization())
	firstSocket := fake.awaitSocket(t)

	// The fake never sends keepalives, so the listener should give up on
	// the connection and open a new one.
	fake.awaitSocket(t)

	select {
	case <-firstSocket.closedByClient.Done():
	case <-time.After(waitTimeout):
		t.Error("the silent connection was never closed")
	}
}

func TestListenerRun_FollowsPlannedReconnectWithoutResubscribing(t *testing.T) {
	fake := newFakeEventSub(t)
	subject := startListener(t, fake, newFakeAuthorization())
	firstSocket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	firstSocket.send(t, reconnectMessage(fake.webSocketURL()+"?reconnect=true"))
	secondSocket := fake.awaitSocket(t)

	select {
	case <-firstSocket.closedByClient.Done():
	case <-time.After(waitTimeout):
		t.Fatal("the old connection wasn't closed after the new one was welcomed")
	}
	secondSocket.send(t, chatNotification("after-reconnect"))
	if event := subject.awaitEvent(t); event.TriggerID != twitcheventsub.ChatMessageTriggerID {
		t.Errorf("got event for %q on the new connection", event.TriggerID)
	}
	if subscriptionCount := len(fake.recordedSubscriptions()); subscriptionCount != expectedSubscriptionCount {
		t.Errorf("got %d subscriptions, want %d: subscriptions carry over on a planned reconnect", subscriptionCount, expectedSubscriptionCount)
	}
}

func TestListenerRun_RenewsTokenOnceWhenTwitchRejectsIt(t *testing.T) {
	fake := newFakeEventSub(t)
	fake.rejectedTokens[testAccessToken] = true
	authorization := newFakeAuthorization()
	subject := startListener(t, fake, authorization)
	fake.awaitSocket(t)

	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	authorization.mutex.Lock()
	renewCount := authorization.renewCount
	authorization.mutex.Unlock()
	if renewCount != 1 {
		t.Errorf("renewed the token %d times, want 1", renewCount)
	}
	for _, subscription := range fake.recordedSubscriptions() {
		if subscription.accessToken != renewedAccessToken {
			t.Errorf("subscribed to %s with token %q, want the renewed token", subscription.Type, subscription.accessToken)
		}
	}
}

func TestListenerRun_LogsOutWhenAScopeIsMissing(t *testing.T) {
	fake := newFakeEventSub(t)
	fake.isScopeMissing = true
	authorization := newFakeAuthorization()
	subject := startListener(t, fake, authorization)
	fake.awaitSocket(t)

	err := subject.awaitRunResult(t)

	if !errors.Is(err, twitcheventsub.ErrAuthorizationLost) {
		t.Errorf("got error %v, want ErrAuthorizationLost", err)
	}
	select {
	case <-authorization.sessionEnded:
	default:
		t.Error("the user wasn't logged out")
	}
}

func TestListenerRun_LogsOutWhenTwitchRevokesAuthorization(t *testing.T) {
	fake := newFakeEventSub(t)
	authorization := newFakeAuthorization()
	subject := startListener(t, fake, authorization)
	socket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	socket.send(t, revocationMessage("authorization_revoked", chatMessageType, "1"))

	err := subject.awaitRunResult(t)
	if !errors.Is(err, twitcheventsub.ErrAuthorizationLost) {
		t.Errorf("got error %v, want ErrAuthorizationLost", err)
	}
	select {
	case <-authorization.sessionEnded:
	default:
		t.Error("the user wasn't logged out")
	}
}

func TestListenerRun_ReportsProblemWhenSubscriptionVersionIsRemoved(t *testing.T) {
	fake := newFakeEventSub(t)
	authorization := newFakeAuthorization()
	subject := startListener(t, fake, authorization)
	socket := fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	socket.send(t, revocationMessage("version_removed", followType, "2"))

	state := subject.awaitStatus(t, twitcheventsub.ConnectionStatusProblem)
	if len(state.Problems) != 1 || !strings.Contains(state.Problems[0], followType) {
		t.Errorf("got problems %v, want one about %s", state.Problems, followType)
	}
	select {
	case <-authorization.sessionEnded:
		t.Error("the user was logged out, but only one subscription stopped")
	default:
	}
}

func TestListenerRun_ReturnsNilWhenContextIsCancelled(t *testing.T) {
	fake := newFakeEventSub(t)
	subject := startListener(t, fake, newFakeAuthorization())
	fake.awaitSocket(t)
	subject.awaitStatus(t, twitcheventsub.ConnectionStatusConnected)

	subject.cancel()

	err := subject.awaitRunResult(t)
	if err != nil {
		t.Errorf("got error %v, want nil", err)
	}
}
