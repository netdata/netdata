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

// A dispatched ARM read or Metrics request. Its worker fills the outcome and
// reports once. Holds and the supplemental limit are applied at dispatch.
type queryOperation struct {
	request     plannedRequest
	read        *adviceRequest
	started     time.Time // Dispatch time on the collector clock.
	result      queryBatchResult
	definitions metricDefinitions
	err         error
	at          time.Time
}

type queryPoll struct {
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
	held                        []plannedRequest // Ordinary work waiting for a service hold to expire.
	heldUntil                   time.Time        // First expiry among held work, on the collector clock.
	heldStale                   bool
	sortReads                   bool
	armUntil, dataUntil         time.Time
	events                      chan *queryOperation
	active, succeeded, failed   int
	supplementalUsed            int // Metrics requests started for questions in this poll.
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
		p.markReadAdmission(r)
		op := &queryOperation{read: r, started: p.c.now()}
		armCtx, cancel := context.WithDeadline(p.ctx, p.armUntil)
		go func() {
			defer cancel()
			op.definitions, op.err = p.c.queryExecutor.lookupDefinitions(armCtx, r.wire)
			op.at = p.c.now()
			p.events <- op
		}()
		return true
	}
	return false
}

func (p *queryPoll) finishRead(op *queryOperation) {
	r := op.read
	r.done = true
	// Each read can release only the questions that requested it. Avoid
	// rescanning the fleet for each ARM response during a large rejection.
	for _, q := range r.waiters {
		if !p.waitingForAdvice(q) {
			p.preparedValid = false
			break
		}
	}
	a := p.state.advice[r.key]
	throttled := p.recordServiceResponse(op, op.err)
	if op.err != nil {
		if !throttled {
			a.backoff = min(5*time.Minute, max(time.Minute, 2*a.backoff))
			a.retry = op.at.Add(a.backoff)
		}
		class, detail := batchFailureReason(op.err)
		p.c.Limit("metric-definitions:"+r.key.namespace+":"+class, 1, time.Hour).Warningf("metric definitions lookup failed: namespace=%q class=%s %s", r.wire.namespace, class, detail)
		return
	}
	old := a.definitions
	a.definitions, a.fetched, a.retry, a.backoff = op.definitions, op.at, time.Time{}, 0
	// A name newly advertised for this resource wakes its denied question
	// even if the last confirmation was very recent.
	for _, q := range p.sleepers[r.key] {
		if q.nextCheck.IsZero() {
			continue
		}
		if advertised(op.definitions, q.batch) != 0 && advertised(old, q.batch) == 0 {
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
	} else {
		p.markMetricsAdmission(r.questions)
	}
	op := &queryOperation{request: r, started: p.c.now()}
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
		op.result = p.c.queryExecutor.executeQueryBatch(ctx, r.batch, p.queryTime, p.c.QueryOffset, p.debug)
		op.at = p.c.now()
		p.events <- op
	}()
}

// Each admission, including one that fails transiently, moves the questions
// behind older unserved work in later polls.
func (p *queryPoll) markMetricsAdmission(questions []*metricQuestion) {
	turn := p.state.ticket()
	for _, q := range questions {
		q.turn = turn
		q.recheckEarly = false
	}
}

