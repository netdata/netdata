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

// Resource splits with the same names may work in one request again once the
// cause of the original rejection is isolated. A union is tried after the
// definition TTL and, if Azure rejects it, with the delays used for rechecks.
type mergeKey struct {
	chunk                *metricChunk
	subscription, region string
	mask                 uint32
}

type mergeState struct {
	next  time.Time
	delay time.Duration
}

type mergeAttempt struct {
	key      mergeKey
	fallback []plannedRequest // The requests the union replaces.
}

type recoveryState struct {
	// A missing entry means the configured full chunk is ordinary. An empty
	// entry means every name is presently owned by an outstanding question.
	ordinary       map[resourceChunk][]requestShape
	questions      map[*metricQuestion]struct{}
	advice         map[definitionKey]*definitionAdvice
	merges         map[mergeKey]*mergeState
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
	deadline  time.Time // Ordinary work released from a hold ends with the poll's recovery deadline.
	resent    bool      // Already repeated in this poll after a throttle.
	batch     queryBatch
	cohort    uint64
	questions []*metricQuestion // Original ownership survives a combined wire request.
	merge     *mergeAttempt     // A trial union of ordinary requests.
}

func (s *recoveryState) plan(batches []queryBatch, now time.Time, maxResources int, ttl time.Duration) []plannedRequest {
	s.holds.expire(now)
	out := make([]plannedRequest, 0, len(batches))
	if len(s.ordinary) == 0 {
		for _, b := range batches {
			out = append(out, plannedRequest{batch: b})
		}
		return out
	}
	// Fill compatible ordinary requests across the original resource page
	// boundaries. Resource splits learned from actual rejections stay distinct.
	type packingKey struct {
		mergeKey
		cohort uint64
	}
	open := make(map[packingKey]int)
	var keys []packingKey
	for _, b := range batches {
		for _, r := range b.Resources {
			shapes, ok := s.ordinary[resourceChunk{b.chunk, r.idKey}]
			if !ok {
				shapes = []requestShape{{mask: b.mask}}
			}
			for _, shape := range shapes {
				k := packingKey{mergeKey{b.chunk, b.SubscriptionID, b.Region, shape.mask}, shape.cohort}
				index, ok := open[k]
				if !ok {
					batch := selectNames(b, shape.mask)
					batch.Resources = nil
					index = len(out)
					out = append(out, plannedRequest{batch: batch, cohort: shape.cohort})
					keys = append(keys, k)
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
	return s.planMerges(batches, out, func(i int) (mergeKey, uint64) { return keys[i].mergeKey, keys[i].cohort }, now, maxResources, ttl)
}

// Replace same-name requests of different cohorts with one trial union per
// key when it is due and fits a request.
func (s *recoveryState) planMerges(batches []queryBatch, out []plannedRequest, key func(int) (mergeKey, uint64), now time.Time, maxResources int, ttl time.Duration) []plannedRequest {
	due := make(map[*metricChunk]bool, len(batches))
	for _, b := range batches {
		due[b.chunk] = true
	}
	groups := make(map[mergeKey][]int)
	var order []mergeKey
	for i := range out {
		k, _ := key(i)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	// Forget keys that no longer have several cohorts. A chunk that is not due
	// in this poll keeps its schedule.
	for k := range s.merges {
		if due[k.chunk] && !severalCohorts(groups[k], key) {
			delete(s.merges, k)
		}
	}
	merged := make(map[int]bool)
	for _, k := range order {
		members := groups[k]
		if !severalCohorts(members, key) {
			continue
		}
		state := s.merges[k]
		if state == nil {
			if s.merges == nil {
				s.merges = make(map[mergeKey]*mergeState)
			}
			s.merges[k] = &mergeState{next: now.Add(ttl), delay: ttl}
			continue
		}
		if now.Before(state.next) {
			continue
		}
		var union []int
		size := 0
		for _, i := range members {
			if n := len(out[i].batch.Resources); size+n <= maxResources {
				union = append(union, i)
				size += n
			}
		}
		if !severalCohorts(union, key) {
			continue
		}
		request := plannedRequest{batch: out[union[0]].batch, merge: &mergeAttempt{key: k}}
		request.batch.Resources = make([]resourceInfo, 0, size)
		for _, i := range union {
			request.batch.Resources = append(request.batch.Resources, out[i].batch.Resources...)
			request.merge.fallback = append(request.merge.fallback, out[i])
			merged[i] = true
		}
		out = append(out, request)
	}
	if len(merged) == 0 {
		return out
	}
	kept := out[:0]
	for i, r := range out {
		if !merged[i] {
			kept = append(kept, r)
		}
	}
	return kept
}

func severalCohorts(members []int, key func(int) (mergeKey, uint64)) bool {
	for _, i := range members[min(1, len(members)):] {
		_, first := key(members[0])
		if _, cohort := key(i); cohort != first {
			return true
		}
	}
	return false
}

// The members of an accepted union form one cohort from now on.
func (s *recoveryState) adopt(b queryBatch, cohort uint64) {
	for _, r := range b.Resources {
		k := resourceChunk{b.chunk, r.idKey}
		shapes, ok := s.ordinary[k]
		if !ok {
			s.ordinary[k] = []requestShape{{b.mask, cohort}}
			continue
		}
		for i := range shapes {
			if shapes[i].mask == b.mask {
				shapes[i].cohort = cohort
			}
		}
	}
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
