package configdirectortest

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

type inMemoryConnection struct {
	receiver connectionseam.Receiver

	mutex   sync.Mutex
	configs map[string]servedConfig
	phase   connectionPhase

	deliveries deliveryQueue
}

func newInMemoryConnection(configs map[string]servedConfig) *inMemoryConnection {
	return &inMemoryConnection{configs: configs}
}

func (connection *inMemoryConnection) open(receiver connectionseam.Receiver) io.Closer {
	connection.receiver = receiver
	return connection
}

func (connection *inMemoryConnection) Close() error {
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

func (connection *inMemoryConnection) setValue(key string, value any) error {
	config, err := servedConfigFrom(key, value)
	if err != nil {
		return err
	}
	connection.deliverUpdate(func() updateDecision {
		connection.configs[key] = config
		return connection.updateWhenConnected(deltaDocument(key, config))
	})
	return nil
}

func (connection *inMemoryConnection) removeValue(key string) {
	connection.deliverUpdate(func() updateDecision {
		delete(connection.configs, key)
		return connection.updateWhenConnected(fullDocument(connection.configs))
	})
}

func (connection *inMemoryConnection) replaceValues(values map[string]any) error {
	configs, err := servedConfigsFrom(values)
	if err != nil {
		return err
	}
	connection.deliverUpdate(func() updateDecision {
		connection.configs = configs
		return connection.updateWhenConnected(fullDocument(connection.configs))
	})
	return nil
}

func (connection *inMemoryConnection) completeInitialization() {
	connection.deliverUpdate(func() updateDecision {
		if connection.phase != connectionPhaseInitializing {
			return noUpdate
		}
		connection.phase = connectionPhaseConnected
		return updateDecision{document: fullDocument(connection.configs), delivers: true}
	})
}

func (connection *inMemoryConnection) failInitialization() {
	connection.mutex.Lock()
	fails := connection.phase == connectionPhaseInitializing
	connection.mutex.Unlock()

	if fails {
		connection.receiver.Logger.Error("The test client failed this initialization; the client stays not ready until CompleteInitialization delivers the stored values")
	}
}

func (connection *inMemoryConnection) updateWhenConnected(document string) updateDecision {
	if connection.phase != connectionPhaseConnected {
		return noUpdate
	}
	return updateDecision{document: document, delivers: true}
}

func (connection *inMemoryConnection) deliverUpdate(decide func() updateDecision) {
	connection.mutex.Lock()
	decision := decide()
	connection.mutex.Unlock()

	if decision.delivers {
		connection.deliver(decision.document)
	}
}

func (connection *inMemoryConnection) deliver(document string) {
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
