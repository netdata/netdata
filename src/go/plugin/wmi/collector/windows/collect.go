// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows/internal/wmiquery"
)

const (
	queryCPU  = "SELECT PercentProcessorTime FROM Win32_PerfFormattedData_PerfOS_Processor WHERE Name = '_Total'"
	queryRAM  = "SELECT TotalVisibleMemorySize, FreePhysicalMemory FROM Win32_OperatingSystem"
	queryNIC  = "SELECT Name, BytesReceivedPerSec, BytesSentPerSec FROM Win32_PerfFormattedData_Tcpip_NetworkInterface"
	queryDisk = "SELECT DeviceID, Size, FreeSpace FROM Win32_LogicalDisk WHERE DriveType = 3"
)

func (c *Collector) poll(parent context.Context, inventoryDue bool) (observations, error) {
	ctx, cancel := context.WithTimeout(parent, c.Timeout.Duration())
	defer cancel()
	if err := ctx.Err(); err != nil {
		return observations{}, err
	}

	client, err := c.open(ctx)
	if err != nil {
		if err := ctx.Err(); err != nil {
			return observations{}, err
		}
		return observations{}, safeTransportError("failed to open WMI session", err)
	}
	defer client.Close()

	var snapshot observations
	var fixedDrives []map[string]any
	var diskQueryOK bool
	groups := [...]struct {
		name    string
		enabled *bool
		query   string
		read    func([]map[string]any) bool
	}{
		{name: "CPU", enabled: c.Collectors.CPU, query: queryCPU, read: snapshot.readCPU},
		{name: "memory", enabled: c.Collectors.Memory, query: queryRAM, read: snapshot.readMemory},
		{name: "network", enabled: c.Collectors.Network, query: queryNIC, read: snapshot.readNetwork},
		{name: "disk", enabled: c.Collectors.Disk, query: queryDisk, read: snapshot.readDisks},
	}

	var succeeded bool
	var failures [len(groups)]error
	for i, group := range groups {
		if !enabled(group.enabled) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return observations{}, err
		}
		rows, err := client.Query(ctx, group.query)
		if inventoryDue && group.query == queryDisk && err == nil {
			fixedDrives, diskQueryOK = rows, true
		}
		switch {
		case err != nil:
			failures[i] = safeTransportError(group.name+" WMI query failed", err)
		case !group.read(rows):
			failures[i] = fmt.Errorf("%s WMI query returned no usable observations", group.name)
		default:
			succeeded = true
		}
	}

	if err := ctx.Err(); err != nil {
		return observations{}, err
	}
	if !succeeded {
		return observations{}, fmt.Errorf("no enabled WMI query group succeeded: %w", errors.Join(failures[:]...))
	}
	for i, err := range failures {
		if err != nil {
			c.Limit("wmi:"+groups[i].name, 1, time.Minute).Warning(err)
		}
	}
	if inventoryDue {
		c.refreshInventory(ctx, client, fixedDrives, diskQueryOK)
	}
	// Exhausting the optional inventory budget must not discard successful samples.
	return snapshot, parent.Err()
}

func safeTransportError(operation string, err error) error {
	// Only the transport's classified messages are safe for diagnostics.
	if detail := wmiquery.SafeMessage(err); detail != "" {
		return fmt.Errorf("%s: %s", operation, detail)
	}
	return errors.New(operation)
}
