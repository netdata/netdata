// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"maps"
	"slices"

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

func (c *Collector) vnodeHostname() string {
	if c.vnode != nil {
		return c.vnode.Hostname
	}
	return ""
}

func (c *Collector) vnodeLabels() map[string]string {
	if c.vnode != nil && len(c.vnode.Labels) > 0 {
		return c.vnode.Labels
	}
	return nil
}

func (c *Collector) deviceStoreOwnerKey() string {
	c.deviceLifecycleMu.Lock()
	defer c.deviceLifecycleMu.Unlock()
	if c.deviceLifecycleOwner != "" {
		return c.deviceLifecycleOwner
	}
	return fmt.Sprintf("%p:%s:%d", c, c.Hostname, c.Options.Port)
}

func (c *Collector) deviceStoreCleanupKey() (string, bool) {
	c.deviceLifecycleMu.Lock()
	defer c.deviceLifecycleMu.Unlock()
	if c.deviceLifecycleOwner != "" {
		return c.deviceLifecycleOwner, c.deviceLifecycleManaged
	}
	return fmt.Sprintf("%p:%s:%d", c, c.Hostname, c.Options.Port), false
}

// registerDeviceState exposes the already-configured SNMP job to SNMP-family
// consumers without duplicating job configuration.
func (c *Collector) registerDeviceState(si *snmputils.SysInfo, profileMetadata map[string]ddsnmp.MetaTag) {
	if c.deviceStore == nil {
		return
	}
	vnodeLabels := c.vnodeLabels()
	vendor, model := ddsnmp.ResolveDeviceIdentity(
		firstVendor(si.Vendor, si.Organization),
		si.Model,
		profileMetadata,
		vnodeLabels,
	)

	c.publishDeviceState(ddsnmp.DeviceConnectionInfo{
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
		Vendor:          vendor,
		Model:           model,

		DisableBulkWalk: c.disableBulkWalk,
		ManualProfiles:  c.ManualProfiles,
		VnodeGUID:       c.vnodeGUID(),
		VnodeHostname:   c.vnodeHostname(),
		VnodeLabels:     vnodeLabels,
	})
}

func (c *Collector) publishDeviceState(info ddsnmp.DeviceConnectionInfo) {
	c.deviceLifecycleMu.Lock()
	if c.deviceLifecycleManaged && !c.deviceLifecycleCommitted {
		pending := info
		pending.ManualProfiles = slices.Clone(info.ManualProfiles)
		pending.VnodeLabels = maps.Clone(info.VnodeLabels)
		c.deviceLifecyclePending = &pending
		c.deviceLifecycleMu.Unlock()
		return
	}
	ownerKey := c.deviceLifecycleOwner
	if ownerKey == "" {
		ownerKey = fmt.Sprintf("%p:%s:%d", c, c.Hostname, c.Options.Port)
	}
	c.deviceLifecycleMu.Unlock()
	c.deviceStore.Register(ownerKey, info)
}

func (c *Collector) syncDeviceMetadata(pms []*ddsnmp.ProfileMetrics) {
	if c.deviceMetadataSynced || c.vnode != nil || c.sysInfo == nil {
		return
	}

	metadata := make(map[string]ddsnmp.MetaTag, 2)
	for _, pm := range pms {
		ddsnmp.MergeDeviceIdentityMetadata(metadata, pm.DeviceMetadata)
	}

	c.registerDeviceState(c.sysInfo, metadata)
	c.deviceMetadataSynced = true
}
