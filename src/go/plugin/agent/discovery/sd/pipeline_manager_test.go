// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipelineManager_Start(t *testing.T) {
	tests := map[string]struct {
		setup       func(m *PipelineManager, ctx context.Context)
		key         string
		prepared    sdPipeline
		wantErr     bool
		wantRunning bool
	}{
		"start new pipeline": {
			key:         "test-pipeline",
			prepared:    newMockPipeline("test"),
			wantRunning: true,
		},
		"start replaces existing pipeline": {
			setup: func(m *PipelineManager, ctx context.Context) {
				_ = m.StartPrepared(ctx, "test-pipeline", newMockPipeline("old"))
			},
			key:         "test-pipeline",
			prepared:    newMockPipeline("new"),
			wantRunning: true,
		},
		"start with nil pipeline fails": {
			key:     "test-pipeline",
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()

			var sentGroups []*confgroup.Group
			var mu sync.Mutex

			m := NewPipelineManager(
				logger.New(),
				func(_ context.Context, groups []*confgroup.Group) {
					mu.Lock()
					sentGroups = append(sentGroups, groups...)
					mu.Unlock()
				},
			)

			if tc.setup != nil {
				tc.setup(m, ctx)
			}

			err := m.StartPrepared(ctx, tc.key, tc.prepared)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.wantRunning, m.IsRunning(tc.key))
		})
	}
}

func TestPipelineManager_Stop(t *testing.T) {
	t.Run("stop sends removal for tracked sources", func(t *testing.T) {
		ctx := t.Context()

		var sentGroups []*confgroup.Group
		var mu sync.Mutex

		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, groups []*confgroup.Group) {
				mu.Lock()
				sentGroups = append(sentGroups, groups...)
				mu.Unlock()
			},
		)

		err := m.StartPrepared(
			ctx,
			"test-pipeline",
			newMockPipelineWithGroups("test", []*confgroup.Group{
				{Source: "source1", Configs: []confgroup.Config{}},
				{Source: "source2", Configs: []confgroup.Config{}},
			}),
		)
		require.NoError(t, err)

		// Wait for groups to be received
		time.Sleep(100 * time.Millisecond)

		m.Stop("test-pipeline")

		// Wait for stop to complete
		time.Sleep(100 * time.Millisecond)

		assert.False(t, m.IsRunning("test-pipeline"))

		mu.Lock()
		// Should have initial groups + removal groups
		// Removal groups have nil Configs (not empty slice)
		var removalSources []string
		for _, g := range sentGroups {
			if g.Configs == nil {
				removalSources = append(removalSources, g.Source)
			}
		}
		mu.Unlock()

		assert.ElementsMatch(t, []string{"source1", "source2"}, removalSources)
	})

	t.Run("stop non-existent pipeline is no-op", func(t *testing.T) {
		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, _ []*confgroup.Group) {},
		)

		// Should not panic
		m.Stop("non-existent")
		assert.False(t, m.IsRunning("non-existent"))
	})
}

