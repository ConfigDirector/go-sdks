package servedvalues

import (
	"io"
	"sync"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/connectionseam"
)

type connectionPhase int

const (
	connectionPhaseInitializing connectionPhase = iota
	connectionPhaseConnected
	connectionPhaseClosed
)

// Connection is an in-memory connection that serves the configs it holds and delivers every change
// to them on the calling goroutine, one update at a time, in the order the changes were made.
type Connection struct {
	receiver connectionseam.Receiver

	mutex   sync.Mutex
	configs map[string]Config
	phase   connectionPhase

	deliveries deliveryQueue
}

// NewConnection creates a connection holding configs, which it delivers as the client's first
// update when CompleteInitialization is called.
func NewConnection(configs map[string]Config) *Connection {
	return &Connection{configs: configs}
}

// Open attaches the connection to the client receiving its documents. It is a
// connectionseam.OpenConnection.
func (connection *Connection) Open(receiver connectionseam.Receiver) io.Closer {
	connection.receiver = receiver
	return connection
}

// Close stops every later delivery.
func (connection *Connection) Close() error {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	connection.phase = connectionPhaseClosed
	return nil
}

type updateDecision struct {
	document string
	delivers bool
}

var noUpdate = updateDecision{}

// SetValue stores value under key and, once the first update was delivered, delivers it as an
// update carrying only that key. A value that cannot be served returns an error and nothing
// changes.
func (connection *Connection) SetValue(key string, value any) error {
	config, err := configFrom(key, value)
	if err != nil {
		return err
	}
	connection.deliverUpdate(func() updateDecision {
		connection.configs[key] = config
		return connection.updateWhenConnected(deltaDocument(key, config))
	})
	return nil
}

// RemoveValue removes key and, once the first update was delivered, delivers a full update
// without it.
func (connection *Connection) RemoveValue(key string) {
	connection.deliverUpdate(func() updateDecision {
		delete(connection.configs, key)
		return connection.updateWhenConnected(fullDocument(connection.configs))
	})
}

// ReplaceValues replaces every stored value with values and, once the first update was
// delivered, delivers them as a full update. A value that cannot be served returns an error and
// nothing changes.
func (connection *Connection) ReplaceValues(values map[string]any) error {
	configs, err := ConfigsFrom(values)
	if err != nil {
		return err
	}
	connection.deliverUpdate(func() updateDecision {
		connection.configs = configs
		return connection.updateWhenConnected(fullDocument(connection.configs))
	})
	return nil
}

// CompleteInitialization delivers the stored configs as the client's first update. On a
// connection that delivered it already, or is closed, it does nothing.
func (connection *Connection) CompleteInitialization() {
	connection.deliverUpdate(func() updateDecision {
		if connection.phase != connectionPhaseInitializing {
			return noUpdate
		}
		connection.phase = connectionPhaseConnected
		return updateDecision{document: fullDocument(connection.configs), delivers: true}
	})
}

// FailInitialization logs, at error level, that the first update is not coming yet. On a
// connection that delivered it already, or is closed, it does nothing.
func (connection *Connection) FailInitialization() {
	connection.mutex.Lock()
	fails := connection.phase == connectionPhaseInitializing
	connection.mutex.Unlock()

	if fails {
		connection.receiver.Logger.Error("The test client failed this initialization; the client stays not ready until CompleteInitialization delivers the stored values")
	}
}

func (connection *Connection) updateWhenConnected(document string) updateDecision {
	if connection.phase != connectionPhaseConnected {
		return noUpdate
	}
	return updateDecision{document: document, delivers: true}
}

func (connection *Connection) deliverUpdate(decide func() updateDecision) {
	connection.mutex.Lock()
	decision := decide()
	connection.mutex.Unlock()

	if decision.delivers {
		connection.deliver(decision.document)
	}
}

func (connection *Connection) deliver(document string) {
	for next, delivers := connection.deliveries.enqueue(document); delivers; next, delivers = connection.deliveries.dequeue() {
		connection.receiver.Deliver(next)
	}
}

type deliveryQueue struct {
	mutex      sync.Mutex
	pending    []string
	delivering bool
}

func (queue *deliveryQueue) enqueue(document string) (next string, delivers bool) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.pending = append(queue.pending, document)
	if queue.delivering {
		return "", false
	}
	queue.delivering = true
	return queue.take(), true
}

func (queue *deliveryQueue) dequeue() (next string, delivers bool) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	if len(queue.pending) == 0 {
		queue.delivering = false
		return "", false
	}
	return queue.take(), true
}

func (queue *deliveryQueue) take() string {
	next := queue.pending[0]
	queue.pending = queue.pending[1:]
	return next
}
