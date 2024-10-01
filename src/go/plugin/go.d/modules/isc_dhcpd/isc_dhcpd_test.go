// SPDX-License-Identifier: GPL-3.0-or-later

package isc_dhcpd

import (
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

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestDHCPd_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &DHCPd{}, dataConfigJSON, dataConfigYAML)
}

func TestDHCPd_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestDHCPd_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default": {
			wantFail: true,
			config:   New().Config,
		},
		"'leases_path' not set": {
			wantFail: true,
			config: Config{
				LeasesPath: "",
				Pools: []PoolConfig{
					{Name: "test", Networks: "10.220.252.0/24"},
				},
			},
		},
		"'pools' not set": {
			wantFail: true,
			config: Config{
				LeasesPath: "testdata/dhcpd.leases_ipv4",
			},
		},
		"'pools->pool.networks' invalid syntax": {
			wantFail: true,
			config: Config{
				LeasesPath: "testdata/dhcpd.leases_ipv4",
				Pools: []PoolConfig{
					{Name: "test", Networks: "10.220.252./24"},
				},
			}},
		"ok config ('leases_path' and 'pools' are set)": {
			config: Config{
				LeasesPath: "testdata/dhcpd.leases_ipv4",
				Pools: []PoolConfig{
					{Name: "test", Networks: "10.220.252.0/24"},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dhcpd := New()
			dhcpd.Config = test.config

			if test.wantFail {
				assert.Error(t, dhcpd.Init())
			} else {
				assert.NoError(t, dhcpd.Init())
			}
		})
	}
}

func TestDHCPd_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *DHCPd
		wantFail bool
	}{
		"lease db not exists":                     {prepare: prepareDHCPdLeasesNotExists, wantFail: true},
		"lease db is an empty file":               {prepare: prepareDHCPdLeasesEmpty},
		"lease db ipv4":                           {prepare: prepareDHCPdLeasesIPv4},
		"lease db ipv4 with only inactive leases": {prepare: prepareDHCPdLeasesIPv4Inactive},
		"lease db ipv4 with backup leases":        {prepare: prepareDHCPdLeasesIPv4Backup},
		"lease db ipv6":                           {prepare: prepareDHCPdLeasesIPv6},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dhcpd := test.prepare()
			require.NoError(t, dhcpd.Init())

			if test.wantFail {
				assert.Error(t, dhcpd.Check())
			} else {
				assert.NoError(t, dhcpd.Check())
			}
		})
	}
}

func TestDHCPd_Charts(t *testing.T) {
	dhcpd := New()
	dhcpd.LeasesPath = "leases_path"
	dhcpd.Pools = []PoolConfig{
		{Name: "name", Networks: "192.0.2.0/24"},
	}
	require.NoError(t, dhcpd.Init())

	assert.NotNil(t, dhcpd.Charts())
}

func TestDHCPd_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *DHCPd
		wantCollected map[string]int64
	}{
		"lease db not exists": {
			prepare:       prepareDHCPdLeasesNotExists,
			wantCollected: nil,
		},
		"lease db is an empty file": {
			prepare: prepareDHCPdLeasesEmpty,
			wantCollected: map[string]int64{
				"active_leases_total":          0,
				"dhcp_pool_net1_active_leases": 0,
				"dhcp_pool_net1_utilization":   0,
				"dhcp_pool_net2_active_leases": 0,
				"dhcp_pool_net2_utilization":   0,
				"dhcp_pool_net3_active_leases": 0,
				"dhcp_pool_net3_utilization":   0,
				"dhcp_pool_net4_active_leases": 0,
				"dhcp_pool_net4_utilization":   0,
				"dhcp_pool_net5_active_leases": 0,
				"dhcp_pool_net5_utilization":   0,
				"dhcp_pool_net6_active_leases": 0,
				"dhcp_pool_net6_utilization":   0,
			},
		},
		"lease db ipv4": {
			prepare: prepareDHCPdLeasesIPv4,
			wantCollected: map[string]int64{
				"active_leases_total":          5,
				"dhcp_pool_net1_active_leases": 2,
				"dhcp_pool_net1_utilization":   158,
				"dhcp_pool_net2_active_leases": 1,
				"dhcp_pool_net2_utilization":   39,
				"dhcp_pool_net3_active_leases": 0,
				"dhcp_pool_net3_utilization":   0,
				"dhcp_pool_net4_active_leases": 1,
				"dhcp_pool_net4_utilization":   79,
				"dhcp_pool_net5_active_leases": 0,
				"dhcp_pool_net5_utilization":   0,
				"dhcp_pool_net6_active_leases": 1,
				"dhcp_pool_net6_utilization":   39,
			},
		},
		"lease db ipv4 with only inactive leases": {
			prepare: prepareDHCPdLeasesIPv4Inactive,
			wantCollected: map[string]int64{
				"active_leases_total":          0,
				"dhcp_pool_net1_active_leases": 0,
				"dhcp_pool_net1_utilization":   0,
				"dhcp_pool_net2_active_leases": 0,
				"dhcp_pool_net2_utilization":   0,
				"dhcp_pool_net3_active_leases": 0,
				"dhcp_pool_net3_utilization":   0,
				"dhcp_pool_net4_active_leases": 0,
				"dhcp_pool_net4_utilization":   0,
				"dhcp_pool_net5_active_leases": 0,
				"dhcp_pool_net5_utilization":   0,
				"dhcp_pool_net6_active_leases": 0,
				"dhcp_pool_net6_utilization":   0,
			},
		},
		"lease db ipv4 with backup leases": {
			prepare: prepareDHCPdLeasesIPv4Backup,
			wantCollected: map[string]int64{
				"active_leases_total":          2,
				"dhcp_pool_net1_active_leases": 1,
				"dhcp_pool_net1_utilization":   79,
				"dhcp_pool_net2_active_leases": 0,
				"dhcp_pool_net2_utilization":   0,
				"dhcp_pool_net3_active_leases": 0,
				"dhcp_pool_net3_utilization":   0,
				"dhcp_pool_net4_active_leases": 1,
				"dhcp_pool_net4_utilization":   79,
				"dhcp_pool_net5_active_leases": 0,
				"dhcp_pool_net5_utilization":   0,
				"dhcp_pool_net6_active_leases": 0,
				"dhcp_pool_net6_utilization":   0,
			},
		},
		"lease db ipv6": {
			prepare: prepareDHCPdLeasesIPv6,
			wantCollected: map[string]int64{
				"active_leases_total":          6,
				"dhcp_pool_net1_active_leases": 6,
				"dhcp_pool_net1_utilization":   5454,
				"dhcp_pool_net2_active_leases": 0,
				"dhcp_pool_net2_utilization":   0,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dhcpd := test.prepare()
			require.NoError(t, dhcpd.Init())

			mx := dhcpd.Collect()

			assert.Equal(t, test.wantCollected, mx)
			if len(mx) > 0 {
				module.TestMetricsHasAllChartsDims(t, dhcpd.Charts(), mx)
			}
		})
	}
}

