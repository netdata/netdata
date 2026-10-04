// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/ingest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rumfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/runtimehub"
	"github.com/netdata/netdata/go/plugins/plugin/dem/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func addSite(t *testing.T, hub *runtimehub.Hub, key, generation string) (*agg.Aggregator, func()) {
	t.Helper()
	a := agg.New(5 * time.Minute)
	a.Configure(5*time.Minute, []agg.SiteCfg{{Key: key, PageGroups: 20, Countries: 20}})
	route := ingest.NewRoute(
		config.RumSite{
			Key:            key,
			Name:           "site opaque-secret",
			AllowedOrigins: []string{"https://example.org"},
		},
		a,
	)
	retire, err := hub.Register(
		key,
		&runtimehub.Site{
			Route:      route,
			Aggregator: a,
			Generation: generation,
			Redactor:   secrets.NewRedactor("opaque-secret"),
		},
	)
	require.NoError(t, err)
	t.Cleanup(retire)
	return a, retire
}
func page(a *agg.Aggregator, site, path string) {
	a.Ingest(&beacon.Beacon{
		Site:      site,
		SessionID: path,
		PageGroup: path,
		PageView:  true,
	})
}

func TestLivePreservesQuietSitesAndPaginatesWithoutSkipping(t *testing.T) {
	hub := runtimehub.New()
	s := &source{
		hub: hub,
	}
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
	hub := runtimehub.New()
	s := &source{
		hub: hub,
	}
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
	hub := runtimehub.New()
	s := &source{
		hub: hub,
	}
	a, _ := addSite(t, hub, "site", "generation")
	page(a, "site", "/opaque-secret")
	rows, _, err := s.Live(context.Background(), "", "", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "/[REDACTED]", rows[0].Page)
	original, _ := a.Live("site", 0, 10)
	assert.Equal(t, "/opaque-secret", original[0].Page)
	sites, err := s.Sites(context.Background())
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, "site [REDACTED]", sites[0].Config.Name)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Pages(ctx, "")
	assert.ErrorIs(t, err, context.Canceled)
	_, _, err = s.Live(context.Background(), "", "not-a-cursor", 10)
	assert.Error(t, err)
}
func TestHistoryFunctionsRemainAvailableWithoutActiveSites(t *testing.T) {
	st, err := store.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Now().Unix()
	_, err = st.AppendRumEvent(context.Background(), store.RumEventRecord{
		Site:      "retired",
		SessionID: "session",
		TSUnixUS:  now * 1e6,
		Type:      "pageview",
		Page:      "/checkout",
	})
	require.NoError(t, err)
	components := New(Dependencies{
		History: st,
	}, DefaultConfig())
	handler := components.Functions[0].NewHandler().(*rumfunc.Handler)
	response := handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method: "rum-sessions",
			Args:   []string{"site:retired"},
		},
	)
	require.NotNil(t, response.RawResponse)
	rows := response.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "retired", rows[0][0])
	response = handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method: "rum-session-events",
			Args:   []string{"site:retired", "session_id:session"},
		},
	)
	require.Len(t, response.RawResponse["data"].([][]any), 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (&source{
		hub:     runtimehub.New(),
		history: st,
	}).Sessions(ctx, "", 0, now)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSessionEventsMergePendingAndPersistedWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	hub := runtimehub.New()
	a, retire := addSite(t, hub, "site", "first")
	src := &source{
		hub:     hub,
		history: st,
	}
	handler := rumfunc.New(src)
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
	first := history.New(st, a, secrets.NewRedactor("opaque-secret"))
	a.SetHistorySink(first)
	a.Ingest(
		&beacon.Beacon{
			Site:      "site",
			SessionID: "session",
			PageGroup: "/first",
			PageView:  true,
			Events:    []beacon.Event{{Name: "opaque-secret"}, {Name: "opaque-secret"}},
		},
	)
	pending := request()
	require.Len(t, pending, 3)
	assert.Equal(t, "[REDACTED]", pending[1][3])
	assert.Equal(t, pending[1], pending[2], "identical repeated events are real occurrences")
	flush(first)
	assert.Equal(t, pending, request(), "flushing must not duplicate the live ring")
	second := history.New(st, a, secrets.NewRedactor("opaque-secret"))
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
	assert.Equal(t, []any{"event", "/second", "purchase", ""}, merged[3][1:])
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
	st, err := store.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	hub := runtimehub.New()
	a, _ := addSite(t, hub, "site", "first")
	handler := rumfunc.New(&source{
		hub:     hub,
		history: st,
	})
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
	st, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	hub := runtimehub.New()
	a, _ := addSite(t, hub, "site", "first")
	writer := history.New(st, a, secrets.NewRedactor("opaque-secret"))
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
	handler := rumfunc.New(&source{
		hub:     hub,
		history: st,
	})
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins.d", "FUNCTION_UI_SCHEMA.json"))
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
func TestHistoryFunctionsExplainSampling(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	hub := runtimehub.New()
	a := agg.New(5 * time.Minute)
	cfg := config.RumSite{
		Key:               "shop",
		Name:              "Shop",
		MeasureSampleRate: .25,
		Investigate: &config.RumInvestigate{
			SampleRate: .1,
			AlwaysKeep: []string{},
		},
	}
	a.Configure(5*time.Minute, []agg.SiteCfg{{Key: "shop"}})
	retire, err := hub.Register(
		"shop",
		&runtimehub.Site{
			Route:      ingest.NewRoute(cfg, a),
			Aggregator: a,
			Generation: "first",
		},
	)
	require.NoError(t, err)
	t.Cleanup(retire)
	handler := rumfunc.New(&source{
		hub:     hub,
		history: st,
	})
	for _, method := range []string{"rum-sessions", "rum-errors"} {
		response := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
			Method: method,
		})
		require.NotNil(t, response.RawResponse)
		assert.Contains(
			t,
			response.RawResponse["help"],
			"Shop: measuring 25% of sessions, keeping 10% of measured in full",
		)
	}
}

func TestConfiguredReceiverURLOverridesPreviouslyConfirmedObservedBase(t *testing.T) {
	hub := runtimehub.New()
	addSite(t, hub, "shop", "generation")
	data, _, release, ok := hub.AcquireSite("shop")
	require.True(t, ok)
	defer release()
	server := httptest.NewServer(
		ingest.New(&config.RumCfg{
			TrustedProxies: []string{"127.0.0.1/32"},
			MaxBodyBytes:   262144,
			RateLimit: config.RumRateLimit{
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
	require.Equal(t, server.URL, data.Route.ObservedBase("shop"))
	require.Equal(t, ingest.ReachOK, data.Route.Probe(context.Background(), server.Client(), "shop", server.URL).State)
	revoke := hub.PublishReceiver(
		runtimehub.Availability{
			Serving:   true,
			PublicURL: "https://rum.example.org/new-prefix",
		},
	)
	defer revoke()
	rows, err := (&source{
		hub: hub,
	}).Sites(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "https://rum.example.org/new-prefix", rows[0].PublicBase)
}

func TestJournalFunctionFingerprintDetailAndSavedTimeHelp(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	original := time.Now().Add(-10 * time.Minute).UnixMicro()
	for _, session := range []string{"a", "b", "a"} {
		_, err := st.AppendRumEvent(
			ctx,
			store.RumEventRecord{
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
	handler := rumfunc.New(&source{
		hub:     runtimehub.New(),
		history: st,
	})
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
