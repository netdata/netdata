// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
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

func appendTopologyProfiles(profiles []*ddsnmp.Profile, log *logger.Logger) []*ddsnmp.Profile {
	added := false

	for _, name := range []string{
		topologyLldpProfileName,
		cdpProfileName,
		fdbArpProfileName,
		qBridgeProfileName,
		stpProfileName,
		vtpProfileName,
	} {
		if profilesHaveExtension(profiles, name) {
			continue
		}
		if prof, err := ddsnmp.LoadProfileByName(name); err == nil {
			profiles = append(profiles, prof)
			added = true
			log.Infof("topology autoprobe: appended profile %s", name)
		} else {
			log.Warningf("topology autoprobe: failed to load profile %s: %v", name, err)
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
