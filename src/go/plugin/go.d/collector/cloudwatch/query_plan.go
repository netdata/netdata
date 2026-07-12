// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// plannedQuery is one stable series query. Request-local IDs and batch membership
// are assigned only when a due query is executed.
type plannedQuery struct {
	key        string
	billingKey string // structural metric identity without statistic; internal AWS cost grouping
	target     string
	region     string
	policy     queryPolicy
	seriesName string
	labels     []metrix.Label
	tagLabels  []metrix.Label // non-identity enrichment labels; emitted but not in observedKey
	nilAsZero  bool           // record 0 (vs a gap) when the query returns no datapoint
	rate       bool           // normalize a per-period total to a per-second value on write
	query      cwtypes.MetricDataQuery
}

type instancePresentation struct {
	labels    []metrix.Label
	tagLabels []metrix.Label
	dims      []cwtypes.Dimension
}

type queryPlanCandidate struct {
	targetRef    string
	profile      cwprofiles.ResolvedProfile
	region       string
	series       []compiledSeries
	billingKeys  []string
	presentation instancePresentation
}

type seriesOwnership struct {
	firstWord uint64
	overflow  []uint64
}

func (o *seriesOwnership) claim(ordinal int) bool {
	word := ordinal / 64
	mask := uint64(1) << uint(ordinal%64)
	if word == 0 {
		if o.firstWord&mask != 0 {
			return false
		}
		o.firstWord |= mask
		return true
	}

	index := word - 1
	if index >= len(o.overflow) {
		o.overflow = append(o.overflow, make([]uint64, index-len(o.overflow)+1)...)
	}
	if o.overflow[index]&mask != 0 {
		return false
	}
	o.overflow[index] |= mask
	return true
}

func (c *Collector) invalidateQueryPlan() {
	c.planDirty = true
}

// currentQueryPlan returns the immutable query blueprint for the current target,
// discovery, and tag snapshots. Those inputs change on a much slower cadence than
// Collect, so rebuilding only on invalidation avoids repeating per-series
// allocations during every collection cycle.
func (c *Collector) currentQueryPlan() ([]plannedQuery, error) {
	if !c.planDirty {
		return c.queryPlan, nil
	}
	next, err := c.buildQueryPlan()
	if err != nil {
		return nil, err
	}
	c.queryPlan = next
	c.observations.reconcilePlan(c.queryPlan)
	c.planDirty = false
	return c.queryPlan, nil
}

