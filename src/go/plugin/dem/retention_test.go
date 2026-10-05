// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observedRetention struct {
	*demjournal.Store
	swept chan error
}

func (r *observedRetention) EnforceHistoryRetention(ctx context.Context, days int, bytes int64) error {
	err := r.Store.EnforceHistoryRetention(ctx, days, bytes)
	r.swept <- err
	return err
}

func TestRetentionWithoutSitesAndAfterServiceShutdown(t *testing.T) {
	ctx := context.Background()
	st, err := demjournal.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Now().Unix()
	_, err = rumhistory.NewStore(st).AppendEvent(ctx, rumhistory.EventRecord{
		Site:      "disabled",
		SessionID: "recent",
		TSUnixUS:  now * 1e6,
		Type:      "pageview",
	})
	require.NoError(t, err)
	components := New(Dependencies{
		History: st,
	}, Config{
		History: HistoryConfig{
			Days:     1,
			MaxBytes: 1 << 30,
		},
	})
	service := components.Retention
	observed := &observedRetention{
		Store: st,
		swept: make(chan error, 1),
	}
	service.store = observed
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); service.Run(runCtx) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case err := <-observed.swept:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("retention did not sweep with no site jobs")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention did not stop")
	}
	// The command still owns the journal while native site jobs drain.
	rows, err := rumhistory.NewStore(st).QuerySessions(ctx, "", "", 0, now+1, 2000)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "recent", rows[0].SessionID)
}

type blockingRetention struct{ entered chan struct{} }

func (r *blockingRetention) EnforceHistoryRetention(ctx context.Context, _ int, _ int64) error {
	close(r.entered)
	<-ctx.Done()
	return ctx.Err()
}
func TestRetentionPropagatesCancellationIntoStoreWork(t *testing.T) {
	backend := &blockingRetention{
		entered: make(chan struct{}),
	}
	service := &Retention{
		store:  backend,
		policy: DefaultConfig().History,
		log:    logger.New(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); service.Run(ctx) }()
	<-backend.entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention did not join cancelled query")
	}
}

func TestRetentionReloadsValidPolicyAndRetainsItOnInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dem.conf")
	components := New(
		Dependencies{
			ConfigProvider: func() (Config, error) { return LoadConfig(multipath.New(dir)) },
		},
		DefaultConfig(),
	)
	service := components.Retention
	require.NoError(t, os.WriteFile(path, []byte("history: {days: 7, max_bytes: 4096}"), 0600))
	service.reloadPolicy()
	assert.Equal(t, HistoryConfig{
		Days:     7,
		MaxBytes: 4096,
	}, service.policy)
	require.NoError(t, os.WriteFile(path, []byte("history: {days: 14, max_bytes: 8192}"), 0600))
	service.reloadPolicy()
	assert.Equal(t, HistoryConfig{
		Days:     14,
		MaxBytes: 8192,
	}, service.policy)
	for name, body := range map[string]string{
		"invalid value":  "history: {days: 0, max_bytes: 1}",
		"invalid syntax": "history: [",
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(path, []byte(body), 0600))
			service.reloadPolicy()
			assert.Equal(t, HistoryConfig{
				Days:     14,
				MaxBytes: 8192,
			}, service.policy)
		})
	}
}

type transientRetention struct{ calls chan int }

func (r *transientRetention) EnforceHistoryRetention(context.Context, int, int64) error {
	r.calls <- 1
	return errors.New("temporary filesystem failure")
}
func TestRetentionRetriesTransientFailureBeforeHourlySweep(t *testing.T) {
	backend := &transientRetention{
		calls: make(chan int, 2),
	}
	service := &Retention{
		store:  backend,
		policy: DefaultConfig().History,
		log:    logger.New(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); service.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-backend.calls:
	case <-time.After(time.Second):
		t.Fatal("initial sweep did not run")
	}
	select {
	case <-backend.calls:
	case <-time.After(6 * time.Second):
		t.Fatal("transient failure stalled retention and writer recovery until the hourly sweep")
	}
}

type observedArtifactRetention struct{ sweeps chan HistoryConfig }

