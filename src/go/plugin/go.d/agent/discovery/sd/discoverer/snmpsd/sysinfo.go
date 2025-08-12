// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	RootOidMibSystem = "1.3.6.1.2.1.1"
	OidSysDescr      = "1.3.6.1.2.1.1.1.0"
	OidSysObject     = "1.3.6.1.2.1.1.2.0"
	OidSysUptime     = "1.3.6.1.2.1.1.3.0"
	OidSysContact    = "1.3.6.1.2.1.1.4.0"
	OidSysName       = "1.3.6.1.2.1.1.5.0"
	OidSysLocation   = "1.3.6.1.2.1.1.6.0"
)

var (
	// https://www.iana.org/assignments/enterprise-numbers.txt
	//go:embed "enterprise-numbers.txt"
	enterpriseNumberTxt []byte
	// https://github.com/parthiganesh/snmp-sysObjectID
	//go:embed "sysObjectIDs.json"
	sysObjectIDsJson []byte
)

type SysInfo struct {
	Descr       string `json:"description"`
	Contact     string `json:"contact"`
	Name        string `json:"name"`
	Location    string `json:"location"`
	SysObjectID string `json:"-"`

	Organization string `json:"organization"`
	Vendor       string `json:"vendor"`
	Category     string `json:"category"`
	Model        string `json:"model"`
}

func GetSysInfo(client gosnmp.Handler) (*SysInfo, error) {
	pdus, err := client.WalkAll(RootOidMibSystem)
	if err != nil {
		return nil, err
	}

	si := &SysInfo{
		Name:         "unknown",
		Organization: "Unknown",
	}

	r := strings.NewReplacer(
		"'", "",
		"\n", " ",
		"\r", " ",
		"\x00", "",
		"\"", "",
		"`", "",
		"\\", "",
	)

	for _, pdu := range pdus {
		oid := strings.TrimPrefix(pdu.Name, ".")

		switch oid {
		case OidSysDescr:
			si.Descr, err = PduToString(pdu)
			si.Descr = r.Replace(si.Descr)
		case OidSysObject:
			var sysObj string
			if sysObj, err = PduToString(pdu); err == nil {
				si.SysObjectID = sysObj
			}
		case OidSysContact:
			si.Contact, err = PduToString(pdu)
			si.Contact = r.Replace(si.Contact)
		case OidSysName:
			si.Name, err = PduToString(pdu)
			si.Name = r.Replace(si.Name)
		case OidSysLocation:
			si.Location, err = PduToString(pdu)
			si.Location = r.Replace(si.Location)
		}
		if err != nil {
			return nil, fmt.Errorf("OID '%s': %v", pdu.Name, err)
		}
	}

	if si.SysObjectID != "" {
		si.Organization = lookupEnterpriseNumber(si.SysObjectID)
		if v, ok := entNumbersOrgToVendorMap[si.Organization]; ok {
			si.Vendor = v
		}
		si.Organization = r.Replace(si.Organization)

		if meta, ok := lookupDeviceMeta(si.SysObjectID); ok {
			si.Category = meta.Category
			si.Model = meta.Model
		}
	}

	return si, nil
}

type sysObjectIDInfo struct {
	Category string
	Model    string
}

func lookupDeviceMeta(sysObject string) (sysObjectIDInfo, bool) {
	v, ok := sysObjectIDs[sysObject]
	return v, ok
}

var sysObjectIDs = func() map[string]sysObjectIDInfo {
	if len(sysObjectIDsJson) == 0 {
		panic("snmp: sysObjectIDs.json is empty")
	}

	var ids = map[string]sysObjectIDInfo{}
	if err := json.Unmarshal(sysObjectIDsJson, &ids); err != nil {
		panic(fmt.Sprintf("snmp: invalid sysObjectIDs.json: %v", err))
	}
	return ids
}()

func lookupEnterpriseNumber(sysObject string) string {
	return entNumbers[extractEntNumber(sysObject)]
}

var entNumbers = func() map[string]string {
	if len(enterpriseNumberTxt) == 0 {
		panic("snmp: enterprise-numbers.txt is empty")
	}

	mapping := make(map[string]string, 65000)

	vr := strings.NewReplacer("\"", "", "`", "", "\\", "")
	var id string

	sc := bufio.NewScanner(bytes.NewReader(enterpriseNumberTxt))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if _, err := strconv.Atoi(line); err == nil {
			if id == "" {
				id = line
				if _, ok := mapping[id]; ok {
					panic("snmp: duplicate entry number: " + line)
				}
			}
			continue
		}
		if id != "" {
			line = vr.Replace(line)
			if line == "---none---" || line == "Reserved" {
				id = ""
				continue
			}
			mapping[id] = line
			id = ""
		}
	}

	if len(mapping) == 0 {
		panic("snmp: enterprise-numbers mapping is empty after reading enterprise-numbers.txt")
	}

	return mapping
}()

func extractEntNumber(sysObject string) string {
	const rootOidIanaPEN = "1.3.6.1.4.1"

	// .1.3.6.1.4.1.14988.1 => 14988

	sysObject = strings.TrimPrefix(sysObject, ".")

	s := strings.TrimPrefix(sysObject, rootOidIanaPEN+".")

	num, _, ok := strings.Cut(s, ".")
	if !ok {
		return ""
	}

	return num
}

