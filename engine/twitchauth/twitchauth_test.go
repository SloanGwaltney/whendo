package twitchauth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SloanGwaltney/whendo/engine/credentials"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
)

const (
	testClientID        = "test-client-id"
	testDeviceCode      = "test-device-code"
	testUserCode        = "ABCD-EFGH"
	testVerificationURI = "https://www.twitch.tv/activate?device-code=ABCD-EFGH"
	testAccountLogin    = "teststreamer"
	testAccountUserID   = "12345"
	stateChangeTimeout  = 10 * time.Second
)

type memoryCredentialStore struct {
	mutex   sync.Mutex
	secrets map[string]string
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{secrets: make(map[string]string)}
}

func (store *memoryCredentialStore) Get(name string) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secret, isStored := store.secrets[name]
	if !isStored {
		return "", credentials.ErrNotFound
	}
	return secret, nil
}

func (store *memoryCredentialStore) Set(name string, secret string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.secrets[name] = secret
	return nil
}

func (store *memoryCredentialStore) Delete(name string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.secrets, name)
	return nil
}

func (store *memoryCredentialStore) count() int {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return len(store.secrets)
}

// fakeTwitch imitates Twitch's OAuth server. Its fields control how it
// responds; set them before starting any requests.
type fakeTwitch struct {
	server *httptest.Server

	pendingPollCount       int
	deviceCodeExpirySecond int
	isRefreshRejected      bool
	validAccessToken       string

	pollCount     atomic.Int32
	refreshCount  atomic.Int32
	revokeCount   atomic.Int32
	refreshNumber atomic.Int32
}

func newFakeTwitch(t *testing.T) *fakeTwitch {
	t.Helper()
	twitch := &fakeTwitch{deviceCodeExpirySecond: 1800, validAccessToken: "access-token-0"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/device", twitch.handleDeviceCode)
	mux.HandleFunc("POST /oauth2/token", twitch.handleToken)
	mux.HandleFunc("GET /oauth2/validate", twitch.handleValidate)
	mux.HandleFunc("POST /oauth2/revoke", twitch.handleRevoke)
	twitch.server = httptest.NewServer(mux)
	t.Cleanup(twitch.server.Close)
	return twitch
}

func (twitch *fakeTwitch) client() *twitchauth.Client {
	return twitchauth.NewClient(testClientID, twitch.server.URL, twitch.server.Client())
}

func (twitch *fakeTwitch) handleDeviceCode(writer http.ResponseWriter, request *http.Request) {
	if request.FormValue("client_id") != testClientID {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"status": 400, "message": "invalid client"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"device_code":      testDeviceCode,
		"user_code":        testUserCode,
		"verification_uri": testVerificationURI,
		"expires_in":       twitch.deviceCodeExpirySecond,
		"interval":         1,
	})
}

func (twitch *fakeTwitch) handleToken(writer http.ResponseWriter, request *http.Request) {
	switch request.FormValue("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		pollNumber := int(twitch.pollCount.Add(1))
		if pollNumber <= twitch.pendingPollCount {
			writeJSON(writer, http.StatusBadRequest, map[string]any{"status": 400, "message": "authorization_pending"})
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{
			"access_token":  twitch.validAccessToken,
			"refresh_token": "refresh-token-0",
			"expires_in":    14400,
		})
	case "refresh_token":
		twitch.refreshCount.Add(1)
		if twitch.isRefreshRejected {
			writeJSON(writer, http.StatusBadRequest, map[string]any{"status": 400, "message": "Invalid refresh token"})
			return
		}
		// Give concurrent callers a chance to pile up behind this refresh.
		time.Sleep(50 * time.Millisecond)
		refreshNumber := twitch.refreshNumber.Add(1)
		writeJSON(writer, http.StatusOK, map[string]any{
			"access_token":  "refreshed-access-token-" + strconv.Itoa(int(refreshNumber)),
			"refresh_token": "refreshed-refresh-token-" + strconv.Itoa(int(refreshNumber)),
			"expires_in":    14400,
		})
	default:
		writeJSON(writer, http.StatusBadRequest, map[string]any{"status": 400, "message": "unsupported grant type"})
	}
}

func (twitch *fakeTwitch) handleValidate(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Authorization") != "OAuth "+twitch.validAccessToken {
		writeJSON(writer, http.StatusUnauthorized, map[string]any{"status": 401, "message": "invalid access token"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"client_id":  testClientID,
		"login":      testAccountLogin,
		"user_id":    testAccountUserID,
		"scopes":     []string{"user:read:chat"},
		"expires_in": 14000,
	})
}

func (twitch *fakeTwitch) handleRevoke(writer http.ResponseWriter, request *http.Request) {
	twitch.revokeCount.Add(1)
	writer.WriteHeader(http.StatusOK)
}

