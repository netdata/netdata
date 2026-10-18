// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestEnablePreservesEnabledCandidateRetryState(t *testing.T) {
	for _, sourceType := range []string{confgroup.TypeStock, confgroup.TypeUser, confgroup.TypeDiscovered, confgroup.TypeDyncfg} {
		for caller, source := range map[string]string{"daemon": "", "user": "user=test", "saved source": "user=previous"} {
			t.Run(sourceType+"/"+caller, func(t *testing.T) {
				controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
				a := factoryTestConfig(false).SetSourceType(sourceType).SetSource("file=one").Set("option_str", "one")
				b := factoryTestConfig(false).SetSourceType(sourceType).SetSource("file=two").Set("option_str", "two")
				configs := []confgroup.Config{a, b}
				if sourceType == confgroup.TypeDyncfg {
					configs = configs[:1]
				}
				inventory := candidateTestInventory(1, true, configs...)
				inventory.FullName = a.FullName()
				first, err := controller.candidates.prepareInventory(inventory, 0)
				applyCandidateDelta(t, first, err)
				work := first.Decision.Work
				for range len(configs) {
					require.NotNil(t, work)
					delta, err := controller.candidates.prepareOutcome(a.FullName(), work.Token,
						candidateOutcome{Kind: candidateFailed, Listed: true, RetryAfter: 10}, 0)
					applyCandidateDelta(t, delta, err)
					work = delta.Decision.Work
				}
				require.Nil(t, work)
				before := controller.candidates.snapshot(a.FullName())
				require.NotNil(t, before.Wake)
				require.Equal(t, int64(10), before.Wake.Due)
				postimage, err := candidateGraphBinding(a.FullName(), before.Binding)
				require.NoError(t, err)
				mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{ID: a.FullName(), Config: postimage}})
				require.NoError(t, err)
				require.NoError(t, graph.Commit(mutation))
				record, ok := graph.Lookup(a.FullName())
				require.True(t, ok)

				permit, tasks := issueTestJobPermit(t, a.FullName(), 1)
				scope := lifecycle.ResourceTransactionScope{
					ID:        a.FullName(),
					Successor: lifecycle.ResourceIdentity{ID: a.FullName(), Generation: 1},
				}
				transaction, err := controller.Prepare(context.Background(), DynCfgJobRequest{
					Args:         []string{"go.d:collector:module:job", "enable"},
					CallerSource: source,
				}, nil, scope, permit)
				require.NoError(t, err)
				applied, err := transaction.Apply(context.Background())
				require.NoError(t, err)
				_, disposition, current := applied.Ownership()
				require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
				require.Nil(t, current)
				require.Equal(t, 200, applied.ResultStatus())
				require.Equal(t, before, controller.candidates.snapshot(a.FullName()),
					"reconciliation must retain the exact intent, pass, grants, and absolute retry deadline")
				after, ok := graph.Lookup(a.FullName())
				require.True(t, ok)
				require.Equal(t, record, after)
				require.Equal(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
			})
		}
	}
}
