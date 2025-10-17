//go:build cgo
// +build cgo

package as400

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
)

type collectionGroup interface {
	Name() string
	Enabled() bool
	Collect(ctx context.Context) error
}

type systemGroup struct {
	c *Collector
}

func (g *systemGroup) Name() string  { return "system" }
func (g *systemGroup) Enabled() bool { return true }

func (g *systemGroup) Collect(ctx context.Context) error {
	c := g.c

	if err := c.collectSystemStatus(ctx); err != nil {
		if isSQLTemporaryError(err) {
			c.Debugf("system status collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.Errorf("failed to collect system status: %v", err)
		}
	}

	if err := c.collectMemoryPools(ctx); err != nil {
		if isSQLTemporaryError(err) {
			c.Debugf("memory pools collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.Errorf("failed to collect memory pools: %v", err)
		}
	}

	if err := c.collectDiskStatus(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.Warningf("disk status monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("disk status collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.Errorf("failed to collect disk status: %v", err)
		}
	}

	if err := c.collectJobInfo(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.Warningf("job info monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("job info collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.Errorf("failed to collect job info: %v", err)
		}
	}

	if err := c.collectNetworkConnections(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.Warningf("network connections monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("network connections collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.Errorf("failed to collect network connections: %v", err)
		}
	}

	if err := c.collectTempStorage(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.Warningf("temporary storage monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("temporary storage collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.Errorf("failed to collect temporary storage: %v", err)
		}
	}

	return nil
}

type subsystemGroup struct {
	c *Collector
}

func (g *subsystemGroup) Name() string { return "subsystems" }
func (g *subsystemGroup) Enabled() bool {
	return g.c.CollectSubsystemMetrics.IsEnabled()
}

func (g *subsystemGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectSubsystems(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("subsystem monitoring not available on this IBM i version: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect subsystems: %v", err)
	}
	return nil
}

type jobQueueGroup struct {
	c *Collector
}

func (g *jobQueueGroup) Name() string { return "job_queues" }
func (g *jobQueueGroup) Enabled() bool {
	return len(g.c.jobQueueTargets) > 0
}

func (g *jobQueueGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectJobQueues(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("job queue monitoring not available on this IBM i version: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect job queues: %v", err)
	}
	return nil
}

type messageQueueGroup struct {
	c *Collector
}

func (g *messageQueueGroup) Name() string { return "message_queues" }
func (g *messageQueueGroup) Enabled() bool {
	return len(g.c.messageQueueTargets) > 0
}

func (g *messageQueueGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectMessageQueues(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("message queue metrics not available on this IBM i version: %v", err)
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("message queue collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect message queues: %v", err)
	}
	return nil
}

type outputQueueGroup struct {
	c *Collector
}

func (g *outputQueueGroup) Name() string { return "output_queues" }
func (g *outputQueueGroup) Enabled() bool {
	return len(g.c.outputQueueTargets) > 0
}

func (g *outputQueueGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectOutputQueues(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("output queue metrics not available on this IBM i version: %v", err)
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("output queue collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect output queues: %v", err)
	}
	return nil
}

type diskGroup struct {
	c *Collector
}

func (g *diskGroup) Name() string { return "disks" }
func (g *diskGroup) Enabled() bool {
	return g.c.CollectDiskMetrics.IsEnabled()
}

func (g *diskGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectDiskInstancesEnhanced(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("enhanced disk metrics not available, using basic collection: %v", err)
			if basicErr := g.c.collectDiskInstances(ctx); basicErr != nil {
				g.c.Errorf("failed to collect disk instances: %v", basicErr)
			}
		} else {
			g.c.Errorf("failed to collect enhanced disk instances: %v", err)
		}
	}
	return nil
}

type activeJobGroup struct {
	c *Collector
}

func (g *activeJobGroup) Name() string { return "active_jobs" }
func (g *activeJobGroup) Enabled() bool {
	return g.c.CollectActiveJobs.IsEnabled()
}

func (g *activeJobGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectActiveJobs(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("active job metrics not available on this IBM i version: %v", err)
			g.c.CollectActiveJobs = confopt.AutoBoolDisabled
			return nil
		}
		g.c.Errorf("failed to collect active jobs: %v", err)
	}
	return nil
}

type networkInterfaceGroup struct {
	c *Collector
}

func (g *networkInterfaceGroup) Name() string  { return "network_interfaces" }
func (g *networkInterfaceGroup) Enabled() bool { return true }

func (g *networkInterfaceGroup) Collect(ctx context.Context) error {
	if err := g.c.collectNetworkInterfaces(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("network interface monitoring not available on this IBM i version: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect network interfaces: %v", err)
	}
	return nil
}

type systemActivityGroup struct {
	c *Collector
}

func (g *systemActivityGroup) Name() string  { return "system_activity" }
func (g *systemActivityGroup) Enabled() bool { return true }

func (g *systemActivityGroup) Collect(ctx context.Context) error {
	if err := g.c.collectSystemActivity(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("system activity monitoring not available on this IBM i version: %v", err)
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("system activity collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect system activity: %v", err)
	}
	return nil
}

type httpServerGroup struct {
	c *Collector
}

func (g *httpServerGroup) Name() string { return "http_server" }
func (g *httpServerGroup) Enabled() bool {
	return g.c.CollectHTTPServerMetrics.IsEnabled()
}

func (g *httpServerGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectHTTPServerInfo(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("HTTP server metrics not available on this IBM i version: %v", err)
			g.c.CollectHTTPServerMetrics = confopt.AutoBoolDisabled
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("HTTP server metrics collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect HTTP server metrics: %v", err)
	}
	return nil
}

type planCacheGroup struct {
	c *Collector
}

func (g *planCacheGroup) Name() string { return "plan_cache" }
func (g *planCacheGroup) Enabled() bool {
	return g.c.CollectPlanCacheMetrics.IsEnabled()
}

func (g *planCacheGroup) Collect(ctx context.Context) error {
	if !g.Enabled() {
		return nil
	}
	if err := g.c.collectPlanCache(ctx); err != nil {
		if isSQLFeatureError(err) {
			g.c.Warningf("plan cache analysis not available or requires additional authority: %v", err)
			g.c.CollectPlanCacheMetrics = confopt.AutoBoolDisabled
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("plan cache metrics collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.Errorf("failed to collect plan cache metrics: %v", err)
	}
	return nil
}
