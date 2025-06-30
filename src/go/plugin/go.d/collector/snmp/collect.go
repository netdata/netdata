// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/discoverer/snmpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.sysInfo == nil {
		si, err := snmpsd.GetSysInfo(c.snmpClient)
		if err != nil {
			return nil, err
		}

		c.sysInfo = si
		c.addSysUptimeChart()

		if c.CreateVnode {
			c.vnode = c.setupVnode(si)
		}

		if c.DisableLegacyCollection || c.EnableProfiles {
			c.snmpProfiles = c.setupProfiles()
		}
	}

	mx := make(map[string]int64)

	if err := c.collectProfiles(mx); err != nil {
		c.Infof("failed to collect profiles: %v", err)
	}

	if !c.DisableLegacyCollection {
		if err := c.collectSysUptime(mx); err != nil {
			return nil, err
		}

		if c.collectIfMib {
			if err := c.collectNetworkInterfaces(mx); err != nil {
				return nil, err
			}
		}

		if len(c.customOids) > 0 {
			if err := c.collectOIDs(mx); err != nil {
				return nil, err
			}
		}
	}

	return mx, nil
}

func (c *Collector) collectSysUptime(mx map[string]int64) error {
	resp, err := c.snmpClient.Get([]string{snmpsd.OidSysUptime})
	if err != nil {
		return err
	}
	if len(resp.Variables) == 0 {
		return errors.New("no system uptime")
	}
	v, err := pduToInt(resp.Variables[0])
	if err != nil {
		return err
	}

	mx["uptime"] = v / 100 // the time is in hundredths of a second

	return nil
}

func (c *Collector) walkAll(rootOid string) ([]gosnmp.SnmpPDU, error) {
	if c.snmpClient.Version() == gosnmp.Version1 {
		return c.snmpClient.WalkAll(rootOid)
	}
	return c.snmpClient.BulkWalkAll(rootOid)
}

func (c *Collector) setupVnode(si *snmpsd.SysInfo) *vnodes.VirtualNode {
	if c.Vnode.GUID == "" {
		c.Vnode.GUID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(c.Hostname)).String()
	}

	hostnames := []string{c.Vnode.Hostname, si.Name, "snmp-device"}
	i := slices.IndexFunc(hostnames, func(s string) bool { return s != "" })

	c.Vnode.Hostname = fmt.Sprintf("SNMP-%s(%s)", hostnames[i], c.Hostname)

	labels := make(map[string]string)

	for k, v := range c.Vnode.Labels {
		labels[k] = v
	}
	if si.Descr != "" {
		labels["sysDescr"] = si.Descr
	}
	if si.Contact != "" {
		labels["sysContact"] = si.Contact
	}
	if si.Location != "" {
		labels["sysLocation"] = si.Location
	}
	// FIXME: vendor should be obtained from sysDescr, org should be used as a fallback
	labels["vendor"] = si.Organization

	return &vnodes.VirtualNode{
		GUID:     c.Vnode.GUID,
		Hostname: c.Vnode.Hostname,
		Labels:   labels,
	}
}

func (c *Collector) setupProfiles() []*ddsnmp.Profile {
	snmpProfiles := ddsnmp.FindProfiles(c.sysInfo.SysObjectID)
	var profInfo []string
	for _, prof := range snmpProfiles {
		if logger.Level.Enabled(slog.LevelDebug) {
			profInfo = append(profInfo, prof.SourceTree())
		} else {
			name := strings.TrimSuffix(filepath.Base(prof.SourceFile), filepath.Ext(prof.SourceFile))
			profInfo = append(profInfo, name)
		}
	}
	c.Infof("device matched %d profile(s): %s (sysObjectID: %s)",
		len(snmpProfiles), strings.Join(profInfo, ", "), c.sysInfo.SysObjectID)
	return snmpProfiles
}

func pduToInt(pdu gosnmp.SnmpPDU) (int64, error) {
	switch pdu.Type {
	case gosnmp.Counter32, gosnmp.Counter64, gosnmp.Integer, gosnmp.Gauge32, gosnmp.TimeTicks:
		return gosnmp.ToBigInt(pdu.Value).Int64(), nil
	default:
		return 0, fmt.Errorf("unussported type: '%v'", pdu.Type)
	}
}

//func physAddressToString(pdu gosnmp.SnmpPDU) (string, error) {
//	address, ok := pdu.Value.([]uint8)
//	if !ok {
//		return "", errors.New("physAddress is not a []uint8")
//	}
//	parts := make([]string, 0, 6)
//	for _, v := range address {
//		parts = append(parts, fmt.Sprintf("%02X", v))
//	}
//	return strings.Join(parts, ":"), nil
//}
