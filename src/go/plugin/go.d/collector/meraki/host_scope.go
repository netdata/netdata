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

func deviceHostScope(organizationID string, d deviceSnapshot) metrix.HostScope {
	// A length-prefixed organization ID keeps opaque components unambiguous.
	identity := fmt.Sprintf("meraki:%d:%s%s", len(organizationID), organizationID, d.serial)
	guid := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(identity)).String()
	name := strings.TrimSpace(d.name)
	if name == "" {
		name = "meraki-" + d.serial
	}
	info := netdataapi.HostInfo{
		GUID:     guid,
		Hostname: name,
		Labels: map[string]string{
			"_vnode_type": "meraki_device", "organization_id": organizationID, "serial": d.serial,
			"device_name": d.name, "model": d.model, "product_type": d.productType,
			"network_id": d.networkID, "network_name": d.networkName,
		},
	}
	prepared, err := chartemit.PrepareHostInfo(info)
	if err != nil {
		info.Hostname = "meraki-" + guid
		prepared, _ = chartemit.PrepareHostInfo(info) // Deterministic UUID fallback is wire-safe.
	}
	return metrix.HostScope{
		ScopeKey: guid,
		GUID:     guid,
		Hostname: prepared.Hostname,
		Labels:   prepared.Labels,
	}
}
