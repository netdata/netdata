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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.sysInfo == nil {
		si, err := snmpsd.GetSysInfo(c.snmpClient)
		if err != nil {
			return nil, err
		}

		if c.DisableLegacyCollection || c.EnableProfiles {
			c.snmpProfiles = c.setupProfiles(si.SysObjectID)
		}

		if c.ddSnmpColl == nil {
			c.ddSnmpColl = ddsnmpcollector.New(c.snmpClient, c.snmpProfiles, c.Logger)
			c.ddSnmpColl.DoTableMetrics = c.EnableProfilesTableMetrics
		}

		if c.CreateVnode {
			deviceMeta, err := c.ddSnmpColl.CollectDeviceMetadata()
			if err != nil {
				return nil, err
			}
			c.vnode = c.setupVnode(si, deviceMeta)
		}

		c.sysInfo = si

		if !c.DisableLegacyCollection {
			c.addSysUptimeChart()
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

func (c *Collector) setupVnode(si *snmpsd.SysInfo, deviceMeta map[string]map[string]string) *vnodes.VirtualNode {
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

	labels["vendor"] = si.Organization
	if v, ok := orgToVendorMap[si.Organization]; ok {
		labels["vendor"] = v
	}

	for _, meta := range deviceMeta {
		for k, v := range meta {
			if _, ok := labels[k]; !ok {
				labels[k] = v
			}
		}
	}

	return &vnodes.VirtualNode{
		GUID:     c.Vnode.GUID,
		Hostname: c.Vnode.Hostname,
		Labels:   labels,
	}
}

func (c *Collector) setupProfiles(sysObjectID string) []*ddsnmp.Profile {
	snmpProfiles := ddsnmp.FindProfiles(sysObjectID)
	var profInfo []string

	for _, prof := range snmpProfiles {
		if logger.Level.Enabled(slog.LevelDebug) {
			profInfo = append(profInfo, prof.SourceTree())
		} else {
			name := strings.TrimSuffix(filepath.Base(prof.SourceFile), filepath.Ext(prof.SourceFile))
			profInfo = append(profInfo, name)
		}
	}

	c.Infof("device matched %d profile(s): %s (sysObjectID: %s)", len(snmpProfiles), strings.Join(profInfo, ", "), sysObjectID)

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

var orgToVendorMap = map[string]string{
	"allied networks GmbH":                             "Allied",
	"Allied Data Technologies":                         "Allied",
	"Allied Telesis, Inc.":                             "Allied",
	"American Power Conversion Corp.":                  "APC",
	"Arista Networks, Inc. (formerly 'Arastra, Inc.')": "Arista",
	"Aruba PEC S.p.A.":                                 "Aruba",
	"Aruba S.r.l.":                                     "Aruba",
	"Aruba, a Hewlett Packard Enterprise company":      "Aruba",
	"AVAYA":               "Avaya",
	"Avaya Atlanta Lab":   "Avaya",
	"Avaya Communication": "Avaya",
	"Barracuda Networks AG (previous was 'phion Information Technologies')": "Barracuda",
	"Barracuda Networks, Inc.":                                                "Barracuda",
	"barracuda digitale agentur GmbH":                                         "Barracuda",
	"Blade Network Technologies. Inc.":                                        "BLADE",
	"Brocade Communication Systems, Inc. (formerly 'Foundry Networks, Inc.')": "Brocade",
	"Brocade Communication Systems, Inc. (formerly 'Rhapsody Networks Inc.')": "Brocade",
	"Brocade Communications Systems, Inc.":                                    "Brocade",
	"Brocade Communications Systems, Inc. (formerly 'McDATA Corp.')":          "Brocade",
	"Brocade Communications Systems, Inc. (formerly 'McDATA Corporation')":    "Brocade",
	"Brocade Communications Systems, Inc. (formerly 'McDATA,Inc')":            "Brocade",
	"Brocade Communications Systems, Inc. (formerly 'NuView Inc.')":           "Brocade",
	"CIENA Corporation (formerly 'ONI Systems Corp.')":                        "Ciena",
	"Ciena (formerly 'Akara Inc.')":                                           "Ciena",
	"Ciena Corporation":                                                       "Ciena",
	"Ciena Corporation (formerly 'Catena Networks')":                          "Ciena",
	"Cisco Flex Platform":                                                     "Cisco",
	"Cisco Sera":                                                              "Cisco",
	"Cisco SolutionsLab":                                                      "Cisco",
	"Cisco Sytems, Inc.":                                                      "Cisco",
	"Cisco Systems":                                                           "Cisco",
	"Cisco Systems Inc":                                                       "Cisco",
	"Cisco Systems India Private Limited":                                     "Cisco",
	"Cisco Systems, Inc.":                                                     "Cisco",
	"Cisco Systems, Inc. (formerly 'Arch Rock Corporation')":                  "Cisco",
	"ciscoSystems":                                                            "Cisco",
	"D-Link Systems, Inc.":                                                    "D-Link",
	"Dell Inc.":                                                               "Dell",
	"Ericsson AB":                                                             "Ericsson",
	"Ericsson AB - 4G5G (formerly 'Ellemtel Telecommunication Systems Laboratories')": "Ericsson",
	"Ericsson AB - Packet Core Networks":                                              "Ericsson",
	"Ericsson Ahead Communications Systems GmbH":                                      "Ericsson",
	"Ericsson Communications Ltd.":                                                    "Erricson",
	"Ericsson Denmark A/S, Telebit Division":                                          "Ericsson",
	"Ericsson Inc. (formerly 'BelAir Networks')":                                      "Ericsson",
	"Ericsson Mobile Platforms AB":                                                    "Ericsson",
	"Ericsson Nikola Tesla d.d.":                                                      "Ericsson",
	"Ericsson Research Montreal (LMC)":                                                "Ericsson",
	"Ericsson Wireless LAN Systems":                                                   "Ericsson",
	"ERICSSON FIBER ACCESS":                                                           "Ericsson",
	"Extreme Networks":                                                                "Extreme",
	"Extreme Networks (formerly 'Ipanema Technologies')":                              "Extreme",
	"F5 Labs, Inc.":                                                                   "F5",
	"F5 Networks Inc":                                                                 "F5",
	"Fortinet, Inc.":                                                                  "Fortinet",
	"Fortinet. Inc.":                                                                  "Fortinet",
	"Hewlett-Packard":                                                                 "HP",
	"Hewlett-Packard (Schweiz) GmbH":                                                  "HP",
	"Hewlett-Packard Slovakia":                                                        "HP",
	"HUAWEI Technology Co.,Ltd":                                                       "Huawei",
	"Huawei Symantec Technologies Co.,Ltd":                                            "Huawei",
	"Juniper Financial Corp.":                                                         "Juniper",
	"Juniper Networks, Inc.":                                                          "Juniper",
	"Juniper Networks/Funk Software":                                                  "Juniper",
	"Juniper Networks/Unisphere":                                                      "Juniper",
	"KYOCERA Corporation":                                                             "Kyocera",
	"Kyocera Communication Systems Co.Ltd":                                            "Kyocera",
	"Meraki Networks, Inc.":                                                           "Meraki",
	"Network Appliance Corporation":                                                   "NetApp",
	"Nokia (formerly 'Alcatel-Lucent')":                                               "Nokia",
	"Nokia (formerly 'Novarra, Inc.')":                                                "Nokia",
	"Nokia Distributed Access":                                                        "Nokia",
	"Nokia Networks (formerly 'Nokia Siemens Networks')":                              "Nokia",
	"Nokia Shanghai Bell":                                                             "Nokia",
	"NVIDIA Corporation":                                                              "NVIDIA",
	"PALO ALTO NETWORKS":                                                              "Palo Alto",
	"Palo Alto Research Center, Inc.":                                                 "Palo Alto",
	"Palo Alto Software, Inc.":                                                        "Palo Alto",
	"Ruckus Wireless, Inc.":                                                           "Ruckus",
	"SVTO Hewlett-Packard":                                                            "HP",
	"Tejas Networks":                                                                  "Tejas",
	"TP-Link Systems Inc.":                                                            "TP-Link",
	"Ubiquiti Networks, Inc.":                                                         "Ubiquiti",
	"VMware Inc.":                                                                     "VMware",
	"Yokogawa-Hewlett-Packard":                                                        "HP",
	"ZyXEL Communications Corp.":                                                      "Zyxel",
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
