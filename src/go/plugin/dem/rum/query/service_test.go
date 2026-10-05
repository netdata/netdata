// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	rumregistry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type measurementProcessor struct{ *aggregate.Aggregator }

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
	state := diagnostics.New(cfg)
	route := httpapi.NewRoute(cfg, measurementProcessor{a}, state)
	retire, err := hub.Register(
		key,
		&rumregistry.Site{
			Route:       route,
			Diagnostics: state,
			Aggregator:  a,
			Generation:  generation,
			Redactor:    redact.NewRedactor("opaque-secret"),
		},
	)
	require.NoError(t, err)
	t.Cleanup(retire)
	return a, retire
}
func page(a *aggregate.Aggregator, site, path string) {
	a.Ingest(&beacon.Beacon{
		Site:      site,
		SessionID: path,
		PageGroup: path,
	})
}

func TestLivePreservesQuietSitesAndPaginatesWithoutSkipping(t *testing.T) {
	hub := rumregistry.New()
	s := query.New(hub, nil)
	hot, _ := addSite(t, hub, "hot", "first")
	quiet, _ := addSite(t, hub, "quiet", "first")
	for i := 0; i < 6; i++ {
		page(hot, "hot", fmt.Sprintf("/hot/%d", i))
	}
	page(quiet, "quiet", "/quiet/first")
	var paths []string
	var cursor string
	for range 4 {
		rows, next, err := s.Live(context.Background(), "", cursor, 2)
		require.NoError(t, err)
		cursor = next
		for _, r := range rows {
			paths = append(paths, r.Page)
		}
	}
	assert.Equal(t, []string{"/hot/0", "/hot/1", "/hot/2", "/hot/3", "/hot/4", "/hot/5", "/quiet/first"}, paths)
	page(quiet, "quiet", "/quiet/second")
	rows, next, err := s.Live(context.Background(), "", cursor, 2)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/quiet/second", rows[0].Page)
	rows, _, err = s.Live(context.Background(), "", next, 2)
	require.NoError(t, err)
	assert.Empty(t, rows)
}
func TestLiveReplacementAndFilterDoNotReuseAnotherStreamPosition(t *testing.T) {
	hub := rumregistry.New()
	s := query.New(hub, nil)
	a, retire := addSite(t, hub, "a", "old")
	b, _ := addSite(t, hub, "b", "other")
	page(a, "a", "/old")
	page(b, "b", "/other")
	rows, cursor, err := s.Live(context.Background(), "a", "", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	retire()
	replacement, _ := addSite(t, hub, "a", "new")
	page(replacement, "a", "/new")
	rows, _, err = s.Live(context.Background(), "", cursor, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "/other", rows[0].Page)
	assert.Equal(t, "/new", rows[1].Page)
	assert.Equal(t, "new", rows[1].Generation)
}
func TestSourceRedactsCopiesAndRejectsCancelledReads(t *testing.T) {
	hub := rumregistry.New()
	s := query.New(hub, nil)
	a, _ := addSite(t, hub, "site", "generation")
	page(a, "site", "/opaque-secret")
	rows, _, err := s.Live(context.Background(), "", "", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/[REDACTED]", rows[0].Page)
	original, _ := a.Live(0, 10)
	assert.Equal(t, "/opaque-secret", original[0].Page)
	sites, err := s.Sites(context.Background())
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "site [REDACTED]", sites[0].Label)
	assert.Equal(t, query.Capture{Geolocation: "country"}, sites[0].Capture)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Pages(ctx, "")
	assert.ErrorIs(t, err, context.Canceled)
	_, _, err = s.Live(context.Background(), "", "not-a-cursor", 10)
	assert.Error(t, err)
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
	handler := rumfunctions.New(src)
	request := func() [][]any {
		t.Helper()
		response := handler.HandleRaw(
			ctx,
			funcapi.RawMethodRequest{
				Method: "rum-session-events",
				Args:   []string{"site:site", "session_id:session"},
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
	first := history.NewWriter("site", st, a)
	a.SetHistorySink(first)
	a.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "session",
			PageGroup: "/first",
			Events:    []beacon.Event{{Name: "[REDACTED]"}, {Name: "[REDACTED]"}},
		},
	)
	pending := request()
	require.Len(t, pending, 3)
	assert.Equal(t, "[REDACTED]", pending[1][3])
	assert.Equal(t, pending[1], pending[2], "identical repeated events are real occurrences")
	flush(first)
	assert.Equal(t, pending, request(), "flushing must not duplicate the live ring")
	second := history.NewWriter("site", st, a)
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
	assert.Equal(t, []any{"event", "/second", "purchase", "", ""}, merged[3][1:])
	flush(second)
	assert.Equal(t, merged, request())
	retire()
	assert.Equal(t, merged, request(), "retired site history remains available")
	replacement, _ := addSite(t, hub, "site", "replacement")
	replacement.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "session",
			PageGroup: "/reloaded",
			Events:    []beacon.Event{{Name: "reloaded"}},
		},
	)
	reloaded := request()
	require.Len(t, reloaded, 6)
	assert.Equal(t, merged, reloaded[:4], "new site generation must retain the persisted timeline")
	assert.Equal(t, "reloaded", reloaded[5][3])
}

