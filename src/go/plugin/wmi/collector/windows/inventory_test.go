// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInventoryRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		c.Host = "192.0.2.14"
		session := &fakeSession{
			results: validQueryResults(),
		}
		session.results[queryOSInfo] = fixtureRows(map[string]any{
			"CSName":      "FILE01",
			"Caption":     "Microsoft Windows Server 2022 Datacenter Evaluation",
			"ProductType": uint32(3),
			"Version":     "10.0.20348",
			"BuildNumber": "20348",
		})
		session.results[querySystemInfo] = fixtureRows(map[string]any{
			"Manufacturer":              "QEMU",
			"Model":                     "Standard PC",
			"NumberOfLogicalProcessors": uint32(8),
		})
		session.results[queryProcessorInfo] = fixtureRows(map[string]any{
			"Architecture":      uint16(9),
			"Name":              "Test CPU",
			"CurrentClockSpeed": uint32(2000),
		})
		session.results[queryInstalledMemory] = fixtureRows(
			map[string]any{"Capacity": "8589934592"},
			map[string]any{"Capacity": uint64(4294967296)},
		)
		c.newSession = func(context.Context) (querySession, error) { return session, nil }
		require.NoError(t, c.Init(t.Context()))
		initial := c.VirtualNode()
		require.NoError(t, c.Check(t.Context()))
		require.Equal(
			t,
			[]string{queryCPU, queryMemory, querySystem, queryNetworkInterfaces, queryNetworkAdapters, queryDisk},
			session.queries,
			"readiness must only wait for metrics",
		)
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		node := c.VirtualNode()
		assert.Equal(t, initial.GUID, node.GUID)
		assert.Equal(t, "192.0.2.14", initial.Hostname)
		assert.Equal(t, "FILE01", node.Hostname)
		assert.Equal(t, map[string]string{
			"_vnode_type":           "wmi",
			"address":               "192.0.2.14",
			"_os":                   "windows",
			"_os_name":              "Windows Server",
			"_os_version":           "Microsoft Windows Server 2022 Datacenter Evaluation",
			"_os_marketing_version": "2022",
			"_os_edition":           "Datacenter Evaluation",
			"_kernel_version":       "Windows 10.0.20348 Build: 20348",
			"_hw_product_name":      "Standard PC",
			"_hw_sys_vendor":        "QEMU",
			"_virtualization":       "qemu",
			"_virt_detection":       "wmi",
			"_hw_product_type":      "vm",
			"_system_cores":         "8",
			"_system_cpu_model":     "Test CPU",
			"_architecture":         "x86_64",
			"_system_cpu_freq":      "2000000000",
			"_system_ram_total":     "12884901888",
		}, node.Labels)
		session.queries = nil
		metrics, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		require.NotEmpty(t, metrics)
		assert.Equal(
			t,
			[]string{queryCPU, queryMemory, querySystem, queryNetworkInterfaces, queryNetworkAdapters, queryDisk},
			session.queries,
		)
		time.Sleep(inventoryInterval)
		session.results[queryOSInfo] = queryResult{
			err: errors.New("withheld"),
		}
		session.results[queryInstalledMemory] = fixtureRows(map[string]any{"Capacity": "1"}, map[string]any{})
		_, err = collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, "FILE01", c.VirtualNode().Hostname, "keep the last name on query failure")
		assert.NotContains(t, c.VirtualNode().Labels, "_system_ram_total", "a partial DIMM sum is not installed RAM")
		assert.Equal(t, "12884901888", node.Labels["_system_ram_total"], "previously published snapshots are immutable")
		time.Sleep(inventoryInterval)
		session.results[queryOSInfo] = fixtureRows(map[string]any{"CSName": "FILE02"})
		_, err = collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, "FILE02", c.VirtualNode().Hostname)
		assert.Equal(t, initial.GUID, c.VirtualNode().GUID)
		assert.NotContains(t, c.VirtualNode().Labels, "_os_edition")
		time.Sleep(inventoryInterval)
		session.results[queryOSInfo] = fixtureRows(map[string]any{"CSName": "unsafe'name"})
		_, err = collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, initial.Hostname, c.VirtualNode().Hostname)
	})
}

type slowInventorySession struct{ fakeSession }

func (s *slowInventorySession) Query(ctx context.Context, query string) ([]map[string]any, error) {
	if query == queryOSInfo {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.fakeSession.Query(ctx, query)
}

func TestInventoryTimeoutPreservesMetrics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := configuredCollector()
		session := &slowInventorySession{fakeSession{
			results: validQueryResults(),
		}}
		c.newSession = func(context.Context) (querySession, error) { return session, nil }
		require.NoError(t, c.Init(t.Context()))
		start := time.Now()
		metrics, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.NotEmpty(t, metrics)
		assert.Less(t, time.Since(start), c.Timeout.Duration())
		assert.Equal(t, c.Host, c.VirtualNode().Hostname)
	})
}

func TestInventoryDisabledQueries(t *testing.T) {
	for name, vnode := range map[string]string{
		"generated vnode skips the disk capacity query": "",
		"named vnode skips the inventory":               "manual",
	} {
		t.Run(name, func(t *testing.T) {
			c := configuredCollector()
			c.Vnode = vnode
			c.Collectors.Disk = new(false)
			session := &fakeSession{
				results: validQueryResults(),
			}
			c.newSession = func(context.Context) (querySession, error) { return session, nil }
			require.NoError(t, c.Init(t.Context()))
			_, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			if vnode != "" {
				assert.Equal(
					t,
					[]string{queryCPU, queryMemory, querySystem, queryNetworkInterfaces, queryNetworkAdapters},
					session.queries,
				)
				return
			}
			assert.Contains(t, session.queries, queryOSInfo)
			assert.NotContains(t, strings.Join(session.queries, " "), "Win32_LogicalDisk")
		})
	}
}

func TestInstalledMemoryRejectsOverflow(t *testing.T) {
	assert.Empty(t, installedMemory([]map[string]any{{"capacity": uint64(math.MaxUint64)}, {"capacity": uint64(1)}}))
}

func TestInventoryStopsWhenSessionIsLost(t *testing.T) {
	c := configuredCollector()
	session := &fakeSession{
		results: validQueryResults(),
	}
	session.results[queryOSInfo] = fixtureRows(map[string]any{"CSName": "FILE01"})
	session.results[querySystemInfo] = queryResult{
		err: sessionFailure{errors.New("connection reset")},
	}
	c.newSession = func(context.Context) (querySession, error) { return session, nil }
	require.NoError(t, c.Init(t.Context()))

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(
		t,
		[]string{
			queryCPU,
			queryMemory,
			querySystem,
			queryNetworkInterfaces,
			queryNetworkAdapters,
			queryDisk,
			queryOSInfo,
			querySystemInfo,
		},
		session.queries,
	)
	assert.Equal(t, "FILE01", c.VirtualNode().Hostname, "providers read before the failure are published")
}
