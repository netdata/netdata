// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer39Mntr, _          = os.ReadFile("testdata/v3.9/mntr.txt")
	dataMntrNotInWhiteList, _ = os.ReadFile("testdata/mntr_notinwhitelist.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                 dataConfigJSON,
		"dataConfigYAML":                 dataConfigYAML,
		"dataVer39Mntr":                  dataVer39Mntr,
		"dataMntrNotInWhiteListResponse": dataMntrNotInWhiteList,
	} {
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'address'": {
			wantFail: true,
			config: Config{
				Address: "",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare func(t *testing.T) *Collector
	}{
		"not initialized": {
			prepare: func(t *testing.T) *Collector {
				return New()
			},
		},
		"initialized": {
			prepare: func(t *testing.T) *Collector {
				collr := New()
				require.NoError(t, collr.Init(context.Background()))
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *mockZookeeperFetcher
		wantFail bool
	}{
		"success v3.9": {
			wantFail: false,
			prepare:  prepareMockVer39Ok,
		},
		"fails if mntr is not whitelisted": {
			wantFail: true,
			prepare:  prepareMockMntrNotInWhitelist,
		},
		"fails on empty response": {
			wantFail: true,
			prepare:  prepareMockMntrEmptyResponse,
		},
		"fails on fetch error": {
			wantFail: true,
			prepare:  prepareMockErrOnFetch,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			require.NoError(t, collr.Init(context.Background()))
			collr.fetcher = test.prepare()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() *mockZookeeperFetcher
		wantMetrics map[string]int64
	}{
		"success v3.9": {
			prepare: prepareMockVer39Ok,
			wantMetrics: map[string]int64{
				"approximate_data_size":      44,
				"auth_failed_count":          0,
				"avg_latency":                0,
				"connection_drop_count":      0,
				"connection_rejected":        0,
				"ephemerals_count":           0,
				"global_sessions":            0,
				"max_file_descriptor_count":  1048576,
				"max_latency":                0,
				"min_latency":                0,
				"num_alive_connections":      1,
				"open_file_descriptor_count": 77,
				"outstanding_requests":       0,
				"packets_received":           452,
				"packets_sent":               1353,
				"server_state_follower":      0,
				"server_state_leader":        0,
				"server_state_observer":      0,
				"server_state_standalone":    1,
				"stale_requests":             0,
				"stale_requests_dropped":     0,
				"throttled_ops":              0,
				"uptime":                     488,
				"watch_count":                0,
				"znode_count":                5,
			},
		},
		"fails if mntr is not whitelisted": {
			prepare: prepareMockMntrNotInWhitelist,
		},
		"fails on empty response": {
			prepare: prepareMockMntrEmptyResponse,
		},
		"fails on fetch error": {
			prepare: prepareMockErrOnFetch,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			require.NoError(t, collr.Init(context.Background()))
			collr.fetcher = test.prepare()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantMetrics, mx)
			if len(mx) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareMockVer39Ok() *mockZookeeperFetcher {
	return &mockZookeeperFetcher{
		dataMntr: dataVer39Mntr,
	}
}

func prepareMockMntrNotInWhitelist() *mockZookeeperFetcher {
	return &mockZookeeperFetcher{
		dataMntr: dataMntrNotInWhiteList,
	}
}

func prepareMockMntrEmptyResponse() *mockZookeeperFetcher {
	return &mockZookeeperFetcher{}
}

func prepareMockErrOnFetch() *mockZookeeperFetcher {
	return &mockZookeeperFetcher{
		errOnFetch: true,
	}
}

type mockZookeeperFetcher struct {
	dataMntr   []byte
	errOnFetch bool
}

func (m mockZookeeperFetcher) fetch(cmd string) ([]string, error) {
	if m.errOnFetch {
		return nil, errors.New("mock fetch error")
	}

	if cmd != "mntr" {
		return nil, errors.New("invalid command")
	}

	var lines []string

	s := bufio.NewScanner(bytes.NewReader(m.dataMntr))
	for s.Scan() {
		lines = append(lines, s.Text())
	}

	return lines, nil
}
