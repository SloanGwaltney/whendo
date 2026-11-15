package twitcheventsub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/SloanGwaltney/whendo/engine/trigger"
	"github.com/SloanGwaltney/whendo/engine/twitchauth"
)

const (
	// DefaultWebSocketURL is Twitch's EventSub WebSocket server.
	DefaultWebSocketURL = "wss://eventsub.wss.twitch.tv/ws"
	// DefaultHelixBaseURL is the base URL of Twitch's API.
	DefaultHelixBaseURL = "https://api.twitch.tv/helix"

	// The minimum Twitch allows, so a dead connection is noticed in seconds.
	keepaliveTimeoutSeconds        = 10
	keepaliveTimeoutQueryParameter = "keepalive_timeout_seconds"

	// After an unexpected drop, the wait before reconnecting starts here and
	// doubles after each failure, up to the maximum.
	initialRetryDelay = time.Second
	maximumRetryDelay = time.Minute

	// Notifications seen within this window are ignored if Twitch delivers
	// them again, which can happen while two connections overlap during a
	// planned reconnect.
	duplicateNotificationWindow = 10 * time.Minute
)

// ErrAuthorizationLost means Twitch no longer accepts Whendo's access and
// the user has been logged out, so the listener stopped.
var ErrAuthorizationLost = errors.New("twitch no longer accepts Whendo's access")

var errKeepaliveTimeout = errors.New("twitch sent nothing within the keepalive timeout")

// ConnectionStatus says whether Whendo is receiving events from Twitch.
type ConnectionStatus string

const (
	ConnectionStatusReconnecting ConnectionStatus = "reconnecting"
	ConnectionStatusConnected    ConnectionStatus = "connected"
	// ConnectionStatusProblem means Whendo is connected, but some events
	// can't be received. ConnectionState.Problems says which.
	ConnectionStatusProblem ConnectionStatus = "problem"
)

// ConnectionState is everything the UI needs to show the connection status.
type ConnectionState struct {
	Status   ConnectionStatus `json:"status"`
	Problems []string         `json:"problems"`
}

// ListenerConfig holds what a Listener needs.
type ListenerConfig struct {
	ClientID string
	// Session is the Twitch login the listener subscribes with. The listener
	// logs it out when Twitch revokes Whendo's access.
	Session  *twitchauth.Session
	Receiver trigger.Receiver
	Logger   *slog.Logger
	// OnStateChange, if set, is called with the new state each time it
	// changes.
	OnStateChange func(ConnectionState)
	// WebSocketURL and HelixBaseURL default to Twitch's servers. Tests point
	// them at fakes.
	WebSocketURL string
	HelixBaseURL string
	HTTPClient   *http.Client
}

// Listener keeps a connection to Twitch's EventSub open and delivers the
// events it receives as triggers.
type Listener struct {
	clientID      string
	session       *twitchauth.Session
	receiver      trigger.Receiver
	logger        *slog.Logger
	onStateChange func(ConnectionState)
	webSocketURL  string
	helixBaseURL  string
	httpClient    *http.Client

	stateMutex sync.Mutex
	state      ConnectionState

	// Only Run's goroutine touches these, so they need no lock.
	recentNotificationTimes map[string]time.Time
	sessionProblems         []string
}