// A throttle starts or extends a hold. A success clears a headerless backoff
// streak. Other failures say nothing about the service's quota.
func (p *queryPoll) recordServiceResponse(op *queryOperation, err error) bool {
	receipt := throttleFromError(err, op.at)
	if err != nil && receipt.status == 0 {
		return false
	}
	if r := op.read; r != nil {
		p.state.holds.recordARM(r.key, r.subscription, receipt, op.started)
	} else {
		p.state.holds.recordMetrics(op.request.batch, receipt, op.started)
	}
	if receipt.status == 0 {
		return false
	}
	p.preparedValid = false
	return true
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
				if other.evidence != q.evidence || other.cohort != q.cohort || b.chunk != batch.chunk ||
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
			p.startMetrics(plannedRequest{batch: batch, cohort: q.cohort, questions: owners}, ctx)
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
	req, res := op.request, op.result
	b := req.batch
	if len(req.questions) > 0 || res.Err != nil || len(res.incompatible) > 0 {
		p.preparedValid = false
	}
	throttled := p.recordServiceResponse(op, res.Err)
	if m := req.merge; m != nil {
		if res.Err != nil {
			p.rejectMerge(m, res.Err, op.at)
			return
		}
		// The union replaced its members' requests at no extra cost.
		p.supplementalUsed--
		req.cohort = p.state.ticket()
		p.state.adopt(b, req.cohort)
	}
	if res.Err != nil {
		p.failed++
		p.c.logBatchFailure(&b, res.Err, p.queryTime)
		if throttled && len(req.questions) == 0 && !req.resent {
			req.resent = true
			p.hold(req)
		}
		if rejectedMetrics(res.Err) {
			if len(req.questions) == 0 {
				p.state.demote(b)
				p.newRejection(b, req.cohort, false, op.at)
			} else {
				for _, q := range req.questions {
					switch {
					case deniedTogether(q):
						p.deny(q, op.at)
					case len(req.questions) > 1:
						// The combined rectangle failed. Each original owner's
						// strictly smaller request remains independently askable.
						q.cohort = p.state.ticket()
						q.lastPoll = 0
						q.recheckEarly = true
					case len(b.Resources) == 1 && b.mask&(b.mask-1) == 0:
						p.deny(q, op.at)
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
		p.accept(b, nil, req.cohort, res, op.at)
		return
	}
	for _, q := range req.questions {
		delete(p.state.questions, q)
		p.accept(q.batch, q, q.cohort, res, op.at)
	}
}

// The members of a failed union keep their earlier requests in this poll, so
// the attempt costs one request and no sample.
func (p *queryPoll) rejectMerge(m *mergeAttempt, err error, at time.Time) {
	ttl := p.c.definitionTTL()
	if state := p.state.merges[m.key]; state != nil {
		if rejectedMetrics(err) {
			state.next = at.Add(state.delay)
			state.delay += min(state.delay, max(time.Hour, ttl)-state.delay)
		} else {
			state.next = at.Add(ttl)
		}
	}
	class, detail := batchFailureReason(err)
	p.c.Debugf("merging resource batches failed: profile=%s grain=%s class=%s %s", m.key.chunk.profile.Name, m.key.chunk.TimeGrain, class, detail)
	p.primary = append(p.primary, m.fallback...)
}

func (p *queryPoll) hold(r plannedRequest) {
	p.held = append(p.held, r)
	p.heldStale = true
}

// Ordinary work blocked by a service hold stays in the poll. Its unheld cells
// are sent now and the rest when the hold expires before the recovery deadline.
func (p *queryPoll) releaseHeld() {
	at := p.c.now()
	if !p.heldStale && at.Before(p.heldUntil) {
		return
	}
	held := p.held
	p.held, p.heldUntil, p.heldStale = nil, time.Time{}, false
	late := !time.Now().Before(p.deadline)
	for _, r := range held {
		if late && (r.resent || !r.deadline.IsZero()) {
			// Work that was throttled or waited is not sent after the deadline.
			p.held = append(p.held, r)
			continue
		}
		b := r.batch
		free, blocked := metricParts(b, func(row resourceInfo) uint32 {
			mask, expiry := p.state.holds.resourceMetricHolds(b, row, at)
			p.heldUntil = earliestDeadline(p.heldUntil, expiry)
			return b.mask &^ mask
		})
		for _, part := range free {
			r.batch = part
			p.primary = append(p.primary, r)
		}
		// Work that waited for a hold ends with the poll's recovery deadline.
		r.deadline = p.deadline
		for _, part := range blocked {
			r.batch = part
			p.held = append(p.held, r)
		}
	}
}

// The wall-clock time to wake an otherwise idle poll for held ordinary work.
func (p *queryPoll) heldWake() time.Time {
	now := time.Now()
	if len(p.held) == 0 || p.heldStale || p.heldUntil.IsZero() || !now.Before(p.deadline) {
		return time.Time{}
	}
	wake := now.Add(p.heldUntil.Sub(p.c.now()))
	if !wake.Before(p.deadline) {
		return time.Time{}
	}
	return wake
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
		due: make(map[*metricChunk]bool), events: make(chan *queryOperation, slots),
		primary: s.plan(batches, now, c.Limits.MaxBatchResources, c.definitionTTL()),
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
		case op := <-p.events:
			p.finish(op)
			continue
		default:
		}
		// Unheld cells of newly held work are sent at any time. Past the
		// recovery deadline nothing waits for an expiry any more.
		if ctx.Err() == nil && len(p.held) > 0 && (p.heldStale || time.Now().Before(deadline)) {
			p.releaseHeld()
		}
		if ctx.Err() == nil && p.active < slots {
			if len(p.primary) > 0 {
				r := p.primary[0]
				p.primary = p.primary[1:]
				held := s.holds.hasMetrics() && s.holds.metricMask(r.batch, c.now()) != 0
				if m := r.merge; m != nil {
					// A trial union is the one extra request if Azure rejects it.
					if held || p.supplementalUsed >= maxRecoveryMetricsCalls {
						p.primary = append(p.primary, m.fallback...)
						continue
					}
					p.supplementalUsed++
				} else if held {
					p.hold(r)
					continue
				}
				p.startMetrics(r, primaryCtx)
				continue
			}
			if p.hasDueRecovery && len(s.questions) > 0 && p.startRecovery() {
				continue
			}
		}
		// Wake for work a free slot can start later in this poll. Only held
		// ordinary work keeps an otherwise idle poll waiting.
		var wake time.Time
		if ctx.Err() == nil && p.active < slots {
			wake = p.heldWake()
			if p.active > 0 {
				wake = earliestDeadline(wake, p.nextRecoveryWake())
			}
		}
		if wake.IsZero() {
			if p.active == 0 {
				break
			}
			p.finish(<-p.events)
			continue
		}
		timer := time.NewTimer(time.Until(wake))
		select {
		case <-timer.C:
		case op := <-p.events:
			p.finish(op)
		case <-ctx.Done():
		}
		timer.Stop()
	}
	s.pruneAdvice()
	if p.failed > 0 && p.succeeded == 0 {
		return nil, errors.New("all Azure Monitor batch queries failed")
	}
	return p.samples, nil
}
