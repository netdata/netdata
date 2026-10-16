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

func TestDiscoveredTrustRevocationRequiresPipelineIdentity(t *testing.T) {
	for name, test := range map[string]struct {
		incumbentID, replacementID, replacementSource string
		revoked, missingRuntime                       bool
	}{
		"same pipeline missing runtime":       {incumbentID: "pipeline-a", replacementID: "pipeline-a", revoked: true, missingRuntime: true},
		"same pipeline":                       {incumbentID: "pipeline-a", replacementID: "pipeline-a", revoked: true},
		"same pipeline changed target source": {incumbentID: "pipeline-a", replacementID: "pipeline-a", replacementSource: "changed-source", revoked: true},
		"competing pipeline same source":      {incumbentID: "pipeline-a", replacementID: "pipeline-b"},
		"competing pipeline different source": {incumbentID: "pipeline-a", replacementID: "pipeline-b", replacementSource: "competing-source"},
		"missing incumbent identity":          {replacementID: "pipeline-a"},
		"missing replacement identity":        {incumbentID: "pipeline-a"},
		"missing both identities":             {},
	} {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			owner := &intentTestOwner{}
			creator := controller.modules["module"]
			creator.JobConfigIntent = owner
			creator.Create = func() collectorapi.CollectorV1 {
				charts := collectorapi.Charts{}
				module := state.module(nil, false)
				module.ChartsFunc = func() *collectorapi.Charts { return &charts }
				return &trustDurationCollector{
					MockCollectorV1: *module,
				}
			}
			controller.modules["module"] = creator
			dependencies := jobsecrets.NewSecretDependencyIndex(controller.configModules.config.Configs)
			controller.dependencies = dependencies
			commands := runtimeTestNotifications(t, controller)
			activations := runtimeTestBindActivations(t, controller)
			store := &factoryTestAtomicScope{
				value: "1s",
			}
			store.current.Store(true)
			var acquisitions int
			controller.factory.config.ConfigModules.config.Configs = testConfigResolver(
				t,
				testAtomicResolver(t),
				func([]string) (secretresolver.AtomicScope, error) {
					acquisitions++
					return store, nil
				},
			)
			config := factoryTestConfig(false)
			config.Set("timeout", "${store:vault:main:value}")
			config.SetSourceType(confgroup.TypeDiscovered)
			config.SetSource("discovery-source")
			config.SetProvider("discovery")
			config.SetTrustDiscoveredTargets(true)
			config.SetDiscoveryPipelineID(test.incumbentID)
			permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
			scope := lifecycle.ResourceTransactionScope{
				ID: config.FullName(),
				Successor: lifecycle.ResourceIdentity{
					ID:         config.FullName(),
					Generation: 1,
				},
			}
			transaction, err := prepareDiscoveryTestFixture(t, controller, t.Context(), discoveryTestFixture{
				Config: config,
				Status: dyncfg.StatusRunning,
				Source: "pipeline-source-slot",
			}, nil, scope, permit)
			require.NoError(t, err)
			applied, err := transaction.Apply(t.Context())
			require.NoError(t, err)
			_, disposition, current := applied.Ownership()
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Nil(t, current)
			current = runtimeTestApplyActivation(t, activations, config.FullName(), 2)
			require.NotNil(t, current)
			current = runtimeTestApplyNotification(
				t,
				runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready"),
				current,
			)
			t.Cleanup(func() {
				if current != nil {
					require.NoError(t, current.Stop(context.Background()))
					require.NoError(t, current.Finalize())
				}
			})
			require.True(t, dependencies.Affects("vault:main", config.FullName(), true))
			require.Equal(t, 1, acquisitions)
			incumbentRecord, exists := graph.Lookup(config.FullName())
			require.True(t, exists)

			// The same duration is valid when resolved, but cannot decode as literal syntax.
			config.SetTrustDiscoveredTargets(false)
			if test.missingRuntime {
				creator.ActivationDependencies = intentAdmissionDependencies(true)
				controller.modules["module"] = creator
			}
			config.SetDiscoveryPipelineID(test.replacementID)
			if test.replacementSource != "" {
				config.SetSource(test.replacementSource)
			}
			permit, replacementTasks := issueTestJobPermit(t, config.FullName(), 3)
			scope.Current = current.Identity()
			scope.Successor.Generation = 3
			transaction, err = prepareDiscoveryTestFixture(t, controller, t.Context(), discoveryTestFixture{
				Config: config,
				Status: dyncfg.StatusRunning,
				Source: "pipeline-source-slot",
			}, current, scope, permit)
			if !test.revoked {
				require.NoError(t, err)
				applied, err = transaction.Apply(t.Context())
				require.NoError(t, err)
				_, disposition, retained := applied.Ownership()
				require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
				require.Same(t, current, retained)

				record, exists := graph.Lookup(config.FullName())
				require.True(t, exists)
				require.Equal(t, incumbentRecord, record)
				require.True(t, dependencies.Affects("vault:main", config.FullName(), true))
				require.Equal(t, 1, acquisitions)
				require.Equal(
					t,
					lifecycle.LongLivedCensus{},
					tasks.LongLivedCensus(),
					"acceptance released its unused permit",
				)
				require.Equal(t, lifecycle.LongLivedCensus{}, replacementTasks.LongLivedCensus())
				return
			}
			require.NoError(t, err)
			applied, err = transaction.Apply(t.Context())
			require.NoError(t, err)
			_, disposition, current = applied.Ownership()
			// The inventory fixture already applied withdrawal and retired the
			// runtime. This later completion publishes the valid Failed payload.
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Nil(t, current)
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			stored, err := graphRecordConfig(record)
			require.NoError(t, err)
			require.False(t, stored.TrustDiscoveredTargets())
			_, _, _, enabled := owner.snapshot()
			require.Zero(t, enabled, "revoked invalid configuration must release acquisition authority")
			require.False(t, dependencies.Affects("vault:main", config.FullName(), true))
			require.Equal(t, 1, acquisitions)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
			require.EqualValues(t, lifecycle.LongLivedCensus{}, replacementTasks.LongLivedCensus())
		})
	}
}

type trustDurationCollector struct {
	collectorapi.MockCollectorV1 `yaml:",inline"`
	Timeout                      confopt.Duration `yaml:"timeout"`
}
