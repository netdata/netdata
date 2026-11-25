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
	queryCPU     = "SELECT PercentProcessorTime FROM Win32_PerfFormattedData_PerfOS_Processor WHERE Name = '_Total'"
	queryMemory  = "SELECT TotalVisibleMemorySize, FreePhysicalMemory FROM Win32_OperatingSystem"
	queryNetwork = "SELECT Name, BytesReceivedPerSec, BytesSentPerSec FROM Win32_PerfFormattedData_Tcpip_NetworkInterface"
	queryDisk    = "SELECT DeviceID, Size, FreeSpace FROM Win32_LogicalDisk WHERE DriveType = 3"
)

type metricGroup struct {
	name    string
	enabled bool
	query   string
	read    func([]map[string]any) bool
}

// poll opens one session, queries every enabled metric group and, when due,
// refreshes the host inventory in the same session. It fails only when no
// enabled group produced observations; failed groups are logged and leave gaps.
func (c *Collector) poll(parent context.Context, inventoryDue bool) (observations, error) {
	ctx, cancel := context.WithTimeout(parent, c.Timeout.Duration())
	defer cancel()

	session, err := c.newSession(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return observations{}, c.interrupted(parent, ctx)
		}
		return observations{}, safeTransportError("failed to open WMI session", err)
	}
	defer session.Close()

	var snapshot observations
	// The fixed-drive rows also feed the inventory's disk capacity label, so a
	// disabled disk group issues no disk query at all.
	var fixedDrives []map[string]any
	var fixedDrivesRead bool
	groups := [...]metricGroup{
		{name: "CPU", enabled: groupEnabled(c.Collectors.CPU), query: queryCPU, read: snapshot.readCPU},
		{name: "memory", enabled: groupEnabled(c.Collectors.Memory), query: queryMemory, read: snapshot.readMemory},
		{name: "network", enabled: groupEnabled(c.Collectors.Network), query: queryNetwork, read: snapshot.readNetwork},
		{
			name:    "disk",
			enabled: groupEnabled(c.Collectors.Disk),
			query:   queryDisk,
			read: func(rows []map[string]any) bool {
				fixedDrives, fixedDrivesRead = rows, true
				return snapshot.readDisks(rows)
			},
		},
	}

	var succeeded bool
	var failures [len(groups)]error
	for i, group := range groups {
		if !group.enabled {
			continue
		}
		if ctx.Err() != nil {
			return observations{}, c.interrupted(parent, ctx)
		}
		rows, err := session.Query(ctx, group.query)
		switch {
		case err != nil && !succeeded && wmiquery.SessionFailed(err) && ctx.Err() == nil:
			// WinRM connects and logs in with the first query.
			return observations{}, safeTransportError("failed to open WMI session", err)
		case err != nil:
			failures[i] = safeTransportError(group.name+" WMI query failed", err)
		case !group.read(rows):
			failures[i] = fmt.Errorf("%s WMI query returned no usable observations", group.name)
		default:
			succeeded = true
		}
	}

	if ctx.Err() != nil {
		return observations{}, c.interrupted(parent, ctx)
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
		c.refreshInventory(ctx, session, fixedDrives, fixedDrivesRead)
	}
	// Exhausting the optional inventory budget must not discard successful samples.
	return snapshot, parent.Err()
}

// interrupted explains why the poll context ended: an expired collection
// timeout, or the job's own cancellation.
func (c *Collector) interrupted(parent, ctx context.Context) error {
	if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("WMI collection did not finish within the %s timeout: %w", c.Timeout.Duration(), ctx.Err())
	}
	return ctx.Err()
}

// safeTransportError keeps only the transport's classified messages; other
// error text can carry account names or server-supplied details.
func safeTransportError(operation string, err error) error {
	if detail := wmiquery.SafeMessage(err); detail != "" {
		return fmt.Errorf("%s: %s", operation, detail)
	}
	return errors.New(operation)
}
