// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
)

var log = logger.New().With("component", "snmp/sysinfo")

const (
	RootOidMibSystem = "1.3.6.1.2.1.1"
	OidSysDescr      = "1.3.6.1.2.1.1.1.0"
	OidSysObject     = "1.3.6.1.2.1.1.2.0"
	OidSysContact    = "1.3.6.1.2.1.1.4.0"
	OidSysName       = "1.3.6.1.2.1.1.5.0"
	OidSysLocation   = "1.3.6.1.2.1.1.6.0"
)

type SysInfo struct {
	SysObjectID string `json:"-"`

	Descr    string `json:"description"`
	Contact  string `json:"contact"`
	Name     string `json:"name"`
	Location string `json:"location"`

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

	loadOverrides()

	for _, pdu := range pdus {
		oid := strings.TrimPrefix(pdu.Name, ".")

		switch oid {
		case OidSysDescr:
			si.Descr, err = PduToString(pdu)
			si.Descr = valueSanitizer.Replace(si.Descr)
		case OidSysObject:
			var sysObj string
			if sysObj, err = PduToString(pdu); err == nil {
				si.SysObjectID = sysObj
			}
		case OidSysContact:
			si.Contact, err = PduToString(pdu)
			si.Contact = valueSanitizer.Replace(si.Contact)
		case OidSysName:
			si.Name, err = PduToString(pdu)
			si.Name = valueSanitizer.Replace(si.Name)
		case OidSysLocation:
			si.Location, err = PduToString(pdu)
			si.Location = valueSanitizer.Replace(si.Location)
		}
		if err != nil {
			return nil, fmt.Errorf("OID '%s': %v", pdu.Name, err)
		}
	}

	updateMetadata(si)

	return si, nil
}

var valueSanitizer = strings.NewReplacer(
	"'", "",
	"\n", " ",
	"\r", " ",
	"\x00", "",
	"\"", "",
	"`", "",
	"\\", "",
)

// updateMetadata enriches a SysInfo struct with metadata based on its SysObjectID.
// It populates the Organization, Vendor, Category, and Model fields.
func updateMetadata(si *SysInfo) {
	if si == nil || si.SysObjectID == "" {
		return
	}

	rawOrg := lookupEnterpriseNumber(si.SysObjectID)

	var finalCategory string
	var finalModel string
	var finalVendor string

	if overridesData != nil {
		// Apply specific OID overrides for category and model.
		if override, found := overridesData.SysObjectIDs.OIDOverrides[si.SysObjectID]; found {
			if override.Category != "" {
				finalCategory = override.Category
			}
			if override.Model != "" {
				finalModel = override.Model
			}
		}

		// Normalize the category *after* applying the specific override.
		if normalized, found := overridesData.SysObjectIDs.CategoryMap[finalCategory]; found {
			finalCategory = normalized
		}

		// Map the raw organization name to a standardized vendor name.
		if vendor, found := overridesData.EnterpriseNumbers.OrgToVendor[rawOrg]; found {
			finalVendor = vendor
		}
	}

	si.Organization = valueSanitizer.Replace(rawOrg)
	si.Category = finalCategory
	si.Model = finalModel
	si.Vendor = finalVendor
}

var (
	// https://www.iana.org/assignments/enterprise-numbers.txt
	//go:embed "enterprise-numbers.txt"
	enterpriseNumberTxt []byte

	enterpriseNumbers = func() map[string]string {
		if len(enterpriseNumberTxt) == 0 {
			panic("snmp: enterprise-numbers.txt is empty")
		}

		mapping := make(map[string]string, 65000)

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
)

func lookupEnterpriseNumber(sysObject string) string {
	const rootOidIanaPEN = "1.3.6.1.4.1"
	v, ok := strings.CutPrefix(sysObject, rootOidIanaPEN+".") // .1.3.6.1.4.1.14988.1 => 14988.1
	if !ok {
		return ""
	}
	num, _, ok := strings.Cut(v, ".")
	if !ok {
		return ""
	}
	return enterpriseNumbers[num]
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
		return "", fmt.Errorf("unsupported type: '%v'", pdu.Type)
	}
}
