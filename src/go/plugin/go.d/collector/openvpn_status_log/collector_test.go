// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pathNonExistentFile         = "testdata/v2.5.1/non-existent.txt"
	pathEmptyFile               = "testdata/v2.5.1/empty.txt"
	pathStaticKey               = "testdata/v2.5.1/static-key.txt"
	pathStatusVersion1          = "testdata/v2.5.1/version1.txt"
	pathStatusVersion1NoClients = "testdata/v2.5.1/version1-no-clients.txt"
	pathStatusVersion2          = "testdata/v2.5.1/version2.txt"
	pathStatusVersion2NoClients = "testdata/v2.5.1/version2-no-clients.txt"
	pathStatusVersion3          = "testdata/v2.5.1/version3.txt"
	pathStatusVersion3NoClients = "testdata/v2.5.1/version3-no-clients.txt"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
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
		"default config": {
			config: New().Config,
		},
		"unset 'log_path'": {
			wantFail: true,
			config: Config{
				LogPath: "",
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"status version 1":                 {prepare: prepareCaseStatusVersion1},
		"status version 1 with no clients": {prepare: prepareCaseStatusVersion1NoClients},
		"status version 2":                 {prepare: prepareCaseStatusVersion2},
		"status version 2 with no clients": {prepare: prepareCaseStatusVersion2NoClients},
		"status version 3":                 {prepare: prepareCaseStatusVersion3},
		"status version 3 with no clients": {prepare: prepareCaseStatusVersion3NoClients},
		"empty file":                       {prepare: prepareCaseEmptyFile, wantFail: true},
		"non-existent file":                {prepare: prepareCaseNonExistentFile, wantFail: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
		wantNumCharts int
	}{
		"status version 1 with user stats": {
			prepare:       prepareCaseStatusVersion1WithUserStats,
			wantNumCharts: len(charts) + len(userCharts)*2,
		},
		"status version 2 with user stats": {
			prepare:       prepareCaseStatusVersion2WithUserStats,
			wantNumCharts: len(charts) + len(userCharts)*2,
		},
		"status version 3 with user stats": {
			prepare:       prepareCaseStatusVersion2WithUserStats,
			wantNumCharts: len(charts) + len(userCharts)*2,
		},
		"status version with static key": {
			prepare:       prepareCaseStatusStaticKey,
			wantNumCharts: len(charts),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))
			_ = collr.Check(context.Background())
			_ = collr.Collect(context.Background())

			assert.Equal(t, test.wantNumCharts, len(*collr.Charts()))
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		expected map[string]int64
	}{
		"status version 1": {
			prepare: prepareCaseStatusVersion1,
			expected: map[string]int64{
				"bytes_in":  6168,
				"bytes_out": 6369,
				"clients":   2,
			},
		},
		"status version 1 with user stats": {
			prepare: prepareCaseStatusVersion1WithUserStats,
			expected: map[string]int64{
				"bytes_in":                   6168,
				"bytes_out":                  6369,
				"clients":                    2,
				"vpnclient2_bytes_in":        3084,
				"vpnclient2_bytes_out":       3184,
				"vpnclient2_connection_time": 63793143069,
				"vpnclient_bytes_in":         3084,
				"vpnclient_bytes_out":        3185,
				"vpnclient_connection_time":  63793143069,
			},
		},
		"status version 1 with no clients": {
			prepare: prepareCaseStatusVersion1NoClients,
			expected: map[string]int64{
				"bytes_in":  0,
				"bytes_out": 0,
				"clients":   0,
			},
		},
		"status version 2": {
			prepare: prepareCaseStatusVersion2,
			expected: map[string]int64{
				"bytes_in":  6241,
				"bytes_out": 6369,
				"clients":   2,
			},
		},
		"status version 2 with user stats": {
			prepare: prepareCaseStatusVersion2WithUserStats,
			expected: map[string]int64{
				"bytes_in":                   6241,
				"bytes_out":                  6369,
				"clients":                    2,
				"vpnclient2_bytes_in":        3157,
				"vpnclient2_bytes_out":       3184,
				"vpnclient2_connection_time": 264610,
				"vpnclient_bytes_in":         3084,
				"vpnclient_bytes_out":        3185,
				"vpnclient_connection_time":  264609,
			},
		},
		"status version 2 with no clients": {
			prepare: prepareCaseStatusVersion2NoClients,
			expected: map[string]int64{
				"bytes_in":  0,
				"bytes_out": 0,
				"clients":   0,
			},
		},
		"status version 3": {
			prepare: prepareCaseStatusVersion3,
			expected: map[string]int64{
				"bytes_in":  7308,
				"bytes_out": 7235,
				"clients":   2,
			},
		},
		"status version 3 with user stats": {
			prepare: prepareCaseStatusVersion3WithUserStats,
			expected: map[string]int64{
				"bytes_in":                   7308,
				"bytes_out":                  7235,
				"clients":                    2,
				"vpnclient2_bytes_in":        3654,
				"vpnclient2_bytes_out":       3617,
				"vpnclient2_connection_time": 265498,
				"vpnclient_bytes_in":         3654,
				"vpnclient_bytes_out":        3618,
				"vpnclient_connection_time":  265496,
			},
		},
		"status version 3 with no clients": {
			prepare: prepareCaseStatusVersion3NoClients,
			expected: map[string]int64{
				"bytes_in":  0,
				"bytes_out": 0,
				"clients":   0,
			},
		},
		"status with static key": {
			prepare: prepareCaseStatusStaticKey,
			expected: map[string]int64{
				"bytes_in":  19265,
				"bytes_out": 261631,
				"clients":   0,
			},
		},
		"empty file": {
			prepare:  prepareCaseEmptyFile,
			expected: nil,
		},
		"non-existent file": {
			prepare:  prepareCaseNonExistentFile,
			expected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))
			_ = collr.Check(context.Background())

			collected := collr.Collect(context.Background())

			copyConnTime(collected, test.expected)
			assert.Equal(t, test.expected, collected)
		})
	}
}

func prepareCaseStatusVersion1() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion1
	return collr
}

func prepareCaseStatusVersion1WithUserStats() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion1
	collr.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	return collr
}

func prepareCaseStatusVersion1NoClients() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion1NoClients
	return collr
}

func prepareCaseStatusVersion2() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion2
	return collr
}

func prepareCaseStatusVersion2WithUserStats() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion2
	collr.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	return collr
}

func prepareCaseStatusVersion2NoClients() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion2NoClients
	return collr
}

func prepareCaseStatusVersion3() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion3
	return collr
}

func prepareCaseStatusVersion3WithUserStats() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion3
	collr.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	return collr
}

func prepareCaseStatusVersion3NoClients() *Collector {
	collr := New()
	collr.LogPath = pathStatusVersion3NoClients
	return collr
}

func prepareCaseStatusStaticKey() *Collector {
	collr := New()
	collr.LogPath = pathStaticKey
	return collr
}

func prepareCaseEmptyFile() *Collector {
	collr := New()
	collr.LogPath = pathEmptyFile
	return collr
}

func prepareCaseNonExistentFile() *Collector {
	collr := New()
	collr.LogPath = pathNonExistentFile
	return collr
}

func copyConnTime(dst, src map[string]int64) {
	for k, v := range src {
		if !strings.HasSuffix(k, "connection_time") {
			continue
		}
		if _, ok := dst[k]; !ok {
			continue
		}
		dst[k] = v
	}
}
