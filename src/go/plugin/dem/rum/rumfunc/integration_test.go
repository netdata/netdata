// SPDX-License-Identifier: GPL-3.0-or-later

package rumfunc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type measurementProcessor struct{ *aggregate.Aggregator }

// discardCounters ignores history writer accounting, which these reads do not check.
type discardCounters struct{}

func (discardCounters) Add(string, uint64) {}

func (p measurementProcessor) Ingest(b *beacon.Beacon) { p.Aggregator.Ingest(b) }

func addSite(t *testing.T, hub *rumregistry.Registry, key, generation string) (*aggregate.Aggregator, func()) {
	t.Helper()
	a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
		Investigate: aggregate.InvestigateCfg{
			Rate: 1,
		},
		Name:       key,
		PageGroups: 20,
		Countries:  20,
	})
	cfg := config.Site{
		Name:           key,
		DisplayName:    "site opaque-secret",
		AllowedOrigins: []string{"https://example.org"},
	}
	route := httpapi.NewRoute(cfg, measurementProcessor{a}, redact.NewRedactor("opaque-secret"))
	retire, err := hub.Register(
		key,
		&rumregistry.Site{
			Route:      route,
			Aggregator: a,
			Generation: generation,
		},
	)
	require.NoError(t, err)
	t.Cleanup(retire)
	return a, retire
}

func page(a *aggregate.Aggregator, site, path string) {
	a.Ingest(&beacon.Beacon{
		Site:         site,
		ExperienceID: path,
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: path, Revision: 1}},
		SessionID:    path,
		PageGroup:    path,
	})
}

func TestFunctionRowsHaveSiteScopedIdentities(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	retained := history.NewStore(journalStore)
	sites := rumregistry.New()
	fingerprint := "a1b2c3d4e5f6"
	for _, site := range []string{"a", "b"} {
		owner, _ := addSite(t, sites, site, "first")
		page(owner, site, "/shared")
		owner.Snapshot()
		attempted, err := retained.AppendEvent(
			ctx,
			history.EventRecord{
				Site:        site,
				SessionID:   "shared-session",
				ObservedUS:  time.Now().UnixMicro(),
				Type:        "error",
				Page:        "/shared",
				Fingerprint: fingerprint,
				ErrorType:   "TypeError",
				Message:     "failed",
			},
		)
		require.True(t, attempted)
		require.NoError(t, err)
	}
	handler := rumfunc.New(query.New(sites, retained), nil)
	for method, field := range map[string]string{"rum-errors": "fingerprint", "rum-pages": "page", "rum-sessions": "session_id"} {
		t.Run(method, func(t *testing.T) {
			response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
				Method:      method,
				Permissions: "0x1b",
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 2)
			columns := response.RawResponse["columns"].(map[string]any)
			keyIndex := -1
			for _, value := range columns {
				column := value.(map[string]any)
				if unique, _ := column["unique_key"].(bool); unique {
					require.Equal(t, -1, keyIndex, "one collision-safe row identity")
					keyIndex = column["index"].(int)
				}
			}
			require.NotEqual(t, -1, keyIndex)
			assert.NotEqual(
				t,
				rows[0][keyIndex],
				rows[1][keyIndex],
				"identical site-local values must identify different rows",
			)
			valueIndex := columns[field].(map[string]any)["index"].(int)
			assert.Equal(t, rows[0][valueIndex], rows[1][valueIndex], "the original filter value remains unchanged")
		})
	}
}