func TestPipelineManager_Restart(t *testing.T) {
	t.Run("restart uses grace period for overlapping sources", func(t *testing.T) {
		ctx := t.Context()

		var sentGroups []*confgroup.Group
		var mu sync.Mutex

		// First pipeline discovers source1 and source2
		firstPipelineGroups := []*confgroup.Group{
			{Source: "source1", Configs: []confgroup.Config{}},
			{Source: "source2", Configs: []confgroup.Config{}},
		}

		// Second pipeline re-discovers source1 but not source2
		secondPipelineGroups := []*confgroup.Group{
			{Source: "source1", Configs: []confgroup.Config{}},
		}

		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, groups []*confgroup.Group) {
				mu.Lock()
				sentGroups = append(sentGroups, groups...)
				mu.Unlock()
			},
		)

		// Start first pipeline
		err := m.StartPrepared(
			ctx,
			"test-pipeline",
			newMockPipelineWithGroups("v1", firstPipelineGroups),
		)
		require.NoError(t, err)

		// Wait for first pipeline to send groups
		time.Sleep(100 * time.Millisecond)

		// Restart with new config
		err = m.RestartPrepared(
			ctx,
			"test-pipeline",
			newMockPipelineWithGroups("v2", secondPipelineGroups),
		)
		require.NoError(t, err)

		// Wait for second pipeline to send groups
		time.Sleep(100 * time.Millisecond)

		assert.True(t, m.IsRunning("test-pipeline"))

		// source1 should NOT be in pending removals (re-discovered)
		// source2 should be in pending removals (not re-discovered)
		mu.Lock()
		// At this point, no removals should have been sent yet (within grace period)
		// Removal groups have nil Configs (not empty slice)
		var removalSources []string
		for _, g := range sentGroups {
			if g.Configs == nil {
				removalSources = append(removalSources, g.Source)
			}
		}
		mu.Unlock()

		assert.Empty(t, removalSources, "no removals should be sent within grace period")

		// Verify pending removals state
		m.mux.Lock()
		pending, ok := m.pendingRemovals["test-pipeline"]
		m.mux.Unlock()

		assert.True(t, ok, "should have pending removals")
		if ok {
			_, hasSource2 := pending.sources["source2"]
			_, hasSource1 := pending.sources["source1"]
			assert.True(t, hasSource2, "source2 should be pending removal")
			assert.False(t, hasSource1, "source1 should NOT be pending (re-discovered)")
		}
	})

	t.Run("restart with invalid config keeps old pipeline", func(t *testing.T) {
		ctx := t.Context()

		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, _ []*confgroup.Group) {},
		)

		// Start first pipeline
		err := m.StartPrepared(ctx, "test-pipeline", newMockPipeline("v1"))
		require.NoError(t, err)
		assert.True(t, m.IsRunning("test-pipeline"))

		// Invalid prepared state is rejected before disrupting the old pipeline.
		err = m.RestartPrepared(ctx, "test-pipeline", nil)
		assert.Error(t, err)

		// Old pipeline should still be running
		assert.True(t, m.IsRunning("test-pipeline"))
	})
}

func TestPipelineManager_StopAll(t *testing.T) {
	t.Run("stops all pipelines and sends removals", func(t *testing.T) {
		ctx := t.Context()

		var sentGroups []*confgroup.Group
		var mu sync.Mutex

		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, groups []*confgroup.Group) {
				mu.Lock()
				sentGroups = append(sentGroups, groups...)
				mu.Unlock()
			},
		)

		// Start multiple pipelines
		groups := []*confgroup.Group{{Source: "source1", Configs: []confgroup.Config{}}}
		_ = m.StartPrepared(ctx, "pipeline1", newMockPipelineWithGroups("p1", groups))
		_ = m.StartPrepared(ctx, "pipeline2", newMockPipelineWithGroups("p2", groups))
		_ = m.StartPrepared(ctx, "pipeline3", newMockPipelineWithGroups("p3", groups))

		// Wait for pipelines to send groups
		time.Sleep(100 * time.Millisecond)

		assert.True(t, m.IsRunning("pipeline1"))
		assert.True(t, m.IsRunning("pipeline2"))
		assert.True(t, m.IsRunning("pipeline3"))

		m.StopAll()

		// Wait for stop to complete
		time.Sleep(100 * time.Millisecond)

		assert.False(t, m.IsRunning("pipeline1"))
		assert.False(t, m.IsRunning("pipeline2"))
		assert.False(t, m.IsRunning("pipeline3"))
	})
}