func TestFunctionStatusesAndMissingVitalsFromRealSource(t *testing.T) {
	journalStore, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := history.NewStore(journalStore)
	hub := rumregistry.New()
	a, _ := addSite(t, hub, "site", "first")
	handler := rumfunctions.New(query.New(hub, st))
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
					Method: tc.method,
					Args:   tc.args,
				},
			)
			assert.Equal(t, tc.status, response.Status)
		})
	}
	a.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "empty",
			PageGroup: "/empty",
			Vitals:    []beacon.Vital{{Name: beacon.CLS, Value: 0}},
		},
	)
	response := handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method: "rum-session-events",
			Args:   []string{"session_id:empty"},
		},
	)
	require.NotNil(t, response.RawResponse)
	require.Len(t, response.RawResponse["data"].([][]any), 1)
	a.Snapshot()
	response = handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method: "rum-pages",
	})
	rows := response.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.Equal(t, []any{nil, nil, float64(0)}, rows[0][4:7], "absent LCP/INP differ from observed zero CLS")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response = handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method: "rum-session-events",
			Args:   []string{"session_id:empty"},
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
	writer := history.NewWriter("site", st, a)
	a.SetHistorySink(writer)
	a.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "session",
			PageGroup: "/checkout",
			Browser:   "browser",
			Device:    "desktop",
			Vitals:    []beacon.Vital{{Name: beacon.CLS, Value: 0}},
			Errors:    []beacon.Error{{Type: "Error", Message: "checkout failed", Fingerprint: "checkout"}},
		},
	)
	a.Snapshot()
	flushCtx, cancel := context.WithCancel(ctx)
	cancel()
	writer.Run(flushCtx)
	handler := rumfunctions.New(query.New(hub, st))
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
					Method: method,
					Args:   args,
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
		name                           string
		measure, detail                float64
		keep                           []string
		measured, investigated, status string
	}{
		{"fraction", 0.25, 0.1, []string{}, "25% of new browser sessions", "10% baseline of measured sessions", "no_beacons"},
		{"full", 1, 1, nil, "100% of new browser sessions", "100% baseline of measured sessions", "no_beacons"},
		{"problems only", 1, 0, nil, "100% of new browser sessions", "0% baseline of measured sessions + errors, poor vitals", "no_beacons"},
		{"no detail", 1, 0, []string{}, "100% of new browser sessions", "0% baseline of measured sessions", "no_beacons"},
		{"collection disabled", 0, 1, nil, "off (0%)", "100% baseline of measured sessions", "collection_disabled"},
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
			}))
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
			state := diagnostics.New(cfg)
			retire, err := hub.Register("shop", &rumregistry.Site{
				Route:       httpapi.NewRoute(cfg, measurementProcessor{a}, state),
				Diagnostics: state,
				Aggregator:  a,
				Generation:  "first",
			})
			require.NoError(t, err)
			t.Cleanup(retire)
			handler := rumfunctions.New(query.New(hub, st))
			for _, method := range []string{"rum-sessions", "rum-errors"} {
				response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
					Method: method,
				})
				require.NotNil(t, response.RawResponse)
				help := response.RawResponse["help"]
				assert.Contains(t, help, "Current sampling policy:")
				assert.Contains(t, help, "Shop: collection "+tc.measured+"; retained/exported detail "+tc.investigated)
				assert.Contains(t, help, "earlier policies")
			}
			response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
				Method: "rum-sites",
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.status, rows[0][2])
			assert.Equal(t, tc.measured, rows[0][14])
			assert.Equal(t, tc.investigated, rows[0][15])
		})
	}
}

func TestConfiguredReceiverURLOverridesPreviouslyConfirmedObservedBase(t *testing.T) {
	hub := rumregistry.New()
	addSite(t, hub, "shop", "generation")
	data, _, release, ok := hub.AcquireSite("shop")
	require.True(t, ok)
	defer release()
	server := httptest.NewServer(
		httpapi.New(&config.Receiver{
			TrustedProxies: []string{"127.0.0.1/32"},
			MaxBodyBytes:   262144,
			RateLimit: config.RateLimit{
				PerIPPerMin:   120,
				PerSitePerSec: 500,
			},
		}, hub, nil).
			Handler(),
	)
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/rum/shop.js")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, server.URL, data.Diagnostics.ObservedBase())
	require.Equal(
		t,
		diagnostics.ReachOK,
		data.Diagnostics.Probe(context.Background(), server.Client(), server.URL).State,
	)
	revoke := hub.PublishReceiver(
		rumregistry.Availability{
			Serving:   true,
			PublicURL: "https://rum.example.org/new-prefix",
		},
	)
	defer revoke()
	rows, err := (query.New(hub, nil)).Sites(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "https://rum.example.org/new-prefix", rows[0].PublicBase)
}

func TestJournalFunctionFingerprintDetailAndSavedTimeHelp(t *testing.T) {
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
				TSUnixUS:    original,
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
	handler := rumfunctions.New(query.New(rumregistry.New(), st))
	overview := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method: "rum-errors",
		Args:   []string{"site:retired"},
	})
	require.NotNil(t, overview.RawResponse)
	rows := overview.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 3, rows[0][4])
	assert.Nil(t, rows[0][5])
	assert.Nil(t, rows[0][8])
	assert.Nil(t, rows[0][9])
	assert.Contains(t, overview.RawResponse["help"], "saved")
	detail := handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method: "rum-errors",
			Args:   []string{"site:retired", "fingerprint:fp"},
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
			Method: "rum-session-events",
			Args:   []string{"site:retired", "session_id:b"},
		},
	)
	require.NotNil(t, timeline.RawResponse)
	assert.Equal(t, original, timeline.RawResponse["data"].([][]any)[0][0])
	invalid := handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method: "rum-session-events",
			Args:   []string{"session_id:b", "after:-60"},
		},
	)
	assert.Equal(t, 400, invalid.Status, "unused range filters must not be silently accepted")
}
