// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
)

const testEntriesURI = "/redfish/v1/Managers/1/LogServices/1/Entries"

func TestClientContextReadsEveryOpaqueResultPage(t *testing.T) {
	t.Parallel()

	const contextKey = "0123456789abcdef0123456789abcdef"
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		page := r.URL.Query().Get("page")
		memberID := "1"
		next := testEntriesURI + "?page=2"
		if page == "2" {
			memberID = "2"
			next = ""
		}
		writeJSON(w, map[string]any{
			"@odata.id":                    testEntriesURI,
			"@odata.type":                  "#LogEntryCollection.LogEntryCollection",
			"Members@odata.count":          2,
			"Members@odata.nextLink":       next,
			"@Redfish.ClientContext":       contextKey,
			"@Redfish.ClientContextStatus": "Applied",
			"@Redfish.NextEntry":           "opaque-next-entry",
			"Members": []any{map[string]any{
				"@odata.id":    testEntriesURI + "/" + memberID,
				"@odata.type":  "#LogEntry.v1_19_0.LogEntry",
				"Id":           memberID,
				"Name":         "Entry " + memberID,
				"ExactCounter": json.Number("9007199254740993"),
			}},
		})
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	entries, skipped, err := client.fetchClientContextEntries(
		withOperationBudget(context.Background()),
		testEntriesURI,
		contextKey,
		nil,
	)
	require.NoError(t, err)
	assert.Zero(t, skipped)
	assert.Len(t, entries, 2)
	assert.Equal(t, json.Number("9007199254740993"), entries[0]["ExactCounter"])
	assert.Equal(t, int64(2), requests.Load())
}

func TestClientContextRejectsDuplicateSourceMembershipAcrossPages(t *testing.T) {
	t.Parallel()

	const contextKey = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := testEntriesURI + "?page=2"
		if r.URL.Query().Get("page") == "2" {
			next = ""
		}
		writeJSON(w, map[string]any{
			"@odata.id":                    testEntriesURI,
			"@odata.type":                  "#LogEntryCollection.LogEntryCollection",
			"Members@odata.count":          2,
			"Members@odata.nextLink":       next,
			"@Redfish.ClientContext":       contextKey,
			"@Redfish.ClientContextStatus": "Applied",
			"@Redfish.NextEntry":           "opaque-next-entry",
			"Members":                      []any{testLogEntry("1", "2026-07-30T10:00:00Z")},
		})
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	_, _, err := client.fetchClientContextEntries(
		withOperationBudget(context.Background()),
		testEntriesURI,
		contextKey,
		nil,
	)
	require.ErrorContains(t, err, "duplicate source membership")
}

func TestClientContextTreatsODataCountAsCollectionTotalNotWindowSize(t *testing.T) {
	t.Parallel()

	const contextKey = "0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"@odata.id":                    testEntriesURI,
			"@odata.type":                  "#LogEntryCollection.LogEntryCollection",
			"Members@odata.count":          500,
			"@Redfish.ClientContext":       contextKey,
			"@Redfish.ClientContextStatus": "Applied",
			"@Redfish.NextEntry":           "opaque-next-entry",
			"Members":                      []any{testLogEntry("500", "2026-07-30T10:00:00Z")},
		})
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	entries, skipped, err := client.fetchClientContextEntries(
		withOperationBudget(context.Background()),
		testEntriesURI,
		contextKey,
		nil,
	)
	require.NoError(t, err)
	assert.Zero(t, skipped)
	require.Len(t, entries, 1)
	assert.Equal(t, "500", entries[0]["Id"])
}

