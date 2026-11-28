// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"net/http"
	"time"
)

// A worker publishes a receipt before reading the response body. The same
// ingress orders it against other workers' send requests and completions.
type queryEvent struct {
	op      *queryOperation
	send    context.Context
	receipt throttleReceipt
	post    bool
}

type queryOperation struct {
	events chan<- queryEvent
	reply  chan error
	now    func() time.Time

	request plannedRequest
	read    *adviceRequest

	// The worker owns the result, receipt and deadline until it reports completion.
	captured throttleReceipt
	deadline time.Time // Effective deadline of the request sent to the SDK.
	finished queryCompletion

	// The coordinator alone owns admission and any hold deferral.
	attempted, received, deferred bool
	admittedAt                    time.Time
	available                     map[string]uint32
}

type queryOperationKey struct{}

func queryOperationContext(ctx context.Context) (context.Context, *queryOperation) {
	if op, ok := ctx.Value(queryOperationKey{}).(*queryOperation); ok {
		return ctx, op
	}
	now, _ := ctx.Value(collectorClockKey{}).(func() time.Time)
	if now == nil {
		now = time.Now
	}
	op := &queryOperation{now: now}
	return context.WithValue(ctx, queryOperationKey{}, op), op
}

func (op *queryOperation) authorize(ctx context.Context, post bool) error {
	if op.events == nil {
		return ctx.Err()
	}
	select {
	case op.events <- queryEvent{op: op, send: ctx, post: post}:
		// The coordinator always answers published requests, even if the
		// worker's deadline has since expired.
		return <-op.reply
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (op *queryOperation) receive(resp *http.Response) {
	at := op.now()
	op.captured = throttleReceipt{status: resp.StatusCode, headers: resp.Header.Clone(), at: at}
	if op.events != nil {
		// The service response remains valid if cancellation wins the SDK
		// operation. Keep the response independent of the eventual result.
		op.events <- queryEvent{op: op, receipt: op.captured}
	}
}

func (op *queryOperation) complete(e queryCompletion) {
	op.finished = e
	op.events <- queryEvent{op: op}
}

func (p *queryPoll) takeOperation() *queryOperation {
	var op *queryOperation
	if n := len(p.idle); n != 0 {
		op = p.idle[n-1]
		p.idle = p.idle[:n-1]
	} else {
		op = &queryOperation{reply: make(chan error, 1)}
	}
	// Clients that bypass the wire gate use dispatch as their admission time.
	*op = queryOperation{
		events:     p.events,
		reply:      op.reply,
		now:        p.c.now,
		admittedAt: p.c.now(),
	}
	return op
}

func (p *queryPoll) onEvent(e queryEvent) {
	op := e.op
	switch {
	case e.send != nil:
		op.reply <- p.authorizeSend(op, e.send, e.post)
	case e.receipt.status != 0:
		op.received = true
		p.recordServiceResponse(op, e.receipt)
		p.preparedValid = false
	default:
		p.finish(op)
		p.idle = append(p.idle, op)
	}
}

func (p *queryPoll) authorizeSend(op *queryOperation, ctx context.Context, post bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	at := p.c.now()
	if read := op.read; read != nil {
		if len(p.state.holds.arm) != 0 && p.state.holds.armHeld(read.key, read.subscription, at) {
			op.deferred = true
			return errHeldAzureRequest
		}
		if !op.attempted {
			p.markReadAdmission(read)
		}
	} else {
		b := op.request.batch
		if p.state.holds.hasMetrics() {
			if p.state.holds.metricMask(b, at) != 0 {
				op.deferred = true
				op.available = make(map[string]uint32, len(b.Resources))
				for i, r := range b.Resources {
					one := b
					one.Resources = b.Resources[i : i+1]
					op.available[r.idKey] = b.mask &^ p.state.holds.metricMask(one, at)
				}
				return errHeldAzureRequest
			}
		}
		if op.attempted {
			if post && len(op.request.questions) != 0 {
				if p.supplementalUsed == maxRecoveryMetricsCalls {
					return errSupplementalBudgetExhausted
				}
				p.supplementalUsed++
			}
		} else {
			p.markMetricsAdmission(op.request.questions)
		}
	}
	op.admittedAt = at
	op.attempted = true
	return nil
}

func (p *queryPoll) markMetricsAdmission(questions []*metricQuestion) {
	if len(questions) == 0 {
		return
	}
	turn := p.state.ticket()
	for _, q := range questions {
		q.turn = turn
		q.recheckEarly = false
	}
}

func (p *queryPoll) recordServiceResponse(op *queryOperation, receipt throttleReceipt) {
	if r := op.read; r != nil {
		p.state.holds.recordARM(r.key, r.subscription, receipt, op.admittedAt)
	} else {
		p.state.holds.recordMetrics(op.request.batch, receipt, op.admittedAt)
	}
}

func (p *queryPoll) recordCompletedServiceResponse(op *queryOperation, err error, at time.Time) bool {
	if op.received {
		return true
	}
	if err == nil {
		p.recordServiceResponse(op, throttleReceipt{at: at})
		return false
	}
	// Injected clients can return a throttle without a transport receipt.
	receipt := throttleFromError(err, at)
	if receipt.status == 0 {
		return false
	}
	p.recordServiceResponse(op, receipt)
	p.preparedValid = false
	return true
}
