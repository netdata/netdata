// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"

	"github.com/gosnmp/gosnmp"
)

const (
	oidSysUptime         = "1.3.6.1.2.1.1.3.0"
	oidSysName           = "1.3.6.1.2.1.1.5.0"
	rootOidIfMibIfTable  = "1.3.6.1.2.1.2.2"
	rootOidIfMibIfXTable = "1.3.6.1.2.1.31.1.1"
)

func (s *SNMP) collect() (map[string]int64, error) {
	if s.sysName == "" {
		sysName, err := s.getSysName()
		if err != nil {
			return nil, err
		}
		s.sysName = sysName
		s.addSysUptimeChart()
	}

	mx := make(map[string]int64)

	if err := s.collectSysUptime(mx); err != nil {
		return nil, err
	}

	if s.collectIfMib {
		if err := s.collectNetworkInterfaces(mx); err != nil {
			return nil, err
		}
	}

	if len(s.oids) > 0 {
		if err := s.collectOIDs(mx); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (s *SNMP) getSysName() (string, error) {
	resp, err := s.snmpClient.Get([]string{oidSysName})
	if err != nil {
		return "", err
	}
	if len(resp.Variables) == 0 {
		return "", errors.New("no system name")
	}
	return pduToString(resp.Variables[0])
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

func (s *SNMP) collectNetworkInterfaces(mx map[string]int64) error {
	if s.checkMaxReps {
		ok, err := s.adjustMaxRepetitions()
		if err != nil {
			return err
		}

		s.checkMaxReps = false

		if !ok {
			s.collectIfMib = false

			if len(s.oids) == 0 {
				return errors.New("no IF-MIB data returned")
			}

			s.Warning("no IF-MIB data returned")
			return nil
		}
	}

	ifMibTable, err := s.walkAll(rootOidIfMibIfTable)
	if err != nil {
		return err
	}

	ifMibXTable, err := s.walkAll(rootOidIfMibIfXTable)
	if err != nil {
		return err
	}

	if len(ifMibTable) == 0 && len(ifMibXTable) == 0 {
		s.Warning("no IF-MIB data returned")
		s.collectIfMib = false
		return nil
	}

	for _, i := range s.netInterfaces {
		i.updated = false
	}

	pdus := make([]gosnmp.SnmpPDU, 0, len(ifMibTable)+len(ifMibXTable))
	pdus = append(pdus, ifMibTable...)
	pdus = append(pdus, ifMibXTable...)

	for _, pdu := range pdus {
		i := strings.LastIndexByte(pdu.Name, '.')
		if i == -1 {
			continue
		}

		idx := pdu.Name[i+1:]
		oid := strings.TrimPrefix(pdu.Name[:i], ".")

		iface, ok := s.netInterfaces[idx]
		if !ok {
			iface = &netInterface{idx: idx}
		}

		switch oid {
		case oidIfIndex:
			iface.ifIndex, err = pduToInt(pdu)
		case oidIfDescr:
			iface.ifDescr, err = pduToString(pdu)
		case oidIfType:
			iface.ifType, err = pduToInt(pdu)
		case oidIfMtu:
			iface.ifMtu, err = pduToInt(pdu)
		case oidIfSpeed:
			iface.ifSpeed, err = pduToInt(pdu)
		case oidIfAdminStatus:
			iface.ifAdminStatus, err = pduToInt(pdu)
		case oidIfOperStatus:
			iface.ifOperStatus, err = pduToInt(pdu)
		case oidIfInOctets:
			iface.ifInOctets, err = pduToInt(pdu)
		case oidIfInUcastPkts:
			iface.ifInUcastPkts, err = pduToInt(pdu)
		case oidIfInNUcastPkts:
			iface.ifInNUcastPkts, err = pduToInt(pdu)
		case oidIfInDiscards:
			iface.ifInDiscards, err = pduToInt(pdu)
		case oidIfInErrors:
			iface.ifInErrors, err = pduToInt(pdu)
		case oidIfInUnknownProtos:
			iface.ifInUnknownProtos, err = pduToInt(pdu)
		case oidIfOutOctets:
			iface.ifOutOctets, err = pduToInt(pdu)
		case oidIfOutUcastPkts:
			iface.ifOutUcastPkts, err = pduToInt(pdu)
		case oidIfOutNUcastPkts:
			iface.ifOutNUcastPkts, err = pduToInt(pdu)
		case oidIfOutDiscards:
			iface.ifOutDiscards, err = pduToInt(pdu)
		case oidIfOutErrors:
			iface.ifOutErrors, err = pduToInt(pdu)
		case oidIfName:
			iface.ifName, err = pduToString(pdu)
		case oidIfInMulticastPkts:
			iface.ifInMulticastPkts, err = pduToInt(pdu)
		case oidIfInBroadcastPkts:
			iface.ifInBroadcastPkts, err = pduToInt(pdu)
		case oidIfOutMulticastPkts:
			iface.ifOutMulticastPkts, err = pduToInt(pdu)
		case oidIfOutBroadcastPkts:
			iface.ifOutBroadcastPkts, err = pduToInt(pdu)
		case oidIfHCInOctets:
			iface.ifHCInOctets, err = pduToInt(pdu)
		case oidIfHCInUcastPkts:
			iface.ifHCInUcastPkts, err = pduToInt(pdu)
		case oidIfHCInMulticastPkts:
			iface.ifHCInMulticastPkts, err = pduToInt(pdu)
		case oidIfHCInBroadcastPkts:
			iface.ifHCInBroadcastPkts, err = pduToInt(pdu)
		case oidIfHCOutOctets:
			iface.ifHCOutOctets, err = pduToInt(pdu)
		case oidIfHCOutUcastPkts:
			iface.ifHCOutUcastPkts, err = pduToInt(pdu)
		case oidIfHCOutMulticastPkts:
			iface.ifHCOutMulticastPkts, err = pduToInt(pdu)
		case oidIfHCOutBroadcastPkts:
			iface.ifHCOutMulticastPkts, err = pduToInt(pdu)
		case oidIfHighSpeed:
			iface.ifHighSpeed, err = pduToInt(pdu)
		case oidIfAlias:
			iface.ifAlias, err = pduToString(pdu)
		default:
			continue
		}

		if err != nil {
			return fmt.Errorf("OID '%s': %v", pdu.Name, err)
		}

		s.netInterfaces[idx] = iface
		iface.updated = true
	}

	for _, iface := range s.netInterfaces {
		if iface.ifName == "" {
			continue
		}

		typeStr := ifTypeMapping[iface.ifType]
		if s.netIfaceFilterByName.MatchString(iface.ifName) || s.netIfaceFilterByType.MatchString(typeStr) {
			continue
		}

		if !iface.updated {
			delete(s.netInterfaces, iface.idx)
			if iface.hasCharts {
				s.removeNetIfaceCharts(iface)
			}
			continue
		}
		if !iface.hasCharts {
			iface.hasCharts = true
			s.addNetIfaceCharts(iface)
		}

		px := fmt.Sprintf("net_iface_%s_", iface.ifName)
		mx[px+"traffic_in"] = iface.ifHCInOctets * 8 / 1000   // kilobits
		mx[px+"traffic_out"] = iface.ifHCOutOctets * 8 / 1000 // kilobits
		mx[px+"ucast_in"] = iface.ifHCInUcastPkts
		mx[px+"ucast_out"] = iface.ifHCOutUcastPkts
		mx[px+"mcast_in"] = iface.ifHCInMulticastPkts
		mx[px+"mcast_out"] = iface.ifHCOutMulticastPkts
		mx[px+"bcast_in"] = iface.ifHCInBroadcastPkts
		mx[px+"bcast_out"] = iface.ifHCOutBroadcastPkts
		mx[px+"errors_in"] = iface.ifInErrors
		mx[px+"errors_out"] = iface.ifOutErrors
		mx[px+"discards_in"] = iface.ifInDiscards
		mx[px+"discards_out"] = iface.ifOutDiscards

		for _, v := range ifAdminStatusMapping {
			mx[px+"admin_status_"+v] = 0
		}
		mx[px+"admin_status_"+ifAdminStatusMapping[iface.ifAdminStatus]] = 1

		for _, v := range ifOperStatusMapping {
			mx[px+"oper_status_"+v] = 0
		}
		mx[px+"oper_status_"+ifOperStatusMapping[iface.ifOperStatus]] = 1
	}

	if logger.Level.Enabled(slog.LevelDebug) {
		ifaces := make([]*netInterface, 0, len(s.netInterfaces))
		for _, nif := range s.netInterfaces {
			ifaces = append(ifaces, nif)
		}
		sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].ifIndex < ifaces[j].ifIndex })
		for _, iface := range ifaces {
			s.Debugf("found %s", iface)
		}
	}

	return nil
}

