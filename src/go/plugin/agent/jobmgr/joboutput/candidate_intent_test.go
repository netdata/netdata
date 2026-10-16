// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestCandidateFailureTransfersResourceIntentWithBinding(t *testing.T) {
	for _, sourceType := range []string{confgroup.TypeStock, confgroup.TypeUser} {
		t.Run(sourceType, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			checked := make(chan string, 2)
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
				module := &collectorapi.MockCollectorV1{}
				module.CheckFunc = func(context.Context) error {
					checked <- module.Config.OptionStr
					return errors.New("endpoint unavailable")
				}
				return module
			}
			controller.modules["module"] = creator
			owner := &intentTestOwner{}
			installIntentTestOwner(controller, owner)
			commands := runtimeTestBindActivations(t, controller)
			configs := []confgroup.Config{
				acceptedActivationTestConfig(sourceType).Set("option_str", "first").Set("autodetection_retry", 0),
				acceptedActivationTestConfig(sourceType).Set("option_str", "second").Set("autodetection_retry", 0),
			}
			byOption := map[string]confgroup.Config{"first": configs[0], "second": configs[1]}
			inventory := DiscoveredJobInventory{
				FullName: configs[0].FullName(), Version: 1, AutoEnable: true,
				Sources: map[string]uint64{"source": 1},
				Candidates: []DiscoveredJobCandidate{
					{Config: configs[0], Source: "source", Incarnation: 1},
					{Config: configs[1], Source: "source", Incarnation: 1},
				},
			}
			plan, err := controller.PlanInventory(inventory)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			var previous confgroup.Config
			for generation := uint64(1); generation <= 2; generation++ {
				call := commands.next(t, "internal/jobs/accepted-activation")
				config := byOption[<-checked]
				require.NotNil(t, config)
				require.Nil(t, applyActivationTestSubmission(t, call, nil, generation))
				owner.mu.Lock()
				ownsNext := owner.enabled[jobConfigIdentity(config)]
				ownsPrevious := previous != nil && owner.enabled[jobConfigIdentity(previous)]
				count := len(owner.enabled)
				owner.mu.Unlock()
				require.True(t, ownsNext, "last admitted failed candidate owns resource demand")
				require.False(t, ownsPrevious, "replacement must retire predecessor demand")
				require.Equal(t, 1, count)
				record, listed := graph.Lookup(config.FullName())
				if sourceType == confgroup.TypeStock {
					require.False(t, listed, "private recovery must not list failed stock")
				} else {
					require.True(t, listed)
					require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
					exposed, err := graphRecordConfig(record)
					require.NoError(t, err)
					require.Equal(t, config, exposed)
				}
				previous = config
			}
			inventory.Version++
			inventory.Candidates = nil
			inventory.Sources = nil
			plan, err = controller.PlanInventory(inventory)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			_, _, _, enabled := owner.snapshot()
			require.Zero(t, enabled, "source removal retires private and listed demand")
		})
	}
}
