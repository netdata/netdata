// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/netdata/netdata/go/plugins/logger"
)

type adviceRequest struct {
	waiters      []*metricQuestion
	key          definitionKey
	wire         definitionKey
	subscription string
	done         bool
}

type queryCompletion struct {
	result      queryBatchResult
	definitions metricDefinitions
	err         error
	at          time.Time
}

type queryPoll struct {
	// Unsent portions of a locally held request keep its time allowance only in this poll.
	continuations               map[*metricQuestion]time.Time
	startedReads                bool
	hasDueRecovery              bool
	sleepers                    map[definitionKey][]*metricQuestion
	prepared                    []*metricQuestion
	preparedUntil               time.Time // First hold or recheck expiry among skipped questions.
	preparedValid, afterCutoff  bool
	ordinaryActive, readsActive int
	c                           *Collector
	state                       *recoveryState
	ctx                         context.Context
	queryTime, deadline         time.Time
	due                         map[*metricChunk]bool
	reads                       map[definitionKey]*adviceRequest
	pending                     []*adviceRequest
	primary                     []plannedRequest
	sortReads                   bool
	armUntil, dataUntil         time.Time
	events                      chan queryEvent
	idle                        []*queryOperation
	active, succeeded, failed   int
	supplementalUsed            int // Dispatch reserves the first POST for each supplemental operation.
	debug                       bool
	samples                     []metricSample
}

func (p *queryPoll) wantAdvice(q *metricQuestion, force bool) {
	p.hasDueRecovery = true
	b := q.batch
	for _, r := range b.Resources {
		k := r.definitionKey(b.Profile)
		if read := p.reads[k]; read != nil {
			if !read.done {
				read.waiters = append(read.waiters, q)
			}
			continue
		}
		a := p.state.advice[k]
		if a == nil {
			a = &definitionAdvice{}
			if p.state.advice == nil {
				p.state.advice = make(map[definitionKey]*definitionAdvice)
			}
			p.state.advice[k] = a
		}
		if p.queryTime.Before(a.retry) || (!force && p.queryTime.Before(a.fetched.Add(p.c.definitionTTL()))) ||
			p.state.holds.armHeld(k, b.SubscriptionID, p.queryTime) {
			continue
		}
		if p.reads == nil {
			p.reads = make(map[definitionKey]*adviceRequest)
		}
		p.preparedValid = false
		read := &adviceRequest{waiters: []*metricQuestion{q}, key: k, wire: definitionKey{r.ID, b.Profile.MetricNamespace}, subscription: b.SubscriptionID}
		p.reads[k] = read
		p.pending = append(p.pending, read)
		p.sortReads = true
	}
}

func (p *queryPoll) waitingForAdvice(q *metricQuestion) bool {
	if !time.Now().Before(p.armUntil) || len(p.reads) == 0 {
		return false
	}
	for _, r := range q.batch.Resources {
		if read := p.reads[r.definitionKey(q.batch.Profile)]; read != nil && !read.done && !p.state.holds.armHeld(read.key, read.subscription, p.c.now()) {
			return true
		}
	}
	return false
}

func (p *queryPoll) startRead() bool {
	if !time.Now().Before(p.armUntil) {
		return false
	}
	if p.sortReads {
		slices.SortFunc(p.pending, func(a, b *adviceRequest) int {
			left, right := p.state.advice[a.key], p.state.advice[b.key]
			// Rotate the first read as well as all admissions. A lookup
			// started just before the cutoff may only get the unused tail.
			if v := cmp.Compare(left.openingTurn, right.openingTurn); v != 0 {
				return v
			}
			if v := cmp.Compare(left.turn, right.turn); v != 0 {
				return v
			}
			return cmp.Compare(a.key.resourceID, b.key.resourceID)
		})
		p.sortReads = false
	}
	for len(p.pending) > 0 {
		r := p.pending[0]
		p.pending = p.pending[1:]
		if p.state.holds.armHeld(r.key, r.subscription, p.c.now()) {
			r.done = true
			p.preparedValid = false
			continue
		}
		p.active++
		p.readsActive++
		op := p.takeOperation()
		op.read = r
		armCtx, cancel := context.WithDeadline(context.WithValue(p.ctx, queryOperationKey{}, op), p.armUntil)
		go func() {
			defer cancel()
			defs, err := p.c.queryExecutor.lookupDefinitions(armCtx, r.wire)
			op.complete(queryCompletion{definitions: defs, err: err, at: p.c.now()})
		}()
		return true
	}
	return false
}

