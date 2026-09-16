// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	go func() {
		checkpoints := c.diagnosticProvider.Checkpoints()
		if len(checkpoints) == 0 {
			done <- errors.New("no committed checkpoint")
			return
		}
		_, err := checkpoints[0].Capture()
		done <- err
	}()
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
		directory := snmpdiag.DirectoryPath(dir)
		entries, err := snmpdiag.ListCheckpoints(directory)
		if err != nil {
			return snmpdiag.Document{}, err
		}
		if len(entries) == 0 {
			return snmpdiag.Document{}, errors.New("no checkpoints")
		}
		f, err := os.Open(filepath.Join(directory, snmpdiag.TopologyDirectory, entries[len(entries)-1].Filename))
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
	candidate, stopCandidate, candidateDone := start("candidate")
	require.NotEqual(t,
		incumbent.topologyRegistry.producerScope(), candidate.topologyRegistry.producerScope(),
		"ownership assertions need distinct producer scopes",
	)
	hook.Bind(id, topologyLifecycleTestJob{candidate})
	hook.Capture(id, topologyLifecycleTestJob{candidate})
	// The candidate has completed a real generation, but is not admitted.
	require.NotEmpty(t, candidate.diagnosticProvider.Checkpoints())
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
	// Releasing the selected runner updates current status without erasing history.
	stopCandidate()
	require.NoError(t, <-candidateDone)
	require.Eventually(t, func() bool {
		f, err := os.Open(filepath.Join(snmpdiag.DirectoryPath(dir), snmpdiag.LifecycleFilename))
		if err != nil {
			return false
		}
		defer f.Close()
		d, err := snmpdiag.Read(f, snmpdiag.DefaultReadLimits())
		return err == nil && !d.TopologyActive
	}, time.Second, time.Millisecond)
	historical, err := read()
	require.NoError(t, err)
	require.Equal(t, "candidate", historical.Snapshot.ProducerScopeID)
	cancel()
	<-publisherDone
	candidate.Cleanup(ctx)
}

type topologyLifecycleTestJob struct{ c *Collector }

func (topologyLifecycleTestJob) FullName() string   { return "snmp_topology_test" }
func (topologyLifecycleTestJob) ModuleName() string { return "snmp_topology" }
func (topologyLifecycleTestJob) Name() string       { return "test" }
func (topologyLifecycleTestJob) IsRunning() bool    { return true }
func (j topologyLifecycleTestJob) Collector() any   { return j.c }

func TestAbortedFirstSweepSurvivesProviderRelease(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	dir := t.TempDir()
	publisher := snmpdiag.NewPublisher(store, dir)
	coll.diagnosticPublisher = publisher
	publisher.SetTopology("accepted", coll.diagnosticProvider)
	aborted, abort := context.WithCancel(t.Context())
	abort()
	coll.refreshTopology(aborted)
	coll.releaseDiagnosticProvider()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); publisher.Run(ctx) }()
	defer func() { cancel(); <-done }()
	directory := snmpdiag.DirectoryPath(dir)
	var entries []snmpdiag.CheckpointFile
	require.Eventually(t, func() bool {
		var err error
		entries, err = snmpdiag.ListCheckpoints(directory)
		return err == nil && len(entries) == 1
	}, time.Second, time.Millisecond)
	file, err := os.Open(filepath.Join(directory, snmpdiag.TopologyDirectory, entries[0].Filename))
	require.NoError(t, err)
	defer file.Close()
	archive, err := snmpdiag.Read(file, snmpdiag.DefaultReadLimits())
	require.NoError(t, err)
	require.Nil(t, archive.Snapshot.Topology)
	require.NotNil(t, archive.Snapshot.LastAborted)
	require.Equal(t, "canceled", archive.Snapshot.LastAborted.Reason)
	require.NotEmpty(t, archive.Producer.RunID)
}
