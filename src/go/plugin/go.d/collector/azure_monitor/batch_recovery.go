// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"time"
)

const maxRecoveryMetricsCalls = 8

type resourceChunk struct {
	chunk    *metricChunk
	resource string
}

// Different cohorts preserve resource splits: successful halves need not work together.
type requestShape struct {
	mask   uint32
	cohort uint64
}

type metricQuestion struct {
	recheckEarly bool // New ARM evidence may advance one data-plane check.
	id           uint64
	batch        queryBatch
	cohort       uint64
	turn         uint64         // Last admission, independent of ARM completion order.
	evidence     metricEvidence // Only equivalent evidence can share a corrective request.
	rejected     bool           // Refine this exact failed rectangle before asking again.
	lastPoll     uint64
	nextCheck    time.Time
	delay        time.Duration
}

type definitionAdvice struct {
	openingTurn    uint64
	definitions    metricDefinitions
	fetched, retry time.Time
	backoff        time.Duration
	turn           uint64
}

type recoveryState struct {
	// A missing entry means the configured full chunk is ordinary. An empty
	// entry means every name is presently owned by an outstanding question.
	ordinary       map[resourceChunk][]requestShape
	questions      map[*metricQuestion]struct{}
	advice         map[definitionKey]*definitionAdvice
	holds          serviceHolds
	sequence, poll uint64
}

func (s *recoveryState) ticket() uint64 {
	s.sequence++
	return s.sequence
}

func (c *Collector) definitionTTL() time.Duration {
	if c.Discovery.RefreshEvery == 0 {
		return secondsToDuration(defaultDiscoveryEvery)
	}
	return secondsToDuration(c.Discovery.RefreshEvery)
}

func selectNames(batch queryBatch, mask uint32) queryBatch {
	if batch.mask == mask {
		return batch
	}
	batch.mask = mask
	batch.Metrics = nil
	batch.MetricNames = nil
	for i, m := range batch.chunk.metrics {
		if mask&(1<<i) != 0 {
			batch.Metrics = append(batch.Metrics, m)
			batch.MetricNames = append(batch.MetricNames, m.AzureName)
		}
	}
	return batch
}

type plannedRequest struct {
	deadline  time.Time // A continuation inherits the original operation deadline.
	batch     queryBatch
	cohort    uint64
	questions []*metricQuestion // Original ownership survives a combined wire request.
}

func (s *recoveryState) plan(batches []queryBatch, now time.Time, maxResources int) []plannedRequest {
	s.holds.expire(now)
	out := make([]plannedRequest, 0, len(batches))
	if len(s.ordinary) == 0 && !s.holds.hasMetrics() {
		for _, b := range batches {
			out = append(out, plannedRequest{batch: b})
		}
		return out
	}
	// Fill compatible ordinary requests across the original resource page
	// boundaries. Resource splits learned from actual rejections stay distinct.
	type packingKey struct {
		chunk                *metricChunk
		subscription, region string
		shape                requestShape
	}
	open := make(map[packingKey]int)
	for _, b := range batches {
		for i, r := range b.Resources {
			one := b
			one.Resources = b.Resources[i : i+1]
			held := s.holds.metricMask(one, now)
			shapes, ok := s.ordinary[resourceChunk{b.chunk, r.idKey}]
			if !ok {
				shapes = []requestShape{{mask: b.mask}}
			}
			for _, shape := range shapes {
				shape.mask &^= held
				if shape.mask == 0 {
					continue
				}
				k := packingKey{b.chunk, b.SubscriptionID, b.Region, shape}
				index, ok := open[k]
				if !ok {
					batch := selectNames(b, shape.mask)
					batch.Resources = nil
					index = len(out)
					out = append(out, plannedRequest{batch: batch, cohort: shape.cohort})
					open[k] = index
				}
				batch := &out[index].batch
				batch.Resources = append(batch.Resources, r)
				if len(batch.Resources) == maxResources {
					delete(open, k)
				}
			}
		}
	}
	return out
}

func (s *recoveryState) demote(b queryBatch) {
	if s.ordinary == nil {
		s.ordinary = make(map[resourceChunk][]requestShape)
	}
	for _, r := range b.Resources {
		k := resourceChunk{b.chunk, r.idKey}
		shapes, ok := s.ordinary[k]
		if !ok {
			shapes = []requestShape{{mask: 1<<len(b.chunk.metrics) - 1}}
		}
		keep := shapes[:0]
		for _, shape := range shapes {
			shape.mask &^= b.mask
			if shape.mask != 0 {
				keep = append(keep, shape)
			}
		}
		s.ordinary[k] = keep
	}
}

func (s *recoveryState) promote(b queryBatch, cohort uint64) {
	for _, r := range b.Resources {
		k := resourceChunk{b.chunk, r.idKey}
		// Never OR together two separate successes. Their union may be the
		// very request that Azure rejected.
		if cohort == 0 && b.mask == 1<<len(b.chunk.metrics)-1 {
			delete(s.ordinary, k)
			continue
		}
		s.ordinary[k] = append(s.ordinary[k], requestShape{b.mask, cohort})
	}
}

func (s *recoveryState) add(q *metricQuestion) *metricQuestion {
	if s.questions == nil {
		s.questions = make(map[*metricQuestion]struct{})
	}
	q.id = s.ticket()
	s.questions[q] = struct{}{}
	return q
}

// Only current discovery members own evidence. Server holds expire separately.
func (s *recoveryState) reconcile(byProfile map[string][]resourceInfo) {
	if len(s.ordinary) == 0 && len(s.questions) == 0 && len(s.advice) == 0 {
		return
	}
	inventory := make(map[string]map[string]resourceInfo, len(byProfile))
	for profile, resources := range byProfile {
		rows := make(map[string]resourceInfo, len(resources))
		for _, r := range resources {
			rows[r.idKey] = r
		}
		inventory[profile] = rows
	}
	for k := range s.ordinary {
		if _, ok := inventory[k.chunk.profile.Name][k.resource]; !ok {
			delete(s.ordinary, k)
		}
	}
	questions := s.questions
	s.questions = make(map[*metricQuestion]struct{}, len(questions))
	for q := range questions {
		rows := inventory[q.batch.Profile.Name]
		lanes := make(map[subscriptionRegionKey][]resourceInfo)
		for _, old := range q.batch.Resources {
			if r, ok := rows[old.idKey]; ok {
				key := subscriptionRegionKey{stringsTrim(r.SubscriptionID), normalizeRegion(r.Region)}
				lanes[key] = append(lanes[key], r)
			}
		}
		for lane, rs := range lanes {
			child := *q
			child.batch.SubscriptionID, child.batch.Region, child.batch.Resources = lane.SubscriptionID, lane.Region, rs
			if len(rs) < len(q.batch.Resources) {
				child.rejected = false
				child.cohort = s.ticket()
			}
			if len(lanes) > 1 {
				child.id = s.ticket()
			}
			s.questions[&child] = struct{}{}
		}
	}
	s.pruneAdvice()
}

func (s *recoveryState) pruneAdvice() {
	if len(s.advice) == 0 {
		return
	}
	needed := make(map[definitionKey]bool)
	for q := range s.questions {
		for _, r := range q.batch.Resources {
			needed[r.definitionKey(q.batch.Profile)] = true
		}
	}
	for k := range s.advice {
		if !needed[k] {
			delete(s.advice, k)
		}
	}
}
