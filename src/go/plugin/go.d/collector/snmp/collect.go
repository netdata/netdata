// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"syscall"

	"github.com/gosnmp/gosnmp"
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
	if !initializing && !c.PingOnly {
		if err := c.refreshDeviceState(); err != nil {
			return nil, err
		}
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

	if c.ddSnmpColl == nil {
		c.ddSnmpColl = c.newProfileCollector()
	}

	if c.PingOnly || c.Ping.Enabled {
		c.addPingCharts()
	}

	// ping_only never polls profiles, so it publishes its identity here. Normal
	// collection publishes once a profile poll has prepared inputs, in the same
	// cycle and before any sample leaves the collector (syncDeviceMetadata).
	if c.PingOnly || c.ddSnmpColl == nil {
		var metadata map[string]ddsnmp.MetaTag
		if c.ddSnmpColl != nil {
			var err error
			metadata, err = c.ddSnmpColl.CollectDeviceMetadata()
			c.captureCollectionFailures()
			if err != nil {
				return err
			}
		}
		if err := c.applyDeviceMetadata(si, metadata); err != nil {
			return err
		}
	}
	c.initialized = true
	c.nextSysInfoCheck = c.now().Add(sysInfoCheckEvery)

	return nil
}

// newProfileCollector creates the profile owner for the installed client,
// profiles and identity. ping_only needs one only to enrich its generated vnode.
func (c *Collector) newProfileCollector() ddCollector {
	if len(c.snmpProfiles) == 0 || c.PingOnly && (!c.CreateVnode || c.Vnode != "") {
		return nil
	}
	return c.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient:          c.snmpClient,
		Profiles:            c.snmpProfiles,
		Log:                 c.Logger,
		SysObjectID:         c.sysInfo.SysObjectID,
		DisableBulkWalk:     c.disableBulkWalk,
		AcquisitionObserver: ddsnmpcollector.AcquisitionObserverFunc(c.observeNormalProfile),
	})
}

func (c *Collector) initAndConnectSNMPClient() (gosnmp.Handler, error) {
	client, disableBulk, repetitions, err := c.connectSNMPClient()
	if err == nil {
		c.disableBulkWalk, c.adjMaxRepetitions = disableBulk, repetitions
	}
	return client, err
}

// connectSNMPClient derives learned transport settings from authored options.
// A replacement candidate must not alter the incumbent's client or settings.
func (c *Collector) connectSNMPClient() (client gosnmp.Handler, disableBulk bool, repetitions uint32, err error) {
	snmpClient, err := c.initSNMPClient()
	if err != nil {
		return nil, false, 0, snmputils.WithFailure(fmt.Errorf("init: %w", err), "client", "")
	}

	if err := snmpClient.Connect(); err != nil {
		return nil, false, 0, snmputils.WithFailure(fmt.Errorf("connect: %w", err), "connect", "")
	}
	defer func() {
		if err != nil {
			_ = snmpClient.Close()
		}
	}()

	if snmpClient.Version() == gosnmp.Version1 {
		return snmpClient, false, 0, nil
	}

	if c.Options.MaxRepetitions == 0 {
		return snmpClient, true, 0, nil
	}

	probeClient := snmpClient
	if c.normal != nil && c.normal.recorder != nil {
		probeClient = c.normal.recorder.Wrap(snmpClient)
	}
	ok, err := c.adjustMaxRepetitions(probeClient)
	if err != nil {
		return nil, false, 0, snmputils.WithFailure(
			fmt.Errorf("re-adjust max repetitions SNMP client: %w", err),
			"max_repetitions",
			"",
		)
	}
	if !ok {
		c.Warningf("SNMP bulk walk disabled (device may not support GETBULK or max-repetitions adjustment failed)")
		disableBulk = true
	}

	return snmpClient, disableBulk, snmpClient.MaxRepetitions(), nil
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
