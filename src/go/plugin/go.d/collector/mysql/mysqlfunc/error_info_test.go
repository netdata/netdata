// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestErrorInfoHandler(deps Deps) *funcErrorInfo {
	td, _ := deps.(*testDeps)
	router := &router{deps: deps}
	if td != nil {
		router.cfg = td.cfg
	}
	return newFuncErrorInfo(router)
}

func TestFuncErrorInfo_Handle_DBUnavailable(t *testing.T) {
	deps := newTestDeps()
	handler := newTestErrorInfoHandler(deps)

	resp := handler.Handle(context.Background(), errorInfoMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "collector is still initializing")
}

func TestFuncErrorInfo_Handle_CleanupConcurrent(t *testing.T) {
	deps := newTestDeps()
	handler := newTestErrorInfoHandler(deps)

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			resp := handler.Handle(context.Background(), errorInfoMethodID, nil)
			require.NotNil(t, resp)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			deps.cleanup()
		}
	}()

	wg.Wait()
}
