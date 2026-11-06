package configdirector_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ConfigDirector/go-sdks/configdirector"
	"github.com/stretchr/testify/require"
)

func onlyTrackedEventOf(t *testing.T, report telemetryReport) trackedEventEntry {
	t.Helper()
	require.Len(t, report.DiscreteEvents.TrackedEvents, 1)
	return report.DiscreteEvents.TrackedEvents[0]
}

func TestAnEventTrackedForAContextArrivesOnTheNextReportWithItsValueAndDetails(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("checkout-completed", identifiedUser, configdirector.TrackOptions{
		Value:   49.9,
		Details: configdirector.TrackedEventDetails{"plan": "pro", "seats": 3},
	})

	require.Equal(t, trackedEventEntry{
		Name:              "checkout-completed",
		ContextKind:       new("user"),
		ContextIdentifier: new("user-123"),
		Value:             []byte(`49.9`),
		Details:           []byte(`{"plan":"pro","seats":3}`),
	}, withoutHappenedAt(onlyTrackedEventOf(t, reportAfterClosing(t, client, server))))
}

func TestATrackedEventIsStampedWithTheTimeTrackWasCalled(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	before := time.Now().Truncate(time.Millisecond)
	client.Track("checkout-completed", identifiedUser, configdirector.TrackOptions{})
	after := time.Now()
	time.Sleep(20 * time.Millisecond)

	happenedAt, err := time.Parse("2006-01-02T15:04:05.000Z", onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).HappenedAt)
	require.NoError(t, err)
	require.False(t, happenedAt.Before(before), happenedAt)
	require.False(t, happenedAt.After(after), happenedAt)
}

func TestAnEventTrackedWithoutAContextArrivesWithoutOneAndLogsNothing(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("nightly-sync-finished", noUser, configdirector.TrackOptions{})

	entry := onlyTrackedEventOf(t, reportAfterClosing(t, client, server))
	require.Nil(t, entry.ContextKind)
	require.Nil(t, entry.ContextIdentifier)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestAnEventTrackedForAContextWithoutAnIDArrivesWithoutAContextAndLogsNothing(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("nightly-sync-finished", configdirector.UserContext{Kind: "device", Name: "Ada"}, configdirector.TrackOptions{})

	entry := onlyTrackedEventOf(t, reportAfterClosing(t, client, server))
	require.Nil(t, entry.ContextKind)
	require.Nil(t, entry.ContextIdentifier)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestAnEventTrackedForAnAnonymousContextWithholdsItsID(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("checkout-completed", configdirector.UserContext{ID: "user-123", Anonymous: true}, configdirector.TrackOptions{})

	entry := onlyTrackedEventOf(t, reportAfterClosing(t, client, server))
	require.Nil(t, entry.ContextKind)
	require.Nil(t, entry.ContextIdentifier)
}

func TestAnEventTrackedForAContextOfAnotherKindCarriesThatKind(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("export-finished", configdirector.UserContext{ID: "acme", Kind: "tenant"}, configdirector.TrackOptions{})

	entry := onlyTrackedEventOf(t, reportAfterClosing(t, client, server))
	require.Equal(t, new("tenant"), entry.ContextKind)
	require.Equal(t, new("acme"), entry.ContextIdentifier)
}

func TestAnEventTrackedForAnInvalidContextKindIsSentWithoutAContextInTheReportThatCarriesTheKind(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("checkout-completed", proUserWithContextKind("Device"), configdirector.TrackOptions{})

	report := reportAfterClosing(t, client, server)
	entry := onlyTrackedEventOf(t, report)
	require.Nil(t, entry.ContextKind)
	require.Nil(t, entry.ContextIdentifier)
	require.Len(t, report.AggregatedEvents.InvalidContext, 1)
	require.Equal(t, new("Device"), report.AggregatedEvents.InvalidContext[0].Event.KindSample)
	loggedErrors := recordsAt(slog.LevelError, records())
	require.Len(t, loggedErrors, 1)
	require.Equal(t, "Device", attributesOf(loggedErrors[0])["contextKind"])
}

func TestTrackedEventsArriveInCallOrder(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("first", noUser, configdirector.TrackOptions{})
	client.Track("second", noUser, configdirector.TrackOptions{})
	client.Track("first", noUser, configdirector.TrackOptions{})

	entries := reportAfterClosing(t, client, server).DiscreteEvents.TrackedEvents
	require.Len(t, entries, 3)
	require.Equal(t, "first", entries[0].Name)
	require.Equal(t, "second", entries[1].Name)
	require.Equal(t, "first", entries[2].Name)
}

func TestAnEventTrackedBeforeTheClientIsReadyIsStillReported(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t)
	client := newClientAgainst(t, server)

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{})

	require.False(t, client.Ready())
	require.Equal(t, "checkout-completed", onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).Name)
}

