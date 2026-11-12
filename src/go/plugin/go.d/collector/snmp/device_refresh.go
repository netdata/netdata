// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

const sysInfoCheckEvery = time.Minute

// refreshDeviceState runs on the collection owner, before normal metric polling.
// Any answered system scalar that changed rebuilds device state. Only a changed
// sysObjectID makes the incumbent state incompatible and suspends it.
func (c *Collector) refreshDeviceState() error {
	if c.now().Before(c.nextSysInfoCheck) {
		return c.pendingRebuildError()
	}
	defer func() { c.nextSysInfoCheck = c.now().Add(sysInfoCheckEvery) }()

	observed, err := snmputils.GetSysInfo(c.snmpClient)
	if err != nil {
		c.Limit("snmp:sysinfo", 1, 5*time.Minute).Warningf("failed to refresh SNMP system information: %v", err)
		return c.pendingRebuildError()
	}
	if observed.Probe.SeenSysObjectID && observed.SysObjectID != "" &&
		(c.rebuildPending || observed.SysObjectID != c.sysInfo.SysObjectID) {
		if !c.rebuildPending {
			c.withdrawDeviceState()
			c.devicePublished = false
			c.resetDeviceFunctionState()
		}
		c.rebuildPending = true
		// A newly classified device cannot inherit unanswered fields from the
		// previous device's identity.
		return c.rebuildDevice(observed)
	}
	if c.rebuildPending {
		return c.pendingRebuildError()
	}
	next := mergeSystemObservation(c.sysInfo, observed)
	if sameSystemMetadata(c.sysInfo, next) {
		return nil
	}
	// The incumbent state is still compatible, so a failed rebuild keeps it
	// collecting and the next check retries.
	if err := c.rebuildDevice(next); err != nil {
		c.Limit("snmp:rebuild", 1, 5*time.Minute).Warningf("failed to refresh SNMP device state: %v", err)
	}
	return nil
}

func (c *Collector) pendingRebuildError() error {
	if c.rebuildPending {
		return fmt.Errorf("device sysObjectID changed; waiting for compatible SNMP collection state")
	}
	return nil
}

// Unanswered/unsupported fields cannot erase a usable observation. Explicitly
// returned empty strings remain observations and can clear editable metadata.
func mergeSystemObservation(previous, observed *snmputils.SysInfo) *snmputils.SysInfo {
	next := *previous
	if observed.Probe.SeenSysObjectID && observed.SysObjectID != "" {
		next.SysObjectID = observed.SysObjectID
		next.Organization, next.Vendor = observed.Organization, observed.Vendor
		next.Category, next.Model = observed.Category, observed.Model
	}
	if observed.Probe.SeenSysName {
		next.Name = observed.Name
	}
	if observed.Probe.SeenSysDescr {
		next.Descr = observed.Descr
	}
	if observed.Probe.SeenSysContact {
		next.Contact = observed.Contact
	}
	if observed.Probe.SeenSysLocation {
		next.Location = observed.Location
	}
	next.Probe = observed.Probe
	return &next
}

func sameSystemMetadata(a, b *snmputils.SysInfo) bool {
	return a.Name == b.Name && a.Descr == b.Descr && a.Contact == b.Contact && a.Location == b.Location
}

func (c *Collector) applyDeviceMetadata(si *snmputils.SysInfo, metadata map[string]ddsnmp.MetaTag) error {
	var vnode *vnodes.VirtualNode
	if c.CreateVnode && c.Vnode == "" {
		var baseLabels map[string]string
		if c.UpdateEvery >= 1 && c.VnodeDeviceDownThreshold >= 1 {
			baseLabels = map[string]string{
				"_node_stale_after_seconds": strconv.Itoa(c.VnodeDeviceDownThreshold*c.UpdateEvery + 2),
			}
		}
		identity, err := ddsnmp.ComposeDeviceIdentity(si, metadata, ddsnmp.DeviceIdentityOptions{
			Address:    c.Hostname,
			GUID:       c.LocalVnode.GUID,
			Hostname:   c.LocalVnode.Hostname,
			BaseLabels: baseLabels,
			Labels:     c.LocalVnode.Labels,
		})
		if err != nil {
			return err
		}
		vnode = &vnodes.VirtualNode{
			GUID:     identity.GUID,
			Hostname: identity.Hostname,
			Labels:   identity.Labels,
		}
		if c.vnode != nil {
			if _, err := hostoutput.NewDefinition(netdataapi.HostInfo{
				GUID:     vnode.GUID,
				Hostname: vnode.Hostname,
				Labels:   vnode.HostLabels(),
			}); err != nil {
				return err
			}
		}
	}
	c.sysInfo = si
	if vnode != nil {
		c.vnode = vnode
	}
	c.registerDeviceState(si, metadata)
	c.devicePublished = true
	c.publishedProfileInputRevision = 0
	if c.ddSnmpColl != nil {
		c.publishedProfileInputRevision = c.ddSnmpColl.ProfileInputRevision()
	}
	return nil
}