func (r *observedArtifactRetention) Enforce(_ context.Context, days int, bytes int64) (artifacts.Stats, error) {
	r.sweeps <- HistoryConfig{
		Days:     days,
		MaxBytes: bytes,
	}
	return artifacts.Stats{}, nil
}
func TestArtifactSweepStillRunsWhenJournalSweepFails(t *testing.T) {
	artifact := &observedArtifactRetention{
		sweeps: make(chan HistoryConfig, 1),
	}
	service := &Retention{
		store: &transientRetention{
			calls: make(chan int, 1),
		},
		policy:         DefaultConfig().History,
		artifactStore:  artifact,
		artifactPolicy: DefaultConfig().Artifacts,
		log:            logger.New(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); service.Run(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case policy := <-artifact.sweeps:
		assert.Equal(t, DefaultConfig().Artifacts, policy)
	case <-time.After(time.Second):
		t.Fatal("journal failure prevented independent artifact expiry")
	}
}

type historyRetentionFunc func(context.Context, int, int64) error

func (f historyRetentionFunc) EnforceHistoryRetention(ctx context.Context, days int, maxBytes int64) error {
	return f(ctx, days, maxBytes)
}

type artifactRetentionFunc func(context.Context, int, int64) (artifacts.Stats, error)

func (f artifactRetentionFunc) Enforce(ctx context.Context, days int, maxBytes int64) (artifacts.Stats, error) {
	return f(ctx, days, maxBytes)
}
func TestRetentionRetriesOnlyFailedStore(t *testing.T) {
	for _, failedStore := range []string{"history", "artifacts"} {
		t.Run(failedStore, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var historyCalls, artifactCalls atomic.Int32
				service := &Retention{
					store: historyRetentionFunc(func(context.Context, int, int64) error {
						historyCalls.Add(1)
						if failedStore == "history" {
							return errors.New("history unavailable")
						}
						return nil
					}),
					artifactStore: artifactRetentionFunc(func(context.Context, int, int64) (artifacts.Stats, error) {
						artifactCalls.Add(1)
						if failedStore == "artifacts" {
							return artifacts.Stats{}, errors.New("artifacts unavailable")
						}
						return artifacts.Stats{}, nil
					}),
					policy:         DefaultConfig().History,
					artifactPolicy: DefaultConfig().Artifacts,
					log:            logger.NewWithWriter(&bytes.Buffer{}),
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go service.Run(ctx)
				synctest.Wait()
				require.Equal(t, int32(1), historyCalls.Load())
				require.Equal(t, int32(1), artifactCalls.Load())
				time.Sleep(10 * time.Second)
				synctest.Wait()
				if failedStore == "history" {
					assert.Equal(t, int32(3), historyCalls.Load())
					assert.Equal(t, int32(1), artifactCalls.Load())
				} else {
					assert.Equal(t, int32(1), historyCalls.Load())
					assert.Equal(t, int32(3), artifactCalls.Load())
				}
			})
		})
	}
}
func TestRetentionBudgetWarningRequiresSuccessfulSweep(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "stale"}[failed], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs bytes.Buffer
				service := &Retention{
					store: historyRetentionFunc(func(context.Context, int, int64) error { return nil }),
					artifactStore: artifactRetentionFunc(func(context.Context, int, int64) (artifacts.Stats, error) {
						stats := artifacts.Stats{
							ProtectedBytes: DefaultConfig().Artifacts.MaxBytes + 1,
						}
						if failed {
							return stats, errors.New("artifact sweep failed")
						}
						return stats, nil
					}),
					policy:         DefaultConfig().History,
					artifactPolicy: DefaultConfig().Artifacts,
					log:            logger.NewWithWriter(&logs),
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				go service.Run(ctx)
				synctest.Wait()
				if failed {
					assert.Contains(t, logs.String(), "DEM artifact retention failed")
					assert.NotContains(t, logs.String(), "protected run files exceed artifact budget")
				} else {
					assert.Contains(t, logs.String(), "protected run files exceed artifact budget")
				}
			})
		})
	}
}

func TestRetentionMaintainsHourlySweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var historyCalls, artifactCalls atomic.Int32
		service := &Retention{
			store: historyRetentionFunc(func(context.Context, int, int64) error { historyCalls.Add(1); return nil }),
			artifactStore: artifactRetentionFunc(func(context.Context, int, int64) (artifacts.Stats, error) {
				artifactCalls.Add(1)
				return artifacts.Stats{}, nil
			}),
			policy:         DefaultConfig().History,
			artifactPolicy: DefaultConfig().Artifacts,
			log:            logger.NewWithWriter(&bytes.Buffer{}),
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go service.Run(ctx)
		synctest.Wait()
		time.Sleep(time.Hour - time.Second)
		synctest.Wait()
		assert.Equal(t, int32(1), historyCalls.Load())
		assert.Equal(t, int32(1), artifactCalls.Load())
		time.Sleep(time.Second)
		synctest.Wait()
		assert.Equal(t, int32(2), historyCalls.Load())
		assert.Equal(t, int32(2), artifactCalls.Load())
	})
}
