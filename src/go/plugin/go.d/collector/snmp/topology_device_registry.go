// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func firstVendor(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (c *Collector) vnodeGUID() string {
	if c.vnode != nil {
		return c.vnode.GUID
	}
	return ""
}

func (c *Collector) vnodeLabels() map[string]string {
	if c.vnode != nil && len(c.vnode.Labels) > 0 {
		cp := make(map[string]string, len(c.vnode.Labels))
		maps.Copy(cp, c.vnode.Labels)
		return cp
	}
	return nil
}

func (c *Collector) deviceRegistryKey() string {
	return fmt.Sprintf("%p:%s:%d", c, c.Hostname, c.Options.Port)
}

// registerDeviceForTopology exposes the already-configured SNMP job to the
// snmp_topology collector without duplicating job configuration.
func (c *Collector) registerDeviceForTopology(si *snmputils.SysInfo) {
	ddsnmp.DeviceRegistry.Register(c.deviceRegistryKey(), ddsnmp.DeviceConnectionInfo{
		Hostname:        c.Hostname,
		Port:            c.Options.Port,
		SNMPVersion:     c.Options.Version,
		Community:       c.Community,
		V3User:          c.User.Name,
		V3SecurityLevel: c.User.SecurityLevel,
		V3AuthProto:     c.User.AuthProto,
		V3AuthKey:       c.User.AuthKey,
		V3PrivProto:     c.User.PrivProto,
		V3PrivKey:       c.User.PrivKey,
		V3ContextName:   c.User.ContextName,
		MaxRepetitions:  c.adjMaxRepetitions,
		MaxOIDs:         c.Options.MaxOIDs,
		Timeout:         c.Options.Timeout,
		Retries:         c.Options.Retries,
		SysObjectID:     si.SysObjectID,
		SysDescr:        si.Descr,
		SysName:         si.Name,
		SysContact:      si.Contact,
		SysLocation:     si.Location,
		Vendor:          firstVendor(si.Vendor, si.Organization),
		Model:           si.Model,

		DisableBulkWalk: c.disableBulkWalk,
		ManualProfiles:  c.ManualProfiles,
		VnodeGUID:       c.vnodeGUID(),
		VnodeLabels:     c.vnodeLabels(),
	})
}