func TestAReportWithoutTrackedEventsCarriesAnEmptyList(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.BoolValue("new-checkout", false, noUser)
	require.NoError(t, client.Close())

	recorded := await(t, server.telemetryRequests)
	require.Contains(t, string(recorded.bodyJSON), `"trackedEvents":[]`)
}

func TestAnEventWhoseNameHas150CharactersIsReported(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)
	name := strings.Repeat("n", 150)

	client.Track(name, noUser, configdirector.TrackOptions{})

	require.Equal(t, name, onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).Name)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestANameIsCountedInCharactersRatherThanBytes(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	name := strings.Repeat("😀", 150)

	client.Track(name, noUser, configdirector.TrackOptions{})

	require.Equal(t, name, onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).Name)
}

func TestAnEventWhoseNameHas151CharactersIsDroppedWithAWarning(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track(strings.Repeat("n", 151), noUser, configdirector.TrackOptions{})
	client.BoolValue("new-checkout", false, noUser)

	require.Empty(t, reportAfterClosing(t, client, server).DiscreteEvents.TrackedEvents)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

func TestAnEventWhoseNameIsEmptyIsDroppedWithAWarning(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("", noUser, configdirector.TrackOptions{})
	client.BoolValue("new-checkout", false, noUser)

	require.Empty(t, reportAfterClosing(t, client, server).DiscreteEvents.TrackedEvents)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

func TestAnEventDroppedForItsNameAloneSendsNoReport(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("", noUser, configdirector.TrackOptions{})
	require.NoError(t, client.Close())

	expectNoTelemetry(t, server, 300*time.Millisecond)
}

func TestAThousandEventsTrackedBeforeABatchIsSentAllArrive(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	for range 1000 {
		client.Track("checkout-completed", noUser, configdirector.TrackOptions{})
	}

	require.Len(t, reportAfterClosing(t, client, server).DiscreteEvents.TrackedEvents, 1000)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestTheThousandAndFirstEventOfABatchIsDroppedWithOneWarning(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	for range 1000 {
		client.Track("within-the-limit", noUser, configdirector.TrackOptions{})
	}
	client.Track("beyond-the-limit", noUser, configdirector.TrackOptions{})
	client.Track("beyond-the-limit", noUser, configdirector.TrackOptions{})

	entries := reportAfterClosing(t, client, server).DiscreteEvents.TrackedEvents
	require.Len(t, entries, 1000)
	require.Equal(t, "within-the-limit", entries[999].Name)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

func TestEventsTrackedAfterAFullBatchWasSentArriveOnTheFollowingBatchAndLogAgain(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	logger, records := newLogger()
	options := server.options()
	options.Telemetry.FlushInterval = 300 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)

	for range 1001 {
		client.Track("first-batch", noUser, configdirector.TrackOptions{})
	}
	first := server.telemetryReportWithin(t, firstScheduledFlushTimeout)
	for range 1001 {
		client.Track("second-batch", noUser, configdirector.TrackOptions{})
	}
	second := server.nextTelemetryReport(t)

	require.Len(t, first.DiscreteEvents.TrackedEvents, 1000)
	require.Len(t, second.DiscreteEvents.TrackedEvents, 1000)
	require.Equal(t, "second-batch", second.DiscreteEvents.TrackedEvents[0].Name)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 2)
}

func TestARepeatedCauseLogsOnceUntilTheBatchIsSentAndAgainInTheFollowingBatch(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	logger, records := newLogger()
	options := server.options()
	options.Telemetry.FlushInterval = 300 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)

	client.Track("", noUser, configdirector.TrackOptions{})
	client.Track("", noUser, configdirector.TrackOptions{})
	client.Track("carries-the-batch", noUser, configdirector.TrackOptions{})
	server.telemetryReportWithin(t, firstScheduledFlushTimeout)

	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)

	client.Track("", noUser, configdirector.TrackOptions{})

	require.Len(t, recordsAt(slog.LevelWarn, records()), 2)
}

