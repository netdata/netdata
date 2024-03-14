// SPDX-License-Identifier: GPL-3.0-or-later

package netlisteners

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/go.d.plugin/agent/executable"
	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"github.com/ilyam8/hashstructure"
)

var (
	shortName = "net_listeners"
	fullName  = fmt.Sprintf("sd:%s", shortName)
)

func NewDiscoverer(cfg Config) (*Discoverer, error) {
	tags, err := model.ParseTags(cfg.Tags)
	if err != nil {
		return nil, fmt.Errorf("parse tags: %v", err)
	}

	dir := os.Getenv("NETDATA_PLUGINS_DIR")
	if dir == "" {
		dir = executable.Directory
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}

	d := &Discoverer{
		Logger: logger.New().With(
			slog.String("component", "service discovery"),
			slog.String("discoverer", shortName),
		),
		cfgSource: cfg.Source,
		ll: &localListenersExec{
			binPath: filepath.Join(dir, "local-listeners"),
			timeout: time.Second * 5,
		},
		interval:   time.Minute * 2,
		expiryTime: time.Minute * 10,
		cache:      make(map[uint64]*cacheItem),
		started:    make(chan struct{}),
	}

	d.Tags().Merge(tags)

	return d, nil
}

type Config struct {
	Source string `yaml:"-"`
	Tags   string `yaml:"tags"`
}

type (
	Discoverer struct {
		*logger.Logger
		model.Base

		cfgSource string

		interval time.Duration
		ll       localListeners

		expiryTime time.Duration
		cache      map[uint64]*cacheItem // [target.Hash]

		started chan struct{}
	}
	cacheItem struct {
		lastSeenTime time.Time
		tgt          model.Target
	}
	localListeners interface {
		discover(ctx context.Context) ([]byte, error)
	}
)

func (d *Discoverer) String() string {
	return fullName
}

func (d *Discoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
	d.Info("instance is started")
	defer func() { d.Info("instance is stopped") }()

	close(d.started)

	if err := d.discoverLocalListeners(ctx, in); err != nil {
		d.Error(err)
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
				d.Warning(err)
				return
			}
		}
	}
}

func (d *Discoverer) discoverLocalListeners(ctx context.Context, in chan<- []model.TargetGroup) error {
	bs, err := d.ll.discover(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

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
	var tgts []model.Target

	sc := bufio.NewScanner(bytes.NewReader(bs))
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}

		// Protocol|Address|Port|Cmdline
		parts := strings.SplitN(text, "|", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("unexpected data: '%s'", text)
		}

		tgt := target{
			Protocol: parts[0],
			Address:  parts[1],
			Port:     parts[2],
			Comm:     extractComm(parts[3]),
			Cmdline:  parts[3],
		}

		hash, err := calcHash(tgt)
		if err != nil {
			continue
		}

		tgt.hash = hash
		tgt.Tags().Merge(d.Tags())

		tgts = append(tgts, &tgt)
	}

	return tgts, nil
}

type localListenersExec struct {
	binPath string
	timeout time.Duration
}

func (e *localListenersExec) discover(ctx context.Context) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// TCPv4 and UPDv4 sockets in LISTEN state
	// https://github.com/netdata/netdata/blob/master/src/collectors/plugins.d/local_listeners.c
	args := []string{
		"no-udp6",
		"no-tcp6",
		"no-local",
		"no-inbound",
		"no-outbound",
		"no-namespaces",
	}

	cmd := exec.CommandContext(execCtx, e.binPath, args...)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on executing '%s': %v", cmd, err)
	}

	return bs, nil
}

func extractComm(cmdLine string) string {
	i := strings.IndexByte(cmdLine, ' ')
	if i <= 0 {
		return cmdLine
	}
	_, comm := filepath.Split(cmdLine[:i])
	return comm
}

func calcHash(obj any) (uint64, error) {
	return hashstructure.Hash(obj, nil)
}
