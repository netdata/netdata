// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dnsmasq_dhcp

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

const (
	testLeasesPath = "testdata/dnsmasq.leases"
	testConfPath   = "testdata/dnsmasq.conf"
	testConfDir    = "testdata/dnsmasq.d"
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
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.ConfPath = testConfPath
	collr.ConfDir = testConfDir

	assert.NoError(t, collr.Init(context.Background()))
}

func TestCollector_InitEmptyLeasesPath(t *testing.T) {
	collr := New()
	collr.LeasesPath = ""

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_InitInvalidLeasesPath(t *testing.T) {
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.LeasesPath += "!"

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_InitZeroDHCPRanges(t *testing.T) {
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.ConfPath = "testdata/dnsmasq3.conf"
	collr.ConfDir = ""

	assert.NoError(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.ConfPath = testConfPath
	collr.ConfDir = testConfDir

	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.ConfPath = testConfPath
	collr.ConfDir = testConfDir

	require.NoError(t, collr.Init(context.Background()))

	assert.NotNil(t, collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.ConfPath = testConfPath
	collr.ConfDir = testConfDir

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	expected := map[string]int64{
		"dhcp_range_1230::1-1230::64_allocated_leases":              7,
		"dhcp_range_1230::1-1230::64_utilization":                   7,
		"dhcp_range_1231::1-1231::64_allocated_leases":              1,
		"dhcp_range_1231::1-1231::64_utilization":                   1,
		"dhcp_range_1232::1-1232::64_allocated_leases":              1,
		"dhcp_range_1232::1-1232::64_utilization":                   1,
		"dhcp_range_1233::1-1233::64_allocated_leases":              1,
		"dhcp_range_1233::1-1233::64_utilization":                   1,
		"dhcp_range_1234::1-1234::64_allocated_leases":              1,
		"dhcp_range_1234::1-1234::64_utilization":                   1,
		"dhcp_range_192.168.0.1-192.168.0.100_allocated_leases":     6,
		"dhcp_range_192.168.0.1-192.168.0.100_utilization":          6,
		"dhcp_range_192.168.1.1-192.168.1.100_allocated_leases":     5,
		"dhcp_range_192.168.1.1-192.168.1.100_utilization":          5,
		"dhcp_range_192.168.2.1-192.168.2.100_allocated_leases":     4,
		"dhcp_range_192.168.2.1-192.168.2.100_utilization":          4,
		"dhcp_range_192.168.200.1-192.168.200.100_allocated_leases": 1,
		"dhcp_range_192.168.200.1-192.168.200.100_utilization":      1,
		"dhcp_range_192.168.3.1-192.168.3.100_allocated_leases":     1,
		"dhcp_range_192.168.3.1-192.168.3.100_utilization":          1,
		"dhcp_range_192.168.4.1-192.168.4.100_allocated_leases":     1,
		"dhcp_range_192.168.4.1-192.168.4.100_utilization":          1,
		"ipv4_dhcp_hosts":  6,
		"ipv4_dhcp_ranges": 6,
		"ipv6_dhcp_hosts":  5,
		"ipv6_dhcp_ranges": 5,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
}

func TestCollector_CollectFailedToOpenLeasesPath(t *testing.T) {
	collr := New()
	collr.LeasesPath = testLeasesPath
	collr.ConfPath = testConfPath
	collr.ConfDir = testConfDir

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collr.LeasesPath = ""
	assert.Nil(t, collr.Collect(context.Background()))
}

func TestCollector_parseDHCPRangeValue(t *testing.T) {
	tests := map[string]struct {
		input    string
		wantFail bool
	}{
		"ipv4": {
			input: "192.168.0.50,192.168.0.150,12h",
		},
		"ipv4 with netmask": {
			input: "192.168.0.50,192.168.0.150,255.255.255.0,12h",
		},
		"ipv4 with netmask and tag": {
			input: "set:red,1.1.1.50,1.1.2.150, 255.255.252.0",
		},
		"ipv4 with iface": {
			input: "enp3s0, 172.16.1.2, 172.16.1.254, 1h",
		},
		"ipv4 with iface 2": {
			input: "enp2s0.100, 192.168.100.2, 192.168.100.254, 1h",
		},
		"ipv4 static": {
			wantFail: true,
			input:    "192.168.0.0,static",
		},
		"ipv6": {
			input: "1234::2,1234::500",
		},
		"ipv6 slacc": {
			input: "1234::2,1234::500, slaac",
		},
		"ipv6 with with prefix length and lease time": {
			input: "1234::2,1234::500, 64, 12h",
		},
		"ipv6 ra-only": {
			wantFail: true,
			input:    "1234::,ra-only",
		},
		"ipv6 ra-names": {
			wantFail: true,
			input:    "1234::,ra-names",
		},
		"ipv6 ra-stateless": {
			wantFail: true,
			input:    "1234::,ra-stateless",
		},
		"invalid": {
			wantFail: true,
			input:    "192.168.0.0",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := parseDHCPRangeValue(test.input)

			if test.wantFail {
				assert.Emptyf(t, v, "parsing '%s' must fail", test.input)
			} else {
				assert.NotEmptyf(t, v, "parsing '%s' must not fail", test.input)
			}
		})
	}
}