func writeJSON(writer http.ResponseWriter, statusCode int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	err := json.NewEncoder(writer).Encode(body)
	if err != nil {
		panic(err)
	}
}

type sessionUnderTest struct {
	session      *twitchauth.Session
	store        *memoryCredentialStore
	stateChanges chan twitchauth.LoginState
}

func newSessionUnderTest(twitch *fakeTwitch) sessionUnderTest {
	store := newMemoryCredentialStore()
	stateChanges := make(chan twitchauth.LoginState, 16)
	session := twitchauth.NewSession(twitchauth.SessionConfig{
		Client:          twitch.client(),
		CredentialStore: store,
	})
	session.AddStateListener(func(state twitchauth.LoginState) { stateChanges <- state })
	return sessionUnderTest{session: session, store: store, stateChanges: stateChanges}
}

func (subject sessionUnderTest) storeLogin(accessToken string) {
	mustSet(subject.store, "twitch-access-token", accessToken)
	mustSet(subject.store, "twitch-refresh-token", "stored-refresh-token")
	mustSet(subject.store, "twitch-account-login", testAccountLogin)
	mustSet(subject.store, "twitch-account-user-id", testAccountUserID)
}

func mustSet(store *memoryCredentialStore, name string, secret string) {
	err := store.Set(name, secret)
	if err != nil {
		panic(err)
	}
}

func waitForStatus(t *testing.T, stateChanges <-chan twitchauth.LoginState, wantedStatus twitchauth.LoginStatus) twitchauth.LoginState {
	t.Helper()
	timeout := time.After(stateChangeTimeout)
	for {
		select {
		case state := <-stateChanges:
			if state.Status == wantedStatus {
				return state
			}
		case <-timeout:
			t.Fatalf("timed out waiting for login status %q", wantedStatus)
		}
	}
}

func TestRequestDeviceCode_ReturnsCodeAndVerificationLink(t *testing.T) {
	twitch := newFakeTwitch(t)

	authorization, err := twitch.client().RequestDeviceCode(context.Background(), []string{"user:read:chat"})
	if err != nil {
		t.Fatalf("RequestDeviceCode returned an error: %v", err)
	}
	if authorization.UserCode != testUserCode || authorization.VerificationURI != testVerificationURI {
		t.Errorf("got code %q and link %q", authorization.UserCode, authorization.VerificationURI)
	}
	if authorization.PollInterval != time.Second {
		t.Errorf("got poll interval %v, want 1s", authorization.PollInterval)
	}
}

func TestPollForToken_KeepsPollingWhileAuthorizationIsPending(t *testing.T) {
	twitch := newFakeTwitch(t)
	twitch.pendingPollCount = 1
	client := twitch.client()
	authorization, err := client.RequestDeviceCode(context.Background(), nil)
	if err != nil {
		t.Fatalf("RequestDeviceCode returned an error: %v", err)
	}

	token, err := client.PollForToken(context.Background(), authorization, nil)
	if err != nil {
		t.Fatalf("PollForToken returned an error: %v", err)
	}
	if token.AccessToken != twitch.validAccessToken || token.RefreshToken != "refresh-token-0" {
		t.Errorf("got token %+v", token)
	}
	if twitch.pollCount.Load() != 2 {
		t.Errorf("polled %d times, want 2", twitch.pollCount.Load())
	}
}

func TestPollForToken_ReturnsErrDeviceCodeExpiredWhenNeverApproved(t *testing.T) {
	twitch := newFakeTwitch(t)
	twitch.pendingPollCount = 1000
	twitch.deviceCodeExpirySecond = 1
	client := twitch.client()
	authorization, err := client.RequestDeviceCode(context.Background(), nil)
	if err != nil {
		t.Fatalf("RequestDeviceCode returned an error: %v", err)
	}

	_, err = client.PollForToken(context.Background(), authorization, nil)
	if !errors.Is(err, twitchauth.ErrDeviceCodeExpired) {
		t.Errorf("got error %v, want ErrDeviceCodeExpired", err)
	}
}

func TestRefreshToken_ReturnsErrTokenRejectedWhenTwitchRejectsIt(t *testing.T) {
	twitch := newFakeTwitch(t)
	twitch.isRefreshRejected = true

	_, err := twitch.client().RefreshToken(context.Background(), "old-refresh-token")
	if !errors.Is(err, twitchauth.ErrTokenRejected) {
		t.Errorf("got error %v, want ErrTokenRejected", err)
	}
}

func TestValidateToken_ReturnsAccountForValidToken(t *testing.T) {
	twitch := newFakeTwitch(t)

	validation, err := twitch.client().ValidateToken(context.Background(), twitch.validAccessToken)
	if err != nil {
		t.Fatalf("ValidateToken returned an error: %v", err)
	}
	if validation.Login != testAccountLogin || validation.UserID != testAccountUserID {
		t.Errorf("got validation %+v", validation)
	}
}