func TestLogEntryPagePreservesExactNumbersInLosslessJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entry := testLogEntry("1", "2026-07-30T10:00:00Z")
		entry["ExactCounter"] = json.Number("9007199254740993")
		writeJSON(w, logEntryPageDocument(1, []any{entry}, ""))
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	page, err := client.fetchLogEntryPage(
		withOperationBudget(context.Background()),
		testEntriesURI,
		false,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, json.Number("9007199254740993"), page.Entries[0]["ExactCounter"])

	entry, err := client.normalizeLogEntry(
		&graphNode{URI: "/redfish/v1/Managers/1/LogServices/1"},
		page.Entries[0],
		0,
		time.Date(2026, 7, 30, 10, 1, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.Contains(t, entry.Journal.Fields["REDFISH_JSON"], `"ExactCounter":9007199254740993`)
}

func TestSuccessfulLogResponseReportsCompatibilityHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(0, []any{}, ""))
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	stats := &wireStats{failures: make(map[string]int)}

	_, err := client.fetchLogEntryPage(
		withOperationBudget(context.Background()),
		testEntriesURI,
		false,
		stats,
	)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"Redfish compatibility: response OData-Version header is missing",
	}, responseCompatibilityDiagnostics(stats))
}

func TestLogEntryPageFetchesAnnotationOnlyMember(t *testing.T) {
	t.Parallel()

	const entryURI = testEntriesURI + "/1"
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case testEntriesURI:
			writeJSON(w, logEntryPageDocument(1, []any{map[string]any{
				"@odata.id":   entryURI,
				"@odata.type": "#LogEntry.v1_19_0.LogEntry",
			}}, ""))
		case entryURI:
			writeJSON(w, testLogEntry("1", "2026-07-30T10:00:00Z"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	page, err := client.fetchLogEntryPage(
		withOperationBudget(context.Background()),
		testEntriesURI,
		false,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, "test event", page.Entries[0]["Message"])
	assert.Equal(t, int64(2), requests.Load())
}

func TestLogEntryPageUsesFinalRedirectBase(t *testing.T) {
	t.Parallel()

	const (
		finalCollectionURI = "/redfish/v1/redirected/entries/"
		finalEntryURI      = "/redfish/v1/redirected/archive/1"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testEntriesURI:
			http.Redirect(w, r, finalCollectionURI, http.StatusTemporaryRedirect)
		case finalCollectionURI:
			writeJSON(w, map[string]any{
				"@odata.id":              finalCollectionURI,
				"@odata.type":            "#LogEntryCollection.LogEntryCollection",
				"Members@odata.count":    1,
				"Members@odata.nextLink": finalCollectionURI + "page-2",
				"Members": []any{map[string]any{
					"@odata.id":   finalCollectionURI + "1",
					"@odata.type": "#LogEntry.v1_19_0.LogEntry",
				}},
			})
		case finalCollectionURI + "1":
			http.Redirect(w, r, finalEntryURI, http.StatusTemporaryRedirect)
		case finalEntryURI:
			entry := testLogEntry("1", "2026-07-30T10:00:00Z")
			entry["@odata.id"] = finalEntryURI
			writeJSON(w, entry)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	page, err := client.fetchLogEntryPage(
		withOperationBudget(context.Background()), testEntriesURI, false, nil,
	)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, "test event", page.Entries[0]["Message"])
	assert.Equal(t, finalCollectionURI+"page-2", page.Next)
}

func TestClientContextNeverFollowsResultRedirect(t *testing.T) {
	t.Parallel()

	var redirected atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testEntriesURI {
			http.Redirect(w, r, testEntriesURI+"/redirected", http.StatusTemporaryRedirect)
			return
		}
		redirected.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))

	_, _, err := client.fetchClientContextEntries(
		withOperationBudget(context.Background()),
		testEntriesURI,
		"0123456789abcdef0123456789abcdef",
		nil,
	)
	require.Error(t, err)
	assert.Zero(t, redirected.Load())
}

func TestLogPollDoesNotFallbackOrFetchWhenExplicitBackendUnavailable(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.NotFound(w, nil)
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	runtime := redfishruntime.New()
	registration, err := runtime.RegisterBackend(
		"default",
		"default-key",
		t.TempDir(),
		&recordingLogBackend{contains: make(map[string]bool)},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, registration.Close(context.Background()))
	})
	client.setLogRoute(runtime, "isolated")
	node := &graphNode{
		Kind: "log_service",
		Key:  "log-service-key",
		URI:  "/redfish/v1/Managers/1/LogServices/1",
		Data: map[string]any{
			"Entries": map[string]any{"@odata.id": testEntriesURI},
		},
	}
	sourceKey := client.logSourceCursorKey(node.URI)

	err = client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Now(),
		nil,
		false,
	)
	require.ErrorIs(t, err, redfishruntime.ErrBackendUnavailable)
	assert.Zero(t, requests.Load())
	assert.False(t, client.cursor.Source(sourceKey).Initialized)
}