// NewListener returns a Listener. Call Run to connect.
func NewListener(config ListenerConfig) *Listener {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	webSocketURL := config.WebSocketURL
	if webSocketURL == "" {
		webSocketURL = DefaultWebSocketURL
	}
	helixBaseURL := config.HelixBaseURL
	if helixBaseURL == "" {
		helixBaseURL = DefaultHelixBaseURL
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Listener{
		clientID:                config.ClientID,
		session:                 config.Session,
		receiver:                config.Receiver,
		logger:                  logger,
		onStateChange:           config.OnStateChange,
		webSocketURL:            webSocketURL,
		helixBaseURL:            strings.TrimSuffix(helixBaseURL, "/"),
		httpClient:              httpClient,
		state:                   ConnectionState{Status: ConnectionStatusReconnecting},
		recentNotificationTimes: make(map[string]time.Time),
	}
}

// State returns the current connection state.
func (listener *Listener) State() ConnectionState {
	listener.stateMutex.Lock()
	defer listener.stateMutex.Unlock()
	return listener.state
}

// Run stays connected to Twitch and delivers events until ctx is cancelled,
// reconnecting whenever the connection drops. It returns nil when ctx is
// cancelled, and ErrAuthorizationLost when Twitch no longer accepts Whendo's
// access. Run must not be called again until the previous call returns.
func (listener *Listener) Run(ctx context.Context) error {
	connectURL := listener.webSocketURL + "?" + keepaliveTimeoutQueryParameter + "=" + strconv.Itoa(keepaliveTimeoutSeconds)
	retryDelay := initialRetryDelay
	for {
		listener.setState(ConnectionState{Status: ConnectionStatusReconnecting})
		wasEstablished, err := listener.runSession(ctx, connectURL)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, ErrAuthorizationLost) {
			return err
		}
		listener.logDisconnect(err)
		if wasEstablished {
			retryDelay = initialRetryDelay
		}
		err = waitFor(ctx, retryDelay)
		if err != nil {
			return nil
		}
		retryDelay = min(retryDelay*2, maximumRetryDelay)
	}
}

// RunWhileLoggedIn runs the listener whenever the session is logged in, and
// stops it when the user logs out, until ctx is cancelled. Call it once.
func (listener *Listener) RunWhileLoggedIn(ctx context.Context) {
	loginChanges := make(chan struct{}, 1)
	listener.session.AddStateListener(func(twitchauth.LoginState) {
		// The session calls this while locked, so it only signals. The loop
		// below reads the state itself.
		select {
		case loginChanges <- struct{}{}:
		default:
		}
	})

	var running *runningListener
	defer func() {
		if running != nil {
			running.stop()
		}
	}()
	for {
		isLoggedIn := listener.session.State().Status == twitchauth.LoginStatusLoggedIn
		if isLoggedIn && running == nil {
			running = listener.startRunning(ctx)
		}
		if !isLoggedIn && running != nil {
			running.stop()
			running = nil
		}

		// finished is nil while nothing is running, and receiving from a nil
		// channel never succeeds.
		var finished <-chan struct{}
		if running != nil {
			finished = running.finished
		}
		select {
		case <-ctx.Done():
			return
		case <-loginChanges:
		case <-finished:
			// Run stops by itself only when Twitch revoked access, which also
			// logged the user out, so it isn't restarted until the next login.
			running.stop()
			running = nil
		}
	}
}

// runningListener is one call to Run in the background.
type runningListener struct {
	cancel   context.CancelFunc
	finished chan struct{}
}

func (listener *Listener) startRunning(ctx context.Context) *runningListener {
	runContext, cancel := context.WithCancel(ctx)
	running := &runningListener{cancel: cancel, finished: make(chan struct{})}
	go func() {
		defer close(running.finished)
		err := listener.Run(runContext)
		if err != nil {
			listener.logger.Info("stopped receiving Twitch events", "reason", err)
		}
	}()
	return running
}

// stop cancels Run and waits for it to return, so two calls to Run never
// overlap.
func (running *runningListener) stop() {
	running.cancel()
	<-running.finished
}

// runSession connects, subscribes and receives events until the connection
// drops. It reports whether the session got as far as subscribing.
func (listener *Listener) runSession(ctx context.Context, connectURL string) (bool, error) {
	// Cancelling the session's context closes every connection it opened,
	// including one opened for a planned reconnect that never completed.
	sessionContext, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	currentConnection, err := openConnection(sessionContext, connectURL, listener.httpClient, listener.logger)
	if err != nil {
		return false, err
	}
	problems, err := listener.subscribeToAll(sessionContext, currentConnection.sessionID)
	if err != nil {
		return false, err
	}
	listener.sessionProblems = problems
	listener.publishSessionState()
	return true, listener.receiveUntilDisconnected(sessionContext, currentConnection)
}

