// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
)

// deviceHostScope returns the virtual node of an assigned device. Its GUID
// depends only on the organization ID and serial, so renames and network moves
// keep the node. Names and network are metadata.
func deviceHostScope(organizationID string, dev device) metrix.HostScope {
	// The length prefix keeps the two opaque IDs from running into each other.
	identity := fmt.Sprintf("meraki:%d:%s%s", len(organizationID), organizationID, dev.serial)
	guid := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(identity)).String()

	hostname := strings.TrimSpace(dev.name)
	if hostname == "" {
		hostname = "meraki-" + dev.serial
	}
	info := netdataapi.HostInfo{
		GUID:     guid,
		Hostname: hostname,
		Labels: map[string]string{
			"_vnode_type":     "meraki_device",
			"organization_id": organizationID,
			"serial":          dev.serial,
			"device_name":     dev.name,
			"model":           dev.model,
			"product_type":    dev.productType,
			"network_id":      dev.networkID,
			"network_name":    dev.networkName,
		},
	}
	prepared, err := chartemit.PrepareHostInfo(info)
	if err != nil {
		// The name is not wire-safe; the GUID-based hostname always is.
		info.Hostname = "meraki-" + guid
		prepared, _ = chartemit.PrepareHostInfo(info)
	}
	return metrix.HostScope{
		ScopeKey: guid,
		GUID:     guid,
		Hostname: prepared.Hostname,
		Labels:   prepared.Labels,
	}
}
