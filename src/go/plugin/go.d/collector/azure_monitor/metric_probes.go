// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"cmp"
	"maps"
	"math/bits"
	"slices"
	"time"
)

// A request may pack only selections with the same evidence. Two kinds of
// historical observation have the same priority, but disagree on ARM advice.
type metricEvidence uint8

const (
	advertisedMetrics metricEvidence = iota
	measuredWithoutAdvice
	measuredButOmitted
	unknownMetrics
	unadvertisedMetrics
	metricEvidenceCount
)

func (e metricEvidence) priority() int {
	switch e {
	case advertisedMetrics:
		return 0
	case measuredWithoutAdvice, measuredButOmitted:
		return 1
	case unknownMetrics:
		return 2
	default:
		return 3
	}
}

func (e metricEvidence) waitsForReadPriority() bool {
	return e == unknownMetrics || e == unadvertisedMetrics
}

func metricParts(b queryBatch, available func(resourceInfo) uint32) (yes, no []queryBatch) {
	// Resources with the same subset share a request for each side.
	masks := make(map[uint32][]resourceInfo)
	for _, r := range b.Resources {
		m := available(r) & b.mask
		masks[m] = append(masks[m], r)
	}
	for mask, rs := range masks {
		part := b
		part.Resources = rs
		if mask != 0 {
			yes = append(yes, selectNames(part, mask))
		}
		if rest := b.mask &^ mask; rest != 0 {
			no = append(no, selectNames(part, rest))
		}
	}
	return yes, no
}

func advertised(defs metricDefinitions, b queryBatch) uint32 {
	var mask uint32
	for i, m := range b.chunk.metrics {
		if b.mask&(1<<i) != 0 && defs.supports(m.AzureName, b.Aggregation, b.TimeGrain) {
			mask |= 1 << i
		}
	}
	return mask
}

func (p *queryPoll) measured(r resourceInfo, b queryBatch) uint32 {
	row := p.c.observations.resources[resourceIdentityFor(r, b.Profile.Name)]
	if row == nil {
		return 0
	}
	var mask uint32
	for i, m := range b.chunk.metrics {
		if row.metrics[m.Instrument] != nil {
			mask |= 1 << i
		}
	}
	return mask & b.mask
}

func (p *queryPoll) child(q *metricQuestion, b queryBatch, evidence metricEvidence) *metricQuestion {
	child := *q
	child.batch = b
	child.rejected = false
	child.lastPoll = 0
	child.evidence = evidence
	if b.mask != q.batch.mask {
		child.cohort = 0
	}
	// Remember a resource boundary when the exact set of names failed.
	if q.rejected && b.mask == q.batch.mask && len(b.Resources) < len(q.batch.Resources) {
		child.cohort = p.state.ticket()
	}
	return p.state.add(&child)
}

// The masks cover all requested names exactly once. Older samples are useful
// for recovery even when ARM now omits the name or its read failed.
func (p *queryPoll) classify(r resourceInfo, b queryBatch) [metricEvidenceCount]uint32 {
	var masks [metricEvidenceCount]uint32
	measured := p.measured(r, b)
	advice := p.state.advice[r.definitionKey(b.Profile)]
	if advice == nil || advice.definitions == nil {
		masks[measuredWithoutAdvice] = measured
		masks[unknownMetrics] = b.mask &^ measured
		return masks
	}
	listed := advertised(advice.definitions, b)
	masks[advertisedMetrics] = listed
	masks[measuredButOmitted] = measured &^ listed
	masks[unadvertisedMetrics] = b.mask &^ (listed | measured)
	return masks
}

// A mixed 400 is evidence about the whole request only. Each refinement
// partitions that rectangle; a failed union is never promoted from its pieces.
func (p *queryPoll) partition(q *metricQuestion) []*metricQuestion {
	b := q.batch
	groups := make(map[[metricEvidenceCount]uint32][]resourceInfo)
	for _, r := range b.Resources {
		masks := p.classify(r, b)
		groups[masks] = append(groups[masks], r)
	}

	var children []*metricQuestion
	for masks, resources := range groups {
		for i, mask := range masks {
			if mask == 0 {
				continue
			}
			evidence := metricEvidence(i)
			if len(groups) == 1 && mask == b.mask {
				q.evidence = evidence
				return nil
			}
			part := selectNames(b, mask)
			part.Resources = resources
			child := p.child(q, part, evidence)
			if !q.nextCheck.IsZero() {
				// New advice woke part of a sleeping group. Members ARM still
				// omits keep waiting for the group's recheck.
				child.recheckEarly = evidence != unadvertisedMetrics
				p.trackSleeper(child)
			}
			children = append(children, child)
		}
	}
	delete(p.state.questions, q)
	return children
}