type handoverResult struct {
	newConnection *connection
	err           error
}

// receiveUntilDisconnected handles messages until the connection drops,
// including moving to a new connection when Twitch asks for a planned
// reconnect.
func (listener *Listener) receiveUntilDisconnected(ctx context.Context, currentConnection *connection) error {
	keepaliveTimer := time.NewTimer(currentConnection.keepaliveDeadline())
	defer keepaliveTimer.Stop()
	// handoverResults is nil until Twitch asks for a planned reconnect, and
	// receiving from a nil channel never succeeds.
	var handoverResults chan handoverResult

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-keepaliveTimer.C:
			currentConnection.close()
			return errKeepaliveTimeout
		case message, isOpen := <-currentConnection.messages:
			if !isOpen {
				return currentConnection.readError
			}
			keepaliveTimer.Reset(currentConnection.keepaliveDeadline())
			reconnectURL, err := listener.handleMessage(message)
			if err != nil {
				currentConnection.close()
				return err
			}
			if reconnectURL != "" && handoverResults == nil {
				handoverResults = listener.startHandover(ctx, reconnectURL)
			}
		case result := <-handoverResults:
			handoverResults = nil
			if result.err != nil {
				// The old connection keeps working until Twitch closes it, and
				// then Whendo reconnects from the start.
				listener.logger.Warn("couldn't complete Twitch's planned reconnect", "error", result.err)
				continue
			}
			// Subscriptions carry over to the new connection, so there's
			// nothing to recreate.
			err := listener.drainClosedConnection(currentConnection)
			currentConnection = result.newConnection
			if err != nil {
				currentConnection.close()
				return err
			}
			keepaliveTimer.Reset(currentConnection.keepaliveDeadline())
		}
	}
}

// drainClosedConnection closes a connection being replaced and handles the
// messages it had already received, so no event is lost in the handover.
// Events also delivered on the new connection are caught as duplicates.
func (listener *Listener) drainClosedConnection(oldConnection *connection) error {
	oldConnection.close()
	for message := range oldConnection.messages {
		// A reconnect request on the old connection is already being acted on.
		_, err := listener.handleMessage(message)
		if err != nil {
			return err
		}
	}
	return nil
}

// startHandover opens the connection for a planned reconnect in the
// background, so events keep arriving on the old connection meanwhile.
func (listener *Listener) startHandover(ctx context.Context, reconnectURL string) chan handoverResult {
	results := make(chan handoverResult, 1)
	go func() {
		newConnection, err := openConnection(ctx, reconnectURL, listener.httpClient, listener.logger)
		results <- handoverResult{newConnection: newConnection, err: err}
	}()
	return results
}

// handleMessage acts on one message. It returns the URL to move to when
// Twitch asks for a planned reconnect, and ErrAuthorizationLost when Twitch
// revoked Whendo's access.
func (listener *Listener) handleMessage(message incomingMessage) (string, error) {
	switch message.Metadata.MessageType {
	case keepaliveMessageType:
		return "", nil
	case notificationMessageType:
		listener.handleNotification(message)
		return "", nil
	case reconnectMessageType:
		return message.Payload.Session.ReconnectURL, nil
	case revocationMessageType:
		return "", listener.handleRevocation(message.Payload.Subscription)
	default:
		listener.logger.Warn("ignoring an unexpected EventSub message", "type", message.Metadata.MessageType)
		return "", nil
	}
}

func (listener *Listener) handleNotification(message incomingMessage) {
	if listener.isDuplicateNotification(message.Metadata.MessageID) {
		return
	}
	subscription := message.Payload.Subscription
	event, shouldTrigger, err := EventFromNotification(subscription.Type, subscription.Version, message.Payload.Event)
	if err != nil {
		listener.logger.Error("couldn't read a Twitch event", "type", subscription.Type, "version", subscription.Version, "error", err)
		return
	}
	if !shouldTrigger {
		return
	}
	listener.receiver.ReceiveEvent(event)
}

