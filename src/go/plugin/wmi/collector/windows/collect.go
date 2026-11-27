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

// Raw performance counters: the formatted classes describe only a moment around
// each query, not the interval between polls.
const queryCPU = "SELECT Name, PercentInterruptTime, PercentUserTime, PercentPrivilegedTime, PercentDPCTime, " +
	"PercentIdleTime FROM Win32_PerfRawData_Counters_ProcessorInformation"

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

	// Disk intervals span consecutive polls only: across a failed poll, a
	// 32-bit operation count could wrap unnoticed.
	previousDisks := c.lastDisks
	c.lastDisks = nil

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
	// timing is the poller's clock around the query whose rows are being read.
	var timing queryTiming
	groups := [...]metricGroup{
		{name: "CPU", enabled: groupEnabled(c.Collectors.CPU), query: queryCPU, read: snapshot.readCPU},
		{name: "memory", enabled: groupEnabled(c.Collectors.Memory), query: queryMemory, read: snapshot.readMemory},
		{
			name:    "memory counters",
			enabled: groupEnabled(c.Collectors.Memory),
			query:   queryMemoryCounters,
			read:    snapshot.readMemoryCounters,
		},
		{
			name:    "paging file",
			enabled: groupEnabled(c.Collectors.Memory),
			query:   queryPagingFiles,
			read:    snapshot.readPagingFiles,
		},
		{
			name:    "NUMA node",
			enabled: groupEnabled(c.Collectors.Memory),
			query:   queryNUMANodes,
			read:    snapshot.readNUMANodes,
		},
		{
			name:    "system",
			enabled: groupEnabled(c.Collectors.System),
			query:   querySystem,
			read:    func(rows []map[string]any) bool { return snapshot.readSystem(rows, timing) },
		},
		{
			name:    "network",
			enabled: groupEnabled(c.Collectors.Network),
			query:   queryNetworkInterfaces,
			read:    snapshot.readNetworkInterfaces,
		},
		{
			name:    "network adapter",
			enabled: groupEnabled(c.Collectors.Network),
			// Adapters are virtual only when this poll's interface list lacks
			// them; a host reached over the network lists at least one interface.
			needs: func() bool { return len(snapshot.physicalInterfaces) > 0 },
			query: queryNetworkAdapters,
			read:  snapshot.readNetworkAdapters,
		},
		{name: "TCPv4", enabled: groupEnabled(c.Collectors.Network), query: queryTCPv4, read: snapshot.readTCP(ipv4)},
		{name: "TCPv6", enabled: groupEnabled(c.Collectors.Network), query: queryTCPv6, read: snapshot.readTCP(ipv6)},
		{name: "UDPv4", enabled: groupEnabled(c.Collectors.Network), query: queryUDPv4, read: snapshot.readUDP(ipv4)},
		{name: "UDPv6", enabled: groupEnabled(c.Collectors.Network), query: queryUDPv6, read: snapshot.readUDP(ipv6)},
		{
			name:    "disk",
			enabled: groupEnabled(c.Collectors.Disk),
			query:   queryDisk,
			read: func(rows []map[string]any) bool {
				fixedDrives, fixedDrivesRead = rows, true
				return snapshot.readDisks(rows)
			},
		},
		{
			name:    "physical disk",
			enabled: groupEnabled(c.Collectors.Disk),
			query:   queryPhysicalDisks,
			read:    snapshot.readPhysicalDisks,
		},
		{
			name:    "SMB server",
			enabled: groupEnabled(c.Collectors.SMB),
			query:   querySMBServer,
			read:    snapshot.readSMBServer,
		},
		{
			name:    "SMB share",
			enabled: groupEnabled(c.Collectors.SMB),
			query:   querySMBShares,
			read:    snapshot.readSMBShares,
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
		sent := c.now()
		rows, err := session.Query(ctx, group.query)
		timing = queryTiming{
			sent:     sent,
			duration: c.now().Sub(sent),
		}
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
	if snapshot.physicalDisksRead {
		// Like the native Windows Agent, read a disk's model and device ID
		// once, when it first appears.
		if !sessionLost && snapshot.unknownDiskDrive(c.diskDrives) {
			sessionLost = c.lookupDiskDrives(ctx, session, &snapshot)
		}
		c.updatePhysicalDisks(previousDisks, snapshot.physicalDisks)
	}
	if inventoryDue && !sessionLost {
		c.refreshInventory(ctx, session, fixedDrives, fixedDrivesRead)
	}
	// Exhausting the optional inventory budget must not discard successful samples.
	return snapshot, parent.Err()
}

// lookupDiskDrives reads the model and device ID of this poll's disks. The
// labels are optional, so the lookup has a budget of its own, as the inventory
// has, and a lookup that fails or runs out of it only leaves them out until a
// later poll reads them. It reports whether the session failed.
func (c *Collector) lookupDiskDrives(ctx context.Context, session querySession, snapshot *observations) bool {
	deadline, _ := ctx.Deadline()
	ctx, cancel := context.WithTimeout(ctx, min(diskDriveBudget, time.Until(deadline)/2))
	defer cancel()
	rows, err := session.Query(ctx, queryDiskDrives)
	if err != nil {
		c.Limit("wmi:disk drive", 1, time.Minute).Warning(safeTransportError("disk drive WMI query failed", err))
		return wmiquery.SessionFailed(err)
	}
	c.diskDrives = snapshot.diskDrivesOf(rows)
	return false
}

// updatePhysicalDisks completes this poll's disks with their cached model and
// device ID and the values of the interval since the previous poll's sample,
// and keeps the samples for the next poll. Disks that disappeared are
// forgotten.
func (c *Collector) updatePhysicalDisks(previous map[string]physicalDisk, disks []physicalDisk) {
	c.lastDisks = make(map[string]physicalDisk, len(disks))
	drives := make(map[string]diskDrive, len(disks))
	for i := range disks {
		disk := &disks[i]
		if drive, ok := c.diskDrives[disk.device]; ok {
			disk.drive = drive
			drives[disk.device] = drive
		}
		if last, ok := previous[disk.device]; ok {
			disk.interval = disk.intervalSince(last)
		}
		c.lastDisks[disk.device] = *disk
	}
	c.diskDrives = drives
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
