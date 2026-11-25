// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows/internal/wmiquery"
)

const (
	inventoryInterval = 5 * time.Minute
	// inventoryBudget caps the inventory within a poll; it also takes at most
	// half of the remaining poll time, leaving room to close the session.
	inventoryBudget = 5 * time.Second
)

// refreshInventory updates the generated node's hostname and system labels from
// the poll's session. Each provider owns its labels: a failed or skipped
// provider keeps its previous values, and a successful reply that lacks a value
// removes its label. The result is published as a new node snapshot.
func (c *Collector) refreshInventory(
	ctx context.Context,
	session querySession,
	fixedDrives []map[string]any,
	fixedDrivesRead bool,
) {
	c.nextInventory = time.Now().Add(inventoryInterval)
	deadline, _ := ctx.Deadline()
	ctx, cancel := context.WithTimeout(ctx, min(inventoryBudget, time.Until(deadline)/2))
	defer cancel()

	node := c.vnode.Copy()
	labels := node.Labels
	providers := []struct {
		name  string
		query string
		apply func([]map[string]any)
	}{
		{name: "operating system", query: queryOSInfo, apply: func(rows []map[string]any) {
			updateOSInfo(node, onlyRow(rows), canonicalEndpoint(c.Host))
		}},
		{name: "computer system", query: querySystemInfo, apply: func(rows []map[string]any) {
			updateSystemInfo(labels, onlyRow(rows))
		}},
		{name: "processor", query: queryProcessorInfo, apply: func(rows []map[string]any) {
			updateProcessorInfo(labels, onlyRow(rows))
		}},
		{name: "installed memory", query: queryInstalledMemory, apply: func(rows []map[string]any) {
			setLabel(labels, "_system_ram_total", installedMemory(rows))
		}},
	}
	// lost: the session failed, so every later query would fail the same way.
	var lost bool
	for _, provider := range providers {
		if lost || ctx.Err() != nil {
			break
		}
		rows, err := session.Query(ctx, provider.query)
		if err != nil {
			c.logInventoryError(provider.name, err)
			lost = wmiquery.SessionFailed(err)
			continue
		}
		provider.apply(rows)
	}
	if fixedDrivesRead && !lost && ctx.Err() == nil {
		if capacity, err := diskCapacity(ctx, session, fixedDrives); err != nil {
			c.logInventoryError("disk", err)
			lost = wmiquery.SessionFailed(err)
		} else {
			setLabel(labels, "_system_disk_space", capacity)
		}
	}
	if !lost && ctx.Err() == nil {
		if name, ip, err := defaultInterface(ctx, session); err != nil {
			c.logInventoryError("default interface", err)
		} else {
			updateDefaultInterface(labels, name, ip)
		}
	}
	c.vnode = node
}

func (c *Collector) logInventoryError(provider string, err error) {
	c.Limit("wmi:inventory:"+provider, 1, inventoryInterval).
		Warning(safeTransportError(provider+" WMI inventory query failed", err))
}

// onlyRow returns the row of a single-instance reply and nil for any other count.
func onlyRow(rows []map[string]any) map[string]any {
	if len(rows) == 1 {
		return rows[0]
	}
	return nil
}

func textProperty(row map[string]any, key string) string {
	v, _ := row[key].(string)
	return strings.TrimSpace(v)
}

// setLabel removes the label when the value is unknown.
func setLabel(labels map[string]string, key, value string) {
	if value == "" {
		delete(labels, key)
	} else {
		labels[key] = value
	}
}

func positiveDecimal(n uint64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}
