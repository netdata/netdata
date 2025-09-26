// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	"golang.org/x/sync/errgroup"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func (c *Collector) collect() (map[string]int64, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	mx, err := c.collectMetrics()
	if err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectMetrics() (map[string]int64, error) {
	var (
		snmpMx map[string]int64
		pingMx map[string]int64
	)

	ctx := context.Background()

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		m := make(map[string]int64)
		if err := c.collectSNMP(m); err != nil {
			return err
		}
		snmpMx = m
		return nil
	})

	if c.Ping.Enabled && c.prober != nil {
		g.Go(func() error {
			m := make(map[string]int64)
			if err := c.collectPing(m); err != nil {
				c.Errorf("ping: %v", err)
				if isPingUnrecoverableError(err) {
					c.prober = nil
				}
				return nil
			}
			pingMx = m
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	mx := make(map[string]int64, len(snmpMx)+len(pingMx))

	maps.Copy(mx, snmpMx)
	maps.Copy(mx, pingMx)

	return mx, nil
}

func (c *Collector) ensureInitialized() error {
	if c.snmpClient == nil {
		snmpClient, err := c.initAndConnectSNMPClient()
		if err != nil {
			return err
		}
		c.snmpClient = snmpClient
		if c.ddSnmpColl != nil {
			c.ddSnmpColl.SetSNMPClient(snmpClient)
		}
	}

	if c.sysInfo == nil {
		si, err := snmputils.GetSysInfo(c.snmpClient)
		if err != nil {
			return err
		}

		if c.enableProfiles {
			c.snmpProfiles = c.setupProfiles(si)
		}

		if c.ddSnmpColl == nil {
			c.ddSnmpColl = ddsnmpcollector.New(c.snmpClient, c.snmpProfiles, c.Logger, si.SysObjectID)
		}

		if c.CreateVnode {
			deviceMeta, err := c.ddSnmpColl.CollectDeviceMetadata()
			if err != nil {
				return err
			}
			c.vnode = c.setupVnode(si, deviceMeta)
		}

		c.sysInfo = si
	}

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

func (c *Collector) setupProfiles(si *snmputils.SysInfo) []*ddsnmp.Profile {
	snmpProfiles := ddsnmp.FindProfiles(si.SysObjectID, si.Descr, c.ManualProfiles)
	var profInfo []string

	for _, prof := range snmpProfiles {
		if logger.Level.Enabled(slog.LevelDebug) {
			profInfo = append(profInfo, prof.SourceTree())
		} else {
			name := strings.TrimSuffix(filepath.Base(prof.SourceFile), filepath.Ext(prof.SourceFile))
			profInfo = append(profInfo, name)
		}
	}

	c.Infof("device matched %d profile(s): %s (sysObjectID: '%s')",
		len(snmpProfiles), strings.Join(profInfo, ", "), si.SysObjectID)

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
			//c.Config.OptionsConfig.MaxRepetitions = maxReps
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

func isPingUnrecoverableError(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errors.Is(errno, syscall.EPERM) || errors.Is(errno, syscall.EACCES))
}
