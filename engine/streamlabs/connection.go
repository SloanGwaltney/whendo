// Package streamlabs connects to Streamlabs Desktop through its local API:
// JSON-RPC requests, one per line, over a TCP connection that Streamlabs
// trusts without a token because it comes from the same PC. It keeps the
// connection open, retrying while Streamlabs isn't running, and makes the
// requests Whendo needs to show and hide sources. See
// docs/design/streamlabs-desktop.md.
package streamlabs

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

const (
	// DefaultAddress is where Streamlabs Desktop always listens for API
	// clients on the same PC.
	DefaultAddress = "127.0.0.1:28194"

	// Streamlabs is usually on the same PC, so a failed attempt costs almost
	// nothing, and a streamer who opens Streamlabs expects Whendo to notice
	// in seconds. That's why retries don't back off.
	defaultRetryDelay = 5 * time.Second
	// defaultRequestTimeout bounds a request, including retries while
	// Streamlabs is busy, so a run is never held up for long.
	defaultRequestTimeout = 10 * time.Second
	// defaultBusyRetryDelay is the wait before asking again after Streamlabs
	// says it's busy, which lasts seconds at most.
	defaultBusyRetryDelay = 500 * time.Millisecond
)

// ConnectionStatus says whether Whendo can talk to Streamlabs Desktop.
type ConnectionStatus string

const (
	// ConnectionStatusWaiting covers every failure to connect, most often
	// Streamlabs not being open yet.
	ConnectionStatusWaiting   ConnectionStatus = "waiting"
	ConnectionStatusConnected ConnectionStatus = "connected"
)

// Config holds what a Connection needs. Every field is optional.
type Config struct {
	// Address defaults to DefaultAddress. Tests point it at a fake.
	Address string
	Logger  *slog.Logger
	// OnStatusChange, if set, is called with the new status each time it
	// changes.
	OnStatusChange func(ConnectionStatus)
	// RetryDelay, RequestTimeout and BusyRetryDelay default to the values
	// in the design. Tests shorten them.
	RetryDelay     time.Duration
	RequestTimeout time.Duration
	BusyRetryDelay time.Duration
}

// Connection keeps a connection to Streamlabs Desktop open and makes
// requests over it. Call Run to connect; requests made while it isn't
// connected fail with ErrNotConnected.
type Connection struct {
	address        string
	logger         *slog.Logger
	onStatusChange func(ConnectionStatus)
	retryDelay     time.Duration
	requestTimeout time.Duration
	busyRetryDelay time.Duration

	// requestMutex allows one request at a time. Streamlabs disconnects a
	// client whose request arrives cut short, which could happen if two
	// requests were written close together; see the design's Requests
	// section. It also guards nextRequestID.
	requestMutex  sync.Mutex
	nextRequestID int64

	linkMutex   sync.Mutex
	currentLink *link

	statusMutex sync.Mutex
	status      ConnectionStatus
}

// NewConnection returns a Connection. Call Run to connect.
func NewConnection(config Config) *Connection {
	address := config.Address
	if address == "" {
		address = DefaultAddress
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Connection{
		address:        address,
		logger:         logger,
		onStatusChange: config.OnStatusChange,
		retryDelay:     durationOrDefault(config.RetryDelay, defaultRetryDelay),
		requestTimeout: durationOrDefault(config.RequestTimeout, defaultRequestTimeout),
		busyRetryDelay: durationOrDefault(config.BusyRetryDelay, defaultBusyRetryDelay),
		status:         ConnectionStatusWaiting,
	}
}

// Status returns whether Whendo is connected to Streamlabs Desktop.
func (connection *Connection) Status() ConnectionStatus {
	connection.statusMutex.Lock()
	defer connection.statusMutex.Unlock()
	return connection.status
}

// Run stays connected to Streamlabs Desktop until ctx is cancelled,
// connecting again whenever the connection can't be made or is lost. Run
// must not be called again until the previous call returns.
func (connection *Connection) Run(ctx context.Context) {
	defer connection.setStatus(ConnectionStatusWaiting)
	for {
		connection.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		err := waitFor(ctx, connection.retryDelay)
		if err != nil {
			return
		}
	}
}

// runOnce connects, and stays connected until the connection is lost or
// ctx is cancelled.
func (connection *Connection) runOnce(ctx context.Context) {
	openedLink, err := connection.connect(ctx)
	if err != nil {
		// Streamlabs not running yet is the normal case, so this isn't a
		// warning.
		connection.logger.Debug("couldn't connect to Streamlabs Desktop; trying again soon", "address", connection.address, "error", err)
		return
	}
	connection.setLink(openedLink)
	connection.setStatus(ConnectionStatusConnected)
	connection.logger.Info("connected to Streamlabs Desktop", "address", connection.address)

	select {
	case <-ctx.Done():
	case <-openedLink.closed:
		connection.logger.Info("the connection to Streamlabs Desktop ended; reconnecting", "error", openedLink.closeError())
	}
	connection.setLink(nil)
	connection.setStatus(ConnectionStatusWaiting)
	openedLink.close()
}

// connect opens a link and checks that Streamlabs answers on it.
// Streamlabs sends nothing when a client connects, and accepts connections
// before it has finished starting, so an answer is the only sign that it's
// ready.
func (connection *Connection) connect(ctx context.Context) (*link, error) {
	dialer := net.Dialer{Timeout: connection.requestTimeout}
	networkConnection, err := dialer.DialContext(ctx, "tcp", connection.address)
	if err != nil {
		return nil, err
	}
	openedLink := openLink(networkConnection, connection.logger)
	_, err = connection.callOn(ctx, openedLink, scenesServiceResource, activeSceneIDMethod, nil)
	if err != nil {
		openedLink.close()
		return nil, err
	}
	return openedLink, nil
}

func (connection *Connection) setLink(newLink *link) {
	connection.linkMutex.Lock()
	defer connection.linkMutex.Unlock()
	connection.currentLink = newLink
}

func (connection *Connection) link() *link {
	connection.linkMutex.Lock()
	defer connection.linkMutex.Unlock()
	return connection.currentLink
}

func (connection *Connection) setStatus(status ConnectionStatus) {
	connection.statusMutex.Lock()
	hasChanged := connection.status != status
	connection.status = status
	connection.statusMutex.Unlock()
	if hasChanged && connection.onStatusChange != nil {
		connection.onStatusChange(status)
	}
}

func durationOrDefault(duration time.Duration, defaultDuration time.Duration) time.Duration {
	if duration <= 0 {
		return defaultDuration
	}
	return duration
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
