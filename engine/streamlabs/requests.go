package streamlabs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
)

const jsonRPCVersion = "2.0"

// Messages Streamlabs answers with when a request should simply be made
// again shortly: while it loads a scene collection or the streamer drags a
// source in its editor, and when getScenes has been called too often in
// the last second.
const (
	busyMessage      = "API server is busy"
	rateLimitMessage = "Reached the limit of calls"
)

// These errors name Streamlabs Desktop, since they end up in run history as
// the reason a step failed.
var (
	// ErrNotConnected means Whendo isn't connected to Streamlabs Desktop,
	// most often because it isn't open.
	ErrNotConnected = errors.New("Streamlabs Desktop isn't connected")
	// ErrNoAnswer means Streamlabs Desktop didn't answer in time.
	ErrNoAnswer = errors.New("Streamlabs Desktop didn't answer")
	// ErrBusy means Streamlabs Desktop said it was busy until the request
	// ran out of time.
	ErrBusy = errors.New("Streamlabs Desktop stayed busy")
	// ErrConnectionLost means the connection ended while a request waited
	// for its answer.
	ErrConnectionLost = errors.New("the connection to Streamlabs Desktop was lost")
)

// RequestError is an error Streamlabs Desktop answered a request with.
type RequestError struct {
	Code    int
	Message string
}

func (requestError *RequestError) Error() string {
	return fmt.Sprintf("Streamlabs Desktop answered with error %d: %s", requestError.Code, requestError.Message)
}

type requestParameters struct {
	Resource string `json:"resource"`
	Args     []any  `json:"args"`
}

type request struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      int64             `json:"id"`
	Method  string            `json:"method"`
	Params  requestParameters `json:"params"`
}

type answerError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type answer struct {
	// ID is missing from events and null in some errors, so it's kept raw.
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *answerError    `json:"error"`
}

// isTryAgain reports whether Streamlabs refused the request only for now.
func (receivedAnswer answer) isTryAgain() bool {
	if receivedAnswer.Error == nil {
		return false
	}
	message := receivedAnswer.Error.Message
	return strings.Contains(message, busyMessage) || strings.Contains(message, rateLimitMessage)
}

// isFor reports whether the answer belongs to the request with requestID.
// Streamlabs answers some errors, such as being busy, with a null ID. Only
// one request is ever waiting, so such an error belongs to it.
func (receivedAnswer answer) isFor(requestID int64) bool {
	if len(receivedAnswer.ID) == 0 {
		return false
	}
	if bytes.Equal(receivedAnswer.ID, []byte("null")) {
		return receivedAnswer.Error != nil
	}
	var answerID int64
	err := json.Unmarshal(receivedAnswer.ID, &answerID)
	if err != nil {
		return false
	}
	return answerID == requestID
}

// link is one open TCP connection to Streamlabs and the goroutine reading
// its answers.
type link struct {
	networkConnection net.Conn
	logger            *slog.Logger
	closed            chan struct{}

	mutex          sync.Mutex
	pendingID      int64
	pendingAnswers chan answer
	readError      error
	closeOnce      sync.Once
}

func openLink(networkConnection net.Conn, logger *slog.Logger) *link {
	openedLink := &link{
		networkConnection: networkConnection,
		logger:            logger,
		closed:            make(chan struct{}),
	}
	go openedLink.readAnswers()
	return openedLink
}

// readAnswers reads answers line by line, since one read can hold part of
// a line or several lines, and hands each to the request waiting for it.
func (openedLink *link) readAnswers() {
	reader := bufio.NewReader(openedLink.networkConnection)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			openedLink.closeWithError(err)
			return
		}
		openedLink.deliver(line)
	}
}

func (openedLink *link) deliver(line []byte) {
	var receivedAnswer answer
	err := json.Unmarshal(line, &receivedAnswer)
	if err != nil {
		openedLink.logger.Warn("Streamlabs Desktop sent something that isn't JSON; ignoring it", "error", err)
		return
	}
	openedLink.mutex.Lock()
	defer openedLink.mutex.Unlock()
	if openedLink.pendingAnswers == nil || !receivedAnswer.isFor(openedLink.pendingID) {
		// Events, and late answers to requests that already ran out of
		// time, have no one waiting for them.
		return
	}
	openedLink.pendingAnswers <- receivedAnswer
	openedLink.pendingAnswers = nil
}