// isDuplicateNotification reports whether a notification was already seen
// recently, and remembers it if not.
func (listener *Listener) isDuplicateNotification(messageID string) bool {
	now := time.Now()
	for seenMessageID, seenAt := range listener.recentNotificationTimes {
		if now.Sub(seenAt) > duplicateNotificationWindow {
			delete(listener.recentNotificationTimes, seenMessageID)
		}
	}
	_, isDuplicate := listener.recentNotificationTimes[messageID]
	if isDuplicate {
		return true
	}
	listener.recentNotificationTimes[messageID] = now
	return false
}

func (listener *Listener) handleRevocation(subscription subscriptionPayload) error {
	switch subscription.Status {
	case authorizationRevokedReason, userRemovedReason:
		listener.logger.Info("Twitch revoked Whendo's access; logging out", "reason", subscription.Status)
		listener.session.EndSessionRevokedByTwitch()
		return ErrAuthorizationLost
	case versionRemovedReason:
		// Only an update to Whendo can fix this, so it's logged as an error
		// in the hope that someone reports it.
		listener.logger.Error("Twitch retired an EventSub subscription version Whendo uses; Whendo needs an update", "type", subscription.Type, "version", subscription.Version)
		listener.addSessionProblem(fmt.Sprintf("this version of Whendo can no longer receive %s events from Twitch; please update Whendo", subscription.Type))
		return nil
	default:
		listener.logger.Error("Twitch revoked an EventSub subscription", "type", subscription.Type, "reason", subscription.Status)
		listener.addSessionProblem(fmt.Sprintf("Twitch stopped sending %s events (%s)", subscription.Type, subscription.Status))
		return nil
	}
}

func (listener *Listener) addSessionProblem(problem string) {
	listener.sessionProblems = append(listener.sessionProblems, problem)
	listener.publishSessionState()
}

func (listener *Listener) publishSessionState() {
	if len(listener.sessionProblems) == 0 {
		listener.setState(ConnectionState{Status: ConnectionStatusConnected})
		return
	}
	problems := make([]string, len(listener.sessionProblems))
	copy(problems, listener.sessionProblems)
	listener.setState(ConnectionState{Status: ConnectionStatusProblem, Problems: problems})
}

func (listener *Listener) setState(state ConnectionState) {
	listener.stateMutex.Lock()
	listener.state = state
	listener.stateMutex.Unlock()
	if listener.onStateChange != nil {
		listener.onStateChange(state)
	}
}

// logDisconnect logs why a connection ended. Close codes that can only mean
// a bug in Whendo are logged as errors so they get noticed.
func (listener *Listener) logDisconnect(err error) {
	closeCode := websocket.CloseStatus(err)
	if isCloseCodeCausedByWhendo(closeCode) {
		listener.logger.Error("Twitch closed the EventSub connection because of a bug in Whendo; reconnecting", "closeCode", int(closeCode), "error", err)
		return
	}
	listener.logger.Warn("the EventSub connection ended; reconnecting", "closeCode", int(closeCode), "error", err)
}

// Close codes Twitch uses when the client did something wrong.
const (
	closeCodeClientSentInboundTraffic websocket.StatusCode = 4001
	closeCodeClientFailedPingPong     websocket.StatusCode = 4002
	closeCodeConnectionUnused         websocket.StatusCode = 4003
	closeCodeInvalidReconnect         websocket.StatusCode = 4007
)

func isCloseCodeCausedByWhendo(closeCode websocket.StatusCode) bool {
	switch closeCode {
	case closeCodeClientSentInboundTraffic, closeCodeClientFailedPingPong, closeCodeConnectionUnused, closeCodeInvalidReconnect:
		return true
	default:
		return false
	}
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
