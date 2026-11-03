// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	registry "github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/require"
)

func TestRuntimeWithoutInstallURLRetiresAndReplacesEvidence(t *testing.T) {
	hub := registry.New()
	db, err := journal.Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var previous string
	for range 2 {
		c := New(Dependencies{
			Registry: hub,
			History:  history.NewStore(db),
		})
		c.Name = "shop"
		c.AllowedOrigins = []string{"https://site.example"}
		require.NoError(t, c.Init(context.Background()))
		ctx, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- c.Run(ctx, func() { close(ready) })
			close(done)
		}()
		t.Cleanup(func() {
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Error("runtime did not stop")
			}
		})
		select {
		case <-ready:
		case <-time.After(10 * time.Second):
			cancel()
			t.Fatal("runtime did not become ready")
		}
		rows, err := query.New(hub, nil).Sites(context.Background())
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.NotEmpty(t, rows[0].Generation)
		require.NotEqual(t, previous, rows[0].Generation)
		previous = rows[0].Generation
		require.Empty(t, rows[0].ScriptURL)
		require.True(t, rows[0].Rejected.At.IsZero())
		owner, _, release, ok := hub.AcquireSite("shop")
		require.True(t, ok)
		owner.Diagnostics.RejectOrigin("https://wrong.example", time.Now())
		release()
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Fatal("runtime did not retire")
		}
		require.Empty(t, hub.Keys())
	}
}
