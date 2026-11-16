// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type candidateOwnershipParseGates struct {
	sequence atomic.Int32
	entered  chan int
	gates    [2]chan struct{}
	once     [2]sync.Once
}

func (g *candidateOwnershipParseGates) release(i int) { g.once[i].Do(func() { close(g.gates[i]) }) }

type candidateOwnershipParsingCollector struct {
	continuationV2Collector
	gates *candidateOwnershipParseGates
}

func (c *candidateOwnershipParsingCollector) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if raw["option_str"] == "invalid" {
		i := int(c.gates.sequence.Add(1)) - 1
		if i < len(c.gates.gates) {
			c.gates.entered <- i
			<-c.gates.gates[i]
		}
	}
	return unmarshal(&c.Config)
}

func TestCandidateWorkOwnershipReenableRejectsSelectedPayload(t *testing.T) {
	gates := &candidateOwnershipParseGates{
		entered: make(chan int, 8),
	}
	for i := range gates.gates {
		gates.gates[i] = make(chan struct{})
		defer gates.release(i)
	}
	state := &continuationCollectorState{
		check: func(context.Context, string) error { return errors.New("endpoint unavailable") },
	}
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	creator := collectorapi.Creator{
		JobConfigSchema: collectorapi.MockConfigSchema,
		Config:          func() any { return &collectorapi.MockConfiguration{} },
		CreateV2: func() collectorapi.CollectorV2 {
			return &candidateOwnershipParsingCollector{
				continuationV2Collector: continuationV2Collector{
					readinessIntegrationCollector: readinessIntegrationCollector{
						store: metrix.NewCollectorStore(),
					},
					state: state,
				},
				gates: gates,
			}
		},
	}
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Generation:      1,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules: collectorapi.Registry{
			"module": creator,
		},
		Jobs: testRunJobServices(t),
		Discovery: runDiscoveryServices{
			Disabled: true,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		for i := range gates.gates {
			gates.release(i)
		}
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	fixture := &continuationFixture{
		t:          t,
		generation: generation,
		output:     output,
		state:      state,
	}
	cfg := func(source, value, typ string) confgroup.Config {
		return confgroup.Config{
			"module":              "module",
			"name":                "local2",
			"option_str":          value,
			"update_every":        1,
			"autodetection_retry": 60,
		}.SetSource(source).SetSourceType(typ).SetProvider("test")
	}
	d := cfg("d", "predecessor", confgroup.TypeDiscovered)
	inv := joboutput.DiscoveredJobInventory{
		FullName:   d.FullName(),
		Version:    1,
		AutoEnable: true,
		Sources:    map[string]uint64{"d": 1},
		Candidates: []joboutput.DiscoveredJobCandidate{{Config: d, Source: "d", Incarnation: 1}},
	}
	observe := func() {
		t.Helper()
		plan, err := generation.dyncfg.PlanInventory(joboutput.DiscoveredJobInventoryBatch{
			FullName:     inv.FullName,
			Observations: []joboutput.DiscoveredJobInventory{inv},
		})
		require.NoError(t, err)
		require.NoError(t, generation.kernel.SubmitPreparedAndWait(t.Context(), jobmgr.Request{
			UID:     fmt.Sprintf("work-ownership-inventory-%d", inv.Version),
			LaneKey: inv.FullName,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/discovery/inventory",
		}, plan))
	}
	entered := func(i int) {
		t.Helper()
		select {
		case got := <-gates.entered:
			require.Equal(t, i, got)
		case <-time.After(3 * time.Second):
			t.Fatalf("parse %d never entered; output=%s", i, output)
		}
	}
	observe()
	fixture.waitStatus(dyncfg.StatusFailed)
	u := cfg("u", "invalid", confgroup.TypeUser)
	u["option_int"] = "not-an-integer"
	inv.Version++
	inv.Sources["u"] = 1
	inv.Candidates = append(inv.Candidates, joboutput.DiscoveredJobCandidate{
		Config:      u,
		Source:      "u",
		Incarnation: 2,
	})
	observe()
	entered(0)
	fixture.command("disable-before-replacement", "disable", 200)
	fixture.command("reenable-replacement", "enable", 202)
	gates.release(0)
	entered(1)
	s := cfg("s", "masked", confgroup.TypeStock)
	inv.Version++
	inv.Sources["s"] = 1
	inv.Candidates = append(inv.Candidates, joboutput.DiscoveredJobCandidate{
		Config:      s,
		Source:      "s",
		Incarnation: 3,
	})
	observe()
	gates.release(1)
	// The unrelated observation must preserve U's existing execution. Its real
	// decoder rejection then advances D without a third U preparation.
	require.Eventually(
		t,
		func() bool { return len(state.checked()) == 2 },
		2*time.Second,
		time.Millisecond,
		"selected payload rejection must resume D; output=%s",
		output,
	)
	fixture.waitStatus(dyncfg.StatusFailed)
	require.EqualValues(
		t,
		2,
		gates.sequence.Load(),
		"one tentative and one re-enabled U attempt; inventory cannot add another",
	)
	fixture.command("disable-before-retry", "disable", 200)
	fixture.command("reenable", "enable", 202)
	require.Eventually(
		t,
		func() bool { return len(state.checked()) == 3 },
		2*time.Second,
		time.Millisecond,
		"a real enablement transition starts predecessor recovery; output=%s",
		output,
	)
	require.Equal(
		t,
		[]string{"predecessor", "predecessor", "predecessor"},
		state.checked(),
		"invalid user and masked stock cannot reach Check",
	)
	require.EqualValues(t, 2, gates.sequence.Load())
	require.NoError(t, generation.run.DirtyCause())
}
