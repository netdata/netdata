// SPDX-License-Identifier: GPL-3.0-or-later

package dem_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/rumfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/syntheticfunc"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// provider returns the composed process Function provider with the given ID.
func provider(t *testing.T, components dem.Components, id string) funcapi.ProcessFunctionProvider {
	t.Helper()
	for _, p := range components.Functions {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("process Function provider %q is missing", id)
	return funcapi.ProcessFunctionProvider{}
}

func rumHandler(t *testing.T, components dem.Components) *rumfunc.Handler {
	t.Helper()
	return provider(t, components, "rum").NewHandlerWithContext(funcapi.ProcessFunctionContext{}).(*rumfunc.Handler)
}

func syntheticHandler(t *testing.T, components dem.Components) *syntheticfunc.Handler {
	t.Helper()
	return provider(t, components, "synthetics").
		NewHandlerWithContext(funcapi.ProcessFunctionContext{}).(*syntheticfunc.Handler)
}

// privateHistory and privateCaptures are the temporary stores debug runs use.
func privateHistory(t *testing.T) *demjournal.Store {
	t.Helper()
	history, err := demjournal.Open(t.Context(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, history.Close()) })
	return history
}

func privateCaptures(t *testing.T) *artifacts.Store {
	t.Helper()
	captures, err := artifacts.Open("")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	return captures
}

// Hold the scheduled attempt before admission. Inventory reads cannot create
// another attempt or alter its request.
type inventoryExecutor struct{ waiting chan struct{} }

func (inventoryExecutor) Check(context.Context) error { return nil }
func (inventoryExecutor) Rehearse(context.Context, synthetic.Request) error {
	panic("inventory must not rehearse a browser")
}

func (e inventoryExecutor) ExecuteScheduled(
	ctx context.Context,
	_ synthetic.Request,
	state func(synthetic.JobState),
	_ func(time.Time) synthetic.Schedule,
) synthetic.Execution {
	state(synthetic.JobWaiting)
	close(e.waiting)
	<-ctx.Done()
	return synthetic.Execution{
		Drained: true,
		Run: synthetic.Run{
			Outcome: synthetic.Cancelled,
		},
	}
}

func assertAvailability(t *testing.T, components dem.Components, rum, synthetics bool) {
	t.Helper()
	counts := map[string]int{"rum": 6, "synthetics": 5, "dem-history": 1}
	for _, provider := range components.Functions {
		methods := provider.Functions()
		require.Len(t, methods, counts[provider.ID])
		for _, method := range methods {
			assert.NotContains(t, method.Tags, "hidden", method.ID)
			if provider.ID == "dem-history" {
				assert.Nil(t, method.Available)
				continue
			}
			require.NotNil(t, method.Available, method.ID)
			want := rum
			if provider.ID == "synthetics" {
				want = synthetics
			}
			assert.Equal(t, want, method.Available(), method.ID)
		}
	}
}

func TestOnlySyntheticCollectorsWaitForTheBrowserRuntime(t *testing.T) {
	browser := []collectorapi.ActivationDependency{{Kind: "dem-runtime", Name: "browser"}}
	for name, activation := range map[string]func(map[string]any) ([]collectorapi.ActivationDependency, error){
		"runtime host": func(map[string]any) ([]collectorapi.ActivationDependency, error) { return browser, nil },
		"preload":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			components := dem.New(dem.Dependencies{
				History:           privateHistory(t),
				Artifacts:         privateCaptures(t),
				BrowserActivation: activation,
			}, dem.DefaultConfig())
			for _, module := range []string{"rum", "receiver"} {
				assert.Nil(t, components.Collectors[module].ActivationDependencies, module)
			}
			for _, module := range []string{"journey", "lighthouse"} {
				dependencies := components.Collectors[module].ActivationDependencies
				if activation == nil {
					assert.Nil(t, dependencies, module)
					continue
				}
				require.NotNil(t, dependencies, module)
				got, err := dependencies(nil)
				require.NoError(t, err)
				assert.Equal(t, browser, got, module)
			}
		})
	}
}

