// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"fmt"
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

type SysInfo struct {
	Descr        string `json:"description"`
	Contact      string `json:"contact"`
	Name         string `json:"name"`
	Location     string `json:"location"`
	Organization string `json:"organization"`
	SysObjectID  string `json:"-"`
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

	r := strings.NewReplacer("'", "", "\n", " ", "\r", " ", "\x00", "")

	for _, pdu := range pdus {
		oid := strings.TrimPrefix(pdu.Name, ".")

		switch oid {
		case OidSysDescr:
			si.Descr, err = PduToString(pdu)
			si.Descr = r.Replace(si.Descr)
		case OidSysObject:
			var sysObj string
			if sysObj, err = PduToString(pdu); err == nil {
				si.Organization = LookupBySysObject(sysObj)
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

	return si, nil
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
