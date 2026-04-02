// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/stretchr/testify/require"
)

func TestAccumulator_Run_FlushesPendingGroupsWhenDiscoverersExit(t *testing.T) {
	accum := newAccumulator()
	accum.sendEvery = time.Hour
	accum.discoverers = []model.Discoverer{
		newMockDiscoverer("", newMockTargetGroup("test", "mock1")),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates := make(chan []model.TargetGroup)
	done := make(chan struct{})

	go func() {
		defer close(done)
		accum.run(ctx, updates)
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("accumulator exited before delivering pending groups")
	default:
	}

	var got []model.TargetGroup
	select {
	case got = <-updates:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for final accumulator flush")
	}

	require.Len(t, got, 1)
	require.Equal(t, "test", got[0].Source())
	require.Len(t, got[0].Targets(), 1)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("accumulator did not exit after flushing pending groups")
	}
}

func TestAccumulator_Run_ExitsOnCancelWhenFinalFlushBlocks(t *testing.T) {
	accum := newAccumulator()
	accum.sendEvery = time.Hour
	accum.discoverers = []model.Discoverer{
		newMockDiscoverer("", newMockTargetGroup("test", "mock1")),
	}

	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan []model.TargetGroup)
	done := make(chan struct{})

	go func() {
		defer close(done)
		accum.run(ctx, updates)
	}()

	time.Sleep(50 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("accumulator exited before cancellation")
	default:
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("accumulator did not exit after cancellation")
	}
}
