// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/stretchr/testify/require"
)

func TestNormalSNMPFailedReadinessPublishesWithoutTopology(t *testing.T) {
	dir := t.TempDir()
	registry, publisher := NewRegistry(dir)
	// Exercise the real normal-SNMP Creator and failed Init commit with the
	// topology module entirely absent from enabled modules.
	modules := collectorapi.Registry{
		"snmp": registry["snmp"],
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	process, err := composition.NewProcess(composition.Config{
		Input:      reader,
		Output:     io.Discard,
		PluginName: "go.d",
		Modules:    modules,
		Defaults: confgroup.Registry{
			"snmp": {},
		},
		AutoEnable:      true,
		ShutdownTimeout: time.Second,
		Services:        []composition.ProcessService{publisher},
		DiscoveryProviders: []agentdiscovery.ProviderFactory{
			agentdiscovery.NewProviderFactory(
				"test",
				func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
					return diagnosticTestDiscovery{}, true, nil
				},
			),
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	var document snmpdiag.Document
	require.Eventually(t, func() bool {
		file, err := os.Open(snmpdiag.ArchivePath(dir))
		if err != nil {
			return false
		}
		defer file.Close()
		document, err = snmpdiag.Read(file, snmpdiag.DefaultReadLimits())
		return err == nil && len(document.Snapshot.Lifecycle.Cut.Entries) == 1
	}, 3*time.Second, 5*time.Millisecond)
	require.Nil(t, document.Snapshot.Topology)
	row := document.Snapshot.Lifecycle.Cut.Entries[0]
	require.Equal(t, "init", row.LastCompleted.Phase)
	require.Equal(t, "failed", row.LastCompleted.Outcome)
	require.False(t, row.TopologyReady)
	archive, err := snmptopology.InspectDiagnosticDocument(document)
	require.NoError(t, err)
	_, err = archive.Summary()
	require.NoError(t, err)
	_, err = archive.Replay(snmptopology.DefaultDiagnosticQueryOptions())
	require.ErrorContains(t, err, "no replayable topology")
	before, err := os.ReadFile(snmpdiag.ArchivePath(dir))
	require.NoError(t, err)
	require.NoError(t, process.Terminate(ctx))
	require.NoError(t, <-done)
	after, err := os.ReadFile(snmpdiag.ArchivePath(dir))
	require.NoError(t, err)
	require.Equal(t, before, after, "shutdown must preserve useful evidence, not publish cleared state")
}

type diagnosticTestDiscovery struct{}

func (diagnosticTestDiscovery) Run(ctx context.Context, out chan<- []*confgroup.Group) {
	config := confgroup.Config{
		"module":       "snmp",
		"name":         "invalid-device",
		"hostname":     "",
		"update_every": 10,
	}
	config.SetProvider("test")
	config.SetSourceType(confgroup.TypeUser)
	config.SetSource("file=test")
	select {
	case out <- []*confgroup.Group{{Source: "test", Configs: []confgroup.Config{config}}}:
	case <-ctx.Done():
		return
	}
	<-ctx.Done()
}

func TestSNMPRegistryInstancesHaveIndependentState(t *testing.T) {
	first, firstPublisher := NewRegistry(t.TempDir())
	second, secondPublisher := NewRegistry(t.TempDir())
	require.NotSame(t, firstPublisher, secondPublisher)
	require.NotEqual(
		t,
		pointerField(t, first["snmp"].Create(), "deviceStore"),
		pointerField(t, second["snmp"].Create(), "deviceStore"),
	)
	require.NotContains(t, collectorapi.DefaultRegistry, "snmp")
}
