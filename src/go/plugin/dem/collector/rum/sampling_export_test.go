// SPDX-License-Identifier: GPL-3.0-or-later

package rum_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keep the captured SDK's request events and two spans, with current timestamps
// and one extra custom event that identifies each observation in native history.
func samplingExportBeacon(t *testing.T, session, page string, hasError bool) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../rum/faro/testdata/tracing.json")
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	now := time.Now().UTC()
	meta := body["meta"].(map[string]any)
	meta["session"] = map[string]any{"id": session}
	meta["page"] = map[string]any{"id": page, "url": "https://example.org/" + page}
	events := body["events"].([]any)
	for _, rawEvent := range events {
		event := rawEvent.(map[string]any)
		event["timestamp"] = now.Format(time.RFC3339Nano)
		event["attributes"].(map[string]any)["session.id"] = session
	}
	body["events"] = append(events,
		map[string]any{"name": "document_activated", "timestamp": now.Format(time.RFC3339Nano), "attributes": map[string]string{"observation_id": page, "observation_sequence": "1"}},
		map[string]any{"name": page + "-action", "timestamp": now.Format(time.RFC3339Nano)})
	for _, rawResource := range body["traces"].(map[string]any)["resourceSpans"].([]any) {
		for _, rawScope := range rawResource.(map[string]any)["scopeSpans"].([]any) {
			for _, rawSpan := range rawScope.(map[string]any)["spans"].([]any) {
				span := rawSpan.(map[string]any)
				span["startTimeUnixNano"] = strconv.FormatInt(now.Add(-time.Millisecond).UnixNano(), 10)
				span["endTimeUnixNano"] = strconv.FormatInt(now.UnixNano(), 10)
				for _, rawAttribute := range span["attributes"].([]any) {
					attribute := rawAttribute.(map[string]any)
					if attribute["key"] == "session.id" {
						attribute["value"] = map[string]any{"stringValue": session}
					}
				}
			}
		}
	}
	if hasError {
		body["exceptions"] = []map[string]any{
			{"type": "TypeError", "value": "checkout failed", "timestamp": now.Format(time.RFC3339Nano)},
		}
	}
	raw, err = json.Marshal(body)
	require.NoError(t, err)
	return raw
}

