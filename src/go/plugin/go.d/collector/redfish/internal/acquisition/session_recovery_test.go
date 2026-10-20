// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionRecoveryCleanupHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var expire atomic.Bool
	var deletes atomic.Int64
	server := testutil.NewServer(t, testutil.ServerConfig{
		SupportSession: true,
		HandleRequest: func(w http.ResponseWriter, r *http.Request) bool {
			if r.Method == http.MethodDelete {
				deletes.Add(1)
				cancel()
				<-r.Context().Done()
				return true
			}
			if expire.Load() && r.URL.Path == "/redfish/v1/" && r.Header.Get("X-Auth-Token") != "" {
				http.Error(w, "expired", http.StatusUnauthorized)
				return true
			}
			return false
		},
	})
	defer server.Close()
	cfg := testConfig(server.URL, "session")
	cfg.Timeout = confopt.Duration(2 * time.Second)
	client := newTestProtocolClient(t, cfg)
	defer client.Close()
	_, err := client.Acquire(ctx)
	require.NoError(t, err)
	expire.Store(true)

	started := time.Now()
	result, err := client.Acquire(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(started), time.Second, "canceling collection must interrupt recovery logout")
	assert.False(t, result.Available)
	assert.Nil(t, client.sdk)
	assert.Equal(t, int64(1), deletes.Load())
	assert.Equal(t, int64(1), server.SessionCreates.Load())
}