func TestLogServiceAdmissionUsesOnlyLogServiceMembershipCompleteness(t *testing.T) {
	t.Parallel()

	parent := &graphNode{
		Kind:             "manager",
		Key:              "manager",
		AcquisitionState: "readable",
		Data: map[string]any{
			"LogServices": map[string]any{"@odata.id": "/redfish/v1/Managers/1/LogServices"},
		},
	}
	graph := &resourceGraph{
		Complete: false,
		Slices: []graphSlice{
			{
				ParentKey: parent.Key,
				Path:      "LogServices",
				ChildKind: "log_service",
				Complete:  true,
			},
			{
				ParentKey: parent.Key,
				Path:      "EthernetInterfaces",
				ChildKind: "ethernet_interface",
				Complete:  false,
			},
		},
	}

	assert.True(t, logServiceMembershipComplete(graph, []*graphNode{parent}))
	graph.Slices[0].Complete = false
	assert.False(t, logServiceMembershipComplete(graph, []*graphNode{parent}))
}

func TestLogProducerStatePrunesServicesAbsentFromCompleteMembership(t *testing.T) {
	state := logProducerState{}
	state.initialize()
	state.services["present"] = logServiceCounters{Fetched: 1}
	state.services["removed"] = logServiceCounters{Fetched: 2}
	state.clientUnsupported["present"] = true
	state.clientUnsupported["removed"] = true

	state.pruneAbsentLocked(map[string]struct{}{"present": {}})
	require.Contains(t, state.services, "present")
	require.NotContains(t, state.services, "removed")
	require.Contains(t, state.clientUnsupported, "present")
	require.NotContains(t, state.clientUnsupported, "removed")
}

type recordingLogBackend struct {
	mu       sync.Mutex
	appended int
	contains map[string]bool
}

func (b *recordingLogBackend) Append(
	_ context.Context,
	entries []redfishruntime.JournalEntry,
) (redfishruntime.AppendResult, error) {
	b.mu.Lock()
	b.appended += len(entries)
	b.mu.Unlock()
	return redfishruntime.AppendResult{Committed: len(entries)}, nil
}

func (b *recordingLogBackend) Contains(
	_ context.Context,
	keys []string,
) (map[string]bool, error) {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = b.contains[key]
	}
	return result, nil
}

func (b *recordingLogBackend) appendedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.appended
}

func newLogPollClient(
	t *testing.T,
	handler http.Handler,
) (*protocolClient, *recordingLogBackend, *graphNode) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	runtime := redfishruntime.New()
	backend := &recordingLogBackend{contains: make(map[string]bool)}
	registration, err := runtime.RegisterBackend("default", "test-key", t.TempDir(), backend)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, registration.Close(context.Background()))
	})
	client.setEndpointJob("test-redfish")
	client.setLogRoute(runtime, "default")
	node := &graphNode{
		Kind:             "log_service",
		Key:              "log-service-key",
		URI:              "/redfish/v1/Managers/1/LogServices/1",
		Locator:          "/redfish/v1/Managers/1/LogServices/1",
		Data:             map[string]any{"Entries": map[string]any{"@odata.id": testEntriesURI}},
		AcquisitionState: "readable",
		Complete:         true,
		IdentityQuality:  "addressable",
		Parents:          make(map[string]*graphNode),
		RollupParents:    make(map[string]*graphNode),
		SystemOwners:     make(map[string]*graphNode),
	}
	return client, backend, node
}

func logEntryPageDocument(count int, members []any, next string) map[string]any {
	return map[string]any{
		"@odata.id":              testEntriesURI,
		"@odata.type":            "#LogEntryCollection.LogEntryCollection",
		"Members@odata.count":    count,
		"Members@odata.nextLink": next,
		"Members":                members,
	}
}

func testLogEntry(id, created string) map[string]any {
	return map[string]any{
		"@odata.id":   testEntriesURI + "/" + id,
		"@odata.type": "#LogEntry.v1_19_0.LogEntry",
		"Id":          id,
		"Name":        "Entry " + id,
		"EntryType":   "Event",
		"Created":     created,
		"Message":     "test event",
		"Severity":    "OK",
	}
}

