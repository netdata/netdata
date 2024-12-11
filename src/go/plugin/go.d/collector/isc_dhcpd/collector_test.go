// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package isc_dhcpd

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

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Init(t *testing.T) {
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
		"lease db not exists":                     {prepare: prepareDHCPdLeasesNotExists, wantFail: true},
		"lease db is an empty file":               {prepare: prepareDHCPdLeasesEmpty},
		"lease db ipv4":                           {prepare: prepareDHCPdLeasesIPv4},
		"lease db ipv4 with only inactive leases": {prepare: prepareDHCPdLeasesIPv4Inactive},
		"lease db ipv4 with backup leases":        {prepare: prepareDHCPdLeasesIPv4Backup},
		"lease db ipv6":                           {prepare: prepareDHCPdLeasesIPv6},
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
	collr := New()
	collr.LeasesPath = "leases_path"
	collr.Pools = []PoolConfig{
		{Name: "name", Networks: "192.0.2.0/24"},
	}
	require.NoError(t, collr.Init(context.Background()))

	assert.NotNil(t, collr.Charts())
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() *Collector
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
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			if len(mx) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareDHCPdLeasesNotExists() *Collector {
	collr := New()
	collr.Config = Config{
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
	return collr
}

func prepareDHCPdLeasesEmpty() *Collector {
	collr := New()
	collr.Config = Config{
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
	return collr
}

func prepareDHCPdLeasesIPv4() *Collector {
	collr := New()
	collr.Config = Config{
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
	return collr
}

func prepareDHCPdLeasesIPv4Backup() *Collector {
	collr := New()
	collr.Config = Config{
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
	return collr
}

func prepareDHCPdLeasesIPv4Inactive() *Collector {
	collr := New()
	collr.Config = Config{
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
	return collr
}

func prepareDHCPdLeasesIPv6() *Collector {
	collr := New()
	collr.Config = Config{
		LeasesPath: "testdata/dhcpd.leases_ipv6",
		Pools: []PoolConfig{
			{Name: "net1", Networks: "2001:db8::-2001:db8::a"},
			{Name: "net2", Networks: "2001:db8:0:1::-2001:db8:0:1::a"},
		},
	}
	return collr
}
