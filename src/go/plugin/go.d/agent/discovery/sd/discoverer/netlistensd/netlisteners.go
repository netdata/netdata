// SPDX-License-Identifier: GPL-3.0-or-later

package netlistensd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gohugoio/hashstructure"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"
)

var (
	shortName = "net_listeners"
	fullName  = fmt.Sprintf("sd:%s", shortName)
)

type Config struct {
	Source string `yaml:"-"`
	Tags   string `yaml:"tags"`

	Interval *confopt.Duration `yaml:"interval"`
	Timeout  confopt.Duration  `yaml:"timeout"`
}

func NewDiscoverer(cfg Config) (*Discoverer, error) {
	tags, err := model.ParseTags(cfg.Tags)
	if err != nil {
		return nil, fmt.Errorf("parse tags: %v", err)
	}

	interval := time.Minute * 2
	if cfg.Interval != nil {
		interval = cfg.Interval.Duration()
	}
	timeout := time.Second * 5
	if cfg.Timeout.Duration() != 0 {
		timeout = cfg.Timeout.Duration()
	}

	d := &Discoverer{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("discoverer", shortName),
		),
		cfgSource:  cfg.Source,
		ll:         newLocalListeners(timeout),
		interval:   interval,
		timeout:    timeout,
		expiryTime: time.Minute * 10,
		cache:      make(map[uint64]*cacheItem),
		started:    make(chan struct{}),
	}

	d.Tags().Merge(tags)

	return d, nil
}

type (
	Discoverer struct {
		*logger.Logger
		model.Base

		cfgSource string

		interval time.Duration
		timeout  time.Duration
		ll       localListeners

		expiryTime time.Duration
		cache      map[uint64]*cacheItem // [target.Hash]

		started chan struct{}

		successRuns int64
		timeoutRuns int64
	}
	cacheItem struct {
		lastSeenTime time.Time
		tgt          model.Target
	}
)

func (d *Discoverer) String() string {
	return fullName
}

func (d *Discoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	d.Debugf("used config: interval: %s, timeout: %s, cache expiration time: %s", d.interval, d.timeout, d.expiryTime)
	defer func() { d.Info("instance is stopped") }()

	close(d.started)

	if err := d.discoverLocalListeners(ctx, in); err != nil {
		d.Error(err)
		return
	}

	if d.interval == 0 {
		return
	}

	tk := time.NewTicker(d.interval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			if err := d.discoverLocalListeners(ctx, in); err != nil {
				d.Error(err)
				return
			}
		}
	}
}

func (d *Discoverer) discoverLocalListeners(ctx context.Context, in chan<- []model.TargetGroup) error {
	bs, err := d.ll.discover(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// there is no point in continuing pointless attempts/use cpu
			// https://github.com/netdata/netdata/discussions/18751#discussioncomment-10908472
			if d.timeoutRuns++; d.timeoutRuns > 5 && d.successRuns == 0 {
				return err
			}
			d.Warning(err)
			return nil
		}
		return err
	}

	d.successRuns++

	tgts, err := d.parseLocalListeners(bs)
	if err != nil {
		return err
	}

	tggs := d.processTargets(tgts)

	select {
	case <-ctx.Done():
	case in <- tggs:
	}

	return nil
}

func (d *Discoverer) processTargets(tgts []model.Target) []model.TargetGroup {
	tgg := &targetGroup{
		provider: fullName,
		source:   fmt.Sprintf("discoverer=%s,host=localhost", shortName),
	}
	if d.cfgSource != "" {
		tgg.source += fmt.Sprintf(",%s", d.cfgSource)
	}

	if d.expiryTime.Milliseconds() == 0 {
		tgg.targets = tgts
		return []model.TargetGroup{tgg}
	}

	now := time.Now()

	for _, tgt := range tgts {
		hash := tgt.Hash()
		if _, ok := d.cache[hash]; !ok {
			d.cache[hash] = &cacheItem{tgt: tgt}
		}
		d.cache[hash].lastSeenTime = now
	}

	for k, v := range d.cache {
		if now.Sub(v.lastSeenTime) > d.expiryTime {
			delete(d.cache, k)
			continue
		}
		tgg.targets = append(tgg.targets, v.tgt)
	}

	return []model.TargetGroup{tgg}
}

func (d *Discoverer) parseLocalListeners(bs []byte) ([]model.Target, error) {
	const (
		local4 = "127.0.0.1"
		local6 = "::1"
	)

	var targets []target
	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}

		// Protocol|IPAddress|Port|Cmdline
		parts := strings.SplitN(text, "|", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("unexpected data: '%s'", text)
		}

		tgt := target{
			Protocol:  parts[0],
			IPAddress: parts[1],
			Port:      parts[2],
			Comm:      extractComm(parts[3]),
			Cmdline:   parts[3],
		}

		if tgt.Comm == "docker-proxy" {
			continue
		}

		switch tgt.IPAddress {
		case "0.0.0.0":
			tgt.IPAddress = local4
		case "::":
			tgt.IPAddress = local6
		}

		// quick support for https://github.com/netdata/netdata/pull/17866
		// TODO: create both ipv4 and ipv6 targets?
		if tgt.IPAddress == "*" {
			tgt.IPAddress = local4
		}

		tgt.Address = net.JoinHostPort(tgt.IPAddress, tgt.Port)

		hash, err := calcHash(tgt)
		if err != nil {
			continue
		}

		tgt.hash = hash
		tgt.Tags().Merge(d.Tags())

		targets = append(targets, tgt)
	}

	// order: TCP, TCP6, UDP, UDP6
	sort.Slice(targets, func(i, j int) bool {
		tgt1, tgt2 := targets[i], targets[j]
		if tgt1.Protocol != tgt2.Protocol {
			return tgt1.Protocol < tgt2.Protocol
		}

		p1, _ := strconv.Atoi(targets[i].Port)
		p2, _ := strconv.Atoi(targets[j].Port)
		if p1 != p2 {
			return p1 < p2
		}

		return tgt1.IPAddress == local4 || tgt1.IPAddress == local6
	})

	seen := make(map[string]bool)
	tgts := make([]model.Target, len(targets))
	var n int

	for _, tgt := range targets {
		tgt := tgt

		proto := strings.TrimSuffix(tgt.Protocol, "6")
		key := tgt.Protocol + ":" + tgt.Address
		keyLocal4 := proto + ":" + net.JoinHostPort(local4, tgt.Port)
		keyLocal6 := proto + "6:" + net.JoinHostPort(local6, tgt.Port)

		// Filter targets that accept conns on any (0.0.0.0) and additionally on each network interface  (a.b.c.d).
		// Create a target only for localhost. Assumption: any address always goes first.
		if seen[key] || seen[keyLocal4] || seen[keyLocal6] {
			continue
		}
		seen[key] = true

		tgts[n] = &tgt
		n++
	}

	return tgts[:n], nil
}

func extractComm(cmdLine string) string {
	if i := strings.IndexByte(cmdLine, ' '); i != -1 {
		cmdLine = cmdLine[:i]
	}
	_, comm := filepath.Split(cmdLine)
	return strings.TrimSuffix(comm, ":")
}

func calcHash(obj any) (uint64, error) {
	return hashstructure.Hash(obj, nil)
}
