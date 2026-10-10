package telemetry

import (
	"sync"
	"time"
)

type eventQueue struct {
	mutex     sync.Mutex
	ring      []evaluatedConfigEvent
	start     int
	count     int
	dropped   int
	startTime time.Time
}

func newEventQueue(limit int) *eventQueue {
	return &eventQueue{ring: make([]evaluatedConfigEvent, limit)}
}

func (queue *eventQueue) push(event evaluatedConfigEvent, now time.Time) {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	if queue.count == 0 {
		queue.startTime = now
	}
	if queue.count == len(queue.ring) {
		queue.start = (queue.start + 1) % len(queue.ring)
		queue.count--
		queue.dropped++
	}
	queue.ring[(queue.start+queue.count)%len(queue.ring)] = event
	queue.count++
}

type eventBatch struct {
	startTime time.Time
	endTime   time.Time
	events    []evaluatedConfigEvent
	dropped   int
}

func (queue *eventQueue) takeBatch(now time.Time) eventBatch {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	batch := eventBatch{startTime: queue.startTime, endTime: now, events: make([]evaluatedConfigEvent, queue.count), dropped: queue.dropped}
	if queue.count == 0 {
		batch.startTime = now
	}
	for index := range queue.count {
		batch.events[index] = queue.ring[(queue.start+index)%len(queue.ring)]
	}
	queue.resetLocked()
	return batch
}

func (queue *eventQueue) clear() {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()
	queue.resetLocked()
}

func (queue *eventQueue) resetLocked() {
	clear(queue.ring)
	queue.start = 0
	queue.count = 0
	queue.dropped = 0
}

type aggregatedEvent struct {
	event evaluatedConfigEvent
	count int
}

func (batch eventBatch) aggregated() []aggregatedEvent {
	aggregated := make([]aggregatedEvent, 0, len(batch.events))
	indexOf := make(map[evaluatedConfigEvent]int, len(batch.events))
	for _, event := range batch.events {
		if index, seen := indexOf[event]; seen {
			aggregated[index].count++
			continue
		}
		indexOf[event] = len(aggregated)
		aggregated = append(aggregated, aggregatedEvent{event: event, count: 1})
	}
	return aggregated
}

type contextRegistry struct {
	mutex   sync.Mutex
	limit   int
	byID    map[string]Context
	order   []string
	dropped int
}

func newContextRegistry(limit int) *contextRegistry {
	return &contextRegistry{limit: limit, byID: map[string]Context{}}
}

func (registry *contextRegistry) add(context Context) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	if _, held := registry.byID[context.ID]; !held {
		registry.order = append(registry.order, context.ID)
	}
	registry.byID[context.ID] = context
	for len(registry.order) > registry.limit {
		delete(registry.byID, registry.order[0])
		registry.order = registry.order[1:]
		registry.dropped++
	}
}

func (registry *contextRegistry) takeCaptured() (contexts []Context, dropped int) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()

	contexts = make([]Context, 0, len(registry.order))
	for _, id := range registry.order {
		contexts = append(contexts, registry.byID[id])
	}
	dropped = registry.dropped
	registry.resetLocked()
	return contexts, dropped
}

func (registry *contextRegistry) clear() {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	registry.resetLocked()
}

func (registry *contextRegistry) resetLocked() {
	registry.byID = map[string]Context{}
	registry.order = nil
	registry.dropped = 0
}
