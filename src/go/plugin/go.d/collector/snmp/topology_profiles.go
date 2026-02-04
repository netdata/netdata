// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

const (
	lldpProfileName = "_std-lldp-mib.yaml"
	cdpProfileName  = "_std-cdp-mib.yaml"
)

func (c *Collector) appendTopologyProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	if !c.Topology.Autoprobe {
		return profiles
	}

	added := false

	if !profilesHaveExtension(profiles, lldpProfileName) {
		if prof, err := ddsnmp.LoadProfileByName(lldpProfileName); err == nil {
			profiles = append(profiles, prof)
			added = true
			c.Infof("topology autoprobe enabled: appended profile %s", lldpProfileName)
		} else {
			c.Warningf("topology autoprobe: failed to load profile %s: %v", lldpProfileName, err)
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