func TestFunctionAvailabilityWithActiveCollectors(t *testing.T) {
	for _, module := range []string{"rum", "journey", "lighthouse", "receiver"} {
		t.Run(module, func(t *testing.T) {
			components := dem.New(dem.Dependencies{
				History:   privateHistory(t),
				Artifacts: privateCaptures(t),
				Executor: inventoryExecutor{
					waiting: make(chan struct{}),
				},
			}, dem.DefaultConfig())
			assertAvailability(t, components, false, false)
			collector := components.Collectors[module].CreateV2()
			switch c := collector.(type) {
			case *rum.Collector:
				c.Name = "shop"
				c.AllowedOrigins = []string{"https://example.org"}
			case *journey.Collector:
				c.Name = "login"
				c.Script = "test('login', async () => {});"
			case *lighthouse.Collector:
				c.Name = "home"
				c.URL = "https://example.org"
			case *receiver.Collector:
				c.Listen = "127.0.0.1:0"
			}
			require.NoError(t, collector.Init(t.Context()))
			t.Cleanup(func() { collector.Cleanup(context.Background()) })
			require.NoError(t, collector.Check(t.Context()))
			assertAvailability(t, components, false, false)
			runtime := collector.(interface {
				Run(context.Context, func()) error
			})
			ctx, cancel := context.WithCancel(t.Context())
			ready, done := make(chan struct{}), make(chan error, 1)
			go func() { done <- runtime.Run(ctx, func() { close(ready) }); close(done) }()
			t.Cleanup(func() {
				cancel()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("collector did not join")
				}
			})
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("collector did not become ready")
			}
			assertAvailability(t, components, module == "rum", module == "journey" || module == "lighthouse")
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("collector did not stop")
			}
			// No history was written. The callback follows registry retirement; the
			// framework separately keeps already-announced methods callable.
			assertAvailability(t, components, false, false)
		})
	}
}

func TestFunctionAvailabilityFromRetainedHistory(t *testing.T) {
	for _, kind := range []string{"rum", "synthetic"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			history, err := demjournal.Open(t.Context(), root)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, history.Close()) })
			captures := privateCaptures(t)
			components := dem.New(dem.Dependencies{
				History:   history,
				Artifacts: captures,
			}, dem.DefaultConfig())
			assertAvailability(t, components, false, false)
			now := time.Now().UnixMicro()
			if kind == "rum" {
				_, err = rumhistory.NewStore(history).AppendEvent(t.Context(), rumhistory.EventRecord{
					Site:       "retired",
					SessionID:  "session",
					ObservedUS: now,
					Type:       "pageview",
					Page:       "/",
				})
			} else {
				_, err = synthetichistory.NewStore(history).AppendRun(t.Context(), "complete", synthetic.Run{
					ID:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					JobID:        "journey:retired",
					Name:         "retired",
					Kind:         synthetic.Journey,
					StartedUS:    now,
					CompletedUS:  now + 1,
					Outcome:      synthetic.Success,
					CaptureState: "disabled",
				})
			}
			require.NoError(t, err)
			assertAvailability(t, components, kind == "rum", kind == "synthetic")
			require.NoError(t, history.Close())
			history, err = demjournal.Open(t.Context(), root)
			require.NoError(t, err)
			restarted := dem.New(dem.Dependencies{
				History:   history,
				Artifacts: captures,
			}, dem.DefaultConfig())
			assertAvailability(t, restarted, kind == "rum", kind == "synthetic")
		})
	}
}

