// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/snmp/entnum"
)

const (
	rootOidMibSystem = "1.3.6.1.2.1.1"
	oidSysDescr      = "1.3.6.1.2.1.1.1.0"
	oidSysObject     = "1.3.6.1.2.1.1.2.0"
	oidSysUptime     = "1.3.6.1.2.1.1.3.0"
	oidSysContact    = "1.3.6.1.2.1.1.4.0"
	oidSysName       = "1.3.6.1.2.1.1.5.0"
	oidSysLocation   = "1.3.6.1.2.1.1.6.0"
)

type sysInfo struct {
	descr    string
	contact  string
	name     string
	location string

	organization string
}

func (s *SNMP) getSysInfo() (*sysInfo, error) {
	pdus, err := s.snmpClient.WalkAll(rootOidMibSystem)
	if err != nil {
		return nil, err
	}

	si := &sysInfo{
		organization: "Unknown",
	}

	r := strings.NewReplacer("\n", "\\n", "\r", "\\r")

	for _, pdu := range pdus {
		oid := strings.TrimPrefix(pdu.Name, ".")

		switch oid {
		case oidSysDescr:
			if si.descr, err = pduToString(pdu); err == nil {
				si.descr = r.Replace(si.descr)
			}
		case oidSysObject:
			var sysObj string
			if sysObj, err = pduToString(pdu); err == nil {
				org := entnum.LookupBySysObject(sysObj)
				s.Debugf("device sysObject '%s', organization '%s'", sysObj, org)
				if org != "" {
					si.organization = org
				}
			}
		case oidSysContact:
			si.contact, err = pduToString(pdu)
		case oidSysName:
			si.name, err = pduToString(pdu)
		case oidSysLocation:
			si.location, err = pduToString(pdu)
		}
		if err != nil {
			return nil, fmt.Errorf("OID '%s': %v", pdu.Name, err)
		}
	}

	if si.name == "" {
		return nil, errors.New("no system name")
	}

	return si, nil
}

func (s *SNMP) collectSysUptime(mx map[string]int64) error {
	resp, err := s.snmpClient.Get([]string{oidSysUptime})
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
