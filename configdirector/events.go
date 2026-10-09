package configdirector

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
)

// ConfigsUpdated describes one update of config state received from ConfigDirector.
type ConfigsUpdated struct {
	// Keys are the configs the update carried, and the configs whose targeting rules use a
	// segment the update carried, sorted. A watch on any of them fires with its new value.
	Keys []string

	// RemovedKeys are the configs the client held that a full update no longer carries, sorted.
	// A watch on any of them fires with its in-code default value. Empty for a delta update.
	RemovedKeys []string
}

// Hooks holds event handlers attached when a client is created, before it can emit any event.
// Each one is the same as registering it with the matching Client method right after NewClient
// returns, except that an event emitted in between cannot be missed. A nil field attaches
// nothing.
type Hooks struct {
	// OnReady is called once, on the connection's goroutine, when the first config state arrives.
	OnReady func()

	// OnConfigsUpdated is called on the connection's goroutine every time config state arrives,
	// the first time included.
	OnConfigsUpdated func(ConfigsUpdated)

	// OnConfigEvaluated is called on the reading goroutine every time a config is evaluated, and on
	// the connection's goroutine when a watch fires.
	OnConfigEvaluated func(Evaluation)
}

// OnReady registers a handler for the moment the first config state arrives, and returns a func
// that cancels the registration. The handler is called once, on the connection's goroutine, so
// one that blocks delays the update that made the client ready. When the client is ready
// already, the handler is called at once on the calling goroutine and the returned func does
// nothing. A panic in the handler is recovered and logged at error level; the client continues.
// The client holds the handler until it has been called, the returned func is called, or Close
// is called, whichever comes first. After Close, nothing is registered.
func (client *Client) OnReady(handler func()) (unsubscribe func()) {
	assertHandler(handler, "handler")

	client.mutex.Lock()
	registration := readyRegistrationFor(client.closed, client.snapshot != nil)
	unsubscribe = func() {}
	if registration == readyRegistrationWaitsForTheFirstConfigState {
		unsubscribe = registerLocked(client, &client.readyHandlers, func(struct{}) { handler() })
	}
	client.mutex.Unlock()

	if registration == readyRegistrationCallsAtOnce {
		client.callHandler(eventReady, handler)
	}
	return unsubscribe
}

type readyRegistration int

const (
	readyRegistrationDropped readyRegistration = iota
	readyRegistrationWaitsForTheFirstConfigState
	readyRegistrationCallsAtOnce
)

func readyRegistrationFor(closed bool, ready bool) readyRegistration {
	switch {
	case closed:
		return readyRegistrationDropped
	case ready:
		return readyRegistrationCallsAtOnce
	default:
		return readyRegistrationWaitsForTheFirstConfigState
	}
}

// OnConfigsUpdated registers a handler for every update of config state, the first one included,
// and returns a func that cancels the registration. Handlers run on the connection's goroutine,
// so one that blocks delays later updates. A panic in a handler is recovered and logged at
// error level; the other handlers and the connection continue. The client holds the handler
// until the returned func or Close is called. After Close, nothing is registered.
func (client *Client) OnConfigsUpdated(handler func(ConfigsUpdated)) (unsubscribe func()) {
	return register(client, &client.updatedHandlers, handler)
}

// OnConfigEvaluated registers a handler for every evaluation of a config, those that returned
// the in-code default value included, and returns a func that cancels the registration. A read
// calls the handlers on the reading goroutine before it returns, so one that blocks delays the
// read; a watch that fires calls them on the connection's goroutine. A panic in a handler is
// recovered and logged at error level; the other handlers and the read continue. The client
// holds the handler until the returned func or Close is called. After Close, nothing is
// registered.
func (client *Client) OnConfigEvaluated(handler func(Evaluation)) (unsubscribe func()) {
	return register(client, &client.evaluatedHandlers, handler)
}

type eventName string

const (
	eventReady           eventName = "ready"
	eventConfigsUpdated  eventName = "configs-updated"
	eventConfigEvaluated eventName = "config-evaluated"
	eventWatch           eventName = "watch"
)

type registration[T any] struct {
	id      uint64
	handler func(T)
}

type handlerList[T any] struct {
	lastID  uint64
	entries []registration[T]
}

func (list *handlerList[T]) add(handler func(T)) uint64 {
	list.lastID++
	list.entries = append(list.entries, registration[T]{id: list.lastID, handler: handler})
	return list.lastID
}

func (list *handlerList[T]) remove(id uint64) {
	list.entries = slices.DeleteFunc(list.entries, func(entry registration[T]) bool { return entry.id == id })
}

func (list *handlerList[T]) handlers() []func(T) {
	if len(list.entries) == 0 {
		return nil
	}
	handlers := make([]func(T), 0, len(list.entries))
	for _, entry := range list.entries {
		handlers = append(handlers, entry.handler)
	}
	return handlers
}

func (list *handlerList[T]) drain() []func(T) {
	handlers := list.handlers()
	list.entries = nil
	return handlers
}

func (list *handlerList[T]) clear() {
	list.entries = nil
}

func register[T any](client *Client, list *handlerList[T], handler func(T)) (unsubscribe func()) {
	assertHandler(handler, "handler")

	client.mutex.Lock()
	defer client.mutex.Unlock()
	return registerLocked(client, list, handler)
}

func registerLocked[T any](client *Client, list *handlerList[T], handler func(T)) (unsubscribe func()) {
	if client.closed {
		return func() {}
	}

	id := list.add(handler)
	return func() {
		client.mutex.Lock()
		defer client.mutex.Unlock()
		list.remove(id)
	}
}

func emit[T any](client *Client, event eventName, handlers []func(T), payload T) {
	for _, handler := range handlers {
		client.callHandler(event, func() { handler(payload) })
	}
}

func (client *Client) reportEvaluation(evaluated Evaluation) {
	client.mutex.RLock()
	handlers := client.evaluatedHandlers.handlers()
	client.mutex.RUnlock()

	emit(client, eventConfigEvaluated, handlers, evaluated)
}

func (client *Client) callHandler(event eventName, handler func(), attributes ...slog.Attr) {
	defer func() {
		if recovered := recover(); recovered != nil {
			client.settings.logger.LogAttrs(
				context.Background(),
				slog.LevelError,
				"A handler panicked; the panic was recovered and the client continues",
				append([]slog.Attr{slog.String("event", string(event)), slog.Any("panic", recovered)}, attributes...)...,
			)
		}
	}()
	handler()
}

func assertHandler(handler any, what string) {
	if reflect.ValueOf(handler).IsNil() {
		panic(fmt.Sprintf("configdirector: the %s is nil; a registration needs a non-nil func", what))
	}
}