func (s *SNMP) adjustMaxRepetitions() (bool, error) {
	orig := s.Config.Options.MaxRepetitions
	maxReps := s.Config.Options.MaxRepetitions

	for {
		v, err := s.walkAll(oidIfIndex)
		if err != nil {
			return false, err
		}

		if len(v) > 0 {
			if orig != maxReps {
				s.Infof("changed 'max_repetitions' %d => %d", orig, maxReps)
			}
			return true, nil
		}

		if maxReps > 5 {
			maxReps = max(5, maxReps-5)
		} else {
			maxReps--
		}

		if maxReps <= 0 {
			return false, nil
		}

		s.Debugf("no IF-MIB data returned, trying to decrese 'max_repetitions' to %d", maxReps)
		s.snmpClient.SetMaxRepetitions(uint32(maxReps))
	}
}

func (s *SNMP) walkAll(rootOid string) ([]gosnmp.SnmpPDU, error) {
	if s.snmpClient.Version() == gosnmp.Version1 {
		return s.snmpClient.WalkAll(rootOid)
	}
	return s.snmpClient.BulkWalkAll(rootOid)
}

func pduToString(pdu gosnmp.SnmpPDU) (string, error) {
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
	default:
		return "", fmt.Errorf("unussported type: '%v'", pdu.Type)
	}
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

func (s *SNMP) collectOIDs(mx map[string]int64) error {
	for i, end := 0, 0; i < len(s.oids); i += s.Options.MaxOIDs {
		if end = i + s.Options.MaxOIDs; end > len(s.oids) {
			end = len(s.oids)
		}

		oids := s.oids[i:end]
		resp, err := s.snmpClient.Get(oids)
		if err != nil {
			s.Errorf("cannot get SNMP data: %v", err)
			return err
		}

		for i, oid := range oids {
			if i >= len(resp.Variables) {
				continue
			}

			switch v := resp.Variables[i]; v.Type {
			case gosnmp.Boolean,
				gosnmp.Counter32,
				gosnmp.Counter64,
				gosnmp.Gauge32,
				gosnmp.TimeTicks,
				gosnmp.Uinteger32,
				gosnmp.OpaqueFloat,
				gosnmp.OpaqueDouble,
				gosnmp.Integer:
				mx[oid] = gosnmp.ToBigInt(v.Value).Int64()
			default:
				s.Debugf("skipping OID '%s' (unsupported type '%s')", oid, v.Type)
			}
		}
	}

	return nil
}
