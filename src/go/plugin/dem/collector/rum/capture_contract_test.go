// SPDX-License-Identifier: GPL-3.0-or-later
package rum_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureNormalizationThroughNativeHistory(t *testing.T) {
	ctx := context.Background()
	site, hub, store := contractSite(t)
	// Saved inactive credentials still participate in early normalization.
	site.EventLogs.Destination.AuthToken = "redact-me"
	recv := receiver.New(hub)
	recv.Listen = "127.0.0.1:0"
	startJob(t, "receiver", "receiver", recv)
	job, out, stop := startJob(t, "rum", "shop", site)
	const session = "capture-contract"
	const numericID = "123456"
	const uuidID = "550e8400-e29b-41d4-a716-446655440000"
	const page = "/users/:id"
	users := []string{numericID, "", uuidID}
	views := []string{"/users/123456", "/users/987654", "/users/654321"}
	stages := []string{"signed-in-action", "logged-out-action", "second-user-action"}
	previous := "/home"
	for i, view := range views {
		viewID := fmt.Sprintf("view-%d", i+1)
		events := []map[string]any{
			{"name": "view_changed", "attributes": map[string]string{"fromView": previous, "toView": view, "observation_id": viewID, "observation_sequence": fmt.Sprint(i*10 + 2)}},
			{"name": "session_start", "attributes": map[string]string{"observation_id": fmt.Sprintf("start-%d", i), "observation_sequence": "1"}},
			{"name": "session_extend", "attributes": map[string]string{"observation_id": fmt.Sprintf("extend-%d", i), "observation_sequence": "2"}},
			{"name": stages[i], "attributes": map[string]string{"unused": "redact-me"}},
		}
		body := map[string]any{
			"meta": map[string]any{
				"page":    map[string]any{"id": "same-document", "url": "https://example.org/users/123456?private=redact-me#fragment"},
				"view":    map[string]any{"name": view, "id": viewID},
				"session": map[string]any{"id": session},
				"user":    map[string]any{"id": users[i]},
				"browser": map[string]any{"name": "Chrome", "os": "Linux"},
			},
		}
		if i < 2 {
			body["exceptions"] = []any{map[string]any{
				"type": "TypeError", "value": "checkout redact-me failed",
				"stacktrace": map[string]any{"frames": []any{map[string]any{
					"filename": "https://example.org/app.js?request=" + views[i] + "#redact-me",
					"function": "checkout-redact-me", "lineno": 12, "colno": 3,
				}}},
			}}
		}
		if i == 0 {
			body["measurements"] = []any{map[string]any{"type": "web-vitals", "values": map[string]any{"lcp": 4200}, "context": map[string]any{"id": "lcp-metric", "observation_sequence": "3", "element": "#account-123456"}}}
			events = append(events,
				map[string]any{"name": "document_activated", "attributes": map[string]string{"observation_id": "same-document", "observation_sequence": "1"}},
				map[string]any{"name": "faro.performance.navigation", "attributes": map[string]string{"observation_id": "navigation", "observation_sequence": "4", "pageLoadTime": "900", "domContentLoadHandlerTime": "7"}},
				map[string]any{"name": "faro.performance.resource", "attributes": map[string]string{"observation_id": "resource-1", "observation_sequence": "5", "name": "https://example.org/api?token=redact-me", "httpHost": "example.org", "duration": "25", "transferSize": "100", "initiatorType": "fetch"}},
				map[string]any{"name": "faro.performance.resource", "attributes": map[string]string{"observation_id": "resource-2", "observation_sequence": "6", "name": "https://cdn.example.net/app.js?token=redact-me", "httpHost": "cdn.example.net", "duration": "50", "transferSize": "200", "initiatorType": "script"}},
			)
		}
		body["events"] = events
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		sendContractBeacon(t, hub, raw)
		previous = view
	}
	tickUntil(t, job, out, "SET 'p75' = 4200")
	data, _, release, ok := hub.AcquireSite("shop")
	require.True(t, ok)
	snapshot := data.Aggregator.Snapshot()
	release()
	assert.EqualValues(t, 3, snapshot.Counters[aggregate.CounterAccepted])
	assert.EqualValues(t, 1, snapshot.Counters[aggregate.CounterPageviews], "one document/group, with explicit route events retained separately")
	assert.EqualValues(t, 3, snapshot.Counters[aggregate.CounterApplicationViews])
	assert.Zero(t, snapshot.Counters[aggregate.CounterInvalidMeasurements])
	require.Len(t, snapshot.ErrorGroups, 1, "query and fragment changes must not split the error fingerprint")
	assert.EqualValues(t, 2, snapshot.ErrorGroups[0].Count)
	assert.Equal(t, "checkout [REDACTED] failed", snapshot.ErrorGroups[0].Message)
	assert.Equal(t, float64(900), snapshot.Load.P75)
	assert.Equal(t, float64(7), snapshot.DCL.P75)
	assert.Equal(t, float64(25), snapshot.API.P75)
	assert.EqualValues(t, 1, snapshot.FirstPartyResources)
	assert.EqualValues(t, 1, snapshot.ThirdPartyResources)
	service := query.New(hub, store)
	pages, err := service.Pages(ctx, "shop")
	require.NoError(t, err)
	require.Len(t, pages, 1)
	assert.Equal(t, page, pages[0].Page)
	assert.Equal(t, 1, pages[0].Vitals[beacon.LCP].N)
	assert.Zero(t, pages[0].Vitals[beacon.LCP].Lost)
	assert.Equal(t, float64(4200), pages[0].Vitals[beacon.LCP].P75)
	assert.Equal(t, "#account-:id", pages[0].LCPElement)
	assert.False(t, pages[0].FrustrationsKnown)
	active, err := service.SessionEvents(ctx, "shop", session)
	require.NoError(t, err)
	checkCaptureTimeline(t, active, page, stages, users)
	stop() // Drain and retire: subsequent results cannot depend on a live redactor.
	retained, err := service.SessionEvents(ctx, "shop", session)
	require.NoError(t, err)
	assert.Equal(t, active, retained)
	checkCaptureTimeline(t, retained, page, stages, users)
	before := time.Now().Unix() + 10
	for _, id := range []string{numericID, uuidID} {
		rows, err := store.QuerySessions(ctx, "shop", id, 0, before, 10)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, session, rows[0].SessionID)
		assert.ElementsMatch(t, []string{numericID, uuidID}, rows[0].UserIDs)
		assert.EqualValues(t, 2, rows[0].Errors)
	}
	for _, id := range []string{":id", "123", "unknown-user"} {
		rows, err := store.QuerySessions(ctx, "shop", id, 0, before, 10)
		require.NoError(t, err)
		assert.Empty(t, rows, "exact identity membership only")
	}
	errors, err := store.QueryErrors(ctx, "shop", "", 0, before)
	require.NoError(t, err)
	require.Len(t, errors, 1)
	assert.Equal(t, snapshot.ErrorGroups[0].Fingerprint, errors[0].Fingerprint)
	assert.Equal(t, 2, errors[0].CountWindow)
	assert.Equal(t, "checkout [REDACTED] failed", errors[0].Message)
	assert.Equal(t, "checkout-[REDACTED] (https://example.org/app.js:12:3)\n", errors[0].SampleStack)
}

func checkCaptureTimeline(t *testing.T, events []query.SessionEvent, page string, stages, users []string) {
	t.Helper()
	var views []string
	actions := make(map[string]string)
	for _, event := range events {
		assert.Equal(t, page, event.Page)
		assert.NotContains(t, event.Text, "redact-me")
		assert.NotContains(t, event.Text, "session_start")
		assert.NotContains(t, event.Text, "session_extend")
		assert.NotContains(t, event.Text, "faro.performance")
		switch event.Type {
		case "view":
			assert.Equal(t, page, event.Text)
			views = append(views, event.UserID)
		case "event":
			actions[event.Text] = event.UserID
		}
	}
	assert.Equal(t, users, views, "all explicit transitions survive equal normalized labels, with original attribution")
	assert.Equal(t, map[string]string{stages[0]: users[0], stages[1]: users[1], stages[2]: users[2]}, actions)
}
