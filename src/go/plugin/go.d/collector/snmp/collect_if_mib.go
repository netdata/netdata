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
	rootOidIfMibIfTable  = "1.3.6.1.2.1.2.2"
	rootOidIfMibIfXTable = "1.3.6.1.2.1.31.1.1"
)

func (c *Collector) collectNetworkInterfaces(mx map[string]int64) error {
	if c.checkMaxReps {
		ok, err := c.adjustMaxRepetitions()
		if err != nil {
			return err
		}

		c.checkMaxReps = false

		if !ok {
			c.collectIfMib = false

			if len(c.customOids) == 0 {
				return errors.New("no IF-MIB data returned")
			}

			c.Warning("no IF-MIB data returned")
			return nil
		}
	}

	ifMibTable, err := c.walkAll(rootOidIfMibIfTable)
	if err != nil {
		return err
	}

	ifMibXTable, err := c.walkAll(rootOidIfMibIfXTable)
	if err != nil {
		return err
	}

	if len(ifMibTable) == 0 && len(ifMibXTable) == 0 {
		c.Warning("no IF-MIB data returned")
		c.collectIfMib = false
		return nil
	}

	for _, i := range c.netInterfaces {
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

		iface, ok := c.netInterfaces[idx]
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

		c.netInterfaces[idx] = iface
		iface.updated = true
	}

	for _, iface := range c.netInterfaces {
		if iface.ifName == "" {
			continue
		}

		typeStr := ifTypeMapping[iface.ifType]
		if c.netIfaceFilterByName.MatchString(iface.ifName) || c.netIfaceFilterByType.MatchString(typeStr) {
			continue
		}

		if !iface.updated {
			delete(c.netInterfaces, iface.idx)
			if iface.hasCharts {
				c.removeNetIfaceCharts(iface)
			}
			continue
		}
		if !iface.hasCharts {
			iface.hasCharts = true
			c.addNetIfaceCharts(iface)
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
		ifaces := make([]*netInterface, 0, len(c.netInterfaces))
		for _, nif := range c.netInterfaces {
			ifaces = append(ifaces, nif)
		}
		sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].ifIndex < ifaces[j].ifIndex })
		for _, iface := range ifaces {
			c.Debugf("found %s", iface)
		}
	}

	return nil
}

func (c *Collector) adjustMaxRepetitions() (bool, error) {
	orig := c.Config.Options.MaxRepetitions
	maxReps := c.Config.Options.MaxRepetitions

	for {
		v, err := c.walkAll(oidIfIndex)
		if err != nil {
			return false, err
		}

		if len(v) > 0 {
			if orig != maxReps {
				c.Infof("changed 'max_repetitions' %d => %d", orig, maxReps)
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

		c.Debugf("no IF-MIB data returned, trying to decrese 'max_repetitions' to %d", maxReps)
		c.snmpClient.SetMaxRepetitions(uint32(maxReps))
	}
}
