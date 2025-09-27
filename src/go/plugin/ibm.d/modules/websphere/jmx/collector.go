//go:build cgo
// +build cgo

package jmx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/jmx/contexts"
	jmxproto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/websphere/jmx"
)

// Collector implements the WebSphere JMX module.
type Collector struct {
	framework.Collector

	Config Config

	identity     common.Identity
	client       jmxClient
	poolSelector matcher.Matcher
	jmsSelector  matcher.Matcher
	appSelector  matcher.Matcher
}

// jmxClient abstracts the protocol client for easier testing.
type jmxClient interface {
	Start(ctx context.Context) error
	Shutdown()
	FetchJVM(ctx context.Context) (*jmxproto.JVMStats, error)
	FetchThreadPools(ctx context.Context, maxItems int) ([]jmxproto.ThreadPool, error)
	FetchJDBCPools(ctx context.Context, maxItems int) ([]jmxproto.JDBCPool, error)
	FetchJCAPools(ctx context.Context, maxItems int) ([]jmxproto.JCAPool, error)
	FetchJMSDestinations(ctx context.Context, maxItems int) ([]jmxproto.JMSDestination, error)
	FetchApplications(ctx context.Context, maxItems int, includeSessions, includeTransactions bool) ([]jmxproto.ApplicationMetric, error)
}

// CollectOnce performs a single scrape iteration.
func (c *Collector) CollectOnce() error {
	if c.client == nil {
		return errors.New("websphere_jmx: protocol client not initialised")
	}

	stats, err := c.client.FetchJVM(context.Background())
	if err != nil {
		return fmt.Errorf("websphere_jmx: fetch JVM metrics: %w", err)
	}
	if stats == nil {
		return errors.New("websphere_jmx: empty JVM payload")
	}
	c.exportJVM(stats)

	if c.Config.CollectThreadPoolMetrics.IsEnabled() {
		if err := c.collectThreadPools(context.Background()); err != nil {
			c.Warningf("failed to collect thread pool metrics: %v", err)
		}
	}

	if c.Config.CollectJDBCMetrics.IsEnabled() {
		if err := c.collectJDBCPools(context.Background()); err != nil {
			c.Warningf("failed to collect JDBC metrics: %v", err)
		}
	}

	if c.Config.CollectJCAMetrics.IsEnabled() {
		if err := c.collectJCAPools(context.Background()); err != nil {
			c.Warningf("failed to collect JCA metrics: %v", err)
		}
	}

	if c.Config.CollectJMSMetrics.IsEnabled() {
		if err := c.collectJMSDestinations(context.Background()); err != nil {
			c.Warningf("failed to collect JMS metrics: %v", err)
		}
	}

	if c.Config.CollectWebAppMetrics.IsEnabled() {
		if err := c.collectApplications(context.Background()); err != nil {
			c.Warningf("failed to collect application metrics: %v", err)
		}
	}

	return nil
}

// Ensure Collector satisfies the framework contract.
var _ framework.CollectorImpl = (*Collector)(nil)

// exportJVM maps JVM statistics into framework contexts.
func (c *Collector) exportJVM(stats *jmxproto.JVMStats) {
	labels := contexts.EmptyLabels{}

	heapUsed := int64(stats.Heap.Used)
	heapCommitted := int64(stats.Heap.Committed)
	heapMax := int64(stats.Heap.Max)
	contexts.JVM.HeapMemory.Set(c.State, labels, contexts.JVMHeapMemoryValues{
		Used:      heapUsed,
		Committed: heapCommitted,
		Max:       heapMax,
	})

	var heapUsage int64
	if stats.Heap.Max > 0 {
		percentage := (stats.Heap.Used / stats.Heap.Max) * 100
		heapUsage = common.FormatPercent(percentage)
	}
	contexts.JVM.HeapUsage.Set(c.State, labels, contexts.JVMHeapUsageValues{Usage: heapUsage})

	contexts.JVM.NonHeapMemory.Set(c.State, labels, contexts.JVMNonHeapMemoryValues{
		Used:      int64(stats.NonHeap.Used),
		Committed: int64(stats.NonHeap.Committed),
	})

	contexts.JVM.GCCycles.Set(c.State, labels, contexts.JVMGCCyclesValues{Collections: int64(stats.GC.Count)})
	contexts.JVM.GCTime.Set(c.State, labels, contexts.JVMGCTimeValues{Time: int64(stats.GC.Time)})

	contexts.JVM.Threads.Set(c.State, labels, contexts.JVMThreadsValues{
		Total:  int64(stats.Threads.Count),
		Daemon: int64(stats.Threads.Daemon),
	})

	contexts.JVM.ThreadStates.Set(c.State, labels, contexts.JVMThreadStatesValues{
		Peak:    int64(stats.Threads.Peak),
		Started: int64(stats.Threads.Started),
	})

	contexts.JVM.Classes.Set(c.State, labels, contexts.JVMClassesValues{
		Loaded:   int64(stats.Classes.Loaded),
		Unloaded: int64(stats.Classes.Unloaded),
	})

	contexts.JVM.ProcessCPU.Set(c.State, labels, contexts.JVMProcessCPUValues{
		Cpu: common.FormatPercent(stats.CPU.ProcessUsage),
	})

	contexts.JVM.Uptime.Set(c.State, labels, contexts.JVMUptimeValues{Uptime: int64(stats.Uptime)})
}