func TestFunctionAvailabilityFromCapturesAlone(t *testing.T) {
	root := t.TempDir()
	captures, err := artifacts.Open(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	history := privateHistory(t)
	components := dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, dem.DefaultConfig())
	assertAvailability(t, components, false, false)
	const id = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	work, err := captures.Begin(id, "lighthouse:retired", "")
	require.NoError(t, err)
	assertAvailability(t, components, false, false)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), []byte("<p>report</p>"), 0600))
	_, err = captures.Finalize(t.Context(), id, []synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}})
	require.NoError(t, err)
	assertAvailability(t, components, false, true)
	require.NoError(t, captures.Close())
	captures, err = artifacts.Open(root)
	require.NoError(t, err)
	restarted := dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, dem.DefaultConfig())
	assertAvailability(t, restarted, false, true)
	// Expiry does not withdraw discovery mid-process, but a real restart derives
	// availability from the now-empty published namespace.
	_, err = captures.Enforce(t.Context(), 30, 1)
	require.NoError(t, err)
	assertAvailability(t, restarted, false, true)
	require.NoError(t, captures.Close())
	captures, err = artifacts.Open(root)
	require.NoError(t, err)
	assertAvailability(t, dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, dem.DefaultConfig()), false, false)
}

func TestHistoryFunctionsRemainAvailableWithoutActiveSites(t *testing.T) {
	journalStore, err := demjournal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journalStore.Close()) })
	st := rumhistory.NewStore(journalStore)
	now := time.Now().Unix()
	_, err = st.AppendEvent(context.Background(), rumhistory.EventRecord{
		Site:       "retired",
		SessionID:  "session",
		ObservedUS: now * 1e6,
		Type:       "pageview",
		Page:       "/checkout",
	})
	require.NoError(t, err)
	components := dem.New(dem.Dependencies{
		History:   journalStore,
		Artifacts: privateCaptures(t),
	}, dem.DefaultConfig())
	handler := rumHandler(t, components)
	response := handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method:      "rum-sessions",
			Args:        []string{"site:retired"},
			Permissions: "0x1b",
		},
	)
	require.NotNil(t, response.RawResponse)
	rows := response.RawResponse["data"].([][]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "retired", rows[0][0])
	response = handler.HandleRaw(
		context.Background(),
		funcapi.RawMethodRequest{
			Method:      "rum-session-events",
			Args:        []string{"site:retired", "session_id:session"},
			Permissions: "0x1b",
		},
	)
	require.Len(t, response.RawResponse["data"].([][]any), 1)
}

// Retained grouping must work with no live collector, receiver or browser runtime.
func TestSiteAssociationHistoryFunctionsWithoutActiveJobs(t *testing.T) {
	ctx := context.Background()
	journal, err := demjournal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journal.Close()) })
	now := time.Now().Add(-time.Minute).UnixMicro()
	rum := rumhistory.NewStore(journal)
	runs := synthetichistory.NewStore(journal)
	for i, association := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""} {
		_, err := rum.AppendEvent(ctx, rumhistory.EventRecord{
			Site: "browser-traffic", VnodeGUID: association, SessionID: "same-session",
			ObservedUS: now + int64(i), Type: "pageview", Page: "/",
		})
		require.NoError(t, err)
		_, err = runs.AppendRun(ctx, "complete", synthetic.Run{
			ID:    []string{"run-storefront", "run-documentation", "run-unassigned"}[i],
			JobID: "journey:checkout", Kind: synthetic.Journey, Name: "checkout", VnodeGUID: association,
			StartedUS: now + int64(i), CompletedUS: now + int64(i) + 1, Outcome: synthetic.Success,
		})
		require.NoError(t, err)
	}
	require.NoError(t, runs.Sync(ctx))
	components := dem.New(dem.Dependencies{History: journal, Artifacts: privateCaptures(t)}, dem.DefaultConfig())
	for name, tc := range map[string]struct {
		method string
		handle func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse
	}{
		"RUM":       {"rum-sessions", rumHandler(t, components).HandleRaw},
		"synthetic": {"synthetics-runs", syntheticHandler(t, components).HandleRaw},
	} {
		t.Run(name, func(t *testing.T) {
			for _, association := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"} {
				response := tc.handle(ctx, funcapi.RawMethodRequest{
					Method: tc.method, Args: []string{"vnode_guid:" + association}, Permissions: "0x7b",
				})
				require.NotNil(t, response.RawResponse)
				rows := response.RawResponse["data"].([][]any)
				require.Len(t, rows, 1)
				assert.Equal(
					t,
					association,
					rows[0][response.RawResponse["columns"].(map[string]any)["vnode_guid"].(map[string]any)["index"].(int)],
				)
			}
			response := tc.handle(ctx, funcapi.RawMethodRequest{
				Method:      tc.method,
				Permissions: "0x7b",
			})
			require.NotNil(t, response.RawResponse)
			rows := response.RawResponse["data"].([][]any)
			require.Len(t, rows, 3)
			var groups []any
			for _, row := range rows {
				index := response.RawResponse["columns"].(map[string]any)["vnode_guid"].(map[string]any)["index"].(int)
				groups = append(groups, row[index])
			}
			assert.ElementsMatch(t, []any{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""}, groups)
		})
	}
}