// exchange sends one request and waits for its answer.
func (openedLink *link) exchange(ctx context.Context, sentRequest request) (answer, error) {
	line, err := json.Marshal(sentRequest)
	if err != nil {
		return answer{}, fmt.Errorf("encoding the %s request: %w", sentRequest.Method, err)
	}
	// Streamlabs splits what it reads on newlines, and encoding/json escapes
	// newlines inside strings, so the request is exactly one line.
	line = append(line, '\n')

	answers := make(chan answer, 1)
	openedLink.mutex.Lock()
	openedLink.pendingID = sentRequest.ID
	openedLink.pendingAnswers = answers
	openedLink.mutex.Unlock()
	defer openedLink.stopWaiting()

	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		err = openedLink.networkConnection.SetWriteDeadline(deadline)
		if err != nil {
			return answer{}, fmt.Errorf("setting how long sending may take: %w", err)
		}
	}
	_, err = openedLink.networkConnection.Write(line)
	if err != nil {
		return answer{}, fmt.Errorf("%w: %w", ErrConnectionLost, err)
	}

	select {
	case receivedAnswer := <-answers:
		return receivedAnswer, nil
	case <-openedLink.closed:
		return answer{}, ErrConnectionLost
	case <-ctx.Done():
		return answer{}, ctx.Err()
	}
}

func (openedLink *link) stopWaiting() {
	openedLink.mutex.Lock()
	defer openedLink.mutex.Unlock()
	openedLink.pendingAnswers = nil
}

func (openedLink *link) closeWithError(err error) {
	openedLink.closeOnce.Do(func() {
		openedLink.mutex.Lock()
		openedLink.readError = err
		openedLink.mutex.Unlock()
		closeErr := openedLink.networkConnection.Close()
		if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			openedLink.logger.Debug("closing the connection to Streamlabs Desktop failed", "error", closeErr)
		}
		close(openedLink.closed)
	})
}

func (openedLink *link) close() {
	openedLink.closeWithError(net.ErrClosed)
}

func (openedLink *link) closeError() error {
	openedLink.mutex.Lock()
	defer openedLink.mutex.Unlock()
	return openedLink.readError
}

// call makes a request over the current connection and returns its
// result, undecoded.
func (connection *Connection) call(ctx context.Context, resource string, method string, args []any) (json.RawMessage, error) {
	currentLink := connection.link()
	if currentLink == nil {
		return nil, ErrNotConnected
	}
	return connection.callOn(ctx, currentLink, resource, method, args)
}

// callOn makes a request over openedLink, asking again while Streamlabs
// says it's busy, until the request timeout.
func (connection *Connection) callOn(ctx context.Context, openedLink *link, resource string, method string, args []any) (json.RawMessage, error) {
	if args == nil {
		args = []any{}
	}
	connection.requestMutex.Lock()
	defer connection.requestMutex.Unlock()

	requestCtx, cancel := context.WithTimeout(ctx, connection.requestTimeout)
	defer cancel()
	// Once Streamlabs has said it's busy, running out of time means it
	// stayed busy, even if time ran out while waiting for an answer.
	hasBeenBusy := false
	for {
		connection.nextRequestID++
		// Streamlabs refuses a request whose ID is 0, so IDs start at 1.
		sentRequest := request{
			JSONRPC: jsonRPCVersion,
			ID:      connection.nextRequestID,
			Method:  method,
			Params:  requestParameters{Resource: resource, Args: args},
		}
		receivedAnswer, err := openedLink.exchange(requestCtx, sentRequest)
		if err != nil {
			return nil, describeExchangeError(ctx, err, hasBeenBusy)
		}
		if receivedAnswer.isTryAgain() {
			hasBeenBusy = true
			err = waitFor(requestCtx, connection.busyRetryDelay)
			if err != nil {
				return nil, describeExchangeError(ctx, err, hasBeenBusy)
			}
			continue
		}
		if receivedAnswer.Error != nil {
			return nil, &RequestError{Code: receivedAnswer.Error.Code, Message: receivedAnswer.Error.Message}
		}
		return receivedAnswer.Result, nil
	}
}

// describeExchangeError turns running out of time into ErrBusy or
// ErrNoAnswer, unless the caller cancelled.
func describeExchangeError(callerCtx context.Context, err error, hasBeenBusy bool) error {
	if callerCtx.Err() != nil {
		return callerCtx.Err()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if hasBeenBusy {
		return ErrBusy
	}
	return ErrNoAnswer
}
