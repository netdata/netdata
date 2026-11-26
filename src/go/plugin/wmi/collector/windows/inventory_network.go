// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"fmt"
	"math"
	"net/netip"
	"strings"
)

const (
	queryDefaultRoutes = "SELECT InterfaceIndex FROM Win32_IP4RouteTable WHERE Destination = '0.0.0.0' AND Mask = '0.0.0.0'"
	// The interface index in these projections is a validated uint32.
	queryAdapterNameFormat = "SELECT InterfaceIndex, NetConnectionID FROM Win32_NetworkAdapter WHERE InterfaceIndex = %d"
	queryAdapterIPsFormat  = "SELECT InterfaceIndex, IPAddress FROM Win32_NetworkAdapterConfiguration WHERE InterfaceIndex = %d"
)

// defaultInterface returns the IPv4 default route's interface name and address.
// It returns empty values unless every default route uses one interface that has
// exactly one global unicast IPv4 address.
func defaultInterface(ctx context.Context, session querySession) (name, ip string, err error) {
	routes, err := session.Query(ctx, queryDefaultRoutes)
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

	adapters, err := session.Query(ctx, fmt.Sprintf(queryAdapterNameFormat, index))
	if err != nil {
		return "", "", err
	}
	name, _ = adapterRow(adapters, index)["netconnectionid"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", nil
	}

	configs, err := session.Query(ctx, fmt.Sprintf(queryAdapterIPsFormat, index))
	if err != nil {
		return "", "", err
	}
	addresses, _ := adapterRow(configs, index)["ipaddress"].([]string)
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

// adapterRow returns the only row when it belongs to the requested interface.
func adapterRow(rows []map[string]any, index uint64) map[string]any {
	if len(rows) != 1 {
		return nil
	}
	got := readUnsigned(rows[0]["interfaceindex"])
	if !got.valid || got.value != index {
		return nil
	}
	return rows[0]
}

func updateDefaultInterface(labels map[string]string, name, ip string) {
	setLabel(labels, "_net_default_iface", name)
	setLabel(labels, "_net_default_iface_ip", ip)
	detection := ""
	if ip != "" {
		detection = "wmi"
	}
	setLabel(labels, "_net_default_iface_detection", detection)
}
