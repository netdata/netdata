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
// Only a usable changed sysObjectID justifies discarding device polling state.
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
	base := c.sysInfo
	if c.pendingSysInfo != nil {
		base = c.pendingSysInfo
	}
	next := mergeSystemObservation(base, observed)
	if observed.Probe.SeenSysObjectID && observed.SysObjectID != "" &&
		(c.rebuildPending || observed.SysObjectID != c.sysInfo.SysObjectID) {
		if !c.rebuildPending {
			c.withdrawDeviceState()
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
	observedOIDs := observedSystemOIDs(observed)
	systemChanged := !sameSystemMetadata(c.sysInfo, next)
	if c.pendingSysInfo == nil && !systemChanged &&
		(c.ddSnmpColl == nil || !c.ddSnmpColl.NeedsProfileInputRefresh(observedOIDs)) {
		return nil
	}

	c.pendingSysInfo = next
	var refreshErr error
	if c.ddSnmpColl != nil {
		refreshErr = c.ddSnmpColl.RefreshProfileInputs(observedOIDs)
		c.captureCollectionFailures()
		if refreshErr != nil {
			c.Limit("snmp:metadata", 1, sysInfoCheckEvery).
				Warningf("failed to refresh profile metadata: %v", refreshErr)
		}
	}
	if err := c.applyDeviceMetadata(next, c.preparedProfileMetadata()); err != nil {
		c.Limit("snmp:metadata", 1, sysInfoCheckEvery).Warningf("failed to update device metadata: %v", err)
		return nil
	}
	// Failed profile inputs stay pending and retry on the next check.
	if refreshErr == nil {
		c.pendingSysInfo = nil
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

func observedSystemOIDs(si *snmputils.SysInfo) []string {
	var oids []string
	for _, field := range []struct {
		seen bool
		oid  string
	}{
		{si.Probe.SeenSysObjectID && si.SysObjectID != "", snmputils.OidSysObject},
		{si.Probe.SeenSysDescr, snmputils.OidSysDescr},
		{si.Probe.SeenSysName, snmputils.OidSysName},
		{si.Probe.SeenSysContact, snmputils.OidSysContact},
		{si.Probe.SeenSysLocation, snmputils.OidSysLocation},
	} {
		if field.seen {
			oids = append(oids, field.oid)
		}
	}
	return oids
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
	changed := !sameSystemMetadata(c.sysInfo, si)
	var inputRevision uint64
	if c.ddSnmpColl != nil {
		inputRevision = c.ddSnmpColl.ProfileInputRevision()
		changed = changed || inputRevision != c.publishedProfileInputRevision
	}
	c.sysInfo = si
	if vnode != nil {
		c.vnode = vnode
	}
	if changed {
		c.chartMetadataVersion++
		c.refreshAuxiliaryChartLabels()
	}
	c.registerDeviceState(si, metadata)
	c.publishedProfileInputRevision = inputRevision
	return nil
}

// syncDeviceMetadata publishes profile inputs that normal collection prepared
// or recovered since the last publication.
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

// rebuildDevice runs only for normal polling jobs; ping_only keeps setup-only SNMP.
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
	c.pendingSysInfo, c.rebuildPending = nil, false
	c.publishedProfileInputRevision = 0
	c.resetDevicePresentation()
	_ = previous.Close()
	return c.applyDeviceMetadata(si, nil)
}

func (c *Collector) resetDevicePresentation() {
	for _, chart := range *c.charts {
		if !isPingChart(chart) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
	clear(c.seenScalarMetrics)
	clear(c.seenTableMetrics)
	clear(c.seenProfiles)
	c.chartMetadataVersion++
	c.normal.initialization = nil
	c.normal.latest, c.normal.failed, c.normal.cut = nil, nil, nil
	c.normal.bgpSources, c.normal.licenseSources = nil, nil
	if c.normal.recorder != nil {
		for ordinal := uint64(1); ordinal <= c.normal.recorder.Cursor(); ordinal++ {
			c.normal.initialization = append(c.normal.initialization, c.normal.recorder.Operation(ordinal))
		}
	}
	if c.PingOnly || c.Ping.Enabled {
		c.addPingCharts()
	}
}

// A confirmed incompatible classification invalidates Function data even when
// profile resolution or replacement connection setup cannot finish yet.
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
