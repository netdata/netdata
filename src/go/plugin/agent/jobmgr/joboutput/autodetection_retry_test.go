// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestAutoDetectionRetryIndexDispatchesOnlyCurrentDueEntries(
	t *testing.T,
) {
	tests := map[string]struct {
		arrange func(
			*autoDetectionRetryIndex,
			confgroup.Config,
		) confgroup.Config
		clocks       []int
		wantDispatch int
	}{
		"due retry dispatches": {
			arrange: func(
				index *autoDetectionRetryIndex,
				config confgroup.Config,
			) confgroup.Config {
				index.schedule(config, 2)
				return config
			},
			clocks:       []int{1, 2},
			wantDispatch: 1,
		},
		"replacement invalidates older token": {
			arrange: func(
				index *autoDetectionRetryIndex,
				config confgroup.Config,
			) confgroup.Config {
				index.schedule(config, 1)
				replacement, err := config.Clone()
				if err != nil {
					panic(err)
				}
				replacement.Set("option", "replacement")
				index.schedule(replacement, 2)
				return replacement
			},
			clocks:       []int{1, 2},
			wantDispatch: 1,
		},
		"cancel removes exact pending entry": {
			arrange: func(
				index *autoDetectionRetryIndex,
				config confgroup.Config,
			) confgroup.Config {
				index.schedule(config, 1)
				index.cancel(config.FullName())
				return config
			},
			clocks: []int{1, 2},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			commands := &autoDetectionRetryTestCommands{}
			var planned []autoDetectionRetryToken
			require.NoError(t, index.bind(
				commands,
				func(
					_ confgroup.Config,
					token autoDetectionRetryToken,
				) (jobmgr.WorkPlan, error) {
					planned = append(planned, token)
					return jobmgr.WorkPlan{}, nil
				},
				7,
			))
			config := autoDetectionRetryTestConfig("job")
			expected := test.arrange(index, config)
			for _, clock := range test.clocks {
				require.NoError(t, index.dispatchDue(
					context.Background(),
					clock,
				))
			}

			require.Len(t, commands.submitted, test.wantDispatch)
			require.Len(t, planned, test.wantDispatch)
			require.False(t, commands.waited)
			if test.wantDispatch != 0 {
				require.Equal(t, expected.UID(), planned[0].uid)
			}
		})
	}
}

func TestAutoDetectionRetryIndexHasNoFixedPopulationLimit(t *testing.T) {
	tests := map[string]struct {
		population int
	}{
		"former active-job limit": {
			population: 256,
		},
		"former active-job limit plus one": {
			population: 257,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			commands := &autoDetectionRetryTestCommands{}
			require.NoError(t, index.bind(
				commands,
				func(
					confgroup.Config,
					autoDetectionRetryToken,
				) (jobmgr.WorkPlan, error) {
					return jobmgr.WorkPlan{}, nil
				},
				1,
			))
			for number := range test.population {
				index.schedule(
					autoDetectionRetryTestConfig(
						fmt.Sprintf("job-%03d", number),
					),
					1,
				)
			}
			require.NoError(t, index.dispatchDue(
				context.Background(),
				1,
			))
			require.Len(t, commands.submitted, test.population)
		})
	}
}

type autoDetectionRetryTestCommands struct {
	submitted []jobmgr.Request
	plans     []jobmgr.WorkPlan
	waited    bool
}

func (artc *autoDetectionRetryTestCommands) SubmitPrepared(
	_ context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	artc.submitted = append(artc.submitted, request)
	artc.plans = append(artc.plans, plan)
	return nil
}

func (artc *autoDetectionRetryTestCommands) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	artc.waited = true
	return nil
}

func autoDetectionRetryTestConfig(name string) confgroup.Config {
	return confgroup.Config{
		"module":              "module",
		"name":                name,
		"update_every":        1,
		"autodetection_retry": 1,
		"__source_type__":     confgroup.TypeDyncfg,
		"__source__":          "user=test",
		"__provider__":        "test",
	}
}