// syncDeviceMetadata publishes device identity after a profile poll, whenever
// the profile collector has newly prepared inputs. Samples only come from
// profiles with prepared inputs, so the vnode exists before any sample leaves
// the collector, and a poll that prepared nothing publishes nothing: a new or
// rebuilt device is never visible without its profile metadata.
func (c *Collector) syncDeviceMetadata() {
	if c.sysInfo == nil || c.publishedProfileInputRevision == c.ddSnmpColl.ProfileInputRevision() {
		return
	}
	if err := c.applyDeviceMetadata(c.sysInfo, c.ddSnmpColl.PreparedDeviceMetadata()); err != nil {
		c.Limit("snmp:metadata", 1, sysInfoCheckEvery).Warningf("failed to update device metadata: %v", err)
	}
}

func (c *Collector) preparedProfileMetadata() map[string]ddsnmp.MetaTag {
	if c.ddSnmpColl == nil {
		return nil
	}
	return c.ddSnmpColl.PreparedDeviceMetadata()
}

// rebuildDevice replaces every piece of device-derived state with a fresh one for
// si. The candidate is prepared first, so a failure leaves the incumbent intact.
// Identity and chart changes are published when the new state reports
// (syncDeviceMetadata, retireStaleCharts), not here.
// It runs only for normal polling jobs; ping_only keeps setup-only SNMP.
func (c *Collector) rebuildDevice(si *snmputils.SysInfo) error {
	profiles, profileContext := c.resolveProfiles(si)
	if len(profiles) == 0 {
		return snmputils.WithFailure(noMetricProfilesError(si), "profile_resolution", "no_profiles")
	}
	client, disableBulk, repetitions, err := c.connectSNMPClient()
	if err != nil {
		return err
	}
	// The diagnostic wrapper is owned by this attempt; transfer its underlying
	// connection so finishing the attempt cannot restore the retired client.
	previous := c.snmpClient
	c.snmpClient = client
	if c.normal.client != nil {
		previous, c.normal.client = c.normal.client, client
		c.snmpClient = c.normal.recorder.Wrap(client)
	}
	c.sysInfo, c.snmpProfiles = si, profiles
	c.disableBulkWalk, c.adjMaxRepetitions = disableBulk, repetitions
	c.ddSnmpColl = c.newProfileCollector()
	c.activateProfiles(profiles, profileContext)
	c.rebuildPending = false
	c.devicePublished, c.publishedProfileInputRevision = false, 0
	c.resetDeviceFunctionState()
	c.resetDevicePresentation()
	_ = previous.Close()
	return nil
}

func (c *Collector) resetDevicePresentation() {
	// Existing charts stay until the new state reports. installChart redefines
	// the ones it reports again; retireStaleCharts removes the rest after the
	// first successful poll, so a failed first poll only leaves gaps.
	c.staleCharts = make(map[string]struct{}, len(*c.charts))
	for _, chart := range *c.charts {
		c.staleCharts[chart.ID] = struct{}{}
	}
	clear(c.seenScalarMetrics)
	clear(c.tableMetricCharts)
	clear(c.seenProfiles)
	c.normal.initialization = nil
	c.normal.latest, c.normal.failed, c.normal.cut = nil, nil, nil
	c.normal.bgpSources, c.normal.licenseSources = nil, nil
	if c.normal.recorder != nil {
		for ordinal := uint64(1); ordinal <= c.normal.recorder.Cursor(); ordinal++ {
			c.normal.initialization = append(c.normal.initialization, c.normal.recorder.Operation(ordinal))
		}
	}
	if c.Ping.Enabled {
		c.addPingCharts()
	}
}

// Function data belongs to the device state it was collected with. A changed
// sysObjectID clears it even when the rebuild cannot finish yet.
func (c *Collector) resetDeviceFunctionState() {
	c.ifaceCache.mu.Lock()
	clear(c.ifaceCache.interfaces)
	c.ifaceCache.lastUpdate, c.ifaceCache.updateTime = time.Time{}, time.Time{}
	c.ifaceCache.mu.Unlock()
	c.licensing.cache.store(time.Time{}, nil)
	if c.bgp != nil {
		c.bgp.peerCache.mu.Lock()
		clear(c.bgp.peerCache.entries)
		c.bgp.peerCache.lastUpdate, c.bgp.peerCache.lastFailure = time.Time{}, time.Time{}
		c.bgp.peerCache.lastError = ""
		c.bgp.peerCache.mu.Unlock()
		c.bgp.collectError, c.bgp.collectFailedSources = nil, nil
	}
}
