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
// region+period are its scheduling group (drives retention re-emit scheduling).
type querySample struct {
	seriesName string
	labels     []metrix.Label
	value      float64
	region     string
	period     int
}

// plannedQuery ties a synthetic GetMetricData query Id back to the series it
// populates and the region/period that determine its client and window.
type plannedQuery struct {
	id         string
	region     string
	period     int
	seriesName string
	labels     []metrix.Label
	nilAsZero  bool // record 0 (vs a gap) when the query returns no datapoint
	query      cwtypes.MetricDataQuery
}

// queryGroupKey groups queries that share a CloudWatch client and time window;
// it is also the per-(region, period) scheduling unit.
type queryGroupKey struct {
	region string
	period int
}

func (q plannedQuery) groupKey() queryGroupKey {
	return queryGroupKey{region: q.region, period: q.period}
}

func (s querySample) groupKey() queryGroupKey {
	return queryGroupKey{region: s.region, period: s.period}
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

// buildQueryPlan builds one GetMetricData query per (instance × metric ×
// statistic) across the discovery snapshot. Query Ids are q0..qN; each maps back
// to its series name and instance labels via the returned plannedQuery.
func (c *Collector) buildQueryPlan() []plannedQuery {
	var plan []plannedQuery
	idx := 0
	for _, prof := range c.profiles {
		nDims := len(prof.Config.Instance.Dimensions)
		for _, region := range c.regions() {
			instances := c.discovery.Instances[discoveryKey{Profile: prof.Name, Region: region}]
			for _, inst := range instances {
				if len(inst.DimensionValues) != nDims {
					continue // defensive: snapshot/profile mismatch
				}
				labels, dims := c.instanceLabelsAndDims(prof, region, inst)
				plan = append(plan, c.metricQueries(prof, region, labels, dims, &idx)...)
			}
		}
	}
	return plan
}

// instanceLabelsAndDims builds the metrix identity labels
// ({account_id, region, <identifying dimension labels>}) and the full CloudWatch
// dimension set for one discovered instance. Constant (match-and-query-only)
// dimensions are included in the query dimensions but omitted from the labels. The
// returned label slice is shared read-only by all of the instance's planned
// queries, so callers must not append to it.
func (c *Collector) instanceLabelsAndDims(prof cwprofiles.ResolvedProfile, region string, inst discoveredInstance) ([]metrix.Label, []cwtypes.Dimension) {
	pdims := prof.Config.Instance.Dimensions
	dims := make([]cwtypes.Dimension, len(pdims))
	labels := make([]metrix.Label, 0, len(pdims)+2)
	labels = append(labels,
		metrix.Label{Key: "account_id", Value: c.accountID},
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
func (c *Collector) metricQueries(prof cwprofiles.ResolvedProfile, region string, labels []metrix.Label, dims []cwtypes.Dimension, idx *int) []plannedQuery {
	var out []plannedQuery
	for _, m := range prof.Config.Metrics {
		period := prof.Config.EffectivePeriod(m)
		for _, stat := range m.Statistics {
			token := cwprofiles.NormalizeStatistic(stat)
			id := fmt.Sprintf("q%d", *idx)
			*idx++
			out = append(out, plannedQuery{
				id:         id,
				region:     region,
				period:     period,
				seriesName: cwprofiles.ExportedSeriesName(prof.Name, m.ID, token),
				labels:     labels,
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
