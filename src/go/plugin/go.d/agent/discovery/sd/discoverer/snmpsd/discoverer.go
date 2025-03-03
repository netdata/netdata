// SPDX-License-Identifier: GPL-3.0-or-later

package snmpsd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gohugoio/hashstructure"
	"github.com/gosnmp/gosnmp"
	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/filepersister"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/iprange"
)

const (
	defaultRescanInterval          = time.Minute * 30
	defaultTimeout                 = time.Second * 1
	defaultParallelScansPerNetwork = 32
	defaultDeviceCacheTTL          = time.Hour * 12
)

func NewDiscoverer(cfg Config) (*Discoverer, error) {
	subnets, err := cfg.validateAndParse()
	if err != nil {
		return nil, err
	}

	cfgHash, _ := hashstructure.Hash(cfg, nil)

	d := &Discoverer{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("discoverer", "snmp"),
		),
		started: make(chan struct{}),
		cfgHash: cfgHash,
		subnets: subnets,
		newSnmpClient: func() (gosnmp.Handler, func()) {
			return gosnmp.NewHandler(), func() {}
		},

		rescanInterval:          defaultRescanInterval,
		timeout:                 defaultTimeout,
		parallelScansPerNetwork: defaultParallelScansPerNetwork,
		deviceCacheTTL:          defaultDeviceCacheTTL,

		firstDiscovery: true,
		status:         newDiscoveryStatus(),
	}

	if cfg.RescanInterval != nil && *cfg.RescanInterval >= 0 {
		d.rescanInterval = cfg.RescanInterval.Duration()
	}
	if cfg.Timeout > 0 {
		d.timeout = cfg.Timeout.Duration()
	}
	if cfg.ParallelScansPerNetwork > 0 {
		d.parallelScansPerNetwork = cfg.ParallelScansPerNetwork
	}
	if cfg.DeviceCacheTTL != nil && *cfg.DeviceCacheTTL >= 0 {
		d.deviceCacheTTL = cfg.DeviceCacheTTL.Duration()
	}

	return d, nil
}

type (
	Discoverer struct {
		*logger.Logger
		model.Base

		started chan struct{}
		cfgHash uint64

		subnets []subnet

		newSnmpClient func() (gosnmp.Handler, func())

		parallelScansPerNetwork int
		rescanInterval          time.Duration
		timeout                 time.Duration
		deviceCacheTTL          time.Duration

		firstDiscovery bool
		status         *discoveryStatus
	}
	subnet struct {
		str        string
		ips        iprange.Range
		credential CredentialConfig
	}
)

func (d *Discoverer) String() string {
	return "sd:snmp"
}

func (d *Discoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	defer func() { d.Info("instance is stopped") }()

	close(d.started)

	d.loadFileStatus()

	d.discoverNetworks(ctx, in)

	if d.rescanInterval <= 0 {
		filepersister.Save(statusFileName(), d.status)
		return
	}

	tk := time.NewTicker(d.rescanInterval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			d.discoverNetworks(ctx, in)
		}
	}
}

func (d *Discoverer) discoverNetworks(ctx context.Context, in chan<- []model.TargetGroup) {
	now := time.Now()

	doProbing := !d.firstDiscovery ||
		d.status.ConfigHash != d.cfgHash ||
		now.After(d.status.LastDiscoveryTime.Add(d.rescanInterval))

	defer func() {
		if isDone(ctx) {
			return
		}
		d.firstDiscovery = false

		if doProbing {
			d.status.LastDiscoveryTime = now
		}

		if d.status.updated.Swap(false) || d.status.ConfigHash != d.cfgHash {
			d.status.ConfigHash = d.cfgHash
			filepersister.Save(statusFileName(), d.status)
		}
	}()

	d.Infof("discovery mode: %s", map[bool]string{true: "active probing", false: "using cache"}[doProbing])

	p := pool.New()
	for _, sub := range d.subnets {
		sub := sub
		p.Go(func() { d.discoverNetwork(ctx, in, sub, doProbing) })
	}
	p.Wait()
}

func (d *Discoverer) discoverNetwork(ctx context.Context, in chan<- []model.TargetGroup, sub subnet, doProbing bool) {
	tgg := newTargetGroup(sub)
	p := pool.New().WithMaxGoroutines(d.parallelScansPerNetwork)

	for ip := range sub.ips.Iterate() {
		ipAddr := ip.String()

		if doProbing {
			p.Go(func() { d.probeIPAddress(ctx, sub, ipAddr, tgg) })
		} else {
			d.useCacheIPAddress(sub, ipAddr, tgg)
		}
	}
	p.Wait()

	send(ctx, in, tgg)
}

func (d *Discoverer) useCacheIPAddress(sub subnet, ip string, tgg *targetGroup) {
	if dev := d.status.get(sub, ip); dev != nil {
		tg := newTarget(ip, sub.credential, dev.SysInfo)
		tgg.addTarget(tg)
	}
}

func (d *Discoverer) probeIPAddress(ctx context.Context, sub subnet, ip string, tgg *targetGroup) {
	if isDone(ctx) {
		return
	}

	now := time.Now()

	dev := d.status.get(sub, ip)

	// Use the cached device if available and not expired
	if dev != nil && (d.deviceCacheTTL == 0 || now.Before(dev.DiscoverTime.Add(d.deviceCacheTTL))) {
		if d.firstDiscovery {
			if d.deviceCacheTTL == 0 {
				d.Infof("device '%s': found in cache (sysName: '%s', network: '%s', cache never expires)",
					ip, dev.SysInfo.Name, subKey(sub))
			} else {
				untilProbe := dev.DiscoverTime.Add(d.deviceCacheTTL).Sub(now).Round(time.Second)
				d.Infof("device '%s': found in cache (sysName: '%s', network: '%s', next probe in %s)",
					ip, dev.SysInfo.Name, subKey(sub), untilProbe)
			}
		}
		tg := newTarget(ip, sub.credential, dev.SysInfo)
		tgg.addTarget(tg)
		return
	}

	si, err := d.getSnmpSysInfo(sub, ip)
	if err != nil {
		if dev == nil {
			// First-time discovery failure - log at debug level as this is expected for many IPs
			d.Debugf("device '%s': probe failed (network: '%s'): %v", ip, subKey(sub), err)
		} else {
			// Previously discovered device is now unreachable
			d.Warningf("lost connection to previously discovered SNMP device '%s' (sysName: '%s', network: '%s'): %v",
				ip, dev.SysInfo.Name, subKey(sub), err)
		}
		d.status.del(sub, ip)
		d.status.updated.Store(dev != nil)
		return
	}

	d.Infof("device '%s': successfully discovered (sysName: '%s', network: '%s')", ip, si.Name, subKey(sub))
	d.status.put(sub, ip, &discoveredDevice{DiscoverTime: now, SysInfo: *si})
	d.status.updated.Store(true)
	tg := newTarget(ip, sub.credential, *si)
	tgg.addTarget(tg)
}

func (d *Discoverer) getSnmpSysInfo(sub subnet, ip string) (*SysInfo, error) {
	client, cleanup := d.newSnmpClient()
	defer cleanup()

	client.SetTarget(ip)
	client.SetTimeout(d.timeout)
	client.SetRetries(0)
	setCredential(client, sub.credential)

	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	defer func() { _ = client.Close() }()

	return GetSysInfo(client)
}

func send(ctx context.Context, in chan<- []model.TargetGroup, tgg model.TargetGroup) {
	select {
	case <-ctx.Done():
	case in <- []model.TargetGroup{tgg}:
	}
}
func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
