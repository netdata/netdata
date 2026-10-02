// SPDX-License-Identifier: GPL-3.0-or-later
package dem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetentionWithoutSitesAndAfterServiceShutdown(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	now := time.Now().Unix()
	old := time.Now().Add(-48 * time.Hour).Unix()
	_, _, err = st.WriteRumHistoryBatch(ctx, store.RumHistoryBatch{
		Sessions: []store.RumSessionRecord{
			{Site: "disabled", SessionID: "old", StartedAt: old, LastAt: old},
			{Site: "disabled", SessionID: "recent", StartedAt: now, LastAt: now},
		},
		Events: []store.RumSessionEventRecord{
			{Site: "disabled", SessionID: "old", TSUnixUS: old * 1e6, Type: "pageview"},
			{Site: "disabled", SessionID: "recent", TSUnixUS: now * 1e6, Type: "pageview"},
		},
	})
	require.NoError(t, err)
	_, service := NewRegistry(Dependencies{
		History: st,
	}, Config{
		History: HistoryConfig{
			Days:     1,
			MaxBytes: 1 << 30,
		},
	})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); service.Run(runCtx) }()
	t.Cleanup(func() { cancel(); <-done })
	require.Eventually(t, func() bool {
		rows, err := st.QueryRumSessions(ctx, "", 0, now+1, 2000)
		return err == nil && len(rows) == 1 && rows[0].SessionID == "recent"
	}, time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention did not stop")
	}
	// The command still owns the database while native site jobs drain.
	rows, err := st.QueryRumSessions(ctx, "", 0, now+1, 2000)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "recent", rows[0].SessionID)
}

type blockingRetention struct{ entered chan struct{} }

func (r *blockingRetention) EnforceRumHistoryRetention(ctx context.Context, _ int, _ int64) error {
	close(r.entered)
	<-ctx.Done()
	return ctx.Err()
}
func TestRetentionPropagatesCancellationIntoSQLWork(t *testing.T) {
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
	_, service := NewRegistry(
		Dependencies{
			ConfigProvider: func() (Config, error) { return LoadConfig(multipath.New(dir)) },
		},
		DefaultConfig(),
	)
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