func TestFunctionsExposeRetainedIdentityAfterLogoutAndRetirement(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	retained := history.NewStore(journalStore)
	sites := rumregistry.New()
	owner, retire := addSite(t, sites, "shop", "first")
	writer := history.NewWriter(retained, discardCounters{})
	owner.SetHistorySink(writer)
	// A shared SDK session can contain login, logout, and another login.
	ids := []string{"550e8400-e29b-41d4-a716-446655440000", "", "123456"}
	now := time.Now().Add(-time.Minute)
	for i, id := range ids {
		owner.Ingest(&beacon.Beacon{
			Site:         "shop",
			ExperienceID: "document",
			SessionID:    "shared",
			UserID:       id,
			PageGroup:    "/checkout",
			Received:     now.Add(time.Duration(i) * time.Second),
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}, {Name: "milestone"}},
		})
	}
	handler := rumfunc.New(query.New(sites, retained), nil)
	timeline := func() [][]any {
		response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method:      "rum-session-events",
			Args:        []string{"site:shop", "session_id:shared"},
			Permissions: "0x1b",
		})
		require.NotNil(t, response.RawResponse)
		return response.RawResponse["data"].([][]any)
	}
	pending := timeline()
	// Only the explicit document activation creates a document view.
	require.Len(t, pending, 4)
	assert.Equal(t, ids[0], pending[0][5])
	for i, id := range ids {
		assert.Equal(t, id, pending[i+1][5])
	}
	flushCtx, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flushCtx)
	assert.Equal(t, pending, timeline(), "journal/live overlap includes event attribution")
	retire()
	assert.Equal(t, pending, timeline(), "history remains attributable without a running site")
	for _, id := range []string{ids[0], ids[2]} {
		response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method:      "rum-sessions",
			Args:        []string{"site:shop", "user_id:" + id},
			Permissions: "0x1b",
		})
		require.NotNil(t, response.RawResponse)
		rows := response.RawResponse["data"].([][]any)
		require.Len(t, rows, 1)
		assert.Equal(t, []string{ids[2], ids[0]}, rows[0][12])
		assert.EqualValues(t, 1, rows[0][4])
		columns := response.RawResponse["columns"].(map[string]any)
		assert.Contains(t, columns, "user_ids")
		assert.NotContains(t, columns, "user")
	}
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "rum-sessions",
		Args:        []string{"site:shop", "user_id:123"},
		Permissions: "0x1b",
	})
	require.NotNil(t, response.RawResponse)
	assert.Empty(t, response.RawResponse["data"])
}

func TestMeasurementIdentitySurvivesLiveHistoryAndFunctions(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := history.NewStore(journalStore)
	hub := rumregistry.New()
	owner, retire := addSite(t, hub, "shop", "first")
	writer := history.NewWriter(store, discardCounters{})
	owner.SetHistorySink(writer)
	service := query.New(hub, store)
	handler := rumfunc.New(service, nil)
	now := time.Now()
	b := &beacon.Beacon{
		Site:         "shop",
		Received:     now,
		SessionID:    "session",
		ExperienceID: "opaque-secret-document",
		PageGroup:    "/entry",
		View:         "checkout",
		ViewID:       "view-one",
		Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
		Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "opaque-secret-metric", Revision: 1, Value: 0.3}},
	}
	owner.Ingest(b)
	owner.Ingest(b)
	b.Events = []beacon.Event{{Kind: beacon.EventView, ID: "view-two", Revision: 1, Attrs: map[string]string{"toView": "payment"}}}
	b.View, b.ViewID = "payment", "view-two"
	b.Vitals[0].Revision, b.Vitals[0].Value = 2, 0.5
	owner.Ingest(b)
	pages, err := service.Pages(ctx, "shop", "")
	require.NoError(t, err)
	require.Len(t, pages, 1)
	require.Equal(t, "/entry", pages[0].Page)
	require.Equal(t, 1, pages[0].PageviewsWindow)
	require.Equal(t, 1, pages[0].Sessions)
	require.Equal(t, 1, pages[0].Vitals[beacon.CLS].N)
	require.Equal(t, 0.5, pages[0].Vitals[beacon.CLS].P75)
	sites, err := service.Sites(ctx, "")
	require.NoError(t, err)
	require.Equal(t, 1, sites[0].Activity.ObservedSessions)
	require.EqualValues(t, 1, sites[0].Activity.ApplicationViewsWindow)
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "rum-live",
		Permissions: "0x1b",
	}).RawResponse
	require.NotNil(t, response)
	columns := response["columns"].(map[string]any)
	value := func(row []any, key string) any { return row[columns[key].(map[string]any)["index"].(int)] }
	var updates []uint64
	for _, row := range response["data"].([][]any) {
		require.Equal(t, "opaque-secret-document", value(row, "experience_id"), "measurement IDs are not credential-masked display text")
		if value(row, "kind") == "vital" {
			require.Equal(t, "opaque-secret-metric", value(row, "metric_id"))
			require.Equal(t, 0, value(row, "pageview"))
			updates = append(updates, value(row, "revision").(uint64))
		}
	}
	require.Equal(t, []uint64{1, 2}, updates)
	pending, err := service.SessionEvents(ctx, "shop", "session", "")
	require.NoError(t, err)
	flush, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flush)
	persisted, err := service.SessionEvents(ctx, "shop", "session", "")
	require.NoError(t, err)
	require.Equal(t, pending, persisted, "journal overlap must include all identity fields")
	retire()
	afterRetirement, err := service.SessionEvents(ctx, "shop", "session", "")
	require.NoError(t, err)
	require.Equal(t, pending, afterRetirement)
	sessions, err := service.Sessions(ctx, "shop", "", 0, now.Unix()+5, 2000, "")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.EqualValues(t, 1, sessions[0].Pageviews)
	require.EqualValues(t, 1, sessions[0].ApplicationViews)
}