func TestAnEventTrackedManyTimesFromManyGoroutinesAllArrive(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	var goroutines sync.WaitGroup
	for range 10 {
		goroutines.Go(func() {
			for range 50 {
				client.Track("checkout-completed", identifiedUser, configdirector.TrackOptions{Value: 1})
			}
		})
	}
	goroutines.Wait()

	require.Len(t, reportAfterClosing(t, client, server).DiscreteEvents.TrackedEvents, 500)
}

func trackedValueOf(t *testing.T, value any) string {
	t.Helper()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{Value: value})

	return string(onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).Value)
}

func requireValueLeftOutWithAWarning(t *testing.T, value any) {
	t.Helper()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{Value: value})

	entry := onlyTrackedEventOf(t, reportAfterClosing(t, client, server))
	require.Equal(t, "checkout-completed", entry.Name)
	require.Nil(t, entry.Value)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

type decimalAmount struct {
	digits string
}

func (amount decimalAmount) MarshalText() ([]byte, error) {
	return []byte(amount.digits), nil
}

type refusedAmount struct{}

func (refusedAmount) MarshalText() ([]byte, error) {
	return []byte("49.9"), errors.New("the amount cannot be written")
}

type panickingAmount struct{}

func (panickingAmount) MarshalText() ([]byte, error) {
	panic("the amount panicked")
}

type planTier int

func (planTier) MarshalText() ([]byte, error) {
	return []byte("gold"), nil
}

func TestAnEventTrackedWithoutAValueOrDetailsCarriesNeither(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{})
	require.NoError(t, client.Close())

	body := string(await(t, server.telemetryRequests).bodyJSON)
	require.Contains(t, body, `"name":"checkout-completed"`)
	require.NotContains(t, body, `"value":`)
	require.NotContains(t, body, `"details":`)
}

func TestAWholeNumberValueIsSentAsWritten(t *testing.T) {
	t.Parallel()
	require.Equal(t, `3`, trackedValueOf(t, 3))
}

func TestAValueOfZeroIsSent(t *testing.T) {
	t.Parallel()
	require.Equal(t, `0`, trackedValueOf(t, 0))
}

func TestANegativeValueIsSent(t *testing.T) {
	t.Parallel()
	require.Equal(t, `-12.5`, trackedValueOf(t, -12.5))
}

func TestAFloat32ValueIsSentWithTheDigitsItWasWrittenWith(t *testing.T) {
	t.Parallel()
	require.Equal(t, `0.1`, trackedValueOf(t, float32(0.1)))
}

func TestAVeryLargeFloatValueIsSentWithAnExponent(t *testing.T) {
	t.Parallel()
	require.Equal(t, `1e+300`, trackedValueOf(t, 1e300))
}

func TestAUint64AboveTheInt64RangeIsSentWithEveryDigit(t *testing.T) {
	t.Parallel()
	require.Equal(t, `18446744073709551615`, trackedValueOf(t, uint64(math.MaxUint64)))
}