func TestValidateToken_ReturnsErrTokenRejectedForInvalidToken(t *testing.T) {
	twitch := newFakeTwitch(t)

	_, err := twitch.client().ValidateToken(context.Background(), "not-a-valid-token")
	if !errors.Is(err, twitchauth.ErrTokenRejected) {
		t.Errorf("got error %v, want ErrTokenRejected", err)
	}
}

func TestSessionRestore_StaysLoggedOutWithoutStoredLogin(t *testing.T) {
	subject := newSessionUnderTest(newFakeTwitch(t))

	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}
	if subject.session.State().Status != twitchauth.LoginStatusLoggedOut {
		t.Errorf("got status %q, want logged out", subject.session.State().Status)
	}
}

func TestSessionRestore_LogsInWithStoredLogin(t *testing.T) {
	subject := newSessionUnderTest(newFakeTwitch(t))
	subject.storeLogin("stored-access-token")

	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}
	state := subject.session.State()
	if state.Status != twitchauth.LoginStatusLoggedIn || state.AccountLogin != testAccountLogin {
		t.Errorf("got state %+v", state)
	}
}

func TestSessionStartLogin_LogsInAndStoresTokensOnceApproved(t *testing.T) {
	twitch := newFakeTwitch(t)
	subject := newSessionUnderTest(twitch)

	waitingState, err := subject.session.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin returned an error: %v", err)
	}
	if waitingState.Status != twitchauth.LoginStatusWaitingForApproval || waitingState.UserCode != testUserCode {
		t.Errorf("got waiting state %+v", waitingState)
	}

	loggedInState := waitForStatus(t, subject.stateChanges, twitchauth.LoginStatusLoggedIn)
	if loggedInState.AccountLogin != testAccountLogin {
		t.Errorf("got account %q, want %q", loggedInState.AccountLogin, testAccountLogin)
	}
	storedAccessToken, err := subject.store.Get("twitch-access-token")
	if err != nil || storedAccessToken != twitch.validAccessToken {
		t.Errorf("got stored access token %q (error %v)", storedAccessToken, err)
	}
	accountUserID, err := subject.session.AccountUserID()
	if err != nil || accountUserID != testAccountUserID {
		t.Errorf("got account user ID %q (error %v)", accountUserID, err)
	}
}

func TestSessionCancelLogin_StopsWaitingForApproval(t *testing.T) {
	twitch := newFakeTwitch(t)
	twitch.pendingPollCount = 1000
	subject := newSessionUnderTest(twitch)
	_, err := subject.session.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin returned an error: %v", err)
	}

	subject.session.CancelLogin()

	state := subject.session.State()
	if state.Status != twitchauth.LoginStatusLoggedOut || state.LogoutReason != twitchauth.LogoutReasonNone {
		t.Errorf("got state %+v", state)
	}
}

func TestSessionStartLogin_ReportsExpiredCode(t *testing.T) {
	twitch := newFakeTwitch(t)
	twitch.pendingPollCount = 1000
	twitch.deviceCodeExpirySecond = 1
	subject := newSessionUnderTest(twitch)

	_, err := subject.session.StartLogin(context.Background())
	if err != nil {
		t.Fatalf("StartLogin returned an error: %v", err)
	}

	state := waitForStatus(t, subject.stateChanges, twitchauth.LoginStatusLoggedOut)
	if state.LogoutReason != twitchauth.LogoutReasonCodeExpired {
		t.Errorf("got logout reason %q, want code expired", state.LogoutReason)
	}
}

func TestSessionRenewAccessToken_MakesOneRefreshForConcurrentCallers(t *testing.T) {
	twitch := newFakeTwitch(t)
	subject := newSessionUnderTest(twitch)
	subject.storeLogin("rejected-access-token")
	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	const callerCount = 5
	renewedTokens := make(chan string, callerCount)
	var callers sync.WaitGroup
	for range callerCount {
		callers.Go(func() {
			renewedToken, renewError := subject.session.RenewAccessToken(context.Background(), "rejected-access-token")
			if renewError != nil {
				t.Errorf("RenewAccessToken returned an error: %v", renewError)
			}
			renewedTokens <- renewedToken
		})
	}
	callers.Wait()
	close(renewedTokens)

	if twitch.refreshCount.Load() != 1 {
		t.Errorf("refreshed %d times, want 1", twitch.refreshCount.Load())
	}
	for renewedToken := range renewedTokens {
		if renewedToken != "refreshed-access-token-1" {
			t.Errorf("got renewed token %q, want refreshed-access-token-1", renewedToken)
		}
	}
	storedRefreshToken, err := subject.store.Get("twitch-refresh-token")
	if err != nil || storedRefreshToken != "refreshed-refresh-token-1" {
		t.Errorf("got stored refresh token %q (error %v)", storedRefreshToken, err)
	}
}

