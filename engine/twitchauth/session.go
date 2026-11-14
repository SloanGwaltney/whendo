package twitchauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/SloanGwaltney/whendo/engine/credentials"
)

// LoginStatus says whether the user is logged in to Twitch.
type LoginStatus string

const (
	LoginStatusLoggedOut          LoginStatus = "loggedOut"
	LoginStatusWaitingForApproval LoginStatus = "waitingForApproval"
	LoginStatusLoggedIn           LoginStatus = "loggedIn"
)

// LogoutReason says why the user is logged out, so the UI can explain it.
type LogoutReason string

const (
	// LogoutReasonNone means the user never logged in, logged out themselves
	// or cancelled a login.
	LogoutReasonNone LogoutReason = ""
	// LogoutReasonCodeExpired means the user didn't approve the login in time.
	LogoutReasonCodeExpired LogoutReason = "codeExpired"
	// LogoutReasonSessionEnded means Twitch stopped accepting the user's
	// login, for example because it went unused for 30 days or the user
	// disconnected Whendo in Twitch. This is normal, not an error.
	LogoutReasonSessionEnded LogoutReason = "sessionEnded"
	// LogoutReasonLoginFailed means something went wrong during login.
	// LoginState.LoginErrorMessage has the details.
	LogoutReasonLoginFailed LogoutReason = "loginFailed"
)

// LoginState is everything the UI needs to show the user's Twitch login.
type LoginState struct {
	Status LoginStatus `json:"status"`
	// UserCode and VerificationURI are set while waiting for approval.
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	// AccountLogin is the Twitch username, set while logged in.
	AccountLogin string `json:"accountLogin"`
	// LogoutReason and LoginErrorMessage are set while logged out.
	LogoutReason      LogoutReason `json:"logoutReason"`
	LoginErrorMessage string       `json:"loginErrorMessage"`
}

var (
	// ErrNotLoggedIn is returned when an action needs a Twitch login and
	// there isn't one.
	ErrNotLoggedIn = errors.New("not logged in to Twitch")
	// ErrAlreadyLoggedIn is returned when starting a login while logged in.
	// Log out first to log in with a different account.
	ErrAlreadyLoggedIn = errors.New("already logged in to Twitch")
)

// requiredScopes lists every scope the MVP needs. They're all requested at
// first login because adding scopes later forces the user to log in again.
var requiredScopes = []string{
	"channel:read:redemptions",
	"channel:manage:redemptions",
	"user:read:chat",
	"user:write:chat",
	"moderator:read:followers",
}

// Each secret is its own credential because Windows Credential Manager limits
// the size of a single one. The account's login and user ID aren't secret,
// but they're kept with the tokens so the stored login is all in one place.
const (
	accessTokenCredentialName   = "twitch-access-token"
	refreshTokenCredentialName  = "twitch-refresh-token"
	accountLoginCredentialName  = "twitch-account-login"
	accountUserIDCredentialName = "twitch-account-user-id"
)

var storedLoginCredentialNames = []string{
	accessTokenCredentialName,
	refreshTokenCredentialName,
	accountLoginCredentialName,
	accountUserIDCredentialName,
}

const (
	// Twitch requires apps to validate their token every hour.
	validationInterval = time.Hour
	// When validation fails for a reason other than Twitch rejecting the
	// token, such as being offline, retry sooner than the hourly check.
	validationRetryInterval = time.Minute
	// Refresh slightly before the access token expires, so a token handed out
	// doesn't expire while the caller is using it.
	accessTokenRefreshMargin = 5 * time.Minute
)

// SessionConfig holds what a Session needs.
type SessionConfig struct {
	Client          *Client
	CredentialStore credentials.Store
	Logger          *slog.Logger
	// OnStateChange, if set, is called with the new state each time it
	// changes. It's called while the Session is locked, so it must not call
	// back into the Session.
	OnStateChange func(LoginState)
}

// Session is the user's Twitch login. It runs the login flow, stores the
// tokens and hands out a valid access token to the rest of the engine.
//
// Each refresh token works only once, so a Session makes sure only one
// refresh runs at a time and shares its result with every caller.
type Session struct {
	client          *Client
	credentialStore credentials.Store
	logger          *slog.Logger
	onStateChange   func(LoginState)

	mutex         sync.Mutex
	state         LoginState
	token         Token
	accountUserID string
	// generation increases each time the login status changes. Work that
	// started under an earlier generation, such as a refresh or a login
	// attempt that was cancelled, discards its result instead of applying it
	// to a session that no longer exists.
	generation         int
	cancelLoginAttempt context.CancelFunc

	refreshMutex sync.Mutex
}

