// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	topologyLldpProfileName = "_std-topology-lldp-mib.yaml"
	cdpProfileName          = "_std-cdp-mib.yaml"
	fdbArpProfileName       = "_std-topology-fdb-arp-mib.yaml"
	qBridgeProfileName      = "_std-topology-q-bridge-mib.yaml"
	stpProfileName          = "_std-topology-stp-mib.yaml"
	vtpProfileName          = "_std-topology-cisco-vtp-mib.yaml"
)

func (c *Collector) appendTopologyProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	if !c.Topology.Autoprobe {
		return profiles
	}

	added := false

	if !profilesHaveExtension(profiles, topologyLldpProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(topologyLldpProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", topologyLldpProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", topologyLldpProfileName, err)
		}
	}

	if !profilesHaveExtension(profiles, cdpProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(cdpProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", cdpProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", cdpProfileName, err)
		}
	}

	if !profilesHaveExtension(profiles, fdbArpProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(fdbArpProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", fdbArpProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", fdbArpProfileName, err)
		}
	}

	if !profilesHaveExtension(profiles, qBridgeProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(qBridgeProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", qBridgeProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", qBridgeProfileName, err)
		}
	}

	if !profilesHaveExtension(profiles, stpProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(stpProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", stpProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", stpProfileName, err)
		}
	}

	if !profilesHaveExtension(profiles, vtpProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(vtpProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", vtpProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", vtpProfileName, err)
		}
	}

	if !added {
		return profiles
	}

	return ddsnmp.FinalizeProfiles(profiles)
}

func profilesHaveExtension(profiles []*ddsnmp.Profile, name string) bool {
	target := stripExt(name)
	for _, prof := range profiles {
		if prof == nil {
			continue
		}
		if stripExt(filepath.Base(prof.SourceFile)) == target {
			return true
		}
		if prof.HasExtension(target) {
			return true
		}
	}
	return false
}

func stripExt(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
}