// ARM omits this name for every member and Azure rejected them together.
// They sleep and are rechecked as one request instead of being narrowed to
// single resources, which costs about two metered calls per member and one
// recheck per member from then on.
//
// Accepted trade-off: a member Azure would serve while ARM still omits the
// name stays hidden until ARM lists it for that member or Azure accepts the
// whole group. A name with samples in history is measuredButOmitted instead
// and is still narrowed by resource.
func deniedTogether(q *metricQuestion) bool {
	return q.evidence == unadvertisedMetrics && bits.OnesCount32(q.batch.mask) == 1
}

func (p *queryPoll) refine(q *metricQuestion, at time.Time) []*metricQuestion {
	if deniedTogether(q) {
		p.deny(q, at)
		return nil
	}
	b := q.batch
	delete(p.state.questions, q)
	var children []*metricQuestion
	if len(b.Resources) > 1 && q.evidence != unadvertisedMetrics {
		half := len(b.Resources) / 2
		for _, members := range [][]resourceInfo{b.Resources[:half], b.Resources[half:]} {
			part := b
			part.Resources = members
			children = append(children, p.child(q, part, q.evidence))
		}
		return children
	}
	// An opaque failure for one resource offers no indication which name
	// failed. Single names bound the distinct confirmations by the chunk size.
	for mask := b.mask; mask != 0; mask &= mask - 1 {
		bit := mask & -mask
		children = append(children, p.child(q, selectNames(b, bit), q.evidence))
	}
	return children
}

func (p *queryPoll) trackSleeper(q *metricQuestion) {
	if p.sleepers == nil {
		p.sleepers = make(map[definitionKey][]*metricQuestion)
	}
	for _, r := range q.batch.Resources {
		k := r.definitionKey(q.batch.Profile)
		p.sleepers[k] = append(p.sleepers[k], q)
	}
}

func (p *queryPoll) deny(q *metricQuestion, at time.Time) {
	ttl := p.c.definitionTTL()
	if q.delay == 0 {
		q.delay = ttl
	} else {
		ceiling := max(time.Hour, ttl)
		q.delay += min(q.delay, ceiling-q.delay)
	}
	q.nextCheck = at.Add(q.delay)
	q.lastPoll = p.state.poll
	// Each admission clears recheckEarly. If it is set here, new advice
	// arrived while this check was in flight and still deserves a check.
	p.trackSleeper(q)
	q.rejected = false
}

// Split around held cells so an optional failure cannot exclude a disjoint
// working metric or cause its held sibling to be sent inside a later batch.
func (p *queryPoll) splitHeld(q *metricQuestion, at time.Time) ([]*metricQuestion, bool) {
	if !p.state.holds.hasMetrics() {
		return nil, false
	}
	b := q.batch
	free, held := metricParts(b, func(r resourceInfo) uint32 {
		mask, expiry := p.state.holds.resourceMetricHolds(b, r, at)
		p.preparedUntil = earliestDeadline(p.preparedUntil, expiry)
		return b.mask &^ mask
	})
	if len(held) == 0 {
		return nil, false
	}
	if len(free) == 0 {
		return nil, true
	}
	delete(p.state.questions, q)
	var children []*metricQuestion
	for _, part := range free {
		children = append(children, p.child(q, part, q.evidence))
	}
	for _, part := range held {
		children = append(children, p.child(q, part, q.evidence))
	}
	return children, true
}

func (p *queryPoll) readyQuestions(at time.Time) []*metricQuestion {
	var ready []*metricQuestion
	p.preparedUntil = time.Time{}
	// Refinement only creates strictly smaller rectangles. Process children
	// in this pass without mutating the slice being iterated.
	queue := slices.Collect(maps.Keys(p.state.questions))
	for i := 0; i < len(queue); i++ {
		q := queue[i]
		if !p.due[q.batch.chunk] || q.lastPoll == p.state.poll {
			continue
		}
		if at.Before(q.nextCheck) && !q.recheckEarly {
			p.preparedUntil = earliestDeadline(p.preparedUntil, q.nextCheck)
			continue
		}
		if p.waitingForAdvice(q) {
			continue
		}
		if parts, held := p.splitHeld(q, at); held {
			queue = append(queue, parts...)
			continue
		}
		if parts := p.partition(q); len(parts) != 0 {
			queue = append(queue, parts...)
			continue
		}
		if q.rejected {
			queue = append(queue, p.refine(q, at)...)
			continue
		}
		ready = append(ready, q)
	}
	// Queue order breaks equal-age ties. Each actual admission, including a
	// transient failure, moves behind older unserved work across future polls.
	evidenceFirst := p.asked
	slices.SortFunc(ready, func(a, b *metricQuestion) int {
		// Give the oldest work the first turn every poll, then prefer
		// evidence for recovering working samples in the remaining window.
		priority := cmp.Compare(a.evidence.priority(), b.evidence.priority())
		if evidenceFirst && priority != 0 {
			return priority
		}
		if a.turn < b.turn {
			return -1
		}
		if a.turn > b.turn {
			return 1
		}
		if priority != 0 {
			return priority
		}
		return cmp.Compare(a.id, b.id)
	})
	return ready
}