func TestPipelineManager_RunGracePeriodCleanup(t *testing.T) {
	t.Run("expired pending removals are cleaned up", func(t *testing.T) {
		ctx := t.Context()

		var sentGroups []*confgroup.Group
		var mu sync.Mutex

		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, groups []*confgroup.Group) {
				mu.Lock()
				sentGroups = append(sentGroups, groups...)
				mu.Unlock()
			},
		)

		// Manually add a pending removal with expired timestamp
		m.mux.Lock()
		m.pendingRemovals["test-pipeline"] = &pendingRemoval{
			sources:   map[string]struct{}{"expired-source": {}},
			timestamp: time.Now().Add(-65 * time.Second), // older than 1 minute grace period
		}
		m.pipelineSources["test-pipeline"] = map[string]struct{}{"expired-source": {}}
		m.mux.Unlock()

		// Run one iteration of cleanup
		m.processGracePeriodRemovals(ctx)

		// Check that removal was sent
		// Removal groups have nil Configs
		mu.Lock()
		var removalSources []string
		for _, g := range sentGroups {
			if g.Configs == nil {
				removalSources = append(removalSources, g.Source)
			}
		}
		mu.Unlock()

		assert.Contains(t, removalSources, "expired-source")

		// Pending removal should be cleared
		m.mux.Lock()
		_, hasPending := m.pendingRemovals["test-pipeline"]
		m.mux.Unlock()
		assert.False(t, hasPending)
	})

	t.Run("non-expired pending removals are preserved", func(t *testing.T) {
		ctx := t.Context()

		var sentGroups []*confgroup.Group
		var mu sync.Mutex

		m := NewPipelineManager(
			logger.New(),
			func(_ context.Context, groups []*confgroup.Group) {
				mu.Lock()
				sentGroups = append(sentGroups, groups...)
				mu.Unlock()
			},
		)

		// Manually add a pending removal with recent timestamp
		m.mux.Lock()
		m.pendingRemovals["test-pipeline"] = &pendingRemoval{
			sources:   map[string]struct{}{"recent-source": {}},
			timestamp: time.Now(), // just now - not expired
		}
		m.mux.Unlock()

		// Run one iteration of cleanup
		m.processGracePeriodRemovals(ctx)

		// No removal should be sent
		// Removal groups have nil Configs
		mu.Lock()
		var removalSources []string
		for _, g := range sentGroups {
			if g.Configs == nil {
				removalSources = append(removalSources, g.Source)
			}
		}
		mu.Unlock()

		assert.Empty(t, removalSources)

		// Pending removal should still exist
		m.mux.Lock()
		_, hasPending := m.pendingRemovals["test-pipeline"]
		m.mux.Unlock()
		assert.True(t, hasPending)
	})
}

func TestPipelineManager_IsRunning(t *testing.T) {
	ctx := t.Context()

	m := NewPipelineManager(
		logger.New(),
		func(_ context.Context, _ []*confgroup.Group) {},
	)

	assert.False(t, m.IsRunning("test"))

	_ = m.StartPrepared(ctx, "test", newMockPipeline("test"))
	assert.True(t, m.IsRunning("test"))

	m.Stop("test")
	time.Sleep(50 * time.Millisecond)
	assert.False(t, m.IsRunning("test"))
}

func TestPipelineManager_ConcurrentOperations(t *testing.T) {
	// Note: Concurrent operations on the SAME key are not supported and cannot
	// happen in production (ServiceDiscovery.run() processes events sequentially).
	// This test verifies concurrent operations on DIFFERENT keys work correctly.

	ctx := t.Context()

	// Track created and stopped pipelines to detect leaks
	var created, stopped atomic.Int64

	m := NewPipelineManager(
		logger.New(),
		func(_ context.Context, _ []*confgroup.Group) {},
	)

	var wg sync.WaitGroup

	// Concurrent starts for different keys
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("pipeline-%d", i)
			created.Add(1)
			_ = m.StartPrepared(ctx, key, &trackingMockPipeline{stopped: &stopped})
		}(i)
	}

	// Concurrent IsRunning checks
	for i := range 10 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.IsRunning(fmt.Sprintf("pipeline-%d", i))
		}(i)
	}

	wg.Wait()

	// Should have 10 pipelines running (one per unique key)
	for i := range 10 {
		assert.True(t, m.IsRunning(fmt.Sprintf("pipeline-%d", i)))
	}

	// Stop all pipelines
	m.StopAll()

	// Wait for all pipelines to stop
	assert.Eventually(t, func() bool {
		return created.Load() == stopped.Load()
	}, time.Second*5, time.Millisecond*100,
		"leaked pipelines: created=%d, stopped=%d", created.Load(), stopped.Load())
}

type testMockPipeline struct {
	name   string
	groups []*confgroup.Group
}

func newMockPipeline(name string) *testMockPipeline {
	return &testMockPipeline{name: name}
}

func newMockPipelineWithGroups(name string, groups []*confgroup.Group) *testMockPipeline {
	return &testMockPipeline{name: name, groups: groups}
}

func (p *testMockPipeline) Test(context.Context) (bool, error) {
	return false, nil
}

func (p *testMockPipeline) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	// Send initial groups if any
	if len(p.groups) > 0 {
		select {
		case out <- p.groups:
		case <-ctx.Done():
			return
		}
	}

	// Wait for cancellation
	<-ctx.Done()
}

// trackingMockPipeline tracks when it stops for leak detection
type trackingMockPipeline struct {
	stopped *atomic.Int64
}

func (p *trackingMockPipeline) Test(context.Context) (bool, error) {
	return false, nil
}

func (p *trackingMockPipeline) Run(ctx context.Context, _ chan<- []*confgroup.Group) {
	<-ctx.Done()
	p.stopped.Add(1)
}