func TestADurationValueIsSentAsItsNumberOfNanoseconds(t *testing.T) {
	t.Parallel()
	require.Equal(t, `1500000000`, trackedValueOf(t, 1500*time.Millisecond))
}

func TestANumberTypeThatWritesItselfAsTextIsSentAsItsNumber(t *testing.T) {
	t.Parallel()
	require.Equal(t, `3`, trackedValueOf(t, planTier(3)))
}

func TestAJSONNumberValueIsSentWithEveryDigit(t *testing.T) {
	t.Parallel()
	require.Equal(t, `49.90`, trackedValueOf(t, json.Number("49.90")))
}

func TestAJSONNumberWithMoreDigitsThanAFloatHoldsIsSentWithoutRounding(t *testing.T) {
	t.Parallel()
	require.Equal(t, `12345678901234567890.123456789`, trackedValueOf(t, json.Number("12345678901234567890.123456789")))
}

func TestABigIntValueIsSentWithEveryDigit(t *testing.T) {
	t.Parallel()
	value, parsed := new(big.Int).SetString("123456789012345678901234567890", 10)
	require.True(t, parsed)

	require.Equal(t, `123456789012345678901234567890`, trackedValueOf(t, value))
}

func TestABigFloatValueIsSentAsANumber(t *testing.T) {
	t.Parallel()
	require.Equal(t, `49.5`, trackedValueOf(t, big.NewFloat(49.5)))
}

func TestADecimalLibraryTypeIsSentWithEveryDigit(t *testing.T) {
	t.Parallel()
	require.Equal(t, `49.90`, trackedValueOf(t, decimalAmount{digits: "49.90"}))
}

func TestAWholeBigRatValueIsSentAsANumber(t *testing.T) {
	t.Parallel()
	require.Equal(t, `3`, trackedValueOf(t, big.NewRat(3, 1)))
}

func TestANaNValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, math.NaN())
}

func TestAnInfiniteValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, math.Inf(-1))
}

func TestAnInfiniteFloat32ValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, float32(math.Inf(1)))
}

func TestABooleanValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, true)
}

func TestAStringValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, "49.9")
}

func TestAPointerToANumberIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, new(49.9))
}

func TestASliceValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, []int{1})
}

func TestAJSONNumberThatIsNotANumberIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, json.Number("forty"))
}

func TestAnEmptyJSONNumberIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, json.Number(""))
}

func TestAJSONNumberWithLeadingSpaceIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, json.Number(" 5"))
}

func TestAJSONNumberWithTrailingSpaceIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, json.Number("5 "))
}

func TestAJSONNumberHoldingAnotherJSONDocumentIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, json.Number("[1]"))
}

func TestAJSONNumberWithDigitsThatDoNotFormANumberIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, json.Number("1-2"))
}

func TestABigRatWithAFractionIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, big.NewRat(499, 10))
}

func TestANilBigIntIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	var value *big.Int
	requireValueLeftOutWithAWarning(t, value)
}

func TestATimeValueIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, time.Date(2026, time.November, 6, 0, 0, 0, 0, time.UTC))
}

func TestAValueThatCannotWriteItselfIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, refusedAmount{})
}

func TestAValueThatPanicsWritingItselfIsLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireValueLeftOutWithAWarning(t, panickingAmount{})
}

func trackedDetailsOf(t *testing.T, details configdirector.TrackedEventDetails) string {
	t.Helper()
	client, server := newReadyClientAgainst(t, standardPayload())

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{Details: details})

	return string(onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).Details)
}

func requireDetailsLeftOutWithAWarning(t *testing.T, details configdirector.TrackedEventDetails) {
	t.Helper()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{Details: details})

	entry := onlyTrackedEventOf(t, reportAfterClosing(t, client, server))
	require.Equal(t, "checkout-completed", entry.Name)
	require.Nil(t, entry.Details)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

func detailsWithKeys(count int) configdirector.TrackedEventDetails {
	details := configdirector.TrackedEventDetails{}
	for index := range count {
		details["key"+strconv.Itoa(index)] = index
	}
	return details
}