// NewSession returns a logged-out Session. Call Restore to load a login
// stored by an earlier run.
func NewSession(config SessionConfig) *Session {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Session{
		client:          config.Client,
		credentialStore: config.CredentialStore,
		logger:          logger,
		onStateChange:   config.OnStateChange,
		state:           LoginState{Status: LoginStatusLoggedOut},
	}
}

// State returns the current login state.
func (session *Session) State() LoginState {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return session.state
}

// Restore loads the login stored by an earlier run, if there is one. It
// doesn't contact Twitch; KeepValidated checks the login when it starts.
func (session *Session) Restore() error {
	storedValues := make(map[string]string, len(storedLoginCredentialNames))
	for _, credentialName := range storedLoginCredentialNames {
		value, err := session.credentialStore.Get(credentialName)
		if errors.Is(err, credentials.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("loading the stored Twitch login: %w", err)
		}
		storedValues[credentialName] = value
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.generation++
	// The expiry time isn't stored, so it stays unknown until the first
	// validation reports it.
	session.token = Token{
		AccessToken:  storedValues[accessTokenCredentialName],
		RefreshToken: storedValues[refreshTokenCredentialName],
	}
	session.accountUserID = storedValues[accountUserIDCredentialName]
	session.setState(LoginState{
		Status:       LoginStatusLoggedIn,
		AccountLogin: storedValues[accountLoginCredentialName],
	})
	return nil
}

// AccountUserID returns the logged-in Twitch account's user ID, which Twitch's
// APIs use to identify the broadcaster.
func (session *Session) AccountUserID() (string, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.state.Status != LoginStatusLoggedIn {
		return "", ErrNotLoggedIn
	}
	return session.accountUserID, nil
}

// StartLogin requests a login code from Twitch and waits in the background
// for the user to approve it. The returned state holds the code and the link
// to show the user. Later changes are reported through OnStateChange.
func (session *Session) StartLogin(ctx context.Context) (LoginState, error) {
	if session.State().Status == LoginStatusLoggedIn {
		return LoginState{}, ErrAlreadyLoggedIn
	}

	authorization, err := session.client.RequestDeviceCode(ctx, requiredScopes)
	if err != nil {
		return LoginState{}, fmt.Errorf("starting the Twitch login: %w", err)
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.state.Status == LoginStatusLoggedIn {
		return LoginState{}, ErrAlreadyLoggedIn
	}
	session.stopLoginAttempt()
	session.generation++
	// The login outlives the call that started it, so it isn't cancelled
	// with ctx. CancelLogin and Logout stop it instead.
	attemptContext, cancelAttempt := context.WithCancel(context.WithoutCancel(ctx))
	session.cancelLoginAttempt = cancelAttempt
	session.setState(LoginState{
		Status:          LoginStatusWaitingForApproval,
		UserCode:        authorization.UserCode,
		VerificationURI: authorization.VerificationURI,
	})
	go session.completeLogin(attemptContext, session.generation, authorization)
	return session.state, nil
}

// CancelLogin stops waiting for the user to approve a login.
func (session *Session) CancelLogin() {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.state.Status != LoginStatusWaitingForApproval {
		return
	}
	session.stopLoginAttempt()
	session.generation++
	session.setState(LoginState{Status: LoginStatusLoggedOut})
}

// Logout forgets the stored login and tells Twitch to disconnect Whendo.
// The user is logged out even if Twitch can't be reached.
func (session *Session) Logout(ctx context.Context) error {
	session.mutex.Lock()
	session.stopLoginAttempt()
	wasLoggedIn := session.state.Status == LoginStatusLoggedIn
	accessToken := session.token.AccessToken
	clearError := session.endSession(LogoutReasonNone)
	session.mutex.Unlock()

	if wasLoggedIn {
		revokeError := session.client.RevokeToken(ctx, accessToken)
		if revokeError != nil {
			// The token is already forgotten locally and expires on its own,
			// so failing to revoke it doesn't keep the user logged in.
			session.logger.Warn("Twitch may still list Whendo as connected after logout", "error", revokeError)
		}
	}
	if clearError != nil {
		return fmt.Errorf("logging out of Twitch: %w", clearError)
	}
	return nil
}

// AccessToken returns a valid access token, refreshing it first if it's
// about to expire.
func (session *Session) AccessToken(ctx context.Context) (string, error) {
	session.mutex.Lock()
	isLoggedIn := session.state.Status == LoginStatusLoggedIn
	token := session.token
	session.mutex.Unlock()

	if !isLoggedIn {
		return "", ErrNotLoggedIn
	}
	if time.Until(token.ExpiresAt) > accessTokenRefreshMargin {
		return token.AccessToken, nil
	}
	return session.refresh(ctx, token.AccessToken)
}

// RenewAccessToken returns a new access token to replace one that Twitch
// rejected. When several callers report the same rejected token, only one
// refresh is made and they all receive its result. If Twitch rejects the
// refresh too, the user is logged out and ErrNotLoggedIn is returned.
func (session *Session) RenewAccessToken(ctx context.Context, rejectedAccessToken string) (string, error) {
	return session.refresh(ctx, rejectedAccessToken)
}

// KeepValidated checks the login with Twitch immediately and then every hour,
// as Twitch requires, until ctx is cancelled. A login Twitch no longer
// accepts is refreshed, or logged out if refreshing fails.
func (session *Session) KeepValidated(ctx context.Context) {
	for {
		delayUntilNextValidation := validationInterval
		err := session.validate(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			session.logger.Warn("couldn't validate the Twitch login; retrying soon", "error", err)
			delayUntilNextValidation = validationRetryInterval
		}
		err = waitFor(ctx, delayUntilNextValidation)
		if err != nil {
			return
		}
	}
}

func (session *Session) completeLogin(ctx context.Context, attemptGeneration int, authorization DeviceAuthorization) {
	token, err := session.client.PollForToken(ctx, authorization, requiredScopes)
	if errors.Is(err, context.Canceled) {
		return
	}
	if errors.Is(err, ErrDeviceCodeExpired) {
		session.failLoginAttempt(attemptGeneration, LogoutReasonCodeExpired, nil)
		return
	}
	if err != nil {
		session.failLoginAttempt(attemptGeneration, LogoutReasonLoginFailed, err)
		return
	}

	validation, err := session.client.ValidateToken(ctx, token.AccessToken)
	if err != nil {
		session.failLoginAttempt(attemptGeneration, LogoutReasonLoginFailed, err)
		return
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.generation != attemptGeneration {
		return
	}
	err = session.storeLogin(token, validation)
	if err != nil {
		session.logger.Error("couldn't save the Twitch login", "error", err)
		clearError := session.clearStoredLogin()
		if clearError != nil {
			session.logger.Error("couldn't remove the partly saved Twitch login", "error", clearError)
		}
		session.cancelLoginAttempt = nil
		session.generation++
		session.setState(LoginState{
			Status:            LoginStatusLoggedOut,
			LogoutReason:      LogoutReasonLoginFailed,
			LoginErrorMessage: err.Error(),
		})
		return
	}
	session.cancelLoginAttempt = nil
	session.generation++
	session.token = token
	session.accountUserID = validation.UserID
	session.setState(LoginState{
		Status:       LoginStatusLoggedIn,
		AccountLogin: validation.Login,
	})
}

func (session *Session) failLoginAttempt(attemptGeneration int, reason LogoutReason, loginError error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.generation != attemptGeneration {
		return
	}
	failedState := LoginState{Status: LoginStatusLoggedOut, LogoutReason: reason}
	if loginError != nil {
		session.logger.Error("Twitch login failed", "error", loginError)
		failedState.LoginErrorMessage = loginError.Error()
	}
	session.cancelLoginAttempt = nil
	session.generation++
	session.setState(failedState)
}

func (session *Session) refresh(ctx context.Context, staleAccessToken string) (string, error) {
	session.refreshMutex.Lock()
	defer session.refreshMutex.Unlock()

	session.mutex.Lock()
	isLoggedIn := session.state.Status == LoginStatusLoggedIn
	currentToken := session.token
	refreshGeneration := session.generation
	session.mutex.Unlock()

	if !isLoggedIn {
		return "", ErrNotLoggedIn
	}
	if currentToken.AccessToken != staleAccessToken {
		// Another caller refreshed while this one waited.
		return currentToken.AccessToken, nil
	}

	newToken, err := session.client.RefreshToken(ctx, currentToken.RefreshToken)
	if errors.Is(err, ErrTokenRejected) {
		session.endSessionRejectedByTwitch(refreshGeneration, err)
		return "", fmt.Errorf("%w: %w", ErrNotLoggedIn, err)
	}
	if err != nil {
		return "", err
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.generation != refreshGeneration {
		// The user logged out while the refresh was running.
		return "", ErrNotLoggedIn
	}
	session.token = newToken
	err = session.storeToken(newToken)
	if err != nil {
		// The old refresh token no longer works, so the new one is kept in
		// memory. The user stays logged in until Whendo closes.
		session.logger.Error("couldn't save the refreshed Twitch token; the login will be lost when Whendo closes", "error", err)
	}
	return newToken.AccessToken, nil
}

func (session *Session) validate(ctx context.Context) error {
	session.mutex.Lock()
	isLoggedIn := session.state.Status == LoginStatusLoggedIn
	validatedToken := session.token
	validationGeneration := session.generation
	session.mutex.Unlock()

	if !isLoggedIn {
		return nil
	}

	validation, err := session.client.ValidateToken(ctx, validatedToken.AccessToken)
	if errors.Is(err, ErrTokenRejected) {
		_, err = session.refresh(ctx, validatedToken.AccessToken)
		if errors.Is(err, ErrNotLoggedIn) {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.generation != validationGeneration || session.token.AccessToken != validatedToken.AccessToken {
		return nil
	}
	session.token.ExpiresAt = time.Now().Add(validation.ExpiresIn)
	if validation.Login == session.state.AccountLogin {
		return nil
	}
	// The user renamed their Twitch account since the login was stored.
	updatedState := session.state
	updatedState.AccountLogin = validation.Login
	session.setState(updatedState)
	err = session.credentialStore.Set(accountLoginCredentialName, validation.Login)
	if err != nil {
		return fmt.Errorf("saving the renamed Twitch account: %w", err)
	}
	return nil
}

func (session *Session) endSessionRejectedByTwitch(sessionGeneration int, rejection error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.generation != sessionGeneration {
		return
	}
	session.logger.Info("Twitch ended the login; the user needs to log in again", "reason", rejection)
	err := session.endSession(LogoutReasonSessionEnded)
	if err != nil {
		session.logger.Error("couldn't remove the ended Twitch login", "error", err)
	}
}

// endSession must be called with the mutex held. The session ends even if
// the stored login can't be removed; the error is returned so it's reported.
func (session *Session) endSession(reason LogoutReason) error {
	session.generation++
	session.token = Token{}
	session.accountUserID = ""
	session.setState(LoginState{Status: LoginStatusLoggedOut, LogoutReason: reason})
	return session.clearStoredLogin()
}

// stopLoginAttempt must be called with the mutex held.
func (session *Session) stopLoginAttempt() {
	if session.cancelLoginAttempt == nil {
		return
	}
	session.cancelLoginAttempt()
	session.cancelLoginAttempt = nil
}

// setState must be called with the mutex held.
func (session *Session) setState(state LoginState) {
	session.state = state
	if session.onStateChange != nil {
		session.onStateChange(state)
	}
}

func (session *Session) storeLogin(token Token, validation TokenValidation) error {
	err := session.storeToken(token)
	if err != nil {
		return err
	}
	err = session.credentialStore.Set(accountLoginCredentialName, validation.Login)
	if err != nil {
		return fmt.Errorf("saving the Twitch account: %w", err)
	}
	err = session.credentialStore.Set(accountUserIDCredentialName, validation.UserID)
	if err != nil {
		return fmt.Errorf("saving the Twitch account: %w", err)
	}
	return nil
}

func (session *Session) storeToken(token Token) error {
	// The refresh token is saved first because the old one stops working as
	// soon as Twitch issues a new one.
	err := session.credentialStore.Set(refreshTokenCredentialName, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("saving the Twitch refresh token: %w", err)
	}
	err = session.credentialStore.Set(accessTokenCredentialName, token.AccessToken)
	if err != nil {
		return fmt.Errorf("saving the Twitch access token: %w", err)
	}
	return nil
}

func (session *Session) clearStoredLogin() error {
	var deleteErrors []error
	for _, credentialName := range storedLoginCredentialNames {
		err := session.credentialStore.Delete(credentialName)
		if err != nil {
			deleteErrors = append(deleteErrors, err)
		}
	}
	return errors.Join(deleteErrors...)
}
