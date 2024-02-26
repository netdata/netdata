// SPDX-License-Identifier: GPL-3.0-or-later

package hostsocket

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
	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"github.com/ilyam8/hashstructure"
)

type netSocketTargetGroup struct {
	provider string
	source   string
	targets  []model.Target
}

func (g *netSocketTargetGroup) Provider() string        { return g.provider }
func (g *netSocketTargetGroup) Source() string          { return g.source }
func (g *netSocketTargetGroup) Targets() []model.Target { return g.targets }

type NetSocketTarget struct {
	model.Base

	hash uint64

	Protocol string
	Address  string
	Port     string
	Comm     string
	Cmdline  string
}

func (t *NetSocketTarget) TUID() string { return t.tuid() }
func (t *NetSocketTarget) Hash() uint64 { return t.hash }
func (t *NetSocketTarget) tuid() string {
	return fmt.Sprintf("%s_%s_%d", strings.ToLower(t.Protocol), t.Port, t.hash)
}

func NewNetSocketDiscoverer(cfg NetworkSocketConfig) (*NetDiscoverer, error) {
	tags, err := model.ParseTags(cfg.Tags)
	if err != nil {
		return nil, fmt.Errorf("parse tags: %v", err)
	}

	dir := os.Getenv("NETDATA_PLUGINS_DIR")
	if dir == "" {
		dir, _ = os.Getwd()
	}

	d := &NetDiscoverer{
		Logger: logger.New().With(
			slog.String("component", "discovery sd hostsocket"),
		),
		interval: time.Second * 60,
		ll: &localListenersExec{
			binPath: filepath.Join(dir, "local-listeners"),
			timeout: time.Second * 5,
		},
	}
	d.Tags().Merge(tags)

	return d, nil
}

type (
	NetDiscoverer struct {
		*logger.Logger
		model.Base

		interval time.Duration
		ll       localListeners
	}
	localListeners interface {
		discover(ctx context.Context) ([]byte, error)
	}
)

func (d *NetDiscoverer) Discover(ctx context.Context, in chan<- []model.TargetGroup) {
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
				d.Error(err)
				return
			}
		}
	}
}

func (d *NetDiscoverer) discoverLocalListeners(ctx context.Context, in chan<- []model.TargetGroup) error {
	bs, err := d.ll.discover(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

	tggs, err := d.parseLocalListeners(bs)
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case in <- tggs:
	}
	return nil
}

func (d *NetDiscoverer) parseLocalListeners(bs []byte) ([]model.TargetGroup, error) {
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

		tgt := NetSocketTarget{
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

	tgg := &netSocketTargetGroup{
		provider: "hostsocket",
		source:   "net",
		targets:  tgts,
	}

	return []model.TargetGroup{tgg}, nil
}

type localListenersExec struct {
	binPath string
	timeout time.Duration
}

func (e *localListenersExec) discover(ctx context.Context) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, e.binPath, "tcp") // TODO: tcp6?

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}

func extractComm(s string) string {
	i := strings.IndexByte(s, ' ')
	if i <= 0 {
		return ""
	}
	_, comm := filepath.Split(s[:i])
	return comm
}

func calcHash(obj any) (uint64, error) {
	return hashstructure.Hash(obj, nil)
}