type panickingDetail struct{}

func (panickingDetail) MarshalJSON() ([]byte, error) {
	panic("the detail panicked")
}

type malformedDetail struct{}

func (malformedDetail) MarshalJSON() ([]byte, error) {
	return []byte(`{"unterminated":`), nil
}

func TestDetailsOfExactly1024BytesAreSentInTheBytesTheyWereMeasuredIn(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("x", 1016)

	require.Equal(t, `{"k":"`+text+`"}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"k": text}))
}

func TestDetailsOf1025BytesAreLeftOutWithAWarningAndTheEventIsSent(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"k": strings.Repeat("x", 1017)})
}

func TestDetailsHoldingHTMLCharactersAreSentInTheBytesTheyWereMeasuredIn(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("<>&", 338) + "<>"

	require.Equal(t, `{"k":"`+text+`"}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"k": text}))
}

func TestDetailsHoldingNonASCIITextAreMeasuredInBytesWhenTheyFit(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("é", 508)

	require.Equal(t, `{"k":"`+text+`"}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"k": text}))
}

func TestDetailsHoldingNonASCIITextAreMeasuredInBytesWhenTheyDoNotFit(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"k": strings.Repeat("é", 509)})
}

func TestDetailsHoldingTheMostValuesThatFitTheSizeLimitAreSent(t *testing.T) {
	t.Parallel()
	numbers := make([]any, 508)
	for index := range numbers {
		numbers[index] = 1
	}

	require.Len(t, trackedDetailsOf(t, configdirector.TrackedEventDetails{"k": numbers}), 1023)
}

func TestDetailsHoldingMoreValuesThanCouldFitAreLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"k": make([]any, 1000)})
}

func TestDetailsNestedAsDeepAsTheSizeLimitAllowsAreSent(t *testing.T) {
	t.Parallel()
	var nested any = []any{}
	for range 508 {
		nested = []any{nested}
	}

	require.Len(t, trackedDetailsOf(t, configdirector.TrackedEventDetails{"k": nested}), 1024)
}

func TestDetailsWith20TopLevelKeysAreSent(t *testing.T) {
	t.Parallel()
	var sent map[string]any

	require.NoError(t, json.Unmarshal([]byte(trackedDetailsOf(t, detailsWithKeys(20))), &sent))

	require.Len(t, sent, 20)
}

func TestDetailsWith21TopLevelKeysAreLeftOutWithAWarningAndTheEventIsSent(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, detailsWithKeys(21))
}

func TestEmptyDetailsAreSentAsAnEmptyObject(t *testing.T) {
	t.Parallel()
	require.Equal(t, `{}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{}))
}

func TestDetailsHoldNestedObjectsAndArrays(t *testing.T) {
	t.Parallel()

	sent := trackedDetailsOf(t, configdirector.TrackedEventDetails{"cart": map[string]any{"items": []any{"book", 2, 1.5, true, []string{"gift"}}}})

	require.Equal(t, `{"cart":{"items":["book",2,1.5,true,["gift"]]}}`, sent)
}

func TestDetailsHoldAnythingEncodingJSONCanWrite(t *testing.T) {
	t.Parallel()
	type plan struct {
		Name string `json:"name"`
	}

	sent := trackedDetailsOf(t, configdirector.TrackedEventDetails{
		"plan":  plan{Name: "pro"},
		"since": time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC),
	})

	require.Equal(t, `{"plan":{"name":"pro"},"since":"2026-11-06T12:00:00Z"}`, sent)
}

func TestNaNInDetailsIsSentAsNull(t *testing.T) {
	t.Parallel()
	require.Equal(t, `{"score":null}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"score": math.NaN()}))
}

func TestInfinityInDetailsIsSentAsNull(t *testing.T) {
	t.Parallel()
	require.Equal(t, `{"score":null}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"score": math.Inf(1)}))
}

