// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	jobsecrets "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInventoryMetadataQuarantineCannotHideTrustWithdrawal(t *testing.T) {
	for _, pipelineID := range []string{"owner", "other", ""} {
		for _, state := range []string{"accepted", "running", "retry", "disabled"} {
			t.Run(pipelineID+"/"+state, func(t *testing.T) {
				controller, _, _, _, _ := newDynCfgJobTestHarness(t)
				creator := controller.modules["module"]
				controller.modules["snmp"] = creator
				creator.InstancePolicy = collectorapi.InstancePolicySingle
				controller.modules["snmp_topology"] = creator
				trusted := confgroup.Config{"module": "snmp", "name": "topology_job", "option_str": "trusted"}.
					SetSource("target").SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
					SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(true)
				// Same public name, but illegal name for this single-instance module.
				untrusted := confgroup.Config{"module": "snmp_topology", "name": "job", "option_str": "literal"}.
					SetSource("target").SetSourceType(confgroup.TypeDiscovered).SetProvider("test").
					SetDiscoveryPipelineID(pipelineID).SetTrustDiscoveredTargets(false)
				require.Equal(t, trusted.FullName(), untrusted.FullName())
				require.NoError(t, controller.validateCandidateMetadata(trusted))
				require.Error(t, controller.validateCandidateMetadata(untrusted))
				inventory := DiscoveredJobInventory{
					FullName: trusted.FullName(), Version: 1, AutoEnable: true,
					Sources:    map[string]uint64{"target": 1},
					Candidates: []DiscoveredJobCandidate{{Config: trusted, Source: "target", Incarnation: 1}},
				}
				applyInventory := func() {
					t.Helper()
					plan, err := planInventoryTest(controller, inventory)
					require.NoError(t, err)
					tx, err := plan.Transaction.Prepare(t.Context(), nil, lifecycle.ResourceTransactionScope{ID: inventory.FullName}, lifecycle.LongLivedPermit{})
					require.NoError(t, err)
					_, err = tx.Apply(t.Context())
					require.NoError(t, err)
				}
				applyInventory()
				c := controller.candidates
				old := c.snapshot(inventory.FullName).Proposal
				require.NotNil(t, old)
				// Seed only the legitimate incumbent outcome; withdrawal goes through
				// real inventory metadata validation and the transaction commit.
				delta, err := c.prepareOutcome(inventory.FullName, old.Token, candidateOutcome{Kind: candidateAdmitted, Listed: true}, 0)
				applyCandidateDelta(t, delta, err)
				switch state {
				case "running":
					delta, err = c.prepareReady(inventory.FullName, old.Token)
					applyCandidateDelta(t, delta, err)
				case "retry":
					delta, err = c.prepareOutcome(inventory.FullName, old.Token, candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0)
					applyCandidateDelta(t, delta, err)
				case "disabled":
					delta, err = c.prepareDisable(inventory.FullName)
					applyCandidateDelta(t, delta, err)
				}
				before := c.snapshot(inventory.FullName)
				inventory.Version++
				inventory.Candidates[0] = DiscoveredJobCandidate{Config: untrusted, Source: "target", Incarnation: 2}
				applyInventory()
				after := c.snapshot(inventory.FullName)
				retained := pipelineID != "owner"
				assert.Equal(t, retained, candidateAuthorized(after, old.Token.Candidate), "only an owning pipeline may withdraw its grant")
				if state == "accepted" || state == "running" {
					assert.Equal(t, retained, c.valid(inventory.FullName, old.Token), "withdrawal must invalidate incumbent execution authority")
				}
				if retained {
					assert.Equal(t, before.Binding, after.Binding)
					assert.Equal(t, before.Wake, after.Wake, "unrelated observations must retain retry deadlines")
				} else {
					assert.Nil(t, after.Binding, "quarantined metadata cannot replace or retain a withdrawn binding")
					assert.Nil(t, after.Wake)
				}
				if state == "disabled" {
					delta, err = c.prepareEnable(inventory.FullName, 3)
					if retained {
						require.NoError(t, err)
					}
					if err == nil {
						applyCandidateDelta(t, delta, err)
						if retained {
							require.NotNil(t, delta.Decision.Work)
						} else {
							assert.Nil(t, delta.Decision.Work, "ENABLE cannot resurrect trusted work")
						}
					}
				}
				if state == "retry" {
					require.NotNil(t, before.Wake)
					delta, err = c.prepareWake(inventory.FullName, before.Wake.Token, 10)
					applyCandidateDelta(t, delta, err)
					if retained {
						require.NotNil(t, delta.Decision.Work)
					} else {
						assert.Nil(t, delta.Decision.Work, "stale retry cannot resurrect trusted work")
					}
				}
			})
		}
	}
}

