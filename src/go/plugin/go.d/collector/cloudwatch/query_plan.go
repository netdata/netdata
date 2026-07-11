// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// querySample is one observed value for a series identity in a collect cycle.
// account+region+period is its scheduling group (drives retention re-emit scheduling).
type querySample struct {
	seriesName string
	labels     []metrix.Label
	tagLabels  []metrix.Label // non-identity enrichment labels; emitted but not in observedKey
	value      float64
	target     string
	account    string
	region     string
	period     int
}

// plannedQuery ties a synthetic GetMetricData query Id back to the series it
// populates and the account/region/period that determine its client and window.
type plannedQuery struct {
	id         string
	target     string
	account    string
	region     string
	period     int
	seriesName string
	labels     []metrix.Label
	tagLabels  []metrix.Label // non-identity enrichment labels; emitted but not in observedKey
	nilAsZero  bool           // record 0 (vs a gap) when the query returns no datapoint
	query      cwtypes.MetricDataQuery
}

// queryGroupKey groups queries that share a target client and time window.
type queryGroupKey struct {
	target string
	region string
	period int
}

func (q plannedQuery) groupKey() queryGroupKey {
	return queryGroupKey{target: q.target, region: q.region, period: q.period}
}

func (s querySample) groupKey() queryGroupKey {
	return queryGroupKey{target: s.target, region: s.region, period: s.period}
}

func (c *Collector) invalidateQueryPlan() {
	c.planDirty = true
}

// currentQueryPlan returns the immutable query blueprint for the current target,
// discovery, and tag snapshots. Those inputs change on a much slower cadence than
// Collect, so rebuilding only on invalidation avoids repeating per-series
// allocations during every collection cycle.
func (c *Collector) currentQueryPlan() []plannedQuery {
	if !c.planDirty {
		return c.queryPlan
	}
	c.queryPlan = c.buildQueryPlan()
	c.planDirty = false
	return c.queryPlan
}

// queryWindow computes the GetMetricData time window for a metric of the given
// period: effective_offset = max(query_offset, period); end = align(now −
// effective_offset, period); start = end − period. Aligning the end to a period
// boundary AFTER the offset keeps the window period-aligned even when query_offset
// is not a multiple of the period (GetMetricData expects period-aligned bounds),
// and an offset ≥ period keeps the queried bucket already published (e.g. S3 daily
// reads a full, settled day rather than a partial current one).
func queryWindow(now time.Time, period, queryOffset int) (start, end time.Time) {
	periodSec := int64(period)
	effectiveOffset := max(periodSec, int64(queryOffset))
	endSec := now.Unix() - effectiveOffset
	endSec -= endSec % periodSec // align to a period boundary after the offset
	return time.Unix(endSec-periodSec, 0).UTC(), time.Unix(endSec, 0).UTC()
}

// buildQueryPlan follows compiled scope order and keeps the first query for each
// final emitted identity. This resolves dynamic overlap when distinct targets later
// resolve to the same account and discover the same resource.
func (c *Collector) buildQueryPlan() []plannedQuery {
	if c.runtime == nil {
		return nil
	}
	var plan []plannedQuery
	idx := 0
	seen := make(map[string]struct{})
	shadowed := 0
	for _, scope := range c.runtime.Scopes {
		resolved, ok := c.resolvedTargetByRef(scope.Target.Name)
		if !ok {
			continue
		}
		prof := scope.Profile
		nDims := len(prof.Config.Instance.Dimensions)
		instances := c.discovery.Instances[discoveryKey{Target: scope.Target.Name, Profile: prof.Name, Region: scope.Region}]
		for _, inst := range instances {
			if len(inst.DimensionValues) != nDims {
				continue // defensive: snapshot/profile mismatch
			}
			labels, dims := c.instanceLabelsAndDims(resolved.accountID, prof, scope.Region, inst)
			tagLabels := c.tagLabelsFor(scope.Target.Name, resolved.accountID, scope.Region, prof, inst.DimensionValues)
			queries := c.metricQueries(scope.Target.Name, resolved.accountID, prof, scope.Region, labels, tagLabels, dims, &idx)
			for _, query := range queries {
				key := finalSeriesKey(query.seriesName, query.labels)
				if _, ok := seen[key]; ok {
					shadowed++
					continue
				}
				seen[key] = struct{}{}
				plan = append(plan, query)
			}
		}
	}
	if shadowed > 0 {
		c.Limit(logKeyRuleShadowed, 1, recurringLogEvery).
			Warningf("CloudWatch collection rules shadowed %d duplicate final series; earliest rule/target order owns each series", shadowed)
	}
	return plan
}

func finalSeriesKey(seriesName string, labels []metrix.Label) string {
	key := seriesName
	for _, label := range labels {
		key += "\x00" + label.Key + "\x00" + label.Value
	}
	return key
}

// instanceLabelsAndDims builds the metrix identity labels
// ({account_id, region, <identifying dimension labels>}) and the full CloudWatch
// dimension set for one discovered instance. Constant (match-and-query-only)
// dimensions are included in the query dimensions but omitted from the labels. The
// returned label slice is shared read-only by all of the instance's planned
// queries, so callers must not append to it.
func (c *Collector) instanceLabelsAndDims(accountID string, prof cwprofiles.ResolvedProfile, region string, inst discoveredInstance) ([]metrix.Label, []cwtypes.Dimension) {
	pdims := prof.Config.Instance.Dimensions
	dims := make([]cwtypes.Dimension, len(pdims))
	labels := make([]metrix.Label, 0, len(pdims)+2)
	labels = append(labels,
		metrix.Label{Key: "account_id", Value: accountID},
		metrix.Label{Key: "region", Value: region},
	)
	for i, d := range pdims {
		value := inst.DimensionValues[i]
		dims[i] = cwtypes.Dimension{Name: aws.String(d.Name), Value: aws.String(value)}
		if !d.IsConstant() { // constant dims are queried but not emitted as labels
			labels = append(labels, metrix.Label{Key: d.Label, Value: value})
		}
	}
	return labels, dims
}

// metricQueries builds the planned queries for one instance: one per
// (metric × statistic), allocating sequential q<idx> ids through idx.
func (c *Collector) metricQueries(targetRef, accountID string, prof cwprofiles.ResolvedProfile, region string, labels, tagLabels []metrix.Label, dims []cwtypes.Dimension, idx *int) []plannedQuery {
	var out []plannedQuery
	for _, m := range prof.Config.Metrics {
		period := prof.Config.EffectivePeriod(m)
		for _, stat := range m.Statistics {
			token := cwprofiles.NormalizeStatistic(stat)
			id := fmt.Sprintf("q%d", *idx)
			*idx++
			out = append(out, plannedQuery{
				id:         id,
				target:     targetRef,
				account:    accountID,
				region:     region,
				period:     period,
				seriesName: cwprofiles.ExportedSeriesName(prof.Name, m.ID, token),
				labels:     labels,
				tagLabels:  tagLabels,
				nilAsZero:  m.EmitZeroOnNoData(token),
				query: cwtypes.MetricDataQuery{
					Id: aws.String(id),
					MetricStat: &cwtypes.MetricStat{
						Metric: &cwtypes.Metric{
							Namespace:  aws.String(prof.Config.Namespace),
							MetricName: aws.String(m.MetricName),
							Dimensions: dims,
						},
						Period: aws.Int32(int32(period)),
						Stat:   aws.String(cwprofiles.StatString(token)),
					},
				},
			})
		}
	}
	return out
}
