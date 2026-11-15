package twitcheventsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const (
	bytesPerMegabyte = 1024 * 1024
	// The library's default limit of 32 KB per message is close to the size
	// of a long chat message with its fragments and badges. The limit only
	// protects against a broken or hostile message filling memory.
	maximumMessageBytes = 1 * bytesPerMegabyte
	// Messages are handed from the reading goroutine to the listener through
	// a buffer, so a slow listener doesn't stop the connection from reading
	// and answering Twitch's pings.
	messageBufferSize = 64
	// How long to wait for the welcome message after connecting.
	welcomeTimeout = 10 * time.Second
	// Extra time allowed past the keepalive timeout before the connection is
	// treated as dead, so ordinary network delay doesn't cause a reconnect.
	keepaliveGracePeriod = 2 * time.Second
)

var errNoWelcome = errors.New("twitch didn't send a welcome message")

// connection is one EventSub WebSocket connection, from its welcome message
// until it closes.
type connection struct {
	webSocket        *websocket.Conn
	logger           *slog.Logger
	sessionID        string
	keepaliveTimeout time.Duration
	messages         <-chan incomingMessage
	// readError is set before messages is closed, so it's safe to read once
	// messages is closed.
	readError error
}

// openConnection connects to url and waits for Twitch's welcome message.
// Cancelling ctx closes the connection.
func openConnection(ctx context.Context, url string, httpClient *http.Client, logger *slog.Logger) (*connection, error) {
	webSocket, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPClient: httpClient})
	if err != nil {
		return nil, fmt.Errorf("connecting to EventSub: %w", err)
	}
	webSocket.SetReadLimit(maximumMessageBytes)

	messages := make(chan incomingMessage, messageBufferSize)
	openedConnection := &connection{webSocket: webSocket, logger: logger, messages: messages}
	go openedConnection.readMessages(ctx, messages)

	err = openedConnection.awaitWelcome(ctx)
	if err != nil {
		openedConnection.close()
		return nil, err
	}
	return openedConnection, nil
}

func (openedConnection *connection) awaitWelcome(ctx context.Context) error {
	timer := time.NewTimer(welcomeTimeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errNoWelcome
	case message, isOpen := <-openedConnection.messages:
		if !isOpen {
			return fmt.Errorf("waiting for the EventSub welcome message: %w", openedConnection.readError)
		}
		if message.Metadata.MessageType != welcomeMessageType {
			return fmt.Errorf("%w: got a %q message first", errNoWelcome, message.Metadata.MessageType)
		}
		openedConnection.sessionID = message.Payload.Session.ID
		openedConnection.keepaliveTimeout = time.Duration(message.Payload.Session.KeepaliveTimeoutSeconds) * time.Second
		return nil
	}
}

// readMessages reads until the connection fails or ctx is cancelled, which
// closes the connection. Reading constantly is also what answers Twitch's
// pings, which Twitch requires.
func (openedConnection *connection) readMessages(ctx context.Context, messages chan<- incomingMessage) {
	defer close(messages)
	for {
		_, data, err := openedConnection.webSocket.Read(ctx)
		if err != nil {
			openedConnection.readError = err
			return
		}
		var message incomingMessage
		err = json.Unmarshal(data, &message)
		if err != nil {
			openedConnection.readError = fmt.Errorf("reading an EventSub message: %w", err)
			openedConnection.close()
			return
		}
		select {
		case messages <- message:
		case <-ctx.Done():
			openedConnection.readError = ctx.Err()
			return
		}
	}
}

// keepaliveDeadline is how long the connection may go without a message
// before it's treated as dead.
func (openedConnection *connection) keepaliveDeadline() time.Duration {
	return openedConnection.keepaliveTimeout + keepaliveGracePeriod
}

// close closes the connection without the closing handshake. Twitch doesn't
// need one, and waiting for it would hold up the listener.
func (openedConnection *connection) close() {
	err := openedConnection.webSocket.CloseNow()
	if err == nil || errors.Is(err, net.ErrClosed) {
		return
	}
	openedConnection.logger.Warn("couldn't close an EventSub connection cleanly", "error", err)
}
