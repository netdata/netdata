// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"syscall"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"golang.org/x/sync/errgroup"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	initializing := !c.initialized
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	if initializing && c.normal.recorder != nil {
		for ordinal := uint64(1); ordinal <= c.normal.recorder.Cursor(); ordinal++ {
			c.normal.initialization = append(c.normal.initialization, c.normal.recorder.Operation(ordinal))
		}
	}

	if c.PingOnly {
		return c.collectPingOnly(ctx)
	}
	return c.collectDeviceMetrics(ctx)
}

func (c *Collector) collectPingOnly(ctx context.Context) (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectPing(ctx, mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectDeviceMetrics(ctx context.Context) (map[string]int64, error) {
	var (
		snmpMx map[string]int64
		pingMx map[string]int64
	)

	g, groupCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		m := make(map[string]int64)
		if err := c.collectSNMP(m); err != nil {
			return err
		}
		snmpMx = m
		return nil
	})

	if c.Ping.Enabled && c.pingClient != nil {
		g.Go(func() error {
			m := make(map[string]int64)
			if err := c.collectPing(groupCtx, m); err != nil {
				c.Errorf("ping: %v", err)
				if isPingUnrecoverableError(err) {
					c.pingClient = nil
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
		return errors.New("snmp client not initialized")
	}

	if c.initialized {
		return nil
	}

	if err := c.ensureDeviceProfile(); err != nil {
		return err
	}
	si := c.sysInfo

	if c.ddSnmpColl == nil && len(c.snmpProfiles) > 0 {
		c.ddSnmpColl = c.newDdSnmpColl(ddsnmpcollector.Config{
			SnmpClient:          c.snmpClient,
			Profiles:            c.snmpProfiles,
			Log:                 c.Logger,
			SysObjectID:         si.SysObjectID,
			DisableBulkWalk:     c.disableBulkWalk,
			AcquisitionObserver: ddsnmpcollector.AcquisitionObserverFunc(c.observeNormalProfile),
		})
	}

	if c.CreateVnode && c.Vnode == "" {
		var baseLabels map[string]string
		if c.UpdateEvery >= 1 && c.VnodeDeviceDownThreshold >= 1 {
			// Allow for collection and transmission delays.
			baseLabels = map[string]string{
				"_node_stale_after_seconds": strconv.Itoa(c.VnodeDeviceDownThreshold*c.UpdateEvery + 2),
			}
		}
		identity, err := ddsnmp.AcquireDeviceIdentity(si, c.ddSnmpColl, ddsnmp.DeviceIdentityOptions{
			Address:    c.Hostname,
			GUID:       c.LocalVnode.GUID,
			Hostname:   c.LocalVnode.Hostname,
			BaseLabels: baseLabels,
			Labels:     c.LocalVnode.Labels,
		})
		if c.ddSnmpColl != nil {
			c.captureCollectionFailures()
		}
		if err != nil {
			return err
		}
		c.LocalVnode.GUID = identity.GUID
		c.LocalVnode.Hostname = identity.Hostname
		c.vnode = &vnodes.VirtualNode{
			GUID:     identity.GUID,
			Hostname: identity.Hostname,
			Labels:   identity.Labels,
		}
	}

	if c.PingOnly || c.Ping.Enabled {
		c.addPingCharts()
	}

	c.registerDeviceState(si, nil)
	c.initialized = true

	return nil
}

func (c *Collector) initAndConnectSNMPClient() (gosnmp.Handler, error) {
	snmpClient, err := c.initSNMPClient()
	if err != nil {
		return nil, snmputils.WithFailure(fmt.Errorf("init: %w", err), "client", "")
	}

	if err := snmpClient.Connect(); err != nil {
		return nil, snmputils.WithFailure(fmt.Errorf("connect: %w", err), "connect", "")
	}

	if snmpClient.Version() == gosnmp.Version1 {
		return snmpClient, nil
	}

	if c.Options.MaxRepetitions == 0 {
		c.disableBulkWalk = true
		return snmpClient, nil
	}

	if c.adjMaxRepetitions != 0 {
		snmpClient.SetMaxRepetitions(c.adjMaxRepetitions)
	} else {
		probeClient := snmpClient
		if c.normal != nil && c.normal.recorder != nil {
			probeClient = c.normal.recorder.Wrap(snmpClient)
		}
		ok, err := c.adjustMaxRepetitions(probeClient)
		if err != nil {
			return nil, snmputils.WithFailure(fmt.Errorf("re-adjust max repetitions SNMP client: %w", err), "max_repetitions", "")
		}
		if !ok {
			c.Warningf("SNMP bulk walk disabled (device may not support GETBULK or max-repetitions adjustment failed)")
			c.disableBulkWalk = true
		}
		c.adjMaxRepetitions = snmpClient.MaxRepetitions()
	}

	return snmpClient, nil
}

func (c *Collector) adjustMaxRepetitions(snmpClient gosnmp.Handler) (bool, error) {
	ok, err := c.detectBulkWalkSupport(snmpClient)
	if err != nil {
		c.Warningf("bulk support probe error: %v", err)
		return false, nil
	}
	if !ok {
		return false, nil
	}

	orig := c.Options.MaxRepetitions
	maxReps := c.Options.MaxRepetitions
	attempts := 0
	const maxAttempts = 20 // Prevent infinite loops

	for maxReps > 0 && attempts < maxAttempts {
		attempts++

		v, err := snmpClient.BulkWalkAll(snmputils.RootOidMibSystem)
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

func isPingUnrecoverableError(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errors.Is(errno, syscall.EPERM) || errors.Is(errno, syscall.EACCES))
}

func (c *Collector) detectBulkWalkSupport(snmpClient gosnmp.Handler) (bool, error) {
	if snmpClient.Version() == gosnmp.Version1 {
		return false, nil
	}

	// Use a very small max-reps for the probe to be gentle
	orig := snmpClient.MaxRepetitions()
	defer snmpClient.SetMaxRepetitions(orig)
	snmpClient.SetMaxRepetitions(5)

	oids, err := snmpClient.BulkWalkAll(snmputils.RootOidMibSystem)
	if err != nil {
		return false, err
	}
	return len(oids) > 0, nil
}