func (c *Collector) collectThreadPools(ctx context.Context) error {
	max := c.Config.MaxThreadPools
	pools, err := c.client.FetchThreadPools(ctx, max)
	if err != nil {
		return err
	}

	collected := 0
	for _, pool := range pools {
		name := strings.TrimSpace(pool.Name)
		if name == "" {
			continue
		}

		if c.poolSelector != nil && !c.poolSelector.MatchString(name) {
			continue
		}

		if max > 0 && collected >= max {
			break
		}

		labels := contexts.ThreadPoolsLabels{Pool: name}
		contexts.ThreadPools.Size.Set(c.State, labels, contexts.ThreadPoolsSizeValues{
			Size: int64(pool.PoolSize),
			Max:  int64(pool.MaximumPoolSize),
		})
		contexts.ThreadPools.Active.Set(c.State, labels, contexts.ThreadPoolsActiveValues{
			Active: int64(pool.ActiveCount),
		})

		collected++
	}

	return nil
}

func (c *Collector) collectJDBCPools(ctx context.Context) error {
	max := c.Config.MaxJDBCPools
	pools, err := c.client.FetchJDBCPools(ctx, max)
	if err != nil {
		return err
	}

	collected := 0
	const precision = 1000.0

	for _, pool := range pools {
		name := strings.TrimSpace(pool.Name)
		if name == "" {
			continue
		}

		if c.poolSelector != nil && !c.poolSelector.MatchString(name) {
			continue
		}

		if max > 0 && collected >= max {
			break
		}

		labels := contexts.JDBCLabels{Pool: name}

		contexts.JDBC.PoolSize.Set(c.State, labels, contexts.JDBCPoolSizeValues{
			Size: int64(pool.PoolSize),
		})

		contexts.JDBC.PoolUsage.Set(c.State, labels, contexts.JDBCPoolUsageValues{
			Active: int64(pool.NumConnectionsUsed),
			Free:   int64(pool.NumConnectionsFree),
		})

		contexts.JDBC.WaitTime.Set(c.State, labels, contexts.JDBCWaitTimeValues{
			Wait: int64(pool.AvgWaitTime * precision),
		})

		contexts.JDBC.UseTime.Set(c.State, labels, contexts.JDBCUseTimeValues{
			Use: int64(pool.AvgInUseTime * precision),
		})

		contexts.JDBC.ConnectionsTotals.Set(c.State, labels, contexts.JDBCConnectionsTotalsValues{
			Created:   int64(pool.NumConnectionsCreated),
			Destroyed: int64(pool.NumConnectionsDestroyed),
		})

		contexts.JDBC.WaitingThreads.Set(c.State, labels, contexts.JDBCWaitingThreadsValues{
			Waiting: int64(pool.WaitingThreadCount),
		})

		collected++
	}

	return nil
}