func TestSamplingDetailThroughNativeJobs(t *testing.T) {
	for _, tc := range []struct {
		name                string
		promote, future     bool
		wantLogs, wantSpans int
	}{
		{name: "no overrides"},
		{name: "error promotion exports current only", promote: true, wantLogs: 5, wantSpans: 2},
		{name: "promotion selects future detail", promote: true, future: true, wantLogs: 9, wantSpans: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, logs, traces := contractReceiver(t, false)
			site, hub, store := contractSite(t)
			site.Investigate = &config.Investigate{
				SampleRate: new(float64(0)),
				AlwaysKeep: []string{},
			}
			if tc.promote {
				site.Investigate.AlwaysKeep = []string{config.KeepErrors}
			}
			site.EventLogs.Enabled, site.EventLogs.Destination.Endpoint = true, new(endpoint)
			site.Tracing.Enabled, site.Tracing.Destination.Endpoint = true, new(endpoint)
			recv := receiver.New(hub)
			recv.Listen = "127.0.0.1:0"
			startJob(t, "receiver", "receiver", recv)
			_, _, stop := startJob(t, "rum", "shop", site)
			data, _, release, ok := hub.AcquireSite("shop")
			require.True(t, ok)
			a := data.Aggregator
			release()
			const session = "detail-contract"
			sendContractBeacon(t, hub, samplingExportBeacon(t, session, "before", false))
			sendContractBeacon(t, hub, samplingExportBeacon(t, session, "failure", true))
			observations := 2
			if tc.future {
				sendContractBeacon(t, hub, samplingExportBeacon(t, session, "after", false))
				observations++
			}
			// Retirement drains native history and both export queues before queries.
			stop()
			logs.mu.Lock()
			logCount := logs.count
			logs.mu.Unlock()
			traces.mu.Lock()
			traceCount := traces.count
			traces.mu.Unlock()
			assert.Equal(t, tc.wantLogs, logCount)
			assert.Equal(t, tc.wantSpans, traceCount)
			counters := a.Snapshot().Counters
			assert.EqualValues(t, observations, counters[aggregate.CounterAccepted])
			assert.EqualValues(t, observations, counters[aggregate.CounterPageviews])
			assert.EqualValues(t, 1, counters[aggregate.CounterJSErrors])
			assert.EqualValues(t, tc.wantLogs, counters[aggregate.CounterOTLPSent])
			assert.EqualValues(t, tc.wantSpans, counters[aggregate.CounterSpansSent])
			for _, counter := range []string{aggregate.CounterOTLPDropped, aggregate.CounterOTLPErrors, aggregate.CounterSpansDropped, aggregate.CounterSpansErrors, aggregate.CounterHistoryDropped, aggregate.CounterSamplesDropped, aggregate.CounterInvalidMeasurements} {
				assert.Zero(t, counters[counter], counter)
			}
			live, _ := a.Live(0, 10)
			assert.Len(t, live, observations+1, "document activations and the error have separate live rows, independent of detail sampling")
			documents, errorsLive := 0, 0
			for _, row := range live {
				switch row.Kind {
				case "document":
					documents++
				case "error":
					errorsLive++
				}
			}
			assert.Equal(t, observations, documents)
			assert.Equal(t, 1, errorsLive)
			rows, err := store.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+10, 10)
			require.NoError(t, err)
			timeline, err := store.QuerySessionEvents(context.Background(), "shop", session)
			require.NoError(t, err)
			errors, err := store.QueryErrors(context.Background(), "shop", "", 0, time.Now().Unix()+10)
			require.NoError(t, err)
			if !tc.promote {
				assert.Empty(t, rows)
				assert.Empty(t, timeline)
				assert.Empty(t, errors)
				assert.Zero(t, counters[aggregate.CounterHistoryWritten])
				return
			}
			require.Len(t, rows, 1)
			assert.EqualValues(t, observations, rows[0].Pageviews)
			assert.EqualValues(t, 1, rows[0].Errors)
			require.Len(t, errors, 1)
			assert.Equal(t, "checkout failed", errors[0].Message)
			assert.Equal(t, 1, errors[0].CountWindow)
			// Prior context is retained exactly once with its original page. The
			// smaller gRPC totals above exclude this earlier observation entirely.
			actions := make(map[string]string)
			for _, event := range timeline {
				if event.Type == "event" {
					require.NotContains(t, actions, event.Text, "replay must not duplicate prior context")
					actions[event.Text] = event.Page
				}
			}
			want := map[string]string{"before-action": "/before", "failure-action": "/failure"}
			if tc.future {
				want["after-action"] = "/after"
			}
			assert.Equal(t, want, actions)
			assert.EqualValues(t, len(timeline), counters[aggregate.CounterHistoryWritten], fmt.Sprint(timeline))
		})
	}
}

// Disabled collection retains receiver diagnostics. Enabled quiet collection
// reports an empty measurement population without inventing percentile samples.
func TestDisabledCollectionKeepsDiagnosticsWithoutTrafficCharts(t *testing.T) {
	for _, rate := range []float64{0, 0.25, 1} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			site, hub, _ := contractSite(t)
			site.MeasureSampleRate = new(rate)
			recv := receiver.New(hub)
			recv.Listen = "127.0.0.1:0"
			startJob(t, "receiver", "receiver", recv)
			job, out, stop := startJob(t, "rum", "shop", site)
			tickUntil(t, job, out, "SET 'available' = 1")
			stop()
			if rate == 0 {
				assert.NotContains(t, out.String(), "rum.document_views")
				assert.NotContains(t, out.String(), "rum.observed_sessions")
			} else {
				assert.Contains(t, out.String(), "rum.document_views")
				assert.Contains(t, out.String(), "rum.lcp_population")
				assert.Contains(t, out.String(), "SET 'observed' = 0")
				assert.NotContains(t, out.String(), "'rum.lcp'")
			}
		})
	}
}
