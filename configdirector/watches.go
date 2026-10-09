package configdirector

import (
	"log/slog"
	"slices"
)

// WatchBool registers a handler for the boolean value of key as evaluated for userContext, and
// returns a func that cancels the watch. The handler is called with the newly evaluated value
// whenever an update carries the key or a segment its targeting rules use, and with
// defaultValue whenever the config cannot be read, as BoolValue would return it: a full update
// that no longer carries the key fires the watch with defaultValue. Each call also reaches the
// OnConfigEvaluated handlers. Handlers run on the connection's goroutine, so one that blocks
// delays later updates. A panic in a handler is recovered and logged at error level; the other
// watches and the connection continue. The client holds the watch until the returned func,
// UnwatchAll or Close is called. After Close, nothing is registered.
func (client *Client) WatchBool(key string, defaultValue bool, userContext UserContext, handler func(bool)) (unsubscribe func()) {
	return watchValue(client, key, defaultValue, userContext, boolFrom, handler)
}

// WatchString is WatchBool for the text value of key, as StringValue reads it. It fires, runs its handler and is cancelled exactly as WatchBool does: on the
// connection's goroutine, with a panic recovered and logged at error level, until the returned
// func, UnwatchAll or Close is called.
func (client *Client) WatchString(key string, defaultValue string, userContext UserContext, handler func(string)) (unsubscribe func()) {
	return watchValue(client, key, defaultValue, userContext, stringFrom, handler)
}

// WatchInt is WatchBool for the whole number value of key, as IntValue reads it. It fires, runs its handler and is cancelled exactly as WatchBool does: on the
// connection's goroutine, with a panic recovered and logged at error level, until the returned
// func, UnwatchAll or Close is called.
func (client *Client) WatchInt(key string, defaultValue int64, userContext UserContext, handler func(int64)) (unsubscribe func()) {
	return watchValue(client, key, defaultValue, userContext, int64From, handler)
}

// WatchFloat is WatchBool for the number value of key, as FloatValue reads it. It fires, runs its handler and is cancelled exactly as WatchBool does: on the
// connection's goroutine, with a panic recovered and logged at error level, until the returned
// func, UnwatchAll or Close is called.
func (client *Client) WatchFloat(key string, defaultValue float64, userContext UserContext, handler func(float64)) (unsubscribe func()) {
	return watchValue(client, key, defaultValue, userContext, float64From, handler)
}

// WatchJSON is WatchBool for the JSON document of key, as JSONValue reads it. Every time the
// watch fires, newTarget is called for a fresh non-nil pointer holding the in-code default
// value, the document is decoded into it as JSONValue does, and the handler receives that
// pointer: decoded into, or left as newTarget returned it when the config cannot be read. When
// newTarget returns something JSONValue would reject, an error is logged and the handler is not
// called. It fires, runs its handler and is cancelled exactly as WatchBool does: on the
// connection's goroutine, with a panic recovered and logged at error level, until the returned
// func, UnwatchAll or Close is called.
func (client *Client) WatchJSON(key string, newTarget func() any, userContext UserContext, handler func(target any)) (unsubscribe func()) {
	assertHandler(newTarget, "target func")
	assertHandler(handler, "handler")

	return client.addWatch(key, func(snapshot *configSnapshot) {
		target := newTarget()
		targetValue, err := jsonTargetFrom(target)
		if err != nil {
			client.settings.logger.Error(
				"The JSON watch target is not a non-nil pointer; the handler is not called",
				slog.String("key", key),
				slog.Any("error", err),
			)
			return
		}
		client.evaluateJSONAndReport(snapshot, key, targetValue, userContext)
		handler(target)
	})
}

// UnwatchAll cancels every watch on every config. Handlers registered with OnReady,
// OnConfigsUpdated and OnConfigEvaluated stay.
func (client *Client) UnwatchAll() {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.watches.clear()
}

type watch struct {
	id     uint64
	key    string
	notify func(snapshot *configSnapshot)
}

type watchList struct {
	lastID uint64
	byKey  map[string][]watch
}

func newWatchList() watchList {
	return watchList{byKey: map[string][]watch{}}
}

func (list *watchList) add(key string, notify func(snapshot *configSnapshot)) uint64 {
	list.lastID++
	list.byKey[key] = append(list.byKey[key], watch{id: list.lastID, key: key, notify: notify})
	return list.lastID
}

func (list *watchList) remove(key string, id uint64) {
	remaining := slices.DeleteFunc(list.byKey[key], func(entry watch) bool { return entry.id == id })
	if len(remaining) == 0 {
		delete(list.byKey, key)
		return
	}
	list.byKey[key] = remaining
}

func (list *watchList) affectedBy(update ConfigsUpdated) []watch {
	var affected []watch
	for _, key := range update.Keys {
		affected = append(affected, list.byKey[key]...)
	}
	for _, key := range update.RemovedKeys {
		affected = append(affected, list.byKey[key]...)
	}
	return affected
}

func (list *watchList) clear() {
	list.byKey = map[string][]watch{}
}

func watchValue[T any](client *Client, key string, defaultValue T, userContext UserContext, valueFrom func(ConfigState) (T, EvaluationReason), handler func(T)) (unsubscribe func()) {
	assertHandler(handler, "handler")

	return client.addWatch(key, func(snapshot *configSnapshot) {
		value, _ := evaluateValueAndReport(client, snapshot, key, defaultValue, userContext, valueFrom)
		handler(value)
	})
}

func (client *Client) addWatch(key string, notify func(snapshot *configSnapshot)) (unsubscribe func()) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	if client.closed {
		return func() {}
	}

	id := client.watches.add(key, notify)
	return func() {
		client.mutex.Lock()
		defer client.mutex.Unlock()
		client.watches.remove(key, id)
	}
}

func (client *Client) notifyWatches(affected []watch, snapshot *configSnapshot) {
	for _, entry := range affected {
		client.callHandler(eventWatch, func() { entry.notify(snapshot) }, slog.String("key", entry.key))
	}
}
