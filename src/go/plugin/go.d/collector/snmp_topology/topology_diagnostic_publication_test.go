// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticProviderAcquiresImmutableGenerationWithoutRefreshLock(t *testing.T) {
	c := newTestSNMPTopologyCollector()
	c.refreshTopologyRecovering(context.Background())
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := c.diagnosticProvider.Capture(); done <- err }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("diagnostic capture acquired refresh lock")
	}
}

func TestTopologyDiagnosticHookPublishesOnlyAcceptedProvider(t *testing.T) {
	_, store := newTestSNMPTopologyCollectorWithStore()
	store.RegisterJob("device", ddsnmp.DeviceLifecycleInfo{Hostname: "switch.example"})
	dir := t.TempDir()
	publisher := snmpdiag.NewPublisher(store, dir)
	creator := Creator(store, NewTrapEnrichmentHandle(), newTestReverseDNSResolver(), publisher)
	hook := creator.JobConfigLifecycle
	id := collectorapi.JobConfigIdentity{1}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	start := func(scope string) (*Collector, context.CancelFunc, <-chan error) {
		c := creator.CreateV2().(*Collector)
		c.topologyRegistry.producerScopeID = scope
		c.UpdateEvery = 1
		c.RefreshEvery = confopt.LongDuration(time.Second)
		require.NoError(t, c.Init(ctx))
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- c.Run(runCtx) }()
		require.Eventually(t, func() bool {
			generation := c.topologyRegistry.acquireGeneration()
			return generation != nil && generation.sequence > 0
		}, time.Second, time.Millisecond)
		return c, stop, done
	}
	incumbent, stopIncumbent, incumbentDone := start("incumbent")
	hook.Reconcile(collectorapi.JobConfigIdentity{}, hook.Capture(id, topologyLifecycleTestJob{incumbent}), topologyLifecycleTestJob{incumbent})
	publisherDone := make(chan struct{})
	go func() { publisher.Run(ctx); close(publisherDone) }()
	read := func() (snmpdiag.Document, error) {
		f, err := os.Open(snmpdiag.ArchivePath(dir))
		if err != nil {
			return snmpdiag.Document{}, err
		}
		defer f.Close()
		return snmpdiag.Read(f, snmpdiag.DefaultReadLimits())
	}
	require.Eventually(t, func() bool {
		d, err := read()
		return err == nil && d.Snapshot.ProducerScopeID == incumbent.topologyRegistry.producerScope()
	}, time.Second, time.Millisecond)
	before, err := read()
	require.NoError(t, err)
	candidate, stopCandidate, candidateDone := start("candidate")
	require.NotEqual(t,
		incumbent.topologyRegistry.producerScope(), candidate.topologyRegistry.producerScope(),
		"ownership assertions need distinct producer scopes",
	)
	hook.Bind(id, topologyLifecycleTestJob{candidate})
	hook.Capture(id, topologyLifecycleTestJob{candidate})
	// Wait for another real disk publication to prove that merely running the
	// candidate did not select it as the diagnostic provider.
	require.Eventually(t, func() bool {
		d, err := read()
		return err == nil && d.Snapshot.Lifecycle.Cut.CapturedAt.After(before.Snapshot.Lifecycle.Cut.CapturedAt)
	}, 3*time.Second, 5*time.Millisecond)
	actual, err := read()
	require.NoError(t, err)
	require.Equal(t, incumbent.topologyRegistry.producerScope(), actual.Snapshot.ProducerScopeID)
	hook.Reconcile(id, hook.Capture(id, topologyLifecycleTestJob{candidate}), topologyLifecycleTestJob{candidate})
	stopIncumbent()
	require.NoError(t, <-incumbentDone)
	incumbent.Cleanup(ctx)
	require.Eventually(t, func() bool {
		d, err := read()
		return err == nil && d.Snapshot.ProducerScopeID == candidate.topologyRegistry.producerScope()
	}, time.Second, time.Millisecond)
	// Stop the selected runner without config removal. A fresh publisher run
	// forces a checkpoint without waiting for the 30-minute fallback interval.
	cancel()
	<-publisherDone
	stopCandidate()
	require.NoError(t, <-candidateDone)
	nextCtx, nextCancel := context.WithCancel(t.Context())
	defer nextCancel()
	nextDone := make(chan struct{})
	go func() { publisher.Run(nextCtx); close(nextDone) }()
	require.Eventually(t, func() bool {
		d, err := read()
		return err == nil && d.Snapshot.ProducerScopeID == "" && d.Snapshot.Topology == nil
	}, time.Second, time.Millisecond)
	nextCancel()
	<-nextDone
	candidate.Cleanup(ctx)
}

type topologyLifecycleTestJob struct{ c *Collector }

func (topologyLifecycleTestJob) FullName() string   { return "snmp_topology_test" }
func (topologyLifecycleTestJob) ModuleName() string { return "snmp_topology" }
func (topologyLifecycleTestJob) Name() string       { return "test" }
func (topologyLifecycleTestJob) IsRunning() bool    { return true }
func (j topologyLifecycleTestJob) Collector() any   { return j.c }