func TestRegistryPublishesSyntheticHistoryWithoutActiveJobs(t *testing.T) {
	ctx := context.Background()
	history := privateHistory(t)
	captures := privateCaptures(t)
	run := synthetic.Run{
		ID:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		JobID:        "journey:retired",
		Kind:         synthetic.Journey,
		Name:         "retired",
		StartedUS:    time.Now().UnixMicro(),
		Outcome:      synthetic.Failed,
		CaptureState: "disabled",
		Events:       []synthetic.Event{{Kind: "error", Message: "assertion failed"}},
	}
	run.CompletedUS = run.StartedUS + 1
	_, err := synthetichistory.NewStore(history).AppendRun(ctx, "complete", run)
	require.NoError(t, err)
	components := dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, dem.DefaultConfig())
	require.Contains(t, components.Collectors, "journey")
	require.Contains(t, components.Collectors, "lighthouse")
	require.Len(t, provider(t, components, "synthetics").Functions(), 5)
	handler := syntheticHandler(t, components)
	checks := handler.HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "synthetics-checks",
		Permissions: "0x7b",
	})
	require.NotNil(t, checks.RawResponse)
	assert.Empty(t, checks.RawResponse["data"])
	detail := handler.HandleRaw(
		ctx,
		funcapi.RawMethodRequest{
			Method:      "synthetics-run",
			Permissions: "0x1b",
			Args:        []string{"job_id:journey:retired", "run_id:" + run.ID},
		},
	)
	require.NotNil(t, detail.RawResponse, "%v", detail)
	retained := detail.RawResponse["run"].(map[string]any)
	assert.Equal(t, run.ID, retained["id"])
	assert.Equal(t, run.Outcome, retained["outcome"])
	assert.Nil(t, retained["metrics"])
	assert.Len(t, detail.RawResponse["data"], 1)
}

func TestRegistrySyntheticInventoryRedactsTargetCredentials(t *testing.T) {
	waiting := make(chan struct{})
	components := dem.New(dem.Dependencies{
		History:   privateHistory(t),
		Artifacts: privateCaptures(t),
		Executor: inventoryExecutor{
			waiting: waiting,
		},
	}, dem.DefaultConfig())
	collector := components.Collectors["lighthouse"].CreateV2().(*lighthouse.Collector)
	collector.Name = "home"
	collector.URL = "https://example.org/?token=fixture-token&password=fixture-password&page=home"
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- collector.Run(ctx, func() { close(ready) }) }()
	defer func() { cancel(); require.NoError(t, <-done) }()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not become ready")
	}
	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled attempt did not queue")
	}
	checks := syntheticHandler(t, components).HandleRaw(ctx, funcapi.RawMethodRequest{
		Method:      "synthetics-checks",
		Permissions: "0x1b",
	})
	require.NotNil(t, checks.RawResponse)
	require.Len(t, checks.RawResponse["data"], 1)
	encoded, err := json.Marshal(checks.RawResponse["data"])
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "fixture-token")
	assert.NotContains(t, string(encoded), "fixture-password")
	assert.Contains(t, string(encoded), "[REDACTED]")
	assert.Equal(t, "https://example.org/?token=fixture-token&password=fixture-password&page=home", collector.URL)
}