func TestPromotionPreservesReceiptTimeThroughJournalAndFunction(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	store := history.NewStore(journalStore)
	owner := aggregate.New(time.Hour, aggregate.SiteCfg{
		Name: "shop", PageGroups: 20, Countries: 20,
		Investigate: aggregate.InvestigateCfg{Rate: 0, KeepErrors: true},
	})
	writer := history.NewWriter(store, discardCounters{})
	owner.SetHistorySink(writer)
	observed := time.Now().Add(-20 * time.Minute).Truncate(time.Second)
	prior := &beacon.Beacon{
		Site: "shop", SessionID: "promoted", ExperienceID: "document", Received: observed,
		PageGroup: "/prior", UserID: "prior-user",
		Events: []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1, Time: observed.Add(-24 * time.Hour)}},
	}
	require.False(t, owner.Ingest(prior).Investigated)
	rows, err := store.QuerySessions(ctx, "shop", "", observed.Unix(), observed.Unix(), 0, "")
	require.NoError(t, err)
	require.Empty(t, rows)
	trigger := &beacon.Beacon{
		Site: "shop", SessionID: "promoted", ExperienceID: "document", Received: observed.Add(time.Minute),
		PageGroup: "/error", Errors: []beacon.Error{{Fingerprint: "failure", Type: "Error", Message: "boom", Time: observed.Add(-24 * time.Hour)}},
	}
	require.True(t, owner.Ingest(trigger).Investigated)
	flush, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flush)
	// Another session's later receipt is in the same whole second, saved after promotion.
	_, err = store.AppendEvent(ctx, history.EventRecord{Site: "shop", SessionID: "newer", Type: "pageview", ObservedUS: observed.UnixMicro() + 999_999})
	require.NoError(t, err)
	handler := rumfunc.New(query.New(rumregistry.New(), store), nil)
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-sessions", Args: []string{"site:shop", fmt.Sprintf("after:%d", observed.Unix()), fmt.Sprintf("before:%d", observed.Unix())},
		Permissions: "0x1b",
	}).RawResponse
	require.NotNil(t, response)
	require.Equal(t, "last_observed_us", response["default_sort_column"])
	columns := response["columns"].(map[string]any)
	last := columns["last_observed_us"].(map[string]any)
	require.Equal(t, "descending", last["sort"])
	require.Equal(t, "timestamp", last["type"])
	data := response["data"].([][]any)
	require.Len(t, data, 2)
	require.Equal(t, "newer", data[0][1])
	require.Equal(t, observed.UnixMicro()+999_999, data[0][last["index"].(int)])
	require.Equal(t, "promoted", data[1][1])
	require.EqualValues(t, 1, data[1][4])
	require.EqualValues(t, 0, data[1][5])
	require.Equal(t, []string{"prior-user"}, data[1][12])
	timeline := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "rum-session-events", Args: []string{"site:shop", "session_id:promoted"}, Permissions: "0x1b"}).RawResponse
	require.NotNil(t, timeline)
	events := timeline["data"].([][]any)
	require.Len(t, events, 2)
	require.Equal(t, observed.UnixMicro(), events[0][0], "browser event time must not become the native history clock")
	require.Equal(t, observed.Add(time.Minute).UnixMicro(), events[1][0], "identity timeline includes the later promotion trigger")
	// No observation in the gap implies no selected activity, despite observations on both sides.
	gap := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "rum-sessions", Args: []string{"site:shop", fmt.Sprintf("after:%d", observed.Unix()+10), fmt.Sprintf("before:%d", observed.Unix()+20)}, Permissions: "0x1b"}).RawResponse
	require.NotNil(t, gap)
	require.Empty(t, gap["data"])
}