func (c *Collector) collectJCAPools(ctx context.Context) error {
	max := c.Config.MaxJCAPools
	pools, err := c.client.FetchJCAPools(ctx, max)
	if err != nil {
		return err
	}

	const precision = 1000.0
	collected := 0

	for _, pool := range pools {
		name := strings.TrimSpace(pool.Name)
		if name == "" {
			continue
		}

		if c.poolSelector != nil && !c.poolSelector.MatchString(name) {
			continue
		}

		if max > 0 && collected >= max {
			break
		}

		labels := contexts.JCALabels{Pool: name}

		contexts.JCA.PoolSize.Set(c.State, labels, contexts.JCAPoolSizeValues{Size: int64(pool.PoolSize)})

		contexts.JCA.PoolUsage.Set(c.State, labels, contexts.JCAPoolUsageValues{
			Active: int64(pool.NumConnectionsUsed),
			Free:   int64(pool.NumConnectionsFree),
		})

		contexts.JCA.WaitTime.Set(c.State, labels, contexts.JCAWaitTimeValues{
			Wait: int64(pool.AvgWaitTime * precision),
		})

		contexts.JCA.UseTime.Set(c.State, labels, contexts.JCAUseTimeValues{
			Use: int64(pool.AvgInUseTime * precision),
		})

		contexts.JCA.ConnectionsTotals.Set(c.State, labels, contexts.JCAConnectionsTotalsValues{
			Created:   int64(pool.NumConnectionsCreated),
			Destroyed: int64(pool.NumConnectionsDestroyed),
		})

		contexts.JCA.WaitingThreads.Set(c.State, labels, contexts.JCAWaitingThreadsValues{
			Waiting: int64(pool.WaitingThreadCount),
		})

		collected++
	}

	return nil
}

func (c *Collector) collectJMSDestinations(ctx context.Context) error {
	max := c.Config.MaxJMSDestinations
	dests, err := c.client.FetchJMSDestinations(ctx, max)
	if err != nil {
		return err
	}

	collected := 0

	for _, dest := range dests {
		name := strings.TrimSpace(dest.Name)
		if name == "" {
			continue
		}

		if c.jmsSelector != nil && !c.jmsSelector.MatchString(name) {
			continue
		}

		if max > 0 && collected >= max {
			break
		}

		typeLabel := strings.ToLower(strings.TrimSpace(dest.Type))
		if typeLabel == "" {
			typeLabel = "unknown"
		}

		labels := contexts.JMSLabels{
			Destination:      name,
			Destination_type: typeLabel,
		}

		contexts.JMS.MessagesCurrent.Set(c.State, labels, contexts.JMSMessagesCurrentValues{
			Current: int64(dest.MessagesCurrentCount),
		})

		contexts.JMS.MessagesPending.Set(c.State, labels, contexts.JMSMessagesPendingValues{
			Pending: int64(dest.MessagesPendingCount),
		})

		contexts.JMS.MessagesTotal.Set(c.State, labels, contexts.JMSMessagesTotalValues{
			Total: int64(dest.MessagesAddedCount),
		})

		contexts.JMS.Consumers.Set(c.State, labels, contexts.JMSConsumersValues{
			Consumers: int64(dest.ConsumerCount),
		})

		collected++
	}

	return nil
}

func (c *Collector) collectApplications(ctx context.Context) error {
	max := c.Config.MaxApplications
	apps, err := c.client.FetchApplications(ctx, max, c.Config.CollectSessionMetrics.IsEnabled(), c.Config.CollectTransactionMetrics.IsEnabled())
	if err != nil {
		return err
	}

	const precision = 1000.0
	collected := 0

	for _, app := range apps {
		name := strings.TrimSpace(app.Name)
		if name == "" {
			continue
		}

		if c.appSelector != nil && !c.appSelector.MatchString(name) {
			continue
		}

		if max > 0 && collected >= max {
			break
		}

		moduleName := strings.TrimSpace(app.Module)
		labels := contexts.ApplicationsLabels{
			Application: name,
			Module:      moduleName,
		}

		contexts.Applications.Requests.Set(c.State, labels, contexts.ApplicationsRequestsValues{
			Requests: int64(app.Requests),
		})

		contexts.Applications.ResponseTime.Set(c.State, labels, contexts.ApplicationsResponseTimeValues{
			Response_time: int64(app.ResponseTime * precision),
		})

		if c.Config.CollectSessionMetrics.IsEnabled() {
			contexts.Applications.SessionsActive.Set(c.State, labels, contexts.ApplicationsSessionsActiveValues{
				Active: int64(app.ActiveSessions),
			})

			contexts.Applications.SessionsLive.Set(c.State, labels, contexts.ApplicationsSessionsLiveValues{
				Live: int64(app.LiveSessions),
			})

			contexts.Applications.SessionEvents.Set(c.State, labels, contexts.ApplicationsSessionEventsValues{
				Creates:     int64(app.SessionCreates),
				Invalidates: int64(app.SessionInvalidates),
			})
		}

		if c.Config.CollectTransactionMetrics.IsEnabled() {
			contexts.Applications.Transactions.Set(c.State, labels, contexts.ApplicationsTransactionsValues{
				Committed:  int64(app.TransactionsCommitted),
				Rolledback: int64(app.TransactionsRolledback),
			})
		}

		collected++
	}

	return nil
}
