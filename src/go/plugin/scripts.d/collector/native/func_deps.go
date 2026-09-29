// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
)

func (c *Collector) ExecuteFunction(
	ctx context.Context,
	request funcapi.RawMethodRequest,
) (*funcapi.FunctionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	declared := false
	for _, method := range c.definition.methods {
		if method.ID == request.Method {
			declared = true
			break
		}
	}
	if !c.initialized || !c.bound || !declared {
		return nil, fmt.Errorf("native Function is unavailable")
	}
	// Reserve the largest session ID before queue admission. Rejection must not
	// touch an existing process, even when ingress allowed a larger payload.
	if _, err := encodeFunctionRequest(request, "18446744073709551615", ctx); err != nil {
		return nil, err
	}
	if c.definition.Mode == modePersistent {
		result := c.requestPersistent(ctx, &request)
		return result.function, result.err
	}
	frame, err := encodeFunctionRequest(request, "1", ctx)
	if err != nil {
		return nil, err
	}
	input := append(bytes.Clone(c.configInput), frame...)
	data, err := runOperation(ctx, c.definition.Command, "function", input)
	if err != nil {
		c.Errorf("one-shot Function command failed: %s", commandFailureReason(err))
		return nil, err
	}
	result, err := decodeFunctionReply(data, "1", request.Info)
	if err != nil {
		// Decoder errors can contain script-controlled field names or values.
		c.Error("one-shot Function protocol failed: invalid reply")
	}
	return result, err
}

// Never log the original error: command errors can include paths, and output
// belongs to the script. Numeric exit status and fixed categories are safe.
func commandFailureReason(err error) string {
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, context.Canceled):
		return "caller canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "caller deadline exceeded"
	case errors.Is(err, errResponseTooLarge):
		return "response exceeds 64 MiB"
	case errors.As(err, &exitErr):
		if exitErr.ExitCode() < 0 {
			return "command terminated by signal"
		}
		return fmt.Sprintf("command exited with status %d", exitErr.ExitCode())
	default:
		return "command execution failed"
	}
}

func encodeFunctionRequest(request funcapi.RawMethodRequest, id string, ctx context.Context) ([]byte, error) {
	deadlineMS := int64(0)
	if deadline, ok := ctx.Deadline(); ok {
		deadlineMS = deadline.UnixMilli()
	}
	args := request.Args
	if args == nil {
		args = []string{}
	}
	data, err := json.Marshal(struct {
		ID          string   `json:"id"`
		Method      string   `json:"method"`
		Function    string   `json:"function"`
		Info        bool     `json:"info"`
		Args        []string `json:"args"`
		Payload     []byte   `json:"payload_base64,omitempty"`
		ContentType string   `json:"content_type,omitempty"`
		DeadlineMS  int64    `json:"deadline_unix_ms,omitempty"`
		Permissions string   `json:"permissions,omitempty"`
		Source      string   `json:"source,omitempty"`
	}{id, "function", request.Method, request.Info, args, request.Payload, request.ContentType, deadlineMS, request.Permissions, request.Source})
	if err != nil {
		return nil, fmt.Errorf("cannot encode Function request")
	}
	if len(data)+1 > maxMessageBytes {
		return nil, fmt.Errorf("Function request exceeds 64 MiB")
	}
	return append(data, '\n'), nil
}

func decodeFunctionReply(data []byte, id string, info bool) (*funcapi.FunctionResponse, error) {
	var reply struct {
		ID     string             `json:"id"`
		Result *nativefunc.Result `json:"result"`
	}
	if err := decodeMessage(data, "function_reply", &reply); err != nil {
		return nil, err
	}
	if reply.ID != id || reply.Result == nil {
		return nil, fmt.Errorf("invalid Function reply envelope")
	}
	return reply.Result.Response(info)
}
