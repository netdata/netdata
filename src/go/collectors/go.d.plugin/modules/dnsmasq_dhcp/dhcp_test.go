// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq_dhcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLeasesPath = "testdata/dnsmasq.leases"
	testConfPath   = "testdata/dnsmasq.conf"
	testConfDir    = "testdata/dnsmasq.d"
)

func TestNew(t *testing.T) {
	job := New()

	assert.IsType(t, (*DnsmasqDHCP)(nil), job)
}

func TestDnsmasqDHCP_Init(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.ConfPath = testConfPath
	job.ConfDir = testConfDir

	assert.True(t, job.Init())
}

func TestDnsmasqDHCP_InitEmptyLeasesPath(t *testing.T) {
	job := New()
	job.LeasesPath = ""

	assert.False(t, job.Init())
}

func TestDnsmasqDHCP_InitInvalidLeasesPath(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.LeasesPath += "!"

	assert.False(t, job.Init())
}

func TestDnsmasqDHCP_InitZeroDHCPRanges(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.ConfPath = "testdata/dnsmasq3.conf"
	job.ConfDir = ""

	assert.True(t, job.Init())
}

func TestDnsmasqDHCP_Check(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.ConfPath = testConfPath
	job.ConfDir = testConfDir

	require.True(t, job.Init())
	assert.True(t, job.Check())
}

func TestDnsmasqDHCP_Charts(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.ConfPath = testConfPath
	job.ConfDir = testConfDir

	require.True(t, job.Init())

	assert.NotNil(t, job.Charts())
}

func TestDnsmasqDHCP_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestDnsmasqDHCP_Collect(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.ConfPath = testConfPath
	job.ConfDir = testConfDir

	require.True(t, job.Init())
	require.True(t, job.Check())

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

	assert.Equal(t, expected, job.Collect())
}

func TestDnsmasqDHCP_CollectFailedToOpenLeasesPath(t *testing.T) {
	job := New()
	job.LeasesPath = testLeasesPath
	job.ConfPath = testConfPath
	job.ConfDir = testConfDir

	require.True(t, job.Init())
	require.True(t, job.Check())

	job.LeasesPath = ""
	assert.Nil(t, job.Collect())
}
