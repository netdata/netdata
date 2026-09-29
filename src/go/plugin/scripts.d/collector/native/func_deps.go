// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"errors"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
)

// funcDeps adapts a job's Collector to the Function router.
type funcDeps struct {
	c *Collector
}

var _ nativefunc.Deps = funcDeps{}

func (d funcDeps) ExecuteFunction(
	ctx context.Context,
	request funcapi.RawMethodRequest,
) (*funcapi.FunctionResponse, error) {
	return d.c.executeFunction(ctx, request)
}

// functionHandler binds a registered package's Function router to a running job.
func functionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return nativefunc.NewRouter(funcDeps{
		c: c,
	}, c.definition.methods)
}

func (c *Collector) executeFunction(
	ctx context.Context,
	request funcapi.RawMethodRequest,
) (*funcapi.FunctionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	declared := slices.ContainsFunc(
		c.definition.methods,
		func(m funcapi.FunctionConfig) bool { return m.ID == request.Method },
	)
	if !c.initialized || !c.registered || !declared {
		return nil, errors.New("native Function is unavailable")
	}
	// Reserve the largest session ID before queue admission. Rejection must not
	// touch an existing process, even when ingress allowed a larger payload.
	if _, err := encodeFunctionRequest(ctx, maxRequestID, request); err != nil {
		return nil, err
	}
	if c.definition.Mode == modePersistent {
		result := c.requestPersistent(ctx, &request)
		return result.function, result.err
	}
	return c.executeOneshotFunction(ctx, request)
}

func (c *Collector) executeOneshotFunction(
	ctx context.Context,
	request funcapi.RawMethodRequest,
) (*funcapi.FunctionResponse, error) {
	const id = "1"
	frame, err := encodeFunctionRequest(ctx, id, request)
	if err != nil {
		return nil, err
	}
	data, err := runOneshot(ctx, c.definition.command, opFunction, slices.Concat(c.configEnvelope, frame))
	if err != nil {
		c.Errorf("one-shot Function command failed: %s", commandFailureReason(err))
		return nil, err
	}
	result, err := decodeFunctionReply(data, id, request.Info)
	if err != nil {
		// Decoder errors can contain script-controlled field names or values.
		c.Error("one-shot Function protocol failed: invalid reply")
	}
	return result, err
}
