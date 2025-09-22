// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

const oidSysUptime = "1.3.6.1.2.1.1.3.0"

func (c *Collector) collect() (map[string]int64, error) {
	if c.snmpClient == nil {
		snmpClient, err := c.initAndConnectSNMPClient()
		if err != nil {
			return nil, err
		}
		c.snmpClient = snmpClient
		if c.ddSnmpColl != nil {
			c.ddSnmpColl.SetSNMPClient(snmpClient)
		}
	}

	if c.sysInfo == nil {
		si, err := snmputils.GetSysInfo(c.snmpClient)
		if err != nil {
			return nil, err
		}

		if c.enableProfiles {
			c.snmpProfiles = c.setupProfiles(si.SysObjectID)
		}

		if c.ddSnmpColl == nil {
			c.ddSnmpColl = ddsnmpcollector.New(c.snmpClient, c.snmpProfiles, c.Logger, si.SysObjectID)
		}

		if c.CreateVnode {
			deviceMeta, err := c.ddSnmpColl.CollectDeviceMetadata()
			if err != nil {
				return nil, err
			}
			c.vnode = c.setupVnode(si, deviceMeta)
		}

		c.sysInfo = si
	}

	mx := make(map[string]int64)

	if err := c.collectProfiles(mx); err != nil {
		c.Infof("failed to collect profiles: %v", err)
	}

	if !c.DisableLegacyCollection && len(c.customOids) > 0 {
		if err := c.collectOIDs(mx); err != nil {
			return nil, err
		}
	}

	return mx, nil
}

func (c *Collector) collectSysUptime(mx map[string]int64) error {
	resp, err := c.snmpClient.Get([]string{oidSysUptime})
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

func (c *Collector) setupVnode(si *snmputils.SysInfo, deviceMeta map[string]ddsnmp.MetaTag) *vnodes.VirtualNode {
	if c.Vnode.GUID == "" {
		c.Vnode.GUID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte(c.Hostname)).String()
	}

	hostnames := []string{
		c.Vnode.Hostname,
		si.Name,
		"snmp-device",
	}
	i := slices.IndexFunc(hostnames, func(s string) bool { return s != "" })
	c.Vnode.Hostname = hostnames[i]

	labels := map[string]string{
		"_vnode_type":           "snmp",
		"_net_default_iface_ip": c.Hostname,
		"address":               c.Hostname,
	}

	if c.UpdateEvery >= 1 && c.VnodeDeviceDownThreshold >= 1 {
		// Add 2 seconds buffer to account for collection/transmission delays
		v := c.VnodeDeviceDownThreshold*c.UpdateEvery + 2
		labels["_node_stale_after_seconds"] = strconv.Itoa(v)
	}

	labels["sys_object_id"] = si.SysObjectID
	labels["name"] = si.Name
	labels["description"] = si.Descr
	labels["contact"] = si.Contact
	labels["location"] = si.Location
	if si.Vendor != "" {
		labels["vendor"] = si.Vendor
	} else if si.Organization != "" {
		labels["vendor"] = si.Organization
	}
	if si.Category != "" {
		labels["type"] = si.Category
	}
	if si.Model != "" {
		labels["model"] = si.Model
	}

	for k, val := range deviceMeta {
		if v, ok := labels[k]; !ok || v == "" || val.IsExactMatch {
			labels[k] = val.Value
		}
	}

	maps.Copy(labels, c.Vnode.Labels)

	return &vnodes.VirtualNode{
		GUID:     c.Vnode.GUID,
		Hostname: c.Vnode.Hostname,
		Labels:   labels,
	}
}

func (c *Collector) setupProfiles(sysObjectID string) []*ddsnmp.Profile {
	snmpProfiles := ddsnmp.FindProfiles(sysObjectID, c.ManualProfiles)
	var profInfo []string

	for _, prof := range snmpProfiles {
		if logger.Level.Enabled(slog.LevelDebug) {
			profInfo = append(profInfo, prof.SourceTree())
		} else {
			name := strings.TrimSuffix(filepath.Base(prof.SourceFile), filepath.Ext(prof.SourceFile))
			profInfo = append(profInfo, name)
		}
	}

	c.Infof("device matched %d profile(s): %s (sysObjectID: '%s')", len(snmpProfiles), strings.Join(profInfo, ", "), sysObjectID)

	return snmpProfiles
}

func (c *Collector) initAndConnectSNMPClient() (gosnmp.Handler, error) {
	snmpClient, err := c.initSNMPClient()
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	if err := snmpClient.Connect(); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if c.adjMaxRepetitions != 0 {
		snmpClient.SetMaxRepetitions(c.adjMaxRepetitions)
	} else {
		ok, err := c.adjustMaxRepetitions(snmpClient)
		if err != nil {
			return nil, fmt.Errorf("re-adjust max repetitions SNMP client: %w", err)
		}
		if !ok {
			c.Warningf("SNMP bulk walk disabled: table metrics collection unavailable (device may not support GETBULK or max-repetitions adjustment failed)")
		}
		c.adjMaxRepetitions = snmpClient.MaxRepetitions()
		c.snmpBulkWalkOk = ok
	}

	return snmpClient, nil
}
func (c *Collector) adjustMaxRepetitions(snmpClient gosnmp.Handler) (bool, error) {
	orig := c.Config.Options.MaxRepetitions
	maxReps := c.Config.Options.MaxRepetitions
	attempts := 0
	const maxAttempts = 20 // Prevent infinite loops

	for maxReps > 0 && attempts < maxAttempts {
		attempts++

		v, err := walkAll(snmpClient, snmputils.RootOidMibSystem)
		if err != nil {
			return false, err
		}

		if len(v) > 0 {
			//c.Config.Options.MaxRepetitions = maxReps
			if orig != maxReps {
				c.Infof("adjusted max_repetitions: %d → %d (took %d attempts)", orig, maxReps, attempts)
			}
			return true, nil
		}

		// Adaptive decrease strategy
		prevMaxReps := maxReps
		if maxReps > 50 {
			maxReps -= 10
		} else if maxReps > 10 {
			maxReps -= 5
		} else if maxReps > 5 {
			maxReps -= 2
		} else {
			maxReps--
		}

		maxReps = max(0, maxReps) // Ensure non-negative

		c.Debugf("max_repetitions=%d returned no data, trying %d", prevMaxReps, maxReps)
		snmpClient.SetMaxRepetitions(uint32(maxReps))
	}

	// Restore original value since nothing worked
	snmpClient.SetMaxRepetitions(uint32(orig))
	c.Debugf("unable to find working max_repetitions value after %d attempts", attempts)
	return false, nil
}

func walkAll(snmpClient gosnmp.Handler, rootOid string) ([]gosnmp.SnmpPDU, error) {
	if snmpClient.Version() == gosnmp.Version1 {
		return snmpClient.WalkAll(rootOid)
	}
	return snmpClient.BulkWalkAll(rootOid)
}

func pduToInt(pdu gosnmp.SnmpPDU) (int64, error) {
	switch pdu.Type {
	case gosnmp.Counter32, gosnmp.Counter64, gosnmp.Integer, gosnmp.Gauge32, gosnmp.TimeTicks:
		return gosnmp.ToBigInt(pdu.Value).Int64(), nil
	default:
		return 0, fmt.Errorf("unsupported type: '%v'", pdu.Type)
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
