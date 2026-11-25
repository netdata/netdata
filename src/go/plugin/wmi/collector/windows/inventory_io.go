// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"math"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

const (
	queryDiskExtents   = "SELECT Dependent, StartingAddress, EndingAddress FROM Win32_LogicalDiskToPartition"
	queryDefaultRoutes = "SELECT InterfaceIndex FROM Win32_IP4RouteTable WHERE Destination = '0.0.0.0' AND Mask = '0.0.0.0'"
	queryAdapterName   = "SELECT InterfaceIndex, NetConnectionID FROM Win32_NetworkAdapter WHERE InterfaceIndex = "
	queryAdapterIPs    = "SELECT InterfaceIndex, IPAddress FROM Win32_NetworkAdapterConfiguration WHERE InterfaceIndex = "
)

// WMI may return either an absolute or a relative CIM object path.
var logicalDiskReference = regexp.MustCompile(
	`(?i)^(?:(?:\\\\[^\\]+\\)?root\\cimv2:)?Win32_LogicalDisk\.DeviceID="([a-z]:)"$`,
)

func diskInventory(ctx context.Context, client queryClient, fixedDrives []map[string]any) (string, error) {
	if len(fixedDrives) == 0 {
		return "", nil
	}
	drives := make(map[string]bool, len(fixedDrives))
	for _, row := range fixedDrives {
		id, _ := row["deviceid"].(string)
		id = strings.ToUpper(id)
		if len(id) != 2 || id[0] < 'A' || id[0] > 'Z' || id[1] != ':' {
			return "", nil
		}
		if _, duplicate := drives[id]; duplicate {
			return "", nil
		}
		drives[id] = false
	}

	rows, err := client.Query(ctx, queryDiskExtents)
	if err != nil {
		return "", err
	}
	remaining := len(drives)
	var total uint64
	for _, row := range rows {
		path, _ := row["dependent"].(string)
		match := logicalDiskReference.FindStringSubmatch(path)
		if match == nil {
			return "", nil
		}
		id := strings.ToUpper(match[1])
		seen, fixed := drives[id]
		if !fixed {
			continue
		}
		start := readUnsigned(row["startingaddress"])
		end := readUnsigned(row["endingaddress"])
		if seen || !start.valid || !end.valid || start.value > end.value {
			return "", nil
		}
		// The extent includes its ending byte. Check both that addition and
		// the aggregate before proceeding.
		diff := end.value - start.value
		if diff >= math.MaxUint64-total {
			return "", nil
		}
		total += diff + 1
		drives[id] = true
		remaining--
	}
	if remaining != 0 {
		return "", nil
	}
	return strconv.FormatUint(total, 10), nil
}

func networkInventory(ctx context.Context, client queryClient) (name, ip string, err error) {
	routes, err := client.Query(ctx, queryDefaultRoutes)
	if err != nil {
		return "", "", err
	}
	var index uint64
	for _, row := range routes {
		candidate := readUnsigned(row["interfaceindex"])
		if !candidate.valid || candidate.value == 0 || candidate.value > math.MaxUint32 {
			return "", "", nil
		}
		if index != 0 && index != candidate.value {
			return "", "", nil
		}
		index = candidate.value
	}
	if index == 0 {
		return "", "", nil
	}

	indexText := strconv.FormatUint(index, 10)
	adapters, err := client.Query(ctx, queryAdapterName+indexText)
	if err != nil {
		return "", "", err
	}
	adapter := matchingAdapter(adapters, index)
	name, _ = adapter["netconnectionid"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", nil
	}

	configs, err := client.Query(ctx, queryAdapterIPs+indexText)
	if err != nil {
		return "", "", err
	}
	config := matchingAdapter(configs, index)
	addresses, _ := config["ipaddress"].([]string)
	var address netip.Addr
	for _, raw := range addresses {
		candidate, err := netip.ParseAddr(raw)
		if err != nil {
			return "", "", nil
		}
		if !candidate.Is4() || !candidate.IsGlobalUnicast() {
			continue
		}
		if address.IsValid() && address != candidate {
			return "", "", nil
		}
		address = candidate
	}
	if !address.IsValid() {
		return "", "", nil
	}
	return name, address.String(), nil
}

func matchingAdapter(rows []map[string]any, index uint64) map[string]any {
	if len(rows) != 1 {
		return nil
	}
	got := readUnsigned(rows[0]["interfaceindex"])
	if !got.valid || got.value != index {
		return nil
	}
	return rows[0]
}
