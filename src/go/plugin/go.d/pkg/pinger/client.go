// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"
)

type Client interface {
	Probe(ctx context.Context, host string) (Sample, error)
	ProbeAndTrack(ctx context.Context, host string) (Sample, error)
}

type client struct {
	log    *logger.Logger
	cfg    Config
	runner probeRunner
	state  *stateStore
}

func New(cfg Config, log *logger.Logger) (Client, error) {
	return newClient(cfg, log, &defaultRunner{log: ensureLogger(log)})
}

func newClient(cfg Config, log *logger.Logger, runner probeRunner) (*client, error) {
	if runner == nil {
		return nil, errors.New("nil probe runner")
	}

	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &client{
		log:    ensureLogger(log),
		cfg:    cfg,
		runner: runner,
		state:  newStateStore(),
	}, nil
}

func (c *client) Probe(ctx context.Context, host string) (Sample, error) {
	return c.probe(ctx, host, false)
}

func (c *client) ProbeAndTrack(ctx context.Context, host string) (Sample, error) {
	return c.probe(ctx, host, true)
}

func (c *client) probe(ctx context.Context, host string, track bool) (Sample, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	stats, err := c.runner.probe(ctx, host, c.cfg.Probe)
	if err != nil {
		var probeErr *ProbeError
		if errors.As(err, &probeErr) {
			return Sample{}, err
		}
		return Sample{}, &ProbeError{Host: host, Stage: "probe", Err: err}
	}
	if stats == nil {
		return Sample{}, &ProbeError{Host: host, Stage: "probe", Err: fmt.Errorf("nil probe statistics")}
	}

	return deriveSample(host, stats, track, c.state, c.cfg.Analysis), nil
}

func ensureLogger(log *logger.Logger) *logger.Logger {
	if log != nil {
		return log
	}
	return logger.New()
}
