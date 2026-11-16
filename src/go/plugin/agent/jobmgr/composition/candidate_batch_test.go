// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestCandidateBatchSourcePriorityBeforeConstruction(t *testing.T) {
	tiers := []string{"internal", confgroup.TypeStock, confgroup.TypeDiscovered, confgroup.TypeUser, confgroup.TypeDyncfg}
	for _, version := range []string{"v1", "v2"} {
		for mode, auto := range map[string]bool{"automatic": true, "daemon": false} {
			for i := 1; i < len(tiers); i++ {
				for _, lowerFirst := range []bool{true, false} {
					name := fmt.Sprintf("%s/%s/%s-over-%s/lower-first=%t", version, mode, tiers[i], tiers[i-1], lowerFirst)
					t.Run(name, func(t *testing.T) {
						probed := make(chan string, 4)
						release := make(chan struct{})
						defer close(release)
						state := &continuationCollectorState{check: func(_ context.Context, option string) error {
							probed <- option
							if option == "lower" {
								<-release
							}
							return nil
						}}
						config := func(source, tier, option string) confgroup.Config {
							return confgroup.Config{"module": "module", "name": "local2", "update_every": 1, "option_str": option}.
								SetSource(source).SetSourceType(tier).SetProvider("test")
						}
						lower := config("a-source", tiers[i-1], "lower")
						higher := config("z-source", tiers[i], "higher")
						configs := []confgroup.Config{lower, higher}
						if !lowerFirst {
							configs[0], configs[1] = configs[1], configs[0]
						}
						fixture := newContinuationFixture(t, version, auto, state, configs)
						fixture.enableIfPassive(auto)
						select {
						case option := <-probed:
							require.Equal(t, "higher", option, "known lower-tier work must never acquire the process identity")
						case <-time.After(3 * time.Second):
							t.Fatal("complete batch did not release its selected candidate")
						}
						fixture.waitStatus(dyncfg.StatusRunning)
						require.Equal(t, []string{"higher"}, state.checked())
						require.EqualValues(t, 1, state.inits.Load(), "lower priority must be excluded before Init")
						fixture.assertExposed("batch-selected", higher)
						fixture.assertLastMetadata(higher)
						require.NoError(t, fixture.generation.run.DirtyCause())
					})
				}
			}
		}
	}
}
