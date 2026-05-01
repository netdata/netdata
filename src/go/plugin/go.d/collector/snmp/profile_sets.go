// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func (c *Collector) setupProfiles(si *snmputils.SysInfo) []*ddsnmp.Profile {
	matchedProfiles := ddsnmp.FindProfiles(si.SysObjectID, si.Descr, c.ManualProfiles)
	c.logMatchedProfiles(matchedProfiles, si.SysObjectID)

	return selectCollectionProfiles(matchedProfiles)
}

func (c *Collector) logMatchedProfiles(profiles []*ddsnmp.Profile, sysObjectID string) {
	var profInfo []string

	for _, prof := range profiles {
		if logger.Level.Enabled(slog.LevelDebug) {
			profInfo = append(profInfo, prof.SourceTree())
		} else {
			name := strings.TrimSuffix(filepath.Base(prof.SourceFile), filepath.Ext(prof.SourceFile))
			profInfo = append(profInfo, name)
		}
	}

	msg := fmt.Sprintf("device matched %d profile(s): %s (sysObjectID: '%s')", len(profiles), strings.Join(profInfo, ", "), sysObjectID)
	if len(profiles) == 0 {
		c.Warning(msg)
	} else {
		c.Info(msg)
	}
}
