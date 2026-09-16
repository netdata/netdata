// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	jobsecrets "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestDiscoveredTrustRevocationRemovesInvalidLiteralIncumbent(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		charts := collectorapi.Charts{}
		module := state.module(nil, false)
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return &trustDurationCollector{MockCollectorV1: *module}
	}
	controller.modules["module"] = creator
	dependencies := jobsecrets.NewSecretDependencyIndex()
	controller.dependencies = dependencies
	store := &factoryTestAtomicScope{value: "1s"}
	store.current.Store(true)
	var acquisitions int
	controller.factory.config.ConfigModules.config.StoreScope = func([]string) (secretresolver.AtomicScope, error) {
		acquisitions++
		return store, nil
	}
	config := factoryTestConfig(false)
	config.Set("timeout", "${store:vault:main:value}")
	config.SetSourceType(confgroup.TypeDiscovered)
	config.SetSource("discovery-source")
	config.SetProvider("discovery")
	config.SetTrustDiscoveredTargets(true)
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	transaction, err := controller.prepareDiscovered(t.Context(), DiscoveredJobChange{
		Config: config,
		Status: dyncfg.StatusRunning,
	}, nil, scope, permit)
	require.NoError(t, err)
	applied, err := transaction.Apply(t.Context())
	require.NoError(t, err)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionInstalled, disposition)
	require.NotNil(t, current)
	t.Cleanup(func() {
		if current != nil {
			require.NoError(t, current.Stop(context.Background()))
			require.NoError(t, current.Finalize())
		}
	})
	require.True(t, dependencies.Affects("vault:main", config.FullName(), true))
	require.Equal(t, 1, acquisitions)

	// The same duration is valid when resolved, but cannot decode as literal syntax.
	config.SetTrustDiscoveredTargets(false)
	permit, replacementTasks := issueTestJobPermit(t, config.FullName(), 2)
	scope.Current = scope.Successor
	scope.Successor.Generation = 2
	transaction, err = controller.prepareDiscovered(t.Context(), DiscoveredJobChange{
		Config: config,
		Status: dyncfg.StatusRunning,
	}, current, scope, permit)
	require.NoError(t, err)
	applied, err = transaction.Apply(t.Context())
	require.NoError(t, err)
	_, disposition, current = applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, current)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	stored, err := graphRecordConfig(record)
	require.NoError(t, err)
	require.False(t, stored.TrustDiscoveredTargets())
	require.False(t, dependencies.Affects("vault:main", config.FullName(), true))
	require.Equal(t, 1, acquisitions)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, replacementTasks.LongLivedCensus())
}

type trustDurationCollector struct {
	collectorapi.MockCollectorV1 `yaml:",inline"`
	Timeout                      confopt.Duration `yaml:"timeout"`
}