func TestAnInfiniteFloat32InDetailsIsSentAsNull(t *testing.T) {
	t.Parallel()
	require.Equal(t, `{"score":null}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"score": float32(math.Inf(-1))}))
}

func TestNaNInANestedMapIsSentAsNull(t *testing.T) {
	t.Parallel()
	require.Equal(t, `{"cart":{"total":null}}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"cart": map[string]any{"total": math.NaN()}}))
}

func TestNaNInASliceOfAnyIsSentAsNull(t *testing.T) {
	t.Parallel()
	require.Equal(t, `{"scores":[1,null]}`, trackedDetailsOf(t, configdirector.TrackedEventDetails{"scores": []any{1, math.NaN()}}))
}

func TestNaNInASliceOfFloatsLeavesTheDetailsOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"scores": []float64{math.NaN()}})
}

func TestNaNInAStructFieldLeavesTheDetailsOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"score": struct{ Total float64 }{Total: math.NaN()}})
}

func TestAChannelInDetailsLeavesThemOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"results": make(chan int)})
}

func TestADetailThatPanicsWritingItselfLeavesTheDetailsOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"amount": panickingDetail{}})
}

func TestADetailThatWritesMalformedJSONLeavesTheDetailsOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"amount": malformedDetail{}})
}

func TestDetailsHoldingThemselvesAreLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	details := configdirector.TrackedEventDetails{}
	details["self"] = details

	requireDetailsLeftOutWithAWarning(t, details)
}

func TestDetailsHoldingThemselvesManyTimesOverAreLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	details := configdirector.TrackedEventDetails{}
	details["first"] = details
	details["second"] = details
	details["third"] = details

	requireDetailsLeftOutWithAWarning(t, details)
}

func TestASliceHoldingItselfLeavesTheDetailsOutWithAWarning(t *testing.T) {
	t.Parallel()
	items := []any{nil}
	items[0] = items

	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"items": items})
}

func TestDetailsNestedFarBeyondTheSizeLimitAreLeftOutWithAWarning(t *testing.T) {
	t.Parallel()
	var nested any = "bottom"
	for range 100_000 {
		nested = map[string]any{"next": nested}
	}

	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"nested": nested})
}

func TestChangingDetailsAfterTrackDoesNotChangeWhatIsSent(t *testing.T) {
	t.Parallel()
	client, server := newReadyClientAgainst(t, standardPayload())
	items := []any{"book"}
	details := configdirector.TrackedEventDetails{"plan": "pro", "items": items}

	client.Track("checkout-completed", noUser, configdirector.TrackOptions{Details: details})
	details["plan"] = "free"
	items[0] = "pen"

	require.Equal(t, `{"items":["book"],"plan":"pro"}`, string(onlyTrackedEventOf(t, reportAfterClosing(t, client, server)).Details))
}

func TestEachCauseOfOneBatchLogsItsOwnWarning(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("", noUser, configdirector.TrackOptions{})
	client.Track("", noUser, configdirector.TrackOptions{})
	client.Track("bad-value", noUser, configdirector.TrackOptions{Value: math.NaN()})
	client.Track("bad-value", noUser, configdirector.TrackOptions{Value: math.NaN()})
	client.Track("bad-details", noUser, configdirector.TrackOptions{Details: detailsWithKeys(21)})
	client.Track("bad-details", noUser, configdirector.TrackOptions{Details: detailsWithKeys(21)})

	require.Len(t, recordsAt(slog.LevelWarn, records()), 3)
}

func TestAnEventBeyondTheBatchLimitLogsOnlyTheLimitEvenWhenItsValueWouldBeLeftOut(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	for range 1000 {
		client.Track("within-the-limit", noUser, configdirector.TrackOptions{})
	}
	client.Track("beyond-the-limit", noUser, configdirector.TrackOptions{Value: math.NaN()})

	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
}

