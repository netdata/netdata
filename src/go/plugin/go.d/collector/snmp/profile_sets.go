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
	resolved := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID:    si.SysObjectID,
		SysDescr:       si.Descr,
		ManualProfiles: c.ManualProfiles,
		ManualPolicy:   ddsnmp.ManualProfileFallback,
	})
	matchedProfiles := resolved.Profiles()
	c.logMatchedProfiles(matchedProfiles, si.SysObjectID)

	profiles := resolved.Project(ddsnmp.ConsumerMetrics, ddsnmp.ConsumerLicensing, ddsnmp.ConsumerBGP).Profiles()
	if profilesHaveBGP(profiles) {
		c.enableBGPIntegration()
	}

	return profiles
}

func (c *Collector) ensureDeviceProfile() error {
	if c.sysInfo != nil {
		return nil
	}

	si, err := snmputils.GetSysInfo(c.snmpClient)
	if err != nil {
		return err
	}
	c.Debugf("SNMP system identity: %s", formatSysInfoDiagnostic(si))

	profiles := c.snmpProfiles
	if profiles == nil {
		profiles = c.setupProfiles(si)
	}

	if len(profiles) == 0 && !c.PingOnly {
		return noMetricProfilesError(si)
	}

	c.sysInfo = si
	c.snmpProfiles = profiles
	return nil
}

func noMetricProfilesError(si *snmputils.SysInfo) error {
	const missingIdentityRemediation = "configure an applicable metric profile in manual_profiles or set ping_only: true"

	switch {
	case si == nil || si.Probe.PDUCount == 0:
		return fmt.Errorf("no SNMP metric profiles available: system subtree walk returned no PDUs; %s", missingIdentityRemediation)
	case si.SysObjectID == "":
		return fmt.Errorf(
			"no SNMP metric profiles available: system subtree walk returned %d PDU(s) without sysObjectID (%s); %s",
			si.Probe.PDUCount,
			formatSysInfoDiagnostic(si),
			missingIdentityRemediation,
		)
	default:
		return fmt.Errorf(
			"no SNMP metric profiles available for sysObjectID %q; add or update a profile whose selector matches this sysObjectID, "+
				"or set ping_only: true",
			si.SysObjectID,
		)
	}
}

func formatSysInfoDiagnostic(si *snmputils.SysInfo) string {
	if si == nil {
		return "unavailable"
	}

	p := si.Probe
	return fmt.Sprintf(
		"sys_object_id=%q vendor=%q category=%q model=%q probe={pdu_count=%d first_oid=%q last_oid=%q "+
			"sys_descr=%t sys_object_id=%t sys_contact=%t sys_name=%t sys_location=%t}",
		si.SysObjectID,
		si.Vendor,
		si.Category,
		si.Model,
		p.PDUCount,
		p.FirstOID,
		p.LastOID,
		p.SeenSysDescr,
		p.SeenSysObjectID,
		p.SeenSysContact,
		p.SeenSysName,
		p.SeenSysLocation,
	)
}

func (c *Collector) logMatchedProfiles(profiles []*ddsnmp.Profile, sysObjectID string) {
	if len(profiles) == 0 {
		return
	}

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
	c.Info(msg)
}
