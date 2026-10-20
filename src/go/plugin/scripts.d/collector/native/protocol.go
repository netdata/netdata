// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/strictjson"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
)

// Operations appended to the configured command.
const (
	opCollect  = "collect"
	opFunction = "function"
	opDescribe = "describe"
	opServe    = "serve"
)

// Bound encoded messages, including envelopes and whitespace, generously enough
// for complete snapshots, tables and base64 Function payloads. This is a runaway
// output cutoff, not a per-job memory budget; buffers still grow on demand.
const maxMessageBytes = 64 << 20

var errResponseTooLarge = errors.New("script response exceeds 64 MiB")

// errCollectionFailed is the script's fixed persistent reply for a collection
// attempt without a trustworthy snapshot. The session stays usable.
var errCollectionFailed = errors.New("script reported collection_failed")

// maxRequestID is the widest persistent request ID. Function requests reserve
// it before queue admission.
var maxRequestID = strconv.FormatUint(math.MaxUint64, 10)

// Envelope shapes are exact. The payload shape belongs to each operation.
var (
	readyShape = strictjson.Object(nil, "version", "ready")
	replyShape = strictjson.Object(strictjson.Fields{
		"result": snapshotShape,
	}, "id", "error")
	functionReplyShape = strictjson.Object(strictjson.Fields{
		"result": nativefunc.ResultShape(),
	}, "id")
)

func decodeReady(data []byte) error {
	var result struct {
		Version string `json:"version"`
		Ready   bool   `json:"ready"`
	}
	if err := strictjson.Decode(data, readyShape, &result); err != nil {
		return err
	}
	if result.Version != "v1" || !result.Ready {
		return errors.New("expected v1 ready handshake")
	}
	return nil
}

// encodeCollectRequest uses the canonical spelling that Bash peers match
// without a JSON parser.
func encodeCollectRequest(id string) []byte {
	return []byte(`{"id":"` + id + `","method":"collect"}` + "\n")
}

func decodeReply(data []byte, id string) (snapshot, error) {
	var reply struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *string         `json:"error"`
	}
	if err := strictjson.Decode(data, replyShape, &reply); err != nil {
		return snapshot{}, err
	}
	if reply.ID != id {
		return snapshot{}, errors.New("unexpected response id")
	}
	if (len(reply.Result) != 0) == (reply.Error != nil) {
		return snapshot{}, errors.New("reply must contain exactly one of result or error")
	}
	if reply.Error != nil {
		if *reply.Error != "collection_failed" {
			return snapshot{}, errors.New("unknown collection error code")
		}
		return snapshot{}, errCollectionFailed
	}
	return decodeSnapshot(reply.Result)
}

// encodeFunctionRequest encodes one LF-terminated Function request frame. The
// caller's deadline is forwarded so the script can bound its own work.
func encodeFunctionRequest(ctx context.Context, id string, request funcapi.RawMethodRequest) ([]byte, error) {
	var deadlineMS int64
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
	}{
		ID:          id,
		Method:      "function",
		Function:    request.Method,
		Info:        request.Info,
		Args:        args,
		Payload:     request.Payload,
		ContentType: request.ContentType,
		DeadlineMS:  deadlineMS,
		Permissions: request.Permissions,
		Source:      request.Source,
	})
	if err != nil {
		return nil, errors.New("cannot encode Function request")
	}
	if len(data)+1 > maxMessageBytes {
		return nil, errors.New("Function request exceeds 64 MiB")
	}
	return append(data, '\n'), nil
}

func decodeFunctionReply(data []byte, id string, info bool) (*funcapi.FunctionResponse, error) {
	var reply struct {
		ID     string             `json:"id"`
		Result *nativefunc.Result `json:"result"`
	}
	// Numbers stay json.Number so arbitrary Function data keeps large integers.
	if err := strictjson.DecodeUseNumber(data, functionReplyShape, &reply); err != nil {
		return nil, err
	}
	if reply.ID != id || reply.Result == nil {
		return nil, errors.New("invalid Function reply envelope")
	}
	return reply.Result.Response(info)
}
