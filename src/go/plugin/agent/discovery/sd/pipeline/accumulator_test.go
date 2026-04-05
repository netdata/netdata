// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accumulatorDiscoverer func(context.Context, chan<- []model.TargetGroup)

func (fn accumulatorDiscoverer) Discover(ctx context.Context, ch chan<- []model.TargetGroup) {
	fn(ctx, ch)
}

func TestAccumulator_Run_FlushesPendingGroupsWhenDiscoverersExit(t *testing.T) {
	accum := newAccumulator()
	accum.Logger = logger.New()
	accum.sendEvery = time.Hour
	accum.discoverers = []model.Discoverer{
		accumulatorDiscoverer(func(_ context.Context, ch chan<- []model.TargetGroup) {
			ch <- []model.TargetGroup{newMockTargetGroup("test", "mock1")}
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates := make(chan []model.TargetGroup)
	done := make(chan struct{})
	go func() {
		defer close(done)
		accum.run(ctx, updates)
	}()

	time.Sleep(100 * time.Millisecond)

	select {
	case got := <-updates:
		require.Len(t, got, 1)
		assert.Equal(t, "test", got[0].Source())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for final accumulator flush")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("accumulator did not exit after delivering final flush")
	}
}

func TestAccumulator_Run_ExitsOnCancelWhenFinalFlushBlocks(t *testing.T) {
	accum := newAccumulator()
	accum.Logger = logger.New()
	accum.sendEvery = time.Hour
	accum.discoverers = []model.Discoverer{
		accumulatorDiscoverer(func(_ context.Context, ch chan<- []model.TargetGroup) {
			ch <- []model.TargetGroup{newMockTargetGroup("test", "mock1")}
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan []model.TargetGroup)
	done := make(chan struct{})
	go func() {
		defer close(done)
		accum.run(ctx, updates)
	}()

	time.Sleep(100 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("accumulator exited before cancellation instead of waiting for the final flush receiver")
	default:
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("accumulator did not exit after cancellation")
	}
}

func TestAccumulator_FinalSend_PrefersReadyReceiverAfterCancel(t *testing.T) {
	accum := newAccumulator()
	accum.groupsUpdate([]model.TargetGroup{newMockTargetGroup("test", "mock1")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	updates := make(chan []model.TargetGroup, 1)

	accum.finalSend(ctx, updates)

	select {
	case got := <-updates:
		require.Len(t, got, 1)
		assert.Equal(t, "test", got[0].Source())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for canceled finalSend delivery")
	}
}