func (p *queryPoll) finishRead(op *queryOperation) {
	r, e := op.read, op.finished
	r.done = true
	// Each read can release only the questions that requested it. Avoid
	// rescanning the fleet for each ARM response during a large rejection.
	for _, q := range r.waiters {
		if !p.waitingForAdvice(q) {
			p.preparedValid = false
			break
		}
	}
	if op.deferred {
		return
	}
	if !op.attempted {
		// A client can occupy the lookup slot without reaching the wire.
		p.markReadAdmission(r)
	}
	a := p.state.advice[r.key]
	throttled := p.recordCompletedServiceResponse(op, e.err, e.at)
	if e.err != nil {
		if !throttled {
			a.backoff = min(5*time.Minute, max(time.Minute, 2*a.backoff))
			a.retry = e.at.Add(a.backoff)
		}
		class, detail := batchFailureReason(e.err)
		p.c.Limit("metric-definitions:"+r.key.namespace+":"+class, 1, time.Hour).Warningf("metric definitions lookup failed: namespace=%q class=%s %s", r.wire.namespace, class, detail)
		return
	}
	old := a.definitions
	a.definitions, a.fetched, a.retry, a.backoff = e.definitions, e.at, time.Time{}, 0
	// Only previously denied single cells sleep. A newly advertised name
	// wakes immediately even if its last confirmation was very recent.
	for _, q := range p.sleepers[r.key] {
		if q.nextCheck.IsZero() || len(q.batch.Resources) != 1 || q.batch.Resources[0].definitionKey(q.batch.Profile) != r.key {
			continue
		}
		if advertised(e.definitions, q.batch) != 0 && advertised(old, q.batch) == 0 {
			q.recheckEarly = true
			p.preparedValid = false
		}
	}
}

func (p *queryPoll) markReadAdmission(r *adviceRequest) {
	a := p.state.advice[r.key]
	a.turn = p.state.ticket()
	if !p.startedReads {
		a.openingTurn = a.turn
		p.startedReads = true
	}
}

func (p *queryPoll) startMetrics(r plannedRequest, ctx context.Context) {
	p.active++
	if len(r.questions) == 0 {
		p.ordinaryActive++
	}
	op := p.takeOperation()
	op.request = r
	ctx = context.WithValue(ctx, queryOperationKey{}, op)
	end := r.deadline
	if len(r.questions) > 0 && (end.IsZero() || p.dataUntil.Before(end)) {
		end = p.dataUntil
	}
	go func() {
		if !end.IsZero() {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, end)
			defer cancel()
		}
		result := p.c.queryExecutor.executeQueryBatch(ctx, r.batch, p.queryTime, p.c.QueryOffset, p.debug)
		op.complete(queryCompletion{result: result, at: p.c.now()})
	}()
}

// Each newly rejected ordinary request gets the time remaining in this poll,
// up to the configured recovery timeout. Already dispatched calls keep their
// original deadlines; no arrival can move the poll cutoff.
func (p *queryPoll) allowRecovery(at time.Time) {
	end := p.deadline
	if timeout := p.c.queryExecutor.timeout; timeout > 0 && at.Add(timeout).Before(end) {
		end = at.Add(timeout)
	}
	if end.After(p.dataUntil) {
		p.dataUntil = end
	}
	midpoint := at.Add(end.Sub(at) / 2)
	if midpoint.After(p.armUntil) {
		p.armUntil = midpoint
		p.preparedValid = false
	}
}

