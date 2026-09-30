// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
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

// snapshot is a complete collection response.
type snapshot struct {
	Version string         `json:"version"`
	Metrics []metricSample `json:"metrics"`
	Checks  []checkSample  `json:"checks"`
}

type metricSample struct {
	Name   string            `json:"name"`
	Value  *float64          `json:"value"`
	Labels map[string]string `json:"labels"`
}

type checkSample struct {
	ID     string            `json:"id"`
	State  string            `json:"state"`
	Labels map[string]string `json:"labels"`
}

// Message shapes. Label keys are arbitrary; every other object uses exact field
// names. The Function result shape belongs to nativefunc.
var (
	labelsShape   = strictjson.Map(strictjson.Scalar())
	snapshotShape = strictjson.Object(strictjson.Fields{
		"metrics": strictjson.Object(strictjson.Fields{
			"labels": labelsShape,
		}, "name", "value"),
		"checks": strictjson.Object(strictjson.Fields{
			"labels": labelsShape,
		}, "id", "state"),
	}, "version")
	readyShape = strictjson.Object(nil, "version", "ready")
	replyShape = strictjson.Object(strictjson.Fields{
		"result": snapshotShape,
	}, "id", "error")
	functionReplyShape = strictjson.Object(strictjson.Fields{
		"result": nativefunc.ResultShape(),
	}, "id")
)

// decodeSnapshot validates a complete snapshot before any store writes.
func (d packageDefinition) decodeSnapshot(data []byte) (snapshot, error) {
	var result snapshot
	if err := strictjson.Decode(data, snapshotShape, &result); err != nil {
		return result, err
	}
	if result.Version != "v1" {
		return result, errors.New("unsupported response version")
	}
	if err := d.validateMetricSamples(result.Metrics); err != nil {
		return result, err
	}
	if err := d.validateCheckSamples(result.Checks); err != nil {
		return result, err
	}
	return result, nil
}

func (d packageDefinition) validateMetricSamples(samples []metricSample) error {
	seen := map[string]bool{}
	for i, sample := range samples {
		definition, ok := d.metricByName[sample.Name]
		if !ok {
			return fmt.Errorf("metric %d is not declared", i)
		}
		if sample.Value == nil || math.IsNaN(*sample.Value) || math.IsInf(*sample.Value, 0) ||
			(definition.Type == metricCounter && *sample.Value < 0) {
			return fmt.Errorf("metric %d has an invalid value", i)
		}
		if err := validateLabelKeys(sample.Labels); err != nil {
			return err
		}
		key := seriesIdentity(sample.Name, sample.Labels)
		if seen[key] {
			return errors.New("duplicate metric identity")
		}
		seen[key] = true
	}
	return nil
}

func (d packageDefinition) validateCheckSamples(samples []checkSample) error {
	seen := map[string]bool{}
	for i, sample := range samples {
		definition, ok := d.checkByID[sample.ID]
		if !ok {
			return fmt.Errorf("check %d is not declared", i)
		}
		if !slices.Contains(checkStates, sample.State) {
			return fmt.Errorf("check %d has an invalid state", i)
		}
		if err := validateLabelKeys(sample.Labels); err != nil {
			return err
		}
		labels := make(map[string]string, len(definition.ByLabels))
		for _, key := range definition.ByLabels {
			if sample.Labels[key] == "" {
				return fmt.Errorf("check %d is missing an identity label", i)
			}
			labels[key] = sample.Labels[key]
		}
		key := seriesIdentity(sample.ID, labels)
		if seen[key] {
			return errors.New("duplicate check identity")
		}
		seen[key] = true
	}
	return nil
}

func validateLabelKeys(labels map[string]string) error {
	for key := range labels {
		if !reIdentifier.MatchString(key) {
			return errors.New("invalid label key")
		}
	}
	return nil
}

func seriesIdentity(name string, labels map[string]string) string {
	// encoding/json sorts map keys and escapes delimiters, giving an unambiguous identity.
	if len(labels) == 0 {
		return name + ":{}"
	}
	encoded, _ := json.Marshal(labels)
	return name + ":" + string(encoded)
}

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

func (d packageDefinition) decodeReply(data []byte, id string) (snapshot, error) {
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
	return d.decodeSnapshot(reply.Result)
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
