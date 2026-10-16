// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	filediscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestProtectedSourceHistoryAfterSDFileModuleReplacement(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	var constructions atomic.Int32
	creator := controller.modules["module"]
	create := creator.Create
	creator.Create = func() collectorapi.CollectorV1 { constructions.Add(1); return create() }
	controller.modules["snmp"] = creator
	creator.InstancePolicy = collectorapi.InstancePolicySingle
	controller.modules["snmp_topology"] = creator
	path := filepath.Join(t.TempDir(), "etc", "jobs.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
	read := func(payload string) DiscoveredJobCandidate {
		require.NoError(t, os.WriteFile(path, []byte(payload), 0600))
		groups := make(chan []*confgroup.Group, 1)
		filediscovery.NewReader(confgroup.Registry{"snmp": {}, "snmp_topology": {}}, []string{path}).Run(t.Context(), groups)
		batch := <-groups
		require.Len(t, batch, 1)
		require.Len(t, batch[0].Configs, 1)
		return DiscoveredJobCandidate{Config: batch[0].Configs[0], Source: batch[0].Source}
	}
	good := func(value string) confgroup.Config {
		return confgroup.Config{"module": "snmp", "name": "topology_job", "option_str": value}.
			SetSource("discovered-slot").SetSourceType(confgroup.TypeDiscovered).SetProvider("test")
	}
	inventory := DiscoveredJobInventory{
		FullName: "snmp_topology_job", Version: 1, AutoEnable: true, Sources: map[string]uint64{"discovered-slot": 1},
		Candidates: []DiscoveredJobCandidate{{Config: good("incumbent"), Source: "discovered-slot", Incarnation: 1}},
	}
	applyInventory := func() {
		plan, err := planInventoryTest(controller, inventory)
		require.NoError(t, err)
		transaction, err := plan.Transaction.Prepare(t.Context(), nil, lifecycle.ResourceTransactionScope{ID: inventory.FullName}, lifecycle.LongLivedPermit{})
		require.NoError(t, err)
		_, err = transaction.Apply(context.Background())
		require.NoError(t, err)
	}
	c := controller.candidates
	applyInventory()
	initial := c.snapshot(inventory.FullName).Proposal
	require.NotNil(t, initial)
	delta, err := c.prepareOutcome(inventory.FullName, initial.Token, candidateOutcome{Kind: candidateAdmitted, Listed: true}, 0)
	applyCandidateDelta(t, delta, err)
	inventory.Version++
	inventory.Candidates[0] = DiscoveredJobCandidate{Config: good("${store:invalid}"), Source: "discovered-slot", Incarnation: 2}
	applyInventory()
	rejected := c.snapshot(inventory.FullName).Proposal
	require.NotNil(t, rejected)
	delta, err = c.prepareOutcome(inventory.FullName, rejected.Token, candidateOutcome{Kind: candidateRejected}, 0)
	applyCandidateDelta(t, delta, err)

	user := read("- module: snmp\n  name: topology_job\n  option_str: pending\n")
	user.Incarnation = 3
	require.Equal(t, confgroup.TypeUser, user.Config.SourceType())
	require.Equal(t, inventory.FullName, user.Config.FullName())
	require.NoError(t, controller.validateCandidateMetadata(user.Config))
	inventory.Version++
	inventory.Sources[user.Source] = 1
	inventory.Candidates = append(inventory.Candidates, user)
	applyInventory()
	proposed := c.snapshot(inventory.FullName).Proposal
	require.NotNil(t, proposed)

	invalid := read("- module: snmp_topology\n  name: job\n  option_str: invalid\n")
	invalid.Incarnation = 4
	require.Equal(t, inventory.FullName, invalid.Config.FullName(), "registered module/name pairs can share FullName")
	require.Error(t, controller.validateCandidateMetadata(invalid.Config))
	inventory.Version++
	inventory.Candidates[1] = invalid
	applyInventory()
	after := c.snapshot(inventory.FullName)
	require.Equal(t, initial.Token.Candidate, after.Binding.Key)
	require.True(t, candidateAuthorized(after, initial.Token.Candidate))
	require.False(t, c.valid(inventory.FullName, proposed.Token))
	require.Nil(t, after.Proposal)
	require.NotNil(t, after.Cursor, "rejected replacement must preserve consumed observation history")
	require.Nil(t, after.Entries[after.Cursor.Key], "history need not have executable membership")
	require.Zero(t, constructions.Load(), "invalid metadata and protected history must not construct collectors")

	valid := read("- module: snmp\n  name: topology_job\n  option_str: valid-replacement\n")
	valid.Incarnation = 5
	inventory.Version++
	inventory.Candidates[1] = valid
	applyInventory()
	after = c.snapshot(inventory.FullName)
	require.NotNil(t, after.Proposal, "a genuine later valid change must remain eligible")
	require.Equal(t, valid.Config.UID(), after.Proposal.Config.UID())
	require.Equal(t, initial.Token.Candidate, after.Binding.Key, "preflight has not adopted the valid replacement")
}
