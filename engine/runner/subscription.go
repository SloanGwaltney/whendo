package runner

import (
	"log/slog"
	"sync"

	"github.com/SloanGwaltney/whendo/engine/step"
	"github.com/SloanGwaltney/whendo/engine/trigger"
)

// subscriptionBufferSize is how many events a waiting step can fall behind
// by before events are dropped for it. Checking an event takes
// microseconds, so only a bug could fill it.
const subscriptionBufferSize = 64

// subscriptionList is the set of steps currently waiting for events. It
// implements step.EventSource.
type subscriptionList struct {
	logger *slog.Logger

	// mutex guards subscriptions. Events are sent while holding it, so a
	// subscription can't be removed partway through a delivery.
	mutex         sync.Mutex
	subscriptions map[*eventSubscription]struct{}
}

func newSubscriptionList(logger *slog.Logger) subscriptionList {
	return subscriptionList{
		logger:        logger,
		subscriptions: make(map[*eventSubscription]struct{}),
	}
}

// Subscribe starts delivering every event the runner receives to the
// returned subscription.
func (list *subscriptionList) Subscribe() step.EventSubscription {
	subscription := &eventSubscription{
		list:   list,
		events: make(chan trigger.Event, subscriptionBufferSize),
	}
	list.mutex.Lock()
	list.subscriptions[subscription] = struct{}{}
	list.mutex.Unlock()
	return subscription
}

// deliver gives an event to every subscription without waiting, since the
// integration delivering it has to get back to reading its connection.
func (list *subscriptionList) deliver(event trigger.Event) {
	list.mutex.Lock()
	defer list.mutex.Unlock()
	for subscription := range list.subscriptions {
		select {
		case subscription.events <- event:
		default:
			list.logger.Error("dropped an event for a waiting step that fell behind", "trigger", event.TriggerID)
		}
	}
}

func (list *subscriptionList) remove(subscription *eventSubscription) {
	list.mutex.Lock()
	delete(list.subscriptions, subscription)
	list.mutex.Unlock()
}

type eventSubscription struct {
	list   *subscriptionList
	events chan trigger.Event
}

func (subscription *eventSubscription) Events() <-chan trigger.Event {
	return subscription.events
}

// Unsubscribe stops delivery. The channel isn't closed, so a step reading
// it after unsubscribing simply receives nothing more.
func (subscription *eventSubscription) Unsubscribe() {
	subscription.list.remove(subscription)
}
