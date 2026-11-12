// SPDX-License-Identifier: GPL-3.0-or-later

package dem_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestFunctionAvailabilityWithActiveCollectors(t *testing.T) {
	for _, module := range []string{"rum", "journey", "lighthouse", "receiver"} {
		t.Run(module, func(t *testing.T) {
			history, err := demjournal.Open(t.Context(), "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, history.Close()) })
			components := dem.New(dem.Dependencies{
				History: history,
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
			components := dem.New(dem.Dependencies{
				History: history,
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
				History: history,
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
	components := dem.New(dem.Dependencies{
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
		Artifacts: captures,
	}, dem.DefaultConfig()), false, false)
}
