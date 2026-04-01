// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"time"
)

func (c *Collector) ensureTopologySchedulerStarted() {
	if c == nil || c.topologyCache == nil {
		return
	}

	c.topologyMu.Lock()
	if c.topologyRunning {
		c.topologyMu.Unlock()
		return
	}

	if len(c.topologyProfiles) == 0 {
		c.topologyMu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	refreshEvery := c.topologyRefreshEvery()

	c.topologyCancel = cancel
	c.topologyRunning = true
	c.topologyWG.Add(1)
	go func() {
		defer c.topologyWG.Done()
		c.runTopologyRefreshLoop(ctx, refreshEvery)
	}()
	c.topologyMu.Unlock()
}

func (c *Collector) stopTopologyScheduler() {
	if c == nil {
		return
	}

	c.topologyMu.Lock()
	cancel := c.topologyCancel
	running := c.topologyRunning
	c.topologyCancel = nil
	c.topologyRunning = false
	c.topologyMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if running {
		c.topologyWG.Wait()
	}
}

func (c *Collector) runTopologyRefreshLoop(ctx context.Context, refreshEvery time.Duration) {
	c.refreshTopologySnapshotWithLogging()

	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshTopologySnapshotWithLogging()
		}
	}
}

func (c *Collector) refreshTopologySnapshotWithLogging() {
	if err := c.refreshTopologySnapshot(); err != nil {
		c.Warningf("topology refresh failed: %v", err)
	}
}
