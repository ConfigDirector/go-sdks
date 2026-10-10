// Package telemetry reports the configs a client evaluated, and the contexts it evaluated them
// for, to the ConfigDirector SDK server in batches.
package telemetry

import (
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector/internal/transport"
)

const (
	initialFlushDelay     = 5 * time.Second
	closeTimeout          = 5 * time.Second
	evaluationShareTenths = 7
)

// Options configure a Collector. The client validates and defaults them before handing them
// over, so every field is set.
type Options struct {
	// Transport is how the SDK server is reached: SDK key, base URL, meta context, HTTP client
	// and logger.
	Transport transport.Options

	// EventQueueLimit is how many entries, evaluations and distinct contexts together, are held
	// between reports.
	EventQueueLimit int

	// FlushInterval is how long the collector waits between reports after the first.
	FlushInterval time.Duration
}

// Collector collects evaluations and the contexts they were made for and reports them on a
// schedule: a first report 5 seconds after Start, then one every flush interval, and a last one
// on Close. Nothing is sent when nothing was collected.
//
// The queue is the only buffer: EventQueueLimit entries, split 70% evaluations and 30% distinct
// contexts, freed by every report. When a share is full the oldest entry is dropped and counted
// in the next report. A fatal refusal from the SDK server, a 4xx other than 429, stops collection
// for good with one error log.
type Collector struct {
	reporter      *httpReporter
	evaluations   *eventQueue
	contexts      *contextRegistry
	flushInterval time.Duration

	collecting atomic.Bool
	closing    chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

// Start creates a Collector and starts its reporting goroutine.
func Start(options Options) *Collector {
	evaluationLimit := options.EventQueueLimit * evaluationShareTenths / 10
	collector := &Collector{
		reporter:      newHTTPReporter(options.Transport),
		evaluations:   newEventQueue(evaluationLimit),
		contexts:      newContextRegistry(options.EventQueueLimit - evaluationLimit),
		flushInterval: options.FlushInterval,
		closing:       make(chan struct{}),
		done:          make(chan struct{}),
	}
	collector.collecting.Store(true)
	go collector.run()
	return collector
}

// Record queues one evaluation, and captures context when it has an ID and is not anonymous. It
// is on the client's reading path and returns without doing any appreciable work; the digests
// a report needs are computed when it is sent.
func (collector *Collector) Record(evaluation Evaluation, context Context) {
	if !collector.collecting.Load() {
		return
	}

	event := evaluatedConfigEvent{evaluation: evaluation}
	if context.identified() {
		event.contextID = context.ID
		context.Traits = maps.Clone(context.Traits)
		collector.contexts.add(context)
	}
	collector.evaluations.push(event, time.Now())
}

// Close stops collecting and sends what was collected. It returns once that report is sent or
// has failed, or after 5 seconds when a report already in flight holds it up; the reporting
// goroutine then finishes in the background and ends with that report. Calling Close again
// returns at once or waits out the same deadline.
func (collector *Collector) Close() {
	collector.closeOnce.Do(func() {
		collector.collecting.Store(false)
		close(collector.closing)
	})

	timer := time.NewTimer(closeTimeout)
	defer timer.Stop()
	select {
	case <-collector.done:
	case <-timer.C:
		collector.reporter.options.Logger.Warn("Closing the client did not wait for the last telemetry report; it finishes in the background", slog.Duration("closeTimeout", closeTimeout))
	}
}

func (collector *Collector) run() {
	defer close(collector.done)

	delay := initialFlushDelay
	for {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			if collector.flush() == reportRefusedForGood {
				collector.stopCollecting()
				return
			}
			delay = collector.flushInterval
		case <-collector.closing:
			timer.Stop()
			collector.flush()
			return
		}
	}
}

func (collector *Collector) flush() reportOutcome {
	batch := collector.evaluations.takeBatch(time.Now())
	contexts, droppedContexts := collector.contexts.takeCaptured()
	report := eventReport{
		startTime:          batch.startTime,
		endTime:            batch.endTime,
		evaluations:        batch.aggregated(),
		droppedEvaluations: batch.dropped,
		contexts:           contexts,
		droppedContexts:    droppedContexts,
	}
	if report.isEmpty() {
		return reportSkippedEmpty
	}
	return collector.reporter.send(report)
}

func (collector *Collector) stopCollecting() {
	collector.collecting.Store(false)
	collector.evaluations.clear()
	collector.contexts.clear()
}

// Discarding is the telemetry of a client that reports nothing, such as one built by package
// configdirectortest: it records nothing, sends nothing and starts no goroutine.
type Discarding struct{}

// Record discards the evaluation.
func (Discarding) Record(Evaluation, Context) {}

// Close does nothing.
func (Discarding) Close() {}