// A zero deadline imposes no limit.
func earliestDeadline(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// Wake a poll that still has workers in flight when a free slot can do new work.
func (p *queryPoll) nextRecoveryWake() time.Time {
	now := time.Now()
	var wake time.Time
	if now.Before(p.armUntil) {
		wake = p.armUntil
	}
	if p.preparedValid && !p.preparedUntil.IsZero() && p.supplementalUsed < maxRecoveryMetricsCalls && now.Before(p.dataUntil) {
		expiry := now.Add(p.preparedUntil.Sub(p.c.now()))
		if expiry.Before(p.dataUntil) {
			wake = earliestDeadline(wake, expiry)
		}
	}
	return wake
}

func (p *queryPoll) startRecovery() bool {
	now := time.Now()
	if p.dataUntil.IsZero() {
		p.allowRecovery(now)
	}
	if !now.Before(p.dataUntil) {
		return false
	}
	if p.supplementalUsed < maxRecoveryMetricsCalls {
		pastCutoff := !now.Before(p.armUntil)
		at := p.c.now()
		if !p.preparedValid || pastCutoff != p.afterCutoff || (!p.preparedUntil.IsZero() && !at.Before(p.preparedUntil)) {
			p.prepared = p.readyQuestions(at)
			p.preparedValid = true
			p.afterCutoff = pastCutoff
		}
		ready := p.prepared
		if len(ready) > 0 {
			q := ready[0]
			until := p.continuations[q]
			if !until.IsZero() && !time.Now().Before(until) {
				q.lastPoll = p.state.poll
				p.preparedValid = false
				return true
			}
			// While preparing cohorts, give outstanding definitions a chance to
			// identify working counters before spending the cap on denials.
			if q.evidence.waitsForReadPriority() && time.Now().Before(p.armUntil) {
				if len(p.pending) > 0 && p.startRead() {
					return true
				}
				if p.readsActive+p.ordinaryActive > 0 && p.supplementalUsed >= maxRecoveryMetricsCalls/2 {
					return false
				}
			}
			p.supplementalUsed++
			// Questions are already packed rectangles. Recombine compatible
			// waiting fragments only if they have the same resource split.
			batch := q.batch
			owners := []*metricQuestion{q}
			for _, other := range ready[1:] {
				b := other.batch
				if p.continuations[other] != until || other.evidence != q.evidence || other.cohort != q.cohort || b.chunk != batch.chunk ||
					b.mask != batch.mask ||
					b.SubscriptionID != batch.SubscriptionID ||
					b.Region != batch.Region ||
					len(batch.Resources)+len(b.Resources) > p.c.Limits.MaxBatchResources {
					continue
				}
				if len(batch.Resources) == len(q.batch.Resources) {
					batch.Resources = slices.Clone(batch.Resources)
				}
				batch.Resources = append(batch.Resources, b.Resources...)
				owners = append(owners, other)
			}
			p.preparedValid = false
			for _, owner := range owners {
				owner.lastPoll = p.state.poll
			}
			ctx := policy.WithRetryOptions(p.ctx, policy.RetryOptions{MaxRetries: -1})
			p.startMetrics(plannedRequest{batch: batch, cohort: q.cohort, questions: owners, deadline: until}, ctx)
			return true
		}
	}
	hadReads := len(p.pending) > 0 && time.Now().Before(p.armUntil)
	if p.startRead() {
		return true
	}
	if hadReads && len(p.pending) == 0 {
		return true
	}
	// startRead can discard queued reads after a broader server hold. Those
	// questions are now eligible without another completion to wake the loop.
	return false
}

func (p *queryPoll) newRejection(b queryBatch, cohort uint64, confirm bool, at time.Time) {
	if !p.dataUntil.IsZero() {
		p.allowRecovery(time.Now())
	}
	q := &metricQuestion{batch: b, cohort: cohort, turn: p.state.sequence, rejected: !confirm}
	p.state.add(q)
	if !confirm && len(b.Resources) == 1 && b.mask&(b.mask-1) == 0 {
		p.deny(q, at)
	}
	p.wantAdvice(q, true)
}

func (p *queryPoll) finishMetrics(op *queryOperation) {
	req, e := op.request, op.finished
	res := e.result
	b := req.batch
	if len(req.questions) > 0 || res.Err != nil || len(res.incompatible) > 0 {
		p.preparedValid = false
	}
	if op.deferred {
		p.deferMetrics(op)
		return
	}
	// A provider result completes this request. Subsequent incompatibility probes
	// are new work and use the remaining recovery allowance.
	for _, q := range req.questions {
		delete(p.continuations, q)
	}
	if !op.attempted {
		// Injected clients can complete without entering the SDK transport.
		p.markMetricsAdmission(req.questions)
	}
	p.recordCompletedServiceResponse(op, res.Err, e.at)
	if res.Err != nil {
		p.failed++
		p.c.logBatchFailure(&b, res.Err, p.queryTime)
		if rejectedMetrics(res.Err) {
			if len(req.questions) == 0 {
				p.state.demote(b)
				p.newRejection(b, req.cohort, false, e.at)
			} else {
				for _, q := range req.questions {
					switch {
					case len(req.questions) > 1:
						// The combined rectangle failed. Each original owner's
						// strictly smaller request remains independently askable.
						q.cohort = p.state.ticket()
						q.lastPoll = 0
						q.recheckEarly = true
					case len(b.Resources) == 1 && b.mask&(b.mask-1) == 0:
						p.deny(q, e.at)
					default:
						q.rejected = true
						q.lastPoll = 0
					}
				}
			}
		}
		return
	}
	p.succeeded++
	if d := res.Diagnostics; d != nil {
		p.c.Debugf("metrics batch: profile=%s region=%s subscription=%.80q grain=%s resources=%d with_samples=%d empty=%d missing=%d unmatched_rows=%d metric_errors=%d empty_examples=%v missing_examples=%v error_examples=%v %s",
			b.Profile.Name, b.Region, b.SubscriptionID, b.TimeGrain, len(b.Resources), d.withSamples, d.empty, d.missing, d.unmatchedRows, d.metricErrors, d.emptyExamples, d.missingExamples, d.errorExamples, b.debugRequest(p.queryTime, p.c.QueryOffset))
	}
	p.samples = append(p.samples, res.Samples...)
	if len(req.questions) == 0 {
		p.accept(b, nil, req.cohort, res, e.at)
		return
	}
	for _, q := range req.questions {
		delete(p.state.questions, q)
		p.accept(q.batch, q, q.cohort, res, e.at)
	}
}

func (p *queryPoll) requeueOrdinary(r plannedRequest, free []queryBatch) {
	for _, part := range free {
		r.batch = part
		p.primary = append(p.primary, r)
	}
}

func (p *queryPoll) deferMetrics(op *queryOperation) {
	r := op.request
	available := func(row resourceInfo) uint32 { return op.available[row.idKey] }
	if len(r.questions) == 0 {
		r.deadline = op.deadline
		free, _ := metricParts(r.batch, available)
		p.requeueOrdinary(r, free)
		return
	}
	if p.continuations == nil {
		p.continuations = make(map[*metricQuestion]time.Time)
	}
	for _, q := range r.questions {
		p.continuations[q] = op.deadline
	}
	if !op.attempted {
		p.supplementalUsed--
		for _, q := range r.questions {
			q.lastPoll = 0
		}
		return
	}
	// The attempted rectangle keeps its charge and scheduling age. Only
	// cells outside the hold that blocked the replay can be admitted again.
	for _, q := range r.questions {
		free, held := metricParts(q.batch, available)
		if len(free) == 0 {
			continue
		}
		if len(held) == 0 {
			q.lastPoll = 0
			continue
		}
		delete(p.state.questions, q)
		for _, part := range free {
			p.child(q, part, q.evidence)
		}
		for _, part := range held {
			p.child(q, part, q.evidence).lastPoll = p.state.poll
		}
	}
}

func (p *queryPoll) accept(b queryBatch, q *metricQuestion, cohort uint64, res queryBatchResult, at time.Time) {
	errorsByResource := res.incompatible
	if q != nil {
		errorsByResource = res.metricErrors
	}
	if len(errorsByResource) == 0 {
		if q != nil {
			p.state.promote(b, cohort)
		}
		return
	}
	good, bad := metricParts(b, func(r resourceInfo) uint32 { return b.mask &^ errorsByResource[r.idKey] })
	if q != nil {
		for _, part := range good {
			p.state.promote(part, cohort)
		}
	}
	for _, part := range bad {
		if q == nil {
			p.state.demote(part)
		}
		// The embedded error identifies each name separately even inside a 200.
		for _, r := range part.Resources {
			one := part
			one.Resources = []resourceInfo{r}
			for m := one.mask; m != 0; m &= m - 1 {
				cell := selectNames(one, m&-m)
				if q == nil {
					p.newRejection(cell, cohort, true, at)
					continue
				}
				if res.incompatible[r.idKey]&cell.mask == 0 {
					deferred := *q
					deferred.batch = cell
					p.state.add(&deferred)
					continue
				}
				denied := &metricQuestion{batch: cell, cohort: cohort, turn: q.turn, delay: q.delay}
				p.deny(denied, at)
				p.state.add(denied)
			}
		}
	}
}

func (p *queryPoll) finish(op *queryOperation) {
	p.active--
	if op.read != nil {
		p.readsActive--
		p.finishRead(op)
	} else {
		if len(op.request.questions) == 0 {
			p.ordinaryActive--
		}
		p.finishMetrics(op)
	}
}

func (c *Collector) collectQuerySamples(ctx context.Context, batches []queryBatch, now, deadline time.Time) ([]metricSample, error) {
	if len(batches) == 0 {
		return nil, nil
	}
	s := &c.recovery
	s.poll++
	slots := max(1, c.queryExecutor.maxConcurrency)
	p := queryPoll{
		c: c, state: s, ctx: ctx, queryTime: now, deadline: deadline,
		due: make(map[*metricChunk]bool), events: make(chan queryEvent, slots),
		primary: s.plan(batches, now, c.Limits.MaxBatchResources),
		debug:   logger.Level.Enabled(slog.LevelDebug),
	}
	for _, b := range batches {
		p.due[b.chunk] = true
	}
	for q := range s.questions {
		if !q.nextCheck.IsZero() {
			p.trackSleeper(q)
		}
		if p.due[q.batch.chunk] {
			p.wantAdvice(q, false)
		}
	}
	primaryCtx := policy.WithRetryOptions(ctx, policy.RetryOptions{ShouldRetry: retryPrimaryMetrics})
	for {
		select {
		case e := <-p.events:
			p.onEvent(e)
			continue
		default:
		}
		if ctx.Err() == nil && p.active < slots {
			if len(p.primary) > 0 {
				r := p.primary[0]
				p.primary = p.primary[1:]
				if s.holds.hasMetrics() && s.holds.metricMask(r.batch, c.now()) != 0 {
					at := c.now()
					free, _ := metricParts(r.batch, func(row resourceInfo) uint32 {
						one := r.batch
						one.Resources = []resourceInfo{row}
						return one.mask &^ s.holds.metricMask(one, at)
					})
					p.requeueOrdinary(r, free)
					continue
				}
				p.startMetrics(r, primaryCtx)
				continue
			}
			if p.hasDueRecovery && len(s.questions) > 0 && p.startRecovery() {
				continue
			}
		}
		if p.active == 0 {
			break
		}
		if p.active < slots && ctx.Err() == nil {
			if wake := p.nextRecoveryWake(); !wake.IsZero() {
				timer := time.NewTimer(time.Until(wake))
				select {
				case <-timer.C:
				case e := <-p.events:
					p.onEvent(e)
				}
				timer.Stop()
				continue
			}
		}
		p.onEvent(<-p.events)
	}
	s.pruneAdvice()
	if p.failed > 0 && p.succeeded == 0 {
		return nil, errors.New("all Azure Monitor batch queries failed")
	}
	return p.samples, nil
}
