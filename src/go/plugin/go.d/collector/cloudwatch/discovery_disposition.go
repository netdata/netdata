// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

type discoveryDispositionKind uint8

const (
	discoveryDispositionInstall discoveryDispositionKind = iota + 1
	discoveryDispositionRetry
	discoveryDispositionFail
)

type discoveryDisposition struct {
	kind         discoveryDispositionKind
	snapshot     discoverySnapshot
	discoverySig string
	retryAt      time.Time
	err          error
}

type discoveryReport struct {
	total   int
	scopes  int
	summary string
	sig     string
	changed bool
}

type discoveryDiagnostics struct {
	aggregateFailure error
	groupFailures    []operationFailure
	report           *discoveryReport
}

func (c *Collector) aggregateFailureDisposition(err error, canContinue bool) discoveryDisposition {
	if c.discovery.FetchedAt.IsZero() && !canContinue {
		return discoveryDisposition{
			kind: discoveryDispositionFail,
			err:  fmt.Errorf("CloudWatch discovery refresh failed: %w", sanitizeAWSError(err)),
		}
	}
	return discoveryDisposition{
		kind:    discoveryDispositionRetry,
		retryAt: c.now().Add(time.Duration(c.Discovery.RefreshEvery) * time.Second),
	}
}

// applyDiscoveryDisposition is the single parent-cancellation linearization
// point for discovery. Diagnostics and all other effectful work must finish
// before this call; no effect may be added between the gate and the disposition.
func (c *Collector) applyDiscoveryDisposition(ctx context.Context, disposition discoveryDisposition) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch disposition.kind {
	case discoveryDispositionInstall:
		c.discovery = disposition.snapshot
		c.discoverySig = disposition.discoverySig
		c.invalidateTagFetchPlan()
		c.markTagsStale()
		c.invalidateQueryPlan()
		return nil
	case discoveryDispositionRetry:
		c.discovery.ExpiresAt = disposition.retryAt
		return nil
	case discoveryDispositionFail:
		return disposition.err
	default:
		panic("invalid CloudWatch discovery disposition")
	}
}

func newDiscoveryReport(snap discoverySnapshot, previousSig string) discoveryReport {
	byProfile := make(map[string]int)
	for key, instances := range snap.Instances {
		byProfile[key.Profile] += len(instances)
	}
	names := make([]string, 0, len(byProfile))
	for name := range byProfile {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%d", name, byProfile[name])
	}
	summary := b.String()
	total := snap.totalInstances()
	sig := fmt.Sprintf("%d|%s", total, summary)
	return discoveryReport{
		total: total, scopes: len(snap.Instances), summary: summary,
		sig: sig, changed: sig != previousSig,
	}
}

func (c *Collector) emitDiscoveryDiagnostics(diagnostics discoveryDiagnostics) {
	if diagnostics.aggregateFailure != nil {
		c.Limit(logKeyDiscoveryGroupFailed+"_aggregate", 1, recurringLogEvery).
			Warningf("CloudWatch discovery refresh was discarded atomically: %v", sanitizeAWSError(diagnostics.aggregateFailure))
	}
	c.warnOperationFailures(logKeyDiscoveryGroupFailed, "discovery", " (retaining previous dynamic instances where available)", diagnostics.groupFailures)
	if diagnostics.report == nil {
		return
	}
	report := diagnostics.report
	c.Debugf("CloudWatch discovery: %d instance(s) across %d (target, profile, region) scope(s): %s",
		report.total, report.scopes, report.summary)
	if report.changed {
		c.Infof("CloudWatch discovered %d instance(s): %s", report.total, report.summary)
	}
	if report.total >= highInstanceCountWarn {
		c.Limit(logKeyHighInstanceCount, 1, recurringLogEvery).
			Warningf("CloudWatch discovered %d instances; this scales GetMetricData cost — narrow collection rules if this is unexpected", report.total)
	}
}
