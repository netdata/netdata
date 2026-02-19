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
