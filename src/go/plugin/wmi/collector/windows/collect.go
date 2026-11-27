// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows/internal/wmiquery"
)

const (
	// Raw performance counters: the formatted classes describe only a moment around
	// each query, not the interval between polls.
	queryCPU = "SELECT Name, PercentInterruptTime, PercentUserTime, PercentPrivilegedTime, PercentDPCTime, " +
		"PercentIdleTime FROM Win32_PerfRawData_Counters_ProcessorInformation"
	queryMemory = "SELECT TotalVisibleMemorySize, FreePhysicalMemory FROM Win32_OperatingSystem"
	queryDisk   = "SELECT DeviceID, Size, FreeSpace, FileSystem, VolumeSerialNumber, Access FROM Win32_LogicalDisk " +
		"WHERE DriveType = 3"
)

type metricGroup struct {
	name    string
	enabled bool
	// needs, when set, reports whether the earlier queries of this poll gave the
	// observations this query's rows depend on; the query is skipped otherwise.
	needs func() bool
	query string
	read  func([]map[string]any) bool
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
		{
			name:    "network",
			enabled: groupEnabled(c.Collectors.Network),
			query:   queryNetworkInterfaces,
			read:    snapshot.readNetworkInterfaces,
		},
		{
			name:    "network adapter",
			enabled: groupEnabled(c.Collectors.Network),
			// Adapters are virtual only when this poll's interface list lacks them.
			needs: func() bool { return snapshot.physicalInterfaces != nil },
			query: queryNetworkAdapters,
			read:  snapshot.readNetworkAdapters,
		},
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

	// reached: a query got an answer from the host. sessionLost: the session
	// failed, so later queries and the inventory would fail the same way.
	var succeeded, reached, sessionLost bool
	var failures [len(groups)]error
	for i, group := range groups {
		if !group.enabled || (group.needs != nil && !group.needs()) {
			continue
		}
		if ctx.Err() != nil {
			return observations{}, c.interrupted(parent, ctx)
		}
		rows, err := session.Query(ctx, group.query)
		failed := wmiquery.SessionFailed(err)
		if failed && !reached && ctx.Err() == nil {
			// WinRM connects and logs in with the first query.
			return observations{}, safeTransportError("failed to open WMI session", err)
		}
		reached = reached || !failed
		sessionLost = sessionLost || failed
		switch {
		case err != nil:
			failures[i] = safeTransportError(group.name+" WMI query failed", err)
		case !group.read(rows):
			failures[i] = publicError(group.name + " WMI query returned no usable observations")
		default:
			succeeded = true
		}
	}

	if ctx.Err() != nil {
		return observations{}, c.interrupted(parent, ctx)
	}
	if !succeeded {
		var messages []string
		for _, failure := range failures {
			if message, ok := dyncfg.PublicMessage(failure); ok {
				messages = append(messages, message)
			}
		}
		summary := "no enabled WMI query group succeeded: " + strings.Join(messages, "\n")
		return observations{}, dyncfg.NewPublicError(summary, errors.Join(failures[:]...))
	}
	for i, err := range failures {
		if err != nil {
			c.Limit("wmi:"+groups[i].name, 1, time.Minute).Warning(err)
		}
	}
	// Check's poll primes the CPU sample, so the first collection already has
	// an interval. A failed CPU query keeps the previous sample, so the next
	// interval spans both.
	snapshot.cpu = cpuUtilizationBetween(c.lastCPU, snapshot.cpuTimes)
	if !snapshot.cpuTimes.empty() {
		c.lastCPU = snapshot.cpuTimes
	}
	if inventoryDue && !sessionLost {
		c.refreshInventory(ctx, session, fixedDrives, fixedDrivesRead)
	}
	// Exhausting the optional inventory budget must not discard successful samples.
	return snapshot, parent.Err()
}

// interrupted explains why the poll context ended: an expired collection
// timeout, or the job's own cancellation.
func (c *Collector) interrupted(parent, ctx context.Context) error {
	if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// The timeout is a configuration value, so only the wrapping text names it.
		expired := dyncfg.NewPublicError("WMI collection did not finish within the timeout", ctx.Err())
		return fmt.Errorf("%w of %s", expired, c.Timeout.Duration())
	}
	return ctx.Err()
}

// safeTransportError keeps only the operation and the transport's classified
// message, which hold fixed text and protocol status codes. Other error text can
// carry account names or server-supplied details, so it stays the private cause.
func safeTransportError(operation string, err error) error {
	if detail := wmiquery.SafeMessage(err); detail != "" {
		return dyncfg.NewPublicError(operation+": "+detail, err)
	}
	return dyncfg.NewPublicError(operation, err)
}

// publicError reports a failure in fixed text, which the Agent shows even for
// jobs whose options use secret references.
func publicError(message string) error {
	return dyncfg.NewPublicError(message, errors.New(message))
}