// buildQueryPlan follows compiled scope order and assigns each selected exported
// series to its earliest matching rule. An unknown tag-filter result reserves only
// that scope's selected series from lower rules, which keeps failures fail-closed
// without blocking disjoint selections.
func (c *Collector) buildQueryPlan() ([]plannedQuery, error) {
	if c.plan == nil {
		return nil, nil
	}
	budget := newQueryWorkBudget()
	var candidates []queryPlanCandidate
	owned := make(map[string]seriesOwnership)
	presentations := make(map[string]instancePresentation)
	shadowed := 0
	reserved := 0
	maxInstances := c.Limits.MaxInstances
	if maxInstances <= 0 {
		maxInstances = defaultMaxInstances
	}
	for _, scope := range c.plan.Scopes {
		resolved, ok := c.resolvedTargetByRef(scope.Target.Name)
		if !ok {
			continue
		}
		prof := scope.Profile
		nDims := len(prof.Config.Instance.Dimensions)
		dimNames := prof.Config.DimensionNames()
		join := c.plan.TagJoins[prof.Name]
		membershipUnknown := scope.hasTagFilter() && c.tags.membershipUnknown(scope.TagMembershipID)
		instances := c.discovery.Instances[discoveryKey{Target: scope.Target.Name, Profile: prof.Name, Region: scope.Region}]
		for _, inst := range instances {
			if len(inst.DimensionValues) != nDims {
				continue // defensive: snapshot/profile mismatch
			}
			instanceKey := finalInstanceKey(prof.Name, resolved.accountID, scope.Region, prof.Config.Instance.Dimensions, inst.DimensionValues)
			if scope.hasTagFilter() {
				if join == nil {
					reserved += reserveSelectedSeries(owned, instanceKey, scope.SelectedSeries)
					continue
				}
				joinKey, ok := join.instanceJoinKey(dimNames, inst.DimensionValues)
				if !ok {
					reserved += reserveSelectedSeries(owned, instanceKey, scope.SelectedSeries)
					continue
				}
				selected := c.tags.membershipSelected(scope.TagMembershipID, joinKey)
				if !selected && !membershipUnknown {
					continue
				}
				if !selected {
					reserved += reserveSelectedSeries(owned, instanceKey, scope.SelectedSeries)
					continue
				}
			}

			selected := make([]compiledSeries, 0, len(scope.SelectedSeries))
			ownership := owned[instanceKey]
			for _, series := range scope.SelectedSeries {
				if !ownership.claim(series.Ordinal) {
					shadowed++
					continue
				}
				selected = append(selected, series)
			}
			owned[instanceKey] = ownership
			if len(selected) == 0 {
				continue
			}

			presentation, ok := presentations[instanceKey]
			if !ok {
				if len(presentations) == maxInstances {
					return nil, fmt.Errorf("CloudWatch query plan contains more than limits.max_instances=%d final instances", maxInstances)
				}
				labels, dims := c.instanceLabelsAndDims(resolved.accountID, prof, scope.Region, inst)
				presentation = instancePresentation{
					labels: labels, dims: dims,
					tagLabels: c.tagLabelsFor(scope.Target.Name, resolved.accountID, scope.Region, prof, join, inst.DimensionValues),
				}
				presentations[instanceKey] = presentation
			}
			billingKeys := make([]string, len(selected))
			byMetric := make([]string, len(prof.Config.Metrics))
			for i, series := range selected {
				if err := budget.reserveQuery(series.Policy); err != nil {
					return nil, err
				}
				metric := prof.Config.Metrics[series.MetricIndex]
				billingKey := byMetric[series.MetricIndex]
				if billingKey == "" {
					billingKey = metricBillingKey(prof.Config.Namespace, metric.MetricName, presentation.dims)
					byMetric[series.MetricIndex] = billingKey
				}
				billingKeys[i] = billingKey
				budget.addBillingGroup(
					queryBatchKey{target: scope.Target.Name, region: scope.Region, policy: series.Policy},
					billingKey,
				)
			}
			candidates = append(candidates, queryPlanCandidate{
				targetRef: scope.Target.Name, profile: prof, region: scope.Region,
				series: selected, billingKeys: billingKeys, presentation: presentation,
			})
		}
	}
	if err := budget.validateBatches(); err != nil {
		return nil, err
	}
	if shadowed > 0 {
		c.Limit(logKeyRuleShadowed, 1, recurringLogEvery).
			Warningf("CloudWatch collection rules shadowed %d duplicate exported series; earliest matching rule/target order owns each series", shadowed)
	}
	if reserved > 0 {
		c.Limit(logKeyTagRefreshFailed+"_reserved", 1, recurringLogEvery).
			Warningf("CloudWatch resource tag filtering reserved %d exported series from lower-priority rules while membership was unknown", reserved)
	}
	plan := make([]plannedQuery, 0, budget.queries)
	for _, candidate := range candidates {
		plan = append(plan, buildSeriesQueries(candidate)...)
	}
	if err := validatePlannedQueryWork(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func reserveSelectedSeries(owned map[string]seriesOwnership, instanceKey string, series []compiledSeries) int {
	ownership := owned[instanceKey]
	reserved := 0
	for _, item := range series {
		if !ownership.claim(item.Ordinal) {
			continue
		}
		reserved++
	}
	owned[instanceKey] = ownership
	return reserved
}

func finalInstanceKey(profileName, accountID, region string, dimensions []cwprofiles.InstanceDimension, values []string) string {
	var key strings.Builder
	writeLengthPrefixed(&key, profileName)
	writeLengthPrefixed(&key, accountID)
	writeLengthPrefixed(&key, region)
	for i, dimension := range dimensions {
		if dimension.IsConstant() {
			continue
		}
		writeLengthPrefixed(&key, dimension.Label)
		writeLengthPrefixed(&key, values[i])
	}
	return key.String()
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

// buildSeriesQueries constructs the stable queries only after the candidate set
// has passed the query/datapoint/batch preflight.
func buildSeriesQueries(candidate queryPlanCandidate) []plannedQuery {
	out := make([]plannedQuery, 0, len(candidate.series))
	for i, selected := range candidate.series {
		m := candidate.profile.Config.Metrics[selected.MetricIndex]
		pq := plannedQuery{
			target:     candidate.targetRef,
			region:     candidate.region,
			policy:     selected.Policy,
			billingKey: candidate.billingKeys[i],
			seriesName: selected.Name,
			labels:     candidate.presentation.labels,
			tagLabels:  candidate.presentation.tagLabels,
			nilAsZero:  m.EmitZeroOnNoData(selected.Statistic),
			rate:       m.Rate && cwprofiles.IsPerPeriodTotal(selected.Statistic),
			query: cwtypes.MetricDataQuery{
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  aws.String(candidate.profile.Config.Namespace),
						MetricName: aws.String(m.MetricName),
						Dimensions: candidate.presentation.dims,
					},
					Period: aws.Int32(int32(selected.Policy.period / time.Second)),
					Stat:   aws.String(cwprofiles.StatString(selected.Statistic)),
				},
			},
		}
		pq.key = plannedQueryKey(pq)
		out = append(out, pq)
	}
	return out
}

// plannedQueryKey identifies execution state independently of rule names,
// scope order, request-local IDs, and batch membership.
func plannedQueryKey(q plannedQuery) string {
	var key strings.Builder
	writeLengthPrefixed(&key, q.target)
	writeLengthPrefixed(&key, q.region)
	writeLengthPrefixed(&key, q.seriesName)
	for _, label := range q.labels {
		writeLengthPrefixed(&key, label.Key)
		writeLengthPrefixed(&key, label.Value)
	}
	writeLengthPrefixed(&key, strconv.FormatInt(int64(q.policy.period/time.Second), 10))
	writeLengthPrefixed(&key, strconv.FormatInt(int64(q.policy.lookback/time.Second), 10))
	writeLengthPrefixed(&key, strconv.FormatInt(int64(q.policy.publicationDelay/time.Second), 10))
	writeLengthPrefixed(&key, queryNamespace(q.query))
	if q.query.MetricStat != nil {
		writeLengthPrefixed(&key, aws.ToString(q.query.MetricStat.Stat))
		writeLengthPrefixed(&key, strconv.FormatInt(int64(aws.ToInt32(q.query.MetricStat.Period)), 10))
		if metric := q.query.MetricStat.Metric; metric != nil {
			writeLengthPrefixed(&key, aws.ToString(metric.MetricName))
			for _, dim := range metric.Dimensions {
				writeLengthPrefixed(&key, aws.ToString(dim.Name))
				writeLengthPrefixed(&key, aws.ToString(dim.Value))
			}
		}
	}
	return key.String()
}
