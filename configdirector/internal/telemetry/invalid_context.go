package telemetry

import (
	"sync"
	"time"
)

// DistinctContextKindSampleLimit is how many distinct invalid context kinds a report names;
// further invalid submissions in the same report are counted without a sample.
const DistinctContextKindSampleLimit = 20

const invalidContextKindReason = "invalid-context-kind"

// InvalidContextRecording says what recording an invalid context submission did to the samples
// the next report carries, which decides whether the client logs the submission.
type InvalidContextRecording int

const (
	// InvalidContextSampled means the context kind was not among the report's samples yet and is
	// now; the client logs it.
	InvalidContextSampled InvalidContextRecording = iota

	// InvalidContextAlreadySampled means the context kind was among the report's samples already
	// and was logged when it was added.
	InvalidContextAlreadySampled

	// FirstInvalidContextBeyondSamples means the report's samples were full and this is the first
	// submission counted without one; the client logs that further context kinds are not logged
	// individually.
	FirstInvalidContextBeyondSamples

	// InvalidContextBeyondSamples means the report's samples were full and an earlier submission
	// was already counted without one.
	InvalidContextBeyondSamples
)

type invalidContextSubmissions struct {
	mutex                  sync.Mutex
	countsBySample         map[string]int
	sampleOrder            []string
	countBeyondSampleLimit int
	startTime              time.Time
}

func newInvalidContextSubmissions() *invalidContextSubmissions {
	return &invalidContextSubmissions{countsBySample: map[string]int{}}
}

func (submissions *invalidContextSubmissions) record(contextKindSample string, now time.Time) InvalidContextRecording {
	submissions.mutex.Lock()
	defer submissions.mutex.Unlock()

	if len(submissions.sampleOrder) == 0 && submissions.countBeyondSampleLimit == 0 {
		submissions.startTime = now
	}
	if _, sampled := submissions.countsBySample[contextKindSample]; sampled {
		submissions.countsBySample[contextKindSample]++
		return InvalidContextAlreadySampled
	}
	if len(submissions.sampleOrder) < DistinctContextKindSampleLimit {
		submissions.countsBySample[contextKindSample] = 1
		submissions.sampleOrder = append(submissions.sampleOrder, contextKindSample)
		return InvalidContextSampled
	}
	submissions.countBeyondSampleLimit++
	if submissions.countBeyondSampleLimit == 1 {
		return FirstInvalidContextBeyondSamples
	}
	return InvalidContextBeyondSamples
}

type invalidContextEntry struct {
	count             int
	contextKindSample string
	sampled           bool
}

type invalidContextBatch struct {
	startTime time.Time
	endTime   time.Time
	entries   []invalidContextEntry
}

func (submissions *invalidContextSubmissions) takeBatch(now time.Time) invalidContextBatch {
	submissions.mutex.Lock()
	defer submissions.mutex.Unlock()

	batch := invalidContextBatch{startTime: submissions.startTime, endTime: now, entries: make([]invalidContextEntry, 0, len(submissions.sampleOrder)+1)}
	if len(submissions.sampleOrder) == 0 && submissions.countBeyondSampleLimit == 0 {
		batch.startTime = now
	}
	for _, contextKindSample := range submissions.sampleOrder {
		batch.entries = append(batch.entries, invalidContextEntry{count: submissions.countsBySample[contextKindSample], contextKindSample: contextKindSample, sampled: true})
	}
	if submissions.countBeyondSampleLimit > 0 {
		batch.entries = append(batch.entries, invalidContextEntry{count: submissions.countBeyondSampleLimit})
	}
	submissions.resetLocked()
	return batch
}

func (submissions *invalidContextSubmissions) clear() {
	submissions.mutex.Lock()
	defer submissions.mutex.Unlock()
	submissions.resetLocked()
}

func (submissions *invalidContextSubmissions) resetLocked() {
	clear(submissions.countsBySample)
	submissions.sampleOrder = nil
	submissions.countBeyondSampleLimit = 0
}

func (entry invalidContextEntry) wire() wireInvalidContextEvent {
	event := wireInvalidContextEvent{Reason: invalidContextKindReason}
	if entry.sampled {
		event.KindSample = &entry.contextKindSample
	}
	return event
}

type wireInvalidContextEvent struct {
	Reason     string  `json:"reason"`
	KindSample *string `json:"kindSample,omitempty"`
}