func PduToString(pdu gosnmp.SnmpPDU) (string, error) {
	switch pdu.Type {
	case gosnmp.OctetString:
		// TODO: this isn't reliable (e.g. physAddress we need hex.EncodeToString())
		bs, ok := pdu.Value.([]byte)
		if !ok {
			return "", fmt.Errorf("OctetString is not a []byte but %T", pdu.Value)
		}
		return strings.ToValidUTF8(string(bs), "�"), nil
	case gosnmp.Counter32, gosnmp.Counter64, gosnmp.Integer, gosnmp.Gauge32:
		return gosnmp.ToBigInt(pdu.Value).String(), nil
	case gosnmp.ObjectIdentifier:
		v, ok := pdu.Value.(string)
		if !ok {
			return "", fmt.Errorf("ObjectIdentifier is not a string but %T", pdu.Value)
		}
		return strings.TrimPrefix(v, "."), nil
	default:
		return "", fmt.Errorf("unussported type: '%v'", pdu.Type)
	}
}

var entNumbersOrgToVendorMap = map[string]string{
	"Alcatel-Lucent TMC (formerly 'Alcatel SOC')":      "Alcatel-Lucent",
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
	"Blade Network Technologies, Inc.":                                        "IBM",
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
	"Citrix Systems Inc.":                                                     "Citrix",
	"Dialogic Corporation":                                                    "Dialogic",
	"D-Link Systems, Inc.":                                                    "D-Link",
	"Dell Inc.":                                                               "Dell",
	"Eaton Energy Automation Solutions (EAS) Division":                        "Eaton",
	"EATON Wireless":                                                          "Eaton",
	"Ericsson AB":                                                             "Ericsson",
	"Ericsson AB - 4G5G (formerly 'Ellemtel Telecommunication Systems Laboratories')": "Ericsson",
	"Ericsson AB - Packet Core Networks":                                              "Ericsson",
	"Ericsson Ahead Communications Systems GmbH":                                      "Ericsson",
	"Ericsson Communications Ltd.":                                                    "Ericsson",
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
	"Gigamon Systems LLC":                                                             "Gigamon",
	"Hewlett-Packard":                                                                 "HP",
	"Hewlett-Packard (Schweiz) GmbH":                                                  "HP",
	"Hewlett-Packard Slovakia":                                                        "HP",
	"Hewlett Packard Enterprise":                                                      "HPE",
	"HUAWEI Technology Co.,Ltd":                                                       "Huawei",
	"Huawei Symantec Technologies Co.,Ltd":                                            "Huawei",
	"Infinera Corp.":                                                                  "Infinera",
	"InfoBlox Inc.":                                                                   "Infoblox",
	"Infoblox, WinConnect (formerly 'Ipanto')":                                        "Infoblox",
	"Juniper Financial Corp.":                                                         "Juniper",
	"Juniper Networks, Inc.":                                                          "Juniper",
	"Juniper Networks/Funk Software":                                                  "Juniper",
	"Juniper Networks/Unisphere":                                                      "Juniper",
	"KYOCERA Corporation":                                                             "Kyocera",
	"Kyocera Communication Systems Co.Ltd":                                            "Kyocera",
	"McAFee Associates Inc.":                                                          "McAfee",
	"McAfee (formerly 'Secure Computing Corporation')":                                "McAfee",
	"McAfee Inc. (formerly 'Network Associates, Inc.')":                               "McAfee",
	"McAfee, Inc.  (formerly 'Securify, Inc.')":                                       "McAfee",
	"McAfee Inc. (formerly 'Reconnex Corporation')":                                   "McAfee",
	"McAfee, LLC":                                                                     "McAfee",
	"Meraki Networks, Inc.":                                                           "Meraki",
	"Nasuni Corporation":                                                              "Nasuni",
	"NEC Corporation":                                                                 "NEC",
	"NEC Eluminant Technologies, Inc.":                                                "NEC",
	"NEC Platforms, Ltd.":                                                             "NEC",
	"NEC Telenetworx,Ltd":                                                             "NEC",
	"NEC COMPUTERS INTERNATIONAL B.V.":                                                "NEC",
	"NEC informatec systems,ltd.":                                                     "NEC",
	"NEC Electronics Corporation":                                                     "NEC",
	"NEC Unified Solutions":                                                           "NEC",
	"NEC Enterprise Communication Technologies":                                       "NEC",
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
	"SINETICA":                                                                        "Panduit",
	"Sophos Plc":                                                                      "Sophos",
	"SVTO Hewlett-Packard":                                                            "HP",
	"Synology Inc.":                                                                   "Synology",
	"Tejas Networks":                                                                  "Tejas",
	"TP-Link Systems Inc.":                                                            "TP-Link",
	"Ubiquiti Networks, Inc.":                                                         "Ubiquiti",
	"Velocloud Networks, Inc.":                                                        "VeloCloud",
	"Vertiv (formerly 'Emerson Computer Power')":                                      "Vertiv",
	"Vertiv (formerly 'Emerson Energy Systems')":                                      "Vertiv",
	"Vertiv Tech Co.,Ltd. (formerly 'Emerson Network Power Co.,Ltd.')":                "Vertiv",
	"Vertiv (formerly 'Geist Manufacturing, Inc')":                                    "Vertiv",
	"Vertiv Co":                                                                       "Vertiv",
	"VMware Inc.":                                                                     "VMware",
	"WatchGuard Technologies Inc.":                                                    "WatchGuard",
	"Western Digital Corporation":                                                     "Western Digital",
	"Yokogawa-Hewlett-Packard":                                                        "HP",
	"Zebra Technologies Corporation":                                                  "Zebra",
	"ZyXEL Communications Corp.":                                                      "Zyxel",
}
