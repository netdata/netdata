// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestProcessRetiresCommittedLifecycleOnRestartAndShutdown(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	hook := &processLifecycleHook{
		current: make(map[collectorapi.JobConfigIdentity]bool),
		changed: make(chan struct{}, 8),
	}
	config := testProductionProcessConfig(reader, io.Discard)
	config.AutoEnable = true
	config.Modules["test"] = collectorapi.Creator{
		JobConfigLifecycle: hook,
		Create: func() collectorapi.CollectorV1 {
			return &collectorapi.MockCollectorV1{
				FailOnInit: true,
			}
		},
	}
	config.DiscoveryProviders = []agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory(
			"test",
			func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
				return runTestDiscoverer{
					configs: []confgroup.Config{
						{
							"module":          "test",
							"name":            "device",
							"update_every":    1,
							"__source__":      "test",
							"__source_type__": "user",
							"__provider__":    "test",
						},
					},
				}, true, nil
			},
		),
	}
	process, err := NewProcess(config)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	select {
	case <-hook.changed:
	case <-ctx.Done():
		t.Fatal("no committed lifecycle")
	}
	require.NoError(t, process.Restart(ctx))
	require.Eventually(
		t,
		func() bool { hook.mu.Lock(); defer hook.mu.Unlock(); return hook.commits >= 2 },
		time.Second,
		time.Millisecond,
	)
	hook.mu.Lock()
	removes := hook.removes
	hook.mu.Unlock()
	require.Equal(t, 1, removes, "restart must retire prior run projection, even for identical config")
	require.NoError(t, process.Terminate(ctx))
	require.NoError(t, <-done)
	hook.mu.Lock()
	defer hook.mu.Unlock()
	require.Empty(t, hook.current, "shutdown must retire committed lifecycle projections")
}

type processLifecycleSnapshot struct {
	id collectorapi.JobConfigIdentity
}

func (s processLifecycleSnapshot) Identity() collectorapi.JobConfigIdentity { return s.id }

type processLifecycleHook struct {
	mu               sync.Mutex
	current          map[collectorapi.JobConfigIdentity]bool
	commits, removes int
	changed          chan struct{}
}

func (*processLifecycleHook) Project(
	id collectorapi.JobConfigIdentity,
	_ map[string]any,
) collectorapi.JobConfigLifecycleSnapshot {
	return processLifecycleSnapshot{id}
}
func (*processLifecycleHook) Bind(collectorapi.JobConfigIdentity, collectorapi.RuntimeJob) {}

func (*processLifecycleHook) Capture(
	id collectorapi.JobConfigIdentity,
	_ collectorapi.RuntimeJob,
) collectorapi.JobConfigLifecycleSnapshot {
	return processLifecycleSnapshot{id}
}

func (h *processLifecycleHook) Reconcile(
	_ collectorapi.JobConfigIdentity,
	s collectorapi.JobConfigLifecycleSnapshot,
	_ collectorapi.RuntimeJob,
) {
	h.mu.Lock()
	h.current[s.Identity()] = true
	h.commits++
	h.mu.Unlock()
	h.changed <- struct{}{}
}
func (h *processLifecycleHook) Remove(id collectorapi.JobConfigIdentity) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.current, id)
	h.removes++
}
