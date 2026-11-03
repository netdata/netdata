// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// persistentRuntime serializes collection and Function requests onto one
// running session. It exists only between readiness and Run's return.
type persistentRuntime struct {
	requests chan scriptRequest
	done     chan struct{}
}

type scriptRequest struct {
	ctx      context.Context
	function *funcapi.RawMethodRequest // nil for a collection request
	reply    chan scriptResult         // buffered, so Run never blocks on an abandoned caller
}

type scriptResult struct {
	snapshot snapshot
	function *funcapi.FunctionResponse
	err      error
}

// runPersistent owns the session for the job's Run lifetime. Candidate
// validation in Init and Check never launches the script.
func (c *Collector) runPersistent(ctx context.Context, ready func()) error {
	s, err := startSession(ctx, c.definition.command)
	if err != nil {
		return err
	}
	defer s.close()
	if err := c.startup(ctx, s); err != nil {
		return err
	}
	r := &persistentRuntime{
		requests: make(chan scriptRequest),
		done:     make(chan struct{}),
	}
	c.setRuntime(r)
	defer func() {
		c.setRuntime(nil)
		close(r.done)
	}()
	ready()
	return c.serve(ctx, s, r)
}

// startup sends the configuration envelope and waits for the ready handshake,
// both within one timeout.
func (c *Collector) startup(ctx context.Context, s *scriptSession) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	if len(c.configEnvelope) > 0 {
		if err := s.write(ctx, c.configEnvelope); err != nil {
			return fmt.Errorf("persistent configuration: %w", err)
		}
	}
	frame, err := s.read(ctx)
	if err == nil {
		err = decodeReady(frame)
	}
	if err != nil {
		return fmt.Errorf("persistent startup: %w", err)
	}
	return nil
}

func (c *Collector) setRuntime(r *persistentRuntime) {
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()
	c.runtime = r
}

// serve runs one request at a time until cancellation, process exit, an
// unsolicited frame or a terminal exchange failure.
func (c *Collector) serve(ctx context.Context, s *scriptSession, r *persistentRuntime) error {
	var sequence uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.exited:
			return s.exitError()
		case frame := <-s.frames:
			if frame.err != nil {
				return s.transportError(ctx, frame.err)
			}
			return errors.New("unsolicited persistent response")
		case request := <-r.requests:
			// A canceled sender can race with channel delivery. Until this point the
			// request owns no stream bytes, so reject it without disturbing the peer.
			if err := request.ctx.Err(); err != nil {
				request.reply <- scriptResult{
					err: err,
				}
				continue
			}
			sequence++
			result, terminal := c.serveRequest(ctx, s, request, strconv.FormatUint(sequence, 10))
			request.reply <- result
			if terminal {
				return fmt.Errorf("persistent exchange: %w", result.err)
			}
		}
	}
}

// serveRequest performs one request/reply exchange and reports whether its failure
// leaves the stream unusable.
func (c *Collector) serveRequest(
	ctx context.Context,
	s *scriptSession,
	request scriptRequest,
	id string,
) (scriptResult, bool) {
	var result scriptResult
	if request.function == nil {
		// Queue waiting must not consume the admitted exchange budget.
		exchangeCtx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
		defer cancel()
		var err error
		if c.definition.SnapshotFormat == formatLines {
			result.snapshot, err = s.exchangeLines(exchangeCtx, id)
		} else {
			var data []byte
			data, err = s.exchange(exchangeCtx, encodeCollectRequest(id))
			if err == nil {
				result.snapshot, err = decodeReply(data, id)
			}
		}
		result.err = err
		return result, err != nil && !errors.Is(err, errCollectionFailed)
	}
	frame, err := encodeFunctionRequest(request.ctx, id, *request.function)
	if err != nil {
		// A local encoding failure has not touched the stream.
		result.err = err
		return result, false
	}
	data, err := s.exchangeWithDrain(request.ctx, frame, c.Timeout.Duration())
	if err == nil {
		result.function, err = decodeFunctionReply(data, id, request.function.Info)
	}
	result.err = err
	return result, err != nil
}

func (c *Collector) collectPersistent(ctx context.Context) (snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()
	result := c.requestPersistent(ctx, nil)
	return result.snapshot, result.err
}

// requestPersistent queues one request. ctx bounds both queue waiting and the
// wait for the reply; the runtime still completes an admitted exchange.
func (c *Collector) requestPersistent(ctx context.Context, function *funcapi.RawMethodRequest) scriptResult {
	if err := ctx.Err(); err != nil {
		return scriptResult{
			err: err,
		}
	}
	c.runtimeMu.Lock()
	r := c.runtime
	c.runtimeMu.Unlock()
	if r == nil {
		return scriptResult{
			err: errors.New("persistent script is not running"),
		}
	}
	request := scriptRequest{
		ctx:      ctx,
		function: function,
		reply:    make(chan scriptResult, 1),
	}
	select {
	case <-ctx.Done():
		return scriptResult{
			err: ctx.Err(),
		}
	case <-r.done:
		return scriptResult{
			err: errPersistentStopped,
		}
	case r.requests <- request:
	}
	select {
	case <-ctx.Done():
		return scriptResult{
			err: ctx.Err(),
		}
	case <-r.done:
		return scriptResult{
			err: errPersistentStopped,
		}
	case result := <-request.reply:
		return result
	}
}

var errPersistentStopped = errors.New("persistent script stopped")
