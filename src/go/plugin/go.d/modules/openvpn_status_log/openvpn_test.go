// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"

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

func TestOpenVPNStatusLog_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &OpenVPNStatusLog{}, dataConfigJSON, dataConfigYAML)
}

func TestOpenVPNStatusLog_Init(t *testing.T) {
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
			ovpn := New()
			ovpn.Config = test.config

			if test.wantFail {
				assert.Error(t, ovpn.Init())
			} else {
				assert.NoError(t, ovpn.Init())
			}
		})
	}
}

func TestOpenVPNStatusLog_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *OpenVPNStatusLog
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
			ovpn := test.prepare()

			require.NoError(t, ovpn.Init())

			if test.wantFail {
				assert.Error(t, ovpn.Check())
			} else {
				assert.NoError(t, ovpn.Check())
			}
		})
	}
}

func TestOpenVPNStatusLog_Charts(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *OpenVPNStatusLog
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
			ovpn := test.prepare()

			require.NoError(t, ovpn.Init())
			_ = ovpn.Check()
			_ = ovpn.Collect()

			assert.Equal(t, test.wantNumCharts, len(*ovpn.Charts()))
		})
	}
}

func TestOpenVPNStatusLog_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *OpenVPNStatusLog
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
			ovpn := test.prepare()

			require.NoError(t, ovpn.Init())
			_ = ovpn.Check()

			collected := ovpn.Collect()

			copyConnTime(collected, test.expected)
			assert.Equal(t, test.expected, collected)
		})
	}
}

func prepareCaseStatusVersion1() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion1
	return ovpn
}

func prepareCaseStatusVersion1WithUserStats() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion1
	ovpn.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	return ovpn
}

func prepareCaseStatusVersion1NoClients() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion1NoClients
	return ovpn
}

func prepareCaseStatusVersion2() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion2
	return ovpn
}

func prepareCaseStatusVersion2WithUserStats() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion2
	ovpn.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	return ovpn
}

func prepareCaseStatusVersion2NoClients() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion2NoClients
	return ovpn
}

func prepareCaseStatusVersion3() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion3
	return ovpn
}

func prepareCaseStatusVersion3WithUserStats() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion3
	ovpn.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	return ovpn
}

func prepareCaseStatusVersion3NoClients() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStatusVersion3NoClients
	return ovpn
}

func prepareCaseStatusStaticKey() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathStaticKey
	return ovpn
}

func prepareCaseEmptyFile() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathEmptyFile
	return ovpn
}

func prepareCaseNonExistentFile() *OpenVPNStatusLog {
	ovpn := New()
	ovpn.LogPath = pathNonExistentFile
	return ovpn
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