func TestSessionRenewAccessToken_LogsOutWhenRefreshIsRejected(t *testing.T) {
	twitch := newFakeTwitch(t)
	twitch.isRefreshRejected = true
	subject := newSessionUnderTest(twitch)
	subject.storeLogin("rejected-access-token")
	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	_, err = subject.session.RenewAccessToken(context.Background(), "rejected-access-token")

	if !errors.Is(err, twitchauth.ErrNotLoggedIn) {
		t.Errorf("got error %v, want ErrNotLoggedIn", err)
	}
	state := subject.session.State()
	if state.Status != twitchauth.LoginStatusLoggedOut || state.LogoutReason != twitchauth.LogoutReasonSessionEnded {
		t.Errorf("got state %+v", state)
	}
	if subject.store.count() != 0 {
		t.Errorf("%d credentials still stored, want 0", subject.store.count())
	}
}

func TestSessionKeepValidated_RefreshesTokenTwitchRejects(t *testing.T) {
	twitch := newFakeTwitch(t)
	subject := newSessionUnderTest(twitch)
	subject.storeLogin("rejected-access-token")
	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go subject.session.KeepValidated(ctx)

	deadline := time.Now().Add(stateChangeTimeout)
	for twitch.refreshCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if twitch.refreshCount.Load() != 1 {
		t.Errorf("refreshed %d times, want 1", twitch.refreshCount.Load())
	}
}

func TestSessionLogout_ForgetsLoginAndRevokesToken(t *testing.T) {
	twitch := newFakeTwitch(t)
	subject := newSessionUnderTest(twitch)
	subject.storeLogin("stored-access-token")
	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	err = subject.session.Logout(context.Background())
	if err != nil {
		t.Fatalf("Logout returned an error: %v", err)
	}

	if subject.session.State().Status != twitchauth.LoginStatusLoggedOut {
		t.Errorf("got status %q, want logged out", subject.session.State().Status)
	}
	if subject.store.count() != 0 {
		t.Errorf("%d credentials still stored, want 0", subject.store.count())
	}
	if twitch.revokeCount.Load() != 1 {
		t.Errorf("revoked %d times, want 1", twitch.revokeCount.Load())
	}
	_, err = subject.session.AccessToken(context.Background())
	if !errors.Is(err, twitchauth.ErrNotLoggedIn) {
		t.Errorf("got error %v, want ErrNotLoggedIn", err)
	}
}

func TestSessionAccessToken_ReturnsRestoredTokenWithoutRefreshing(t *testing.T) {
	twitch := newFakeTwitch(t)
	subject := newSessionUnderTest(twitch)
	subject.storeLogin("stored-access-token")
	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	accessToken, err := subject.session.AccessToken(context.Background())

	if err != nil || accessToken != "stored-access-token" {
		t.Errorf("got token %q (error %v), want the stored token", accessToken, err)
	}
	if twitch.refreshCount.Load() != 0 {
		t.Errorf("refreshed %d times, want 0", twitch.refreshCount.Load())
	}
}

func TestSessionEndSessionRevokedByTwitch_LogsOutAndForgetsLogin(t *testing.T) {
	twitch := newFakeTwitch(t)
	subject := newSessionUnderTest(twitch)
	subject.storeLogin("stored-access-token")
	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	subject.session.EndSessionRevokedByTwitch()

	state := subject.session.State()
	if state.Status != twitchauth.LoginStatusLoggedOut || state.LogoutReason != twitchauth.LogoutReasonSessionEnded {
		t.Errorf("got state %+v", state)
	}
	if subject.store.count() != 0 {
		t.Errorf("%d credentials still stored, want 0", subject.store.count())
	}
	if twitch.revokeCount.Load() != 0 {
		t.Errorf("revoked %d times; a token Twitch already revoked shouldn't be revoked again", twitch.revokeCount.Load())
	}
}

func TestSessionAddStateListener_NotifiesEveryListener(t *testing.T) {
	subject := newSessionUnderTest(newFakeTwitch(t))
	var secondListenerStates []twitchauth.LoginState
	subject.session.AddStateListener(func(state twitchauth.LoginState) {
		secondListenerStates = append(secondListenerStates, state)
	})
	subject.storeLogin("stored-access-token")

	err := subject.session.Restore()
	if err != nil {
		t.Fatalf("Restore returned an error: %v", err)
	}

	firstListenerState := waitForStatus(t, subject.stateChanges, twitchauth.LoginStatusLoggedIn)
	if firstListenerState.AccountLogin != testAccountLogin {
		t.Errorf("first listener got state %+v", firstListenerState)
	}
	if len(secondListenerStates) != 1 || secondListenerStates[0].Status != twitchauth.LoginStatusLoggedIn {
		t.Errorf("second listener got states %+v", secondListenerStates)
	}
}