func prepareDHCPdLeasesNotExists() *DHCPd {
	dhcpd := New()
	dhcpd.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_not_exists",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "192.168.3.0/25"},
			{Name: "net2", Networks: "10.254.251.0/24"},
			{Name: "net3", Networks: "10.254.252.0/24"},
			{Name: "net4", Networks: "10.254.253.0/25"},
			{Name: "net5", Networks: "10.254.254.0/25"},
			{Name: "net6", Networks: "10.254.255.0/24"},
		},
	}
	return dhcpd
}

func prepareDHCPdLeasesEmpty() *DHCPd {
	dhcpd := New()
	dhcpd.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_empty",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "192.168.3.0/25"},
			{Name: "net2", Networks: "10.254.251.0/24"},
			{Name: "net3", Networks: "10.254.252.0/24"},
			{Name: "net4", Networks: "10.254.253.0/25"},
			{Name: "net5", Networks: "10.254.254.0/25"},
			{Name: "net6", Networks: "10.254.255.0/24"},
		},
	}
	return dhcpd
}

func prepareDHCPdLeasesIPv4() *DHCPd {
	dhcpd := New()
	dhcpd.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_ipv4",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "192.168.3.0/25"},
			{Name: "net2", Networks: "10.254.251.0/24"},
			{Name: "net3", Networks: "10.254.252.0/24"},
			{Name: "net4", Networks: "10.254.253.0/25"},
			{Name: "net5", Networks: "10.254.254.0/25"},
			{Name: "net6", Networks: "10.254.255.0/24"},
		},
	}
	return dhcpd
}

func prepareDHCPdLeasesIPv4Backup() *DHCPd {
	dhcpd := New()
	dhcpd.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_ipv4_backup",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "192.168.3.0/25"},
			{Name: "net2", Networks: "10.254.251.0/24"},
			{Name: "net3", Networks: "10.254.252.0/24"},
			{Name: "net4", Networks: "10.254.253.0/25"},
			{Name: "net5", Networks: "10.254.254.0/25"},
			{Name: "net6", Networks: "10.254.255.0/24"},
		},
	}
	return dhcpd
}

func prepareDHCPdLeasesIPv4Inactive() *DHCPd {
	dhcpd := New()
	dhcpd.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_ipv4_inactive",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "192.168.3.0/25"},
			{Name: "net2", Networks: "10.254.251.0/24"},
			{Name: "net3", Networks: "10.254.252.0/24"},
			{Name: "net4", Networks: "10.254.253.0/25"},
			{Name: "net5", Networks: "10.254.254.0/25"},
			{Name: "net6", Networks: "10.254.255.0/24"},
		},
	}
	return dhcpd
}

func prepareDHCPdLeasesIPv6() *DHCPd {
	dhcpd := New()
	dhcpd.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_ipv6",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "2001:db8::-2001:db8::a"},
			{Name: "net2", Networks: "2001:db8:0:1::-2001:db8:0:1::a"},
		},
	}
	return dhcpd
}