func TestInventoryTrustWithdrawalReleasesDependencies(t *testing.T) {
	for _, pipelineID := range []string{"owner", "other", ""} {
		t.Run(pipelineID, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
				charts := collectorapi.Charts{}
				module := state.module(nil, false)
				module.ChartsFunc = func() *collectorapi.Charts { return &charts }
				return &trustDurationCollector{MockCollectorV1: *module}
			}
			controller.modules["module"] = creator
			dependencies := jobsecrets.NewSecretDependencyIndex(controller.configModules.config.Configs)
			controller.dependencies = dependencies
			commands := runtimeTestNotifications(t, controller)
			activations := runtimeTestBindActivations(t, controller)
			store := &factoryTestAtomicScope{value: "1s"}
			store.current.Store(true)
			controller.factory.config.ConfigModules.config.Configs = testConfigResolver(
				t, testAtomicResolver(t), func([]string) (secretresolver.AtomicScope, error) { return store, nil },
			)
			trusted := factoryTestConfig(false).Set("timeout", "${store:vault:main:value}").
				SetSourceType(confgroup.TypeDiscovered).SetSource("target").SetProvider("test").
				SetDiscoveryPipelineID("owner").SetTrustDiscoveredTargets(true)
			permit, _ := issueTestJobPermit(t, trusted.FullName(), 1)
			scope := lifecycle.ResourceTransactionScope{
				ID:        trusted.FullName(),
				Successor: lifecycle.ResourceIdentity{ID: trusted.FullName(), Generation: 1},
			}
			tx, err := prepareDiscoveryTestFixture(t, controller, t.Context(), discoveryTestFixture{
				Config: trusted, Status: dyncfg.StatusRunning, Source: "target",
			}, nil, scope, permit)
			require.NoError(t, err)
			_, err = tx.Apply(t.Context())
			require.NoError(t, err)
			current := runtimeTestApplyActivation(t, activations, trusted.FullName(), 2)
			require.NotNil(t, current)
			current = runtimeTestApplyNotification(t, runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready"), current)
			t.Cleanup(func() {
				if current != nil {
					require.NoError(t, current.Stop(context.Background()))
					require.NoError(t, current.Finalize())
				}
			})
			require.True(t, dependencies.Affects("vault:main", trusted.FullName(), true))
			before, exists := graph.Lookup(trusted.FullName())
			require.True(t, exists)

			untrusted := confgroup.Config{"module": trusted.FullName(), "name": trusted.FullName(), "timeout": "literal"}.
				SetSourceType(confgroup.TypeDiscovered).SetSource("target").SetProvider("test").
				SetDiscoveryPipelineID(pipelineID).SetTrustDiscoveredTargets(false)
			require.Equal(t, trusted.FullName(), untrusted.FullName())
			require.Error(t, controller.validateCandidateMetadata(untrusted))
			plan, err := planSourceTestConfig(controller, untrusted, true, false, "target")
			require.NoError(t, err)
			scope.Current = current.Identity()
			scope.Successor = lifecycle.ResourceIdentity{}
			tx, err = plan.Transaction.Prepare(t.Context(), current, scope, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			applied, err := tx.Apply(t.Context())
			require.NoError(t, err)
			_, disposition, next := applied.Ownership()
			retained := pipelineID != "owner"
			if retained {
				assert.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
				assert.Same(t, current, next)
			} else {
				assert.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
				assert.Nil(t, next)
			}
			current = next
			after, exists := graph.Lookup(trusted.FullName())
			assert.Equal(t, retained, exists, "invalid replacement must never acquire exposed ownership")
			if retained {
				assert.Equal(t, before, after)
			}
			assert.Equal(t, retained, dependencies.Affects("vault:main", trusted.FullName(), true))
		})
	}
}
