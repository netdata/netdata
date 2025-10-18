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
			c.logErrorOnce("system_status_error", "failed to collect system status: %s", trimDriverMessage(err))
		}
	} else {
		c.clearErrorOnce("system_status_error")
	}

	if err := c.collectMemoryPools(ctx); err != nil {
		if isSQLTemporaryError(err) {
			c.Debugf("memory pools collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.logErrorOnce("memory_pools_error", "failed to collect memory pools: %s", trimDriverMessage(err))
		}
	} else {
		c.clearErrorOnce("memory_pools_error")
	}

	if err := c.collectDiskStatus(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.logOnce("disk_status_unavailable", "disk status monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("disk status collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.logErrorOnce("disk_status_error", "failed to collect disk status: %s", trimDriverMessage(err))
		}
	} else {
		c.clearErrorOnce("disk_status_error")
	}

	if err := c.collectJobInfo(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.logOnce("job_info_unavailable", "job info monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("job info collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.logErrorOnce("job_info_error", "failed to collect job info: %s", trimDriverMessage(err))
		}
	} else {
		c.clearErrorOnce("job_info_error")
	}

	if err := c.collectNetworkConnections(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.logOnce("network_connections_unavailable", "network connections monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("network connections collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.logErrorOnce("network_connections_error", "failed to collect network connections: %s", trimDriverMessage(err))
		}
	} else {
		c.clearErrorOnce("network_connections_error")
	}

	if err := c.collectTempStorage(ctx); err != nil {
		if isSQLFeatureError(err) {
			c.logOnce("temp_storage_unavailable", "temporary storage monitoring not available on this IBM i version: %v", err)
		} else if isSQLTemporaryError(err) {
			c.Debugf("temporary storage collection failed with temporary error, will show gaps: %v", err)
		} else {
			c.logErrorOnce("temp_storage_error", "failed to collect temporary storage: %s", trimDriverMessage(err))
		}
	} else {
		c.clearErrorOnce("temp_storage_error")
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
		g.c.logErrorOnce("subsystems_error", "failed to collect subsystems: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("subsystems_error")
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
		g.c.logErrorOnce("job_queues_error", "failed to collect job queues: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("job_queues_error")
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
		g.c.logErrorOnce("message_queues_error", "failed to collect message queues: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("message_queues_error")
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
		g.c.logErrorOnce("output_queues_error", "failed to collect output queues: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("output_queues_error")
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
			g.c.logOnce("disk_enhanced_unavailable", "enhanced disk metrics not available, using basic collection: %v", err)
			if basicErr := g.c.collectDiskInstances(ctx); basicErr != nil {
				g.c.logErrorOnce("disk_instances_error", "failed to collect disk instances: %s", trimDriverMessage(basicErr))
			}
		} else if isSQLTemporaryError(err) {
			g.c.Debugf("disk instances collection failed with temporary error, will show gaps: %v", err)
		} else {
			g.c.logErrorOnce("disk_instances_enhanced_error", "failed to collect enhanced disk instances: %s", trimDriverMessage(err))
		}
	} else {
		g.c.clearErrorOnce("disk_instances_enhanced_error")
		g.c.clearErrorOnce("disk_instances_error")
	}
	return nil
}

type activeJobGroup struct {
	c *Collector
}

func (g *activeJobGroup) Name() string { return "active_jobs" }
func (g *activeJobGroup) Enabled() bool {
	return g.c.CollectActiveJobs.IsEnabled() && len(g.c.activeJobTargets) > 0
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
		if isSQLTemporaryError(err) {
			g.c.Debugf("active job collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.logErrorOnce("active_jobs_error", "failed to collect active jobs: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("active_jobs_error")
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
		if isSQLTemporaryError(err) {
			g.c.Debugf("network interface collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.logErrorOnce("network_interfaces_error", "failed to collect network interfaces: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("network_interfaces_error")
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
			g.c.logOnce("system_activity_unavailable", "system activity monitoring not available on this IBM i version: %v", err)
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("system activity collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.logErrorOnce("system_activity_error", "failed to collect system activity: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("system_activity_error")
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
		g.c.logErrorOnce("http_server_error", "failed to collect HTTP server metrics: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("http_server_error")
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
			tmsg := trimDriverMessage(err)
			g.c.logErrorOnce("plan_cache_missing_privs", "plan cache analysis not available or requires additional authority: %s", tmsg)
			g.c.CollectPlanCacheMetrics = confopt.AutoBoolDisabled
			return nil
		}
		if isSQLTemporaryError(err) {
			g.c.Debugf("plan cache metrics collection failed with temporary error, will show gaps: %v", err)
			return nil
		}
		g.c.logErrorOnce("plan_cache_error", "failed to collect plan cache metrics: %s", trimDriverMessage(err))
		return nil
	}
	g.c.clearErrorOnce("plan_cache_missing_privs")
	g.c.clearErrorOnce("plan_cache_error")
	return nil
}