func TestTrackDoesNothingInOfflineMode(t *testing.T) {
	t.Parallel()
	logger, records := newLogger()
	options := offlineValues(map[string]any{"new-checkout": true})
	options.Logger = logger
	client := newClient(t, options)

	client.Track("", proUserWithContextKind("Device"), configdirector.TrackOptions{Value: math.NaN(), Details: detailsWithKeys(21)})

	require.Empty(t, recordsAt(slog.LevelWarn, records()))
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestTrackDoesNothingAfterClose(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)
	require.NoError(t, client.Close())

	client.Track("", proUserWithContextKind("Device"), configdirector.TrackOptions{Value: math.NaN(), Details: detailsWithKeys(21)})

	expectNoTelemetry(t, server, 300*time.Millisecond)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

func TestAnEventTrackedAfterATelemetryRefusalIsNotSentAndLogsNoWarning(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.answerTelemetryWith(http.StatusUnauthorized)
	client, records := newLoggedClient(t, server)
	waitForReady(t, client)

	client.Track("refused", noUser, configdirector.TrackOptions{})
	awaitFor(t, server.telemetryRequests, firstScheduledFlushTimeout)
	require.Eventually(t, func() bool { return len(recordsAt(slog.LevelError, records())) == 1 }, awaitTimeout, 10*time.Millisecond)
	expectNoTelemetry(t, server, 100*time.Millisecond)

	client.Track("", noUser, configdirector.TrackOptions{})
	client.Track("after-the-refusal", noUser, configdirector.TrackOptions{})
	require.NoError(t, client.Close())

	expectNoTelemetry(t, server, 200*time.Millisecond)
	require.Empty(t, recordsAt(slog.LevelWarn, records()))
}

func TestAReportTooLargeForConfigDirectorIsDiscardedWithAWarningAndTelemetryContinues(t *testing.T) {
	t.Parallel()
	server := startSDKServer(t, standardPayload().json())
	server.answerTelemetryWith(http.StatusRequestEntityTooLarge)
	logger, records := newLogger()
	options := server.options()
	options.Telemetry.FlushInterval = 100 * time.Millisecond
	options.Logger = logger
	client := newClient(t, options)
	waitForReady(t, client)

	client.Track("too-large", noUser, configdirector.TrackOptions{})
	refused := awaitFor(t, server.telemetryRequests, firstScheduledFlushTimeout)
	server.answerTelemetryWith(http.StatusNoContent)
	client.Track("accepted", noUser, configdirector.TrackOptions{})
	second := server.nextTelemetryReport(t)

	require.Contains(t, string(refused.bodyJSON), `"name":"too-large"`)
	require.Equal(t, "accepted", onlyTrackedEventOf(t, second).Name)
	require.Len(t, recordsAt(slog.LevelWarn, records()), 1)
	require.Empty(t, recordsAt(slog.LevelError, records()))
}

type amountInCents int64

func (amount amountInCents) MarshalText() ([]byte, error) {
	return []byte(strconv.Itoa(int(amount)/100) + "." + strconv.Itoa(int(amount)%100)), nil
}

func TestANumberTypeThatWritesItselfAsANumberIsSentAsItWrites(t *testing.T) {
	t.Parallel()
	require.Equal(t, `49.90`, trackedValueOf(t, amountInCents(4990)))
}

func TestADetailThatWritesInvalidUTF8LeavesTheDetailsOutWithAWarning(t *testing.T) {
	t.Parallel()
	requireDetailsLeftOutWithAWarning(t, configdirector.TrackedEventDetails{"amount": json.RawMessage("\"\xff\"")})
}

func TestAnInvalidUTF8StringInDetailsIsSentAsTheReplacementCharacter(t *testing.T) {
	t.Parallel()
	var sent map[string]string

	require.NoError(t, json.Unmarshal([]byte(trackedDetailsOf(t, configdirector.TrackedEventDetails{"note": "\xff"})), &sent))

	require.Equal(t, map[string]string{"note": "\ufffd"}, sent)
}