func TestFunctionStatusesAndMissingVitalsFromRealSource(t *testing.T) {
	journalStore, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	hub := rumregistry.New()
	a, _ := addSite(t, hub, "site", "first")
	handler := rumfunc.New(query.New(hub, st), nil)
	for name, tc := range map[string]struct {
		method string
		args   []string
		status int
	}{
		"malformed cursor": {"rum-live", []string{"after:invalid"}, 400},
		"invalid range":    {"rum-sessions", []string{"after:garbage"}, 400},
		"inverted range":   {"rum-errors", []string{"after:20", "before:10"}, 400},
		"missing session":  {"rum-session-events", nil, 400},
		"unknown session":  {"rum-session-events", []string{"session_id:missing"}, 404},
	} {
		t.Run(name, func(t *testing.T) {
			response := handler.HandleRaw(
				context.Background(),
				funcapi.RawMethodRequest{
					Method:      tc.method,
					Args:        tc.args,
					Permissions: "0x1b",
				},
			)
			assert.Equal(t, tc.status, response.Status)
		})
	}
	a.Ingest(
		&beacon.Beacon{
			Site:         "site",
			SessionID:    "empty",
			PageGroup:    "/empty",
			ExperienceID: "document",
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
			Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "cls", Revision: 1, Value: 0}},
		},
	)
	response := handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method:      "rum-session-events",
			Args:        []string{"session_id:empty"},
			Permissions: "0x1b",
		},
	)
	require.NotNil(t, response.RawResponse)
	require.Len(t, response.RawResponse["data"].([][]any), 1)
	a.Snapshot()
	response = handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method:      "rum-pages",
		Permissions: "0x1b",
	})
	rows := response.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.Equal(t, []any{nil, nil, float64(0)}, rows[0][4:7], "absent LCP/INP differ from observed zero CLS")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response = handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method:      "rum-session-events",
			Args:        []string{"session_id:empty"},
			Permissions: "0x1b",
		},
	)
	assert.Equal(t, 499, response.Status)
}

func TestFunctionPayloadsValidateAgainstNativeSchema(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	hub := rumregistry.New()
	a, _ := addSite(t, hub, "site", "first")
	writer := history.NewWriter(st, discardCounters{})
	a.SetHistorySink(writer)
	a.Ingest(
		&beacon.Beacon{
			Site:         "site",
			SessionID:    "session",
			PageGroup:    "/checkout",
			Browser:      "browser",
			Device:       "desktop",
			ExperienceID: "document",
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "document", Revision: 1}},
			Vitals:       []beacon.Vital{{Name: beacon.CLS, ID: "cls", Revision: 1, Value: 0}},
			Errors:       []beacon.Error{{Type: "Error", Message: "checkout failed", Fingerprint: "checkout"}},
		},
	)
	a.Snapshot()
	flushCtx, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flushCtx)
	handler := rumfunc.New(query.New(hub, st), nil)
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "plugins.d", "FUNCTION_UI_SCHEMA.json"))
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal(data, &doc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", doc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	for _, method := range []string{"rum-sites", "rum-pages", "rum-live", "rum-sessions", "rum-errors", "rum-session-events"} {
		t.Run(method, func(t *testing.T) {
			var args []string
			if method != "rum-sites" {
				args = append(args, "site:site")
			}
			if method == "rum-session-events" {
				args = append(args, "session_id:session")
			}
			response := handler.HandleRaw(
				ctx,
				funcapi.RawMethodRequest{
					Method:      method,
					Args:        args,
					Permissions: "0x1b",
				},
			)
			require.NotNil(t, response.RawResponse)
			require.NotEmpty(t, response.RawResponse["data"])
			encoded, err := json.Marshal(response.RawResponse)
			require.NoError(t, err)
			var payload any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			require.NoError(t, schema.Validate(payload))
			info := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
				Method: method,
				Info:   true,
				Args:   []string{"info"},
			})
			assert.Equal(t, 200, info.Status)
			assert.Nil(t, info.RawResponse, "native framework must own managed metadata")
			columns := response.RawResponse["columns"].(map[string]any)
			for _, row := range response.RawResponse["data"].([][]any) {
				assert.Len(t, row, len(columns))
			}
		})
	}
}

func TestFunctionsDescribeCurrentSamplingPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		measure, detail        float64
		keep                   []string
		measured, investigated string
	}{
		{"fraction", 0.25, 0.1, []string{}, "25% of new browser sessions", "10% baseline of measured sessions"},
		{"full", 1, 1, nil, "100% of new browser sessions", "100% baseline of measured sessions"},
		{"problems only", 1, 0, nil, "100% of new browser sessions", "0% baseline of measured sessions + errors, poor vitals"},
		{"no detail", 1, 0, []string{}, "100% of new browser sessions", "0% baseline of measured sessions"},
		{"collection disabled", 0, 1, nil, "off (0%)", "100% baseline of measured sessions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			journalStore, err := journal.Open(ctx, "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
			st := history.NewStore(journalStore)
			hub := rumregistry.New()
			t.Cleanup(hub.PublishReceiver(rumregistry.Availability{
				Serving: true,
			}).Close)
			a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
				Name: "shop",
			})
			cfg := config.Site{
				Name:              "shop",
				DisplayName:       "Shop",
				MeasureSampleRate: new(tc.measure),
				Investigate: &config.Investigate{
					SampleRate: new(tc.detail),
					AlwaysKeep: tc.keep,
				},
			}
			retire, err := hub.Register("shop", &rumregistry.Site{
				Route:      httpapi.NewRoute(cfg, measurementProcessor{a}, nil),
				Aggregator: a,
				Generation: "first",
			})
			require.NoError(t, err)
			t.Cleanup(retire)
			handler := rumfunc.New(query.New(hub, st), nil)
			for _, method := range []string{"rum-sessions", "rum-errors"} {
				response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
					Method:      method,
					Permissions: "0x1b",
				})
				require.NotNil(t, response.RawResponse)
				help := response.RawResponse["help"]
				assert.Contains(t, help, "Current sampling policy:")
				assert.Contains(t, help, "Shop: collection "+tc.measured+"; retained/exported detail "+tc.investigated)
				assert.Contains(t, help, "earlier policies")
			}
			response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
				Method:      "rum-sites",
				Permissions: "0x1b",
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.measure > 0, rows[0][3] == 1)
			assert.Equal(t, tc.measured, rows[0][14])
			assert.Equal(t, tc.investigated, rows[0][15])
		})
	}
}

func TestJournalFunctionFingerprintDetailAndReceiptTimeHelp(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	original := time.Now().Add(-10 * time.Minute).UnixMicro()
	for _, session := range []string{"a", "b", "a"} {
		_, err := st.AppendEvent(
			ctx,
			history.EventRecord{
				Site:        "retired",
				SessionID:   session,
				ObservedUS:  original,
				Type:        "error",
				Fingerprint: "fp",
				ErrorType:   "TypeError",
				Message:     "boom",
				Page:        "/checkout",
				Browser:     "Chrome",
			},
		)
		require.NoError(t, err)
	}
	handler := rumfunc.New(query.New(rumregistry.New(), st), nil)
	overview := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "rum-errors",
		Args:        []string{"site:retired"},
		Permissions: "0x1b",
	})
	require.NotNil(t, overview.RawResponse)
	rows := overview.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 3, rows[0][4])
	assert.Nil(t, rows[0][5])
	assert.Nil(t, rows[0][8])
	assert.Nil(t, rows[0][9])
	assert.Contains(t, overview.RawResponse["help"], "received by the Agent")
	detail := handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method:      "rum-errors",
			Args:        []string{"site:retired", "fingerprint:fp"},
			Permissions: "0x1b",
		},
	)
	require.NotNil(t, detail.RawResponse)
	row := detail.RawResponse["data"].([][]any)[0]
	assert.EqualValues(t, 2, row[5])
	assert.Equal(t, "/checkout", row[8])
	assert.Equal(t, "Chrome", row[9])
	timeline := handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method:      "rum-session-events",
			Args:        []string{"site:retired", "session_id:b"},
			Permissions: "0x1b",
		},
	)
	require.NotNil(t, timeline.RawResponse)
	assert.Equal(t, original, timeline.RawResponse["data"].([][]any)[0][0])
	invalid := handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method:      "rum-session-events",
			Args:        []string{"session_id:b", "after:-60"},
			Permissions: "0x1b",
		},
	)
	assert.Equal(t, 400, invalid.Status, "unused range filters must not be silently accepted")
}