func TestLogEntryFallbackIdentityPreservesStructuralFieldBoundaries(t *testing.T) {
	t.Parallel()

	client := &protocolClient{origin: "https://192.0.2.1:443"}
	entry := func(eventID, messageID string) map[string]any {
		return map[string]any{
			"EventId":        eventID,
			"MessageId":      messageID,
			"EventTimestamp": "2026-07-30T10:00:00Z",
			"EntryType":      "Event",
			"GeneratorId":    "manager-1",
		}
	}
	firstKind, firstValue, _, err := client.logEntryIdentity(
		"/redfish/v1/Managers/1/LogServices/1",
		entry("event\x00shifted", "message"),
	)
	require.NoError(t, err)
	secondKind, secondValue, _, err := client.logEntryIdentity(
		"/redfish/v1/Managers/1/LogServices/1",
		entry("event", "shifted\x00message"),
	)
	require.NoError(t, err)
	require.Equal(t, "event_tuple", firstKind)
	require.Equal(t, firstKind, secondKind)
	require.NotEqual(t, firstValue, secondValue)
	require.NotEqual(t,
		client.logEntrySourceKeyFromIdentity("/redfish/v1/Managers/1/LogServices/1", firstKind, firstValue),
		client.logEntrySourceKeyFromIdentity("/redfish/v1/Managers/1/LogServices/1", secondKind, secondValue),
	)
}

func TestLogPollFailsClosedAfterExactEvidenceExpires(t *testing.T) {
	t.Parallel()

	created := "2026-07-30T10:00:00Z"
	client, backend, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(1, []any{testLogEntry("1", created)}, ""))
	}))
	sourceKey := client.logSourceCursorKey(node.URI)
	require.NoError(t, client.cursor.UpdateSource(sourceKey, logSourceCursor{
		Initialized:        true,
		ExactComplete:      false,
		BoundaryUsec:       time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC).UnixMicro(),
		CompleteCountKnown: true,
		LastCompleteCount:  1,
	}, time.Now()))

	err := client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	)
	require.ErrorContains(t, err, "cannot be classified")
	assert.Zero(t, backend.appendedCount())
	state := client.logState.services[node.Key]
	assert.True(t, state.Blocked)
	assert.Equal(t, uint64(1), state.Reconciliation["outside_tail_ambiguous"])
}

func TestLogPollAdvancesGenerationOnlyAfterProvenEmptyClear(t *testing.T) {
	t.Parallel()

	client, backend, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(0, []any{}, ""))
	}))
	sourceKey := client.logSourceCursorKey(node.URI)
	require.NoError(t, client.cursor.UpdateSource(sourceKey, logSourceCursor{
		Initialized:        true,
		ExactComplete:      true,
		BoundaryUsec:       10,
		BoundaryKeys:       []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ExactRecordKeys:    []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		CompleteCountKnown: true,
		LastCompleteCount:  1,
	}, time.Now()))

	require.NoError(t, client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	))
	assert.Zero(t, backend.appendedCount())
	cursor := client.cursor.Source(sourceKey)
	assert.Equal(t, uint64(1), cursor.Generation)
	assert.True(t, cursor.CompleteCountKnown)
	assert.Zero(t, cursor.LastCompleteCount)
	assert.Empty(t, cursor.BoundaryKeys)
	assert.Empty(t, cursor.ExactRecordKeys)
}

func TestLogPollRejectsCountContradictionBeforeJournalOrGenerationAdvance(t *testing.T) {
	t.Parallel()

	client, backend, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(
			0,
			[]any{testLogEntry("unexpected", "2026-07-30T10:00:00Z")},
			"",
		))
	}))
	sourceKey := client.logSourceCursorKey(node.URI)
	require.NoError(t, client.cursor.UpdateSource(sourceKey, logSourceCursor{
		Initialized: true, ExactComplete: true, CompleteCountKnown: true, LastCompleteCount: 1,
	}, time.Now()))

	err := client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	)
	require.ErrorContains(t, err, "observed 1 members, advertised 0")
	assert.Zero(t, backend.appendedCount())
	assert.Zero(t, client.cursor.Source(sourceKey).Generation)
}

