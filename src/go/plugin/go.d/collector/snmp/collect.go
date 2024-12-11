// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.sysInfo == nil {
		si, err := c.getSysInfo()
		if err != nil {
			return nil, err
		}

		c.sysInfo = si
		c.addSysUptimeChart()

		if c.CreateVnode {
			c.vnode = c.setupVnode(si)
		}
	}

	mx := make(map[string]int64)

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

	return mx, nil
}

func (c *Collector) walkAll(rootOid string) ([]gosnmp.SnmpPDU, error) {
	if c.snmpClient.Version() == gosnmp.Version1 {
		return c.snmpClient.WalkAll(rootOid)
	}
	return c.snmpClient.BulkWalkAll(rootOid)
}

func (c *Collector) setupVnode(si *sysInfo) *vnodes.VirtualNode {
	if c.Vnode.GUID == "" {
		c.Vnode.GUID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(c.Hostname)).String()
	}

	hostnames := []string{c.Vnode.Hostname, si.name, "snmp-device"}
	i := slices.IndexFunc(hostnames, func(s string) bool { return s != "" })

	c.Vnode.Hostname = fmt.Sprintf("%s(%s)", hostnames[i], c.Hostname)

	labels := make(map[string]string)

	for k, v := range c.Vnode.Labels {
		labels[k] = v
	}
	if si.descr != "" {
		labels["sysDescr"] = si.descr
	}
	if si.contact != "" {
		labels["sysContact"] = si.contact
	}
	if si.location != "" {
		labels["sysLocation"] = si.location
	}
	// FIXME: vendor should be obtained from sysDescr, org should be used as a fallback
	labels["vendor"] = si.organization

	return &vnodes.VirtualNode{
		GUID:     c.Vnode.GUID,
		Hostname: c.Vnode.Hostname,
		Labels:   labels,
	}
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
