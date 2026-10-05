// SPDX-License-Identifier: GPL-3.0-or-later

package redis

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redis/redisfunc"
)

func TestFuncDepsAdapter_ClientLifecycle(t *testing.T) {
	collr := New()
	deps := funcDepsAdapter{
		collector: collr,
	}

	_, err := deps.Client()
	assert.Error(t, err, "before Init")

	require.NoError(t, collr.Init(context.Background()))
	mock := &mockRedisClient{}
	collr.setClient(mock)

	client, err := deps.Client()
	require.NoError(t, err, "after Init")
	assert.Same(t, mock, client)

	collr.Cleanup(context.Background())
	_, err = deps.Client()
	assert.Error(t, err, "after Cleanup")
	assert.True(t, mock.calledClose)
}

// Functions run concurrently with the job lifecycle; run with -race.
func TestCollector_FunctionDuringCleanup(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	collr.setClient(&mockRedisClient{})
	params := funcapi.ResolveParams(redisfunc.Methods()[0].RequiredParams, nil)

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			resp := collr.funcRouter.Handle(context.Background(), "top-queries", params)
			assert.Contains(t, []int{200, 503}, resp.Status)
		}
	})
	collr.Cleanup(context.Background())
	wg.Wait()
}