func TestMalformedUnidentifiableLogEntryDoesNotBlockValidSibling(t *testing.T) {
	t.Parallel()

	malformed := map[string]any{
		"@odata.type": "#LogEntry.v1_19_0.LogEntry",
		"Name":        "Malformed entry",
		"EntryType":   "Event",
		"Message":     "no identity",
	}
	client, backend, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(
			2,
			[]any{malformed, testLogEntry("valid", "2026-07-30T10:00:00Z")},
			"",
		))
	}))

	require.NoError(t, client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	))
	assert.Equal(t, 1, backend.appendedCount())
	state := client.logState.services[node.Key]
	assert.Equal(t, uint64(1), state.Failed)
	assert.Equal(t, uint64(1), state.Committed)
}

func TestLogPollDetectsPersistedContinuationLoopBeforeRequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	client, _, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.NotFound(w, nil)
	}))
	continuation := testEntriesURI + "?page=2"
	key, err := client.logContinuationKey(continuation, true)
	require.NoError(t, err)
	sourceKey := client.logSourceCursorKey(node.URI)
	require.NoError(t, client.cursor.UpdateSource(sourceKey, logSourceCursor{
		Initialized:       true,
		ExactComplete:     true,
		Mode:              "full",
		ReconcileStarted:  true,
		ReconcileExpected: 2,
		ReconcileFetched:  1,
		Continuation:      continuation,
		ContinuationKeys:  []string{key},
	}, time.Now()))

	err = client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Now(),
		nil,
		false,
	)
	require.ErrorContains(t, err, "pagination loop")
	assert.Zero(t, requests.Load())
}

func TestLogPollRejectsDuplicateSourceMembershipAcrossReconciliationPages(t *testing.T) {
	t.Parallel()

	client, backend, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := testEntriesURI + "?page=2"
		if r.URL.Query().Get("page") == "2" {
			next = ""
		}
		writeJSON(w, logEntryPageDocument(
			2,
			[]any{testLogEntry("1", "2026-07-30T10:00:00Z")},
			next,
		))
	}))

	require.NoError(t, client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	))
	assert.Equal(t, 1, backend.appendedCount())

	err := client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 1, 0, 0, time.UTC),
		nil,
		false,
	)
	require.ErrorContains(t, err, "duplicate source membership")
	assert.Equal(t, 1, backend.appendedCount())
}

func TestLogPollRetainsGenerationWideExactEvidenceAfterFullReconciliation(t *testing.T) {
	t.Parallel()

	client, _, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(
			1,
			[]any{testLogEntry("new", "2026-07-30T10:00:00Z")},
			"",
		))
	}))
	sourceKey := client.logSourceCursorKey(node.URI)
	const priorRecordKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	require.NoError(t, client.cursor.UpdateSource(sourceKey, logSourceCursor{
		Initialized:     true,
		ExactComplete:   true,
		ExactRecordKeys: []string{priorRecordKey},
	}, time.Now()))

	require.NoError(t, client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	))
	cursor := client.cursor.Source(sourceKey)
	assert.True(t, cursor.ExactComplete)
	assert.Contains(t, cursor.ExactRecordKeys, priorRecordKey)
	assert.Len(t, cursor.ExactRecordKeys, 2)
}

func TestLogPollRequiresBackendProofForTimestampLessEntryAfterExactEvidenceExpires(t *testing.T) {
	t.Parallel()

	entry := testLogEntry("1", "2026-07-30T10:00:00Z")
	delete(entry, "Created")
	client, backend, node := newLogPollClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, logEntryPageDocument(1, []any{entry}, ""))
	}))
	sourceKey := client.logSourceCursorKey(node.URI)
	require.NoError(t, client.cursor.UpdateSource(sourceKey, logSourceCursor{
		Initialized:   true,
		ExactComplete: false,
	}, time.Now()))

	err := client.pollLogService(
		withOperationBudget(context.Background()),
		node,
		time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC),
		nil,
		false,
	)
	require.ErrorContains(t, err, "cannot be classified")
	assert.Zero(t, backend.appendedCount())
}