func TestVnodeCapturesHistoryAcrossInPlaceReassignment(t *testing.T) {
	ctx := context.Background()
	owner, err := journal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	store := history.NewStore(owner)
	hub := rumregistry.New()
	service := query.New(hub, store)
	now := time.Now()
	type runtime struct {
		a *aggregate.Aggregator
		w *history.Writer
	}
	start := func(job, group, generation string) runtime {
		a := aggregate.New(5*time.Minute, aggregate.SiteCfg{
			Name:       job,
			VnodeGUID:  group,
			PageGroups: 20,
			Countries:  20,
			Investigate: aggregate.InvestigateCfg{
				Rate:       0,
				KeepErrors: true,
			},
		})
		w := history.NewWriter(store, discardCounters{})
		a.SetHistorySink(w)
		route := httpapi.NewRoute(config.Site{
			Name: job,
		}, measurementProcessor{a}, nil)
		retire, err := hub.Register(job, &rumregistry.Site{
			Route:      route,
			Aggregator: a,
			Generation: generation,
		})
		require.NoError(t, err)
		t.Cleanup(retire)
		return runtime{a, w}
	}
	flush := func(w *history.Writer) { c, cancel := context.WithCancel(ctx); cancel(); w.Run(c) }
	ingest := func(r runtime, job string, at time.Time) {
		b := &beacon.Beacon{
			Site:         job,
			VnodeGUID:    "spoofed",
			SessionID:    "shared",
			ExperienceID: "doc",
			PageGroup:    "/",
			Received:     at,
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "doc", Revision: 1}},
		}
		r.a.Ingest(b)
		assert.Equal(t, "spoofed", b.VnodeGUID, "ingestion must not mutate the browser observation")
		b.Events = nil
		b.Received = at.Add(time.Microsecond)
		b.Errors = []beacon.Error{{Fingerprint: "same", Type: "Error", Message: "broken"}}
		r.a.Ingest(b) // Promotion must retain the original association on preceding context.
	}
	old := start("job-a", "11111111-1111-4111-8111-111111111111", "old")
	other := start("job-b", "11111111-1111-4111-8111-111111111111", "other")
	ingest(old, "job-a", now)
	ingest(other, "job-b", now)
	live, cursor, err := service.Live(ctx, "", "", 2000, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, live, 4)
	for _, row := range live {
		assert.Equal(t, "11111111-1111-4111-8111-111111111111", row.VnodeGUID)
	}
	pages, err := service.Pages(ctx, "", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, pages, 2)
	pending, err := service.SessionEvents(ctx, "job-a", "shared", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, pending, 2)
	for _, row := range pending {
		assert.Equal(t, "11111111-1111-4111-8111-111111111111", row.VnodeGUID)
	}
	flush(old.w)
	flush(other.w)
	persisted, err := service.SessionEvents(ctx, "job-a", "shared", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Equal(t, pending, persisted, "active and journal copies must deduplicate")
	old.a.SetVnodeGUID("22222222-2222-4222-8222-222222222222")
	current := old
	ingest(current, "job-a", now.Add(time.Second))
	flush(current.w)
	rows, err := service.Sessions(ctx, "", "", 0, now.Unix()+2, 2000, "")
	require.NoError(t, err)
	require.Len(t, rows, 3, "same group must not merge jobs; reassignment must not merge groups")
	selected, err := service.Sessions(ctx, "job-a", "", 0, now.Unix()+2, 2000, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, int64(1), selected[0].Errors)
	errors, err := service.Errors(ctx, "", "same", 0, now.Unix()+2, "")
	require.NoError(t, err)
	require.Len(t, errors, 3)
	retained, err := service.SessionEvents(ctx, "job-a", "shared", "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Equal(t, pending, retained, "new config must not relabel previous events")
	filtered, err := service.SessionEvents(ctx, "job-b", "shared", "22222222-2222-4222-8222-222222222222")
	require.NoError(t, err)
	assert.Empty(t, filtered, "selectors combine")
	after, _, err := service.Live(ctx, "", cursor, 2000, "22222222-2222-4222-8222-222222222222")
	require.NoError(t, err)
	require.Len(t, after, 2, "an association filter must not advance another job cursor")
	sites, err := service.Sites(ctx, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "job-b", sites[0].Name)
	// The old GUID still returns retained rows after in-place reassignment.
	for _, method := range []string{"rum-sessions", "rum-errors"} {
		response := rumfunc.New(service, nil).HandleRaw(ctx, funcapi.RawMethodRequest{
			Method:      method,
			Args:        []string{"site:job-a", "vnode_guid:11111111-1111-4111-8111-111111111111"},
			Permissions: "0x1b",
		})
		require.NotEmpty(t, response.RawResponse["data"].([][]any))
		help := response.RawResponse["help"].(string)
		assert.Contains(t, help, "earlier policies", "historical caveat must survive current-identity reassignment")
		assert.Contains(t, help, "do not estimate all visitors or errors")
	}
	// Actual Function requests carry both selectors and expose the stored group.
	handler := rumfunc.New(service, nil)
	response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "rum-session-events",
		Args:        []string{"site:job-a", "vnode_guid:11111111-1111-4111-8111-111111111111", "session_id:shared"},
		Permissions: "0x1b",
	})
	require.Equal(t, 200, response.RawResponse["status"])
	table := response.RawResponse["data"].([][]any)
	require.Len(t, table, 2)
	for _, row := range table {
		assert.Equal(t, "11111111-1111-4111-8111-111111111111", row[11])
	}
}

func TestSessionEventsMergePendingAndPersistedWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	journalStore, err := journal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	hub := rumregistry.New()
	a, retire := addSite(t, hub, "site", "first")
	src := query.New(hub, st)
	handler := rumfunc.New(src, nil)
	request := func() [][]any {
		t.Helper()
		response := handler.HandleRaw(
			ctx,
			funcapi.RawMethodRequest{
				Method:      "rum-session-events",
				Args:        []string{"site:site", "session_id:session"},
				Permissions: "0x1b",
			},
		)
		require.NotNil(t, response.RawResponse, "status=%d message=%s", response.Status, response.Message)
		return response.RawResponse["data"].([][]any)
	}
	flush := func(w *history.Writer) {
		runCtx, cancel := context.WithCancel(ctx)
		cancel()
		w.Run(runCtx)
	}
	first := history.NewWriter(st, discardCounters{})
	a.SetHistorySink(first)
	a.Ingest(
		&beacon.Beacon{
			Site:         "site",
			SessionID:    "session",
			PageGroup:    "/first",
			ExperienceID: "first",
			Events: []beacon.Event{
				{Kind: beacon.EventDocument, ID: "first", Revision: 1},
				{Name: "[REDACTED]"},
				{Name: "[REDACTED]"},
			},
		},
	)
	pending := request()
	require.Len(t, pending, 3)
	assert.Equal(t, "[REDACTED]", pending[1][3])
	assert.Equal(t, pending[1], pending[2], "identical repeated events are real occurrences")
	flush(first)
	assert.Equal(t, pending, request(), "flushing must not duplicate the live ring")
	second := history.NewWriter(st, discardCounters{})
	a.SetHistorySink(second)
	a.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "session",
			PageGroup: "/second",
			Events:    []beacon.Event{{Name: "purchase"}},
		},
	)
	merged := request()
	require.Len(t, merged, 4)
	assert.Equal(t, pending, merged[:3])
	assert.Equal(t, []any{"event", "/second", "purchase", "", "", "", "", "", "", uint64(0), ""}, merged[3][1:])
	flush(second)
	assert.Equal(t, merged, request())
	retire()
	assert.Equal(t, merged, request(), "retired site history remains available")
	replacement, _ := addSite(t, hub, "site", "replacement")
	replacement.Ingest(
		&beacon.Beacon{
			Site:         "site",
			SessionID:    "session",
			PageGroup:    "/reloaded",
			ExperienceID: "reloaded",
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "reloaded", Revision: 1}, {Name: "reloaded"}},
		},
	)
	reloaded := request()
	require.Len(t, reloaded, 6)
	assert.Equal(t, merged, reloaded[:4], "new site generation must retain the persisted timeline")
	assert.Equal(t, "reloaded", reloaded[5][3])
}
