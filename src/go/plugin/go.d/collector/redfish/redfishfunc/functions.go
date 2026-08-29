// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
)

const (
	inventoryMethodID         = "inventory"
	logsMethodID              = "logs"
	maxInventoryResponseBytes = 100*1024*1024 - 1
)

type selectorOption struct {
	id   string
	name string
}

type inventorySelectors struct {
	jobs           []string
	hosts          []string
	resourceKinds  []string
	requiredParams json.RawMessage
}

// Deps is the complete feature-state surface needed by Redfish Functions.
type Deps interface {
	VisitInventoryCatalog(
		ctx context.Context,
		maxJobs int,
		visitJob func(string) bool,
		visitHost func(uri, name string) bool,
		visitKind func(string) bool,
	) bool
	VisitInventorySlice(
		ctx context.Context,
		job, host, resourceKind string,
		visit func(map[string]any) bool,
	) (int, bool)
	AnyBackendAvailable() bool
	LogRoots() map[string]string
}

// Configs returns the process-level Redfish Function declarations.
func Configs(deps Deps) func() []funcapi.FunctionConfig {
	return func() []funcapi.FunctionConfig {
		return []funcapi.FunctionConfig{
			{
				ID:           inventoryMethodID,
				FunctionName: "redfish:inventory",
				Name:         "Redfish Inventory",
				UpdateEvery:  60,
				Help:         "Current standard Redfish hardware inventory and status",
				ResponseType: "table",
				RawRequest:   true,
			},
			{
				ID:           logsMethodID,
				FunctionName: "redfish:logs",
				Name:         "Redfish Logs",
				UpdateEvery:  1,
				Help:         "Query Redfish LogService entries",
				RequireCloud: true,
				Tags:         "logs",
				ResponseType: "logs",
				Available:    deps.AnyBackendAvailable,
				RawRequest:   true,
			},
		}
	}
}

// Handler returns a fresh raw Function handler.
func Handler(deps Deps) func(collectorapi.RuntimeJob) funcapi.MethodHandler {
	return func(_ collectorapi.RuntimeJob) funcapi.MethodHandler {
		return &functionHandler{
			deps: deps,
			logs: newLogsFunction(deps),
		}
	}
}

type functionHandler struct {
	deps Deps
	logs *logsFunction
}

var _ funcapi.RawMethodHandler = (*functionHandler)(nil)

func (h *functionHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (h *functionHandler) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.NotFoundResponse(method)
}

func (h *functionHandler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	switch req.Method {
	case inventoryMethodID:
		return h.handleInventory(ctx, req)
	case logsMethodID:
		return h.logs.handle(ctx, req)
	default:
		return funcapi.NotFoundResponse(req.Method)
	}
}

func (h *functionHandler) Cleanup(context.Context) {
	// Function handlers do not own external resources.
}

func (h *functionHandler) handleInventory(
	ctx context.Context,
	req funcapi.RawMethodRequest,
) *funcapi.FunctionResponse {
	selectors, jobs, catalogFits, err := buildInventorySelectors(ctx, h.deps, maxInventoryResponseBytes)
	if err != nil {
		return inventoryCanceledResponse(err)
	}
	if !catalogFits {
		return inventoryOversizeResponse(0, maxInventoryResponseBytes)
	}
	if req.Info {
		return boundedInventoryResponse(
			inventoryResponsePayload(inventoryColumns(""), []json.RawMessage{}, selectors),
			0,
		)
	}
	if jobs == 0 {
		return funcapi.UnavailableResponse("Redfish inventory has no active collector snapshots")
	}

	selections, err := parseInventorySelections(req.Payload)
	if err != nil {
		return funcapi.ErrorResponse(400, "Redfish inventory query failed: %v", err)
	}
	if err := validateInventorySelections(selectors, selections); err != nil {
		return funcapi.ErrorResponse(400, "Redfish inventory query failed: %v", err)
	}
	return buildInventoryDataResponseAt(ctx, h.deps, selectors, selections, maxInventoryResponseBytes)
}

func buildInventoryDataResponseAt(
	ctx context.Context,
	deps Deps,
	selectors inventorySelectors,
	selections map[string]string,
	maxBytes int,
) *funcapi.FunctionResponse {
	columns := inventoryColumnsForKind(selections["resource_kind"])
	payload := inventoryResponsePayload(buildInventoryColumns(columns), []json.RawMessage{}, selectors)
	baseSize, baseFits, err := boundedJSONSize(payload, maxBytes)
	if err != nil {
		return funcapi.InternalErrorResponse("encode Redfish inventory response: %v", err)
	}
	if !baseFits {
		return inventoryOversizeResponse(0, maxBytes)
	}
	data := payload["data"].([]json.RawMessage)
	encodedSize := baseSize
	overflow := false
	var encodeErr error
	totalRows, found := deps.VisitInventorySlice(
		ctx,
		selections["__job"],
		selections["host"],
		selections["resource_kind"],
		func(row map[string]any) bool {
			if err := ctx.Err(); err != nil {
				return false
			}
			values := inventoryDataRow(row, columns)
			separator := 0
			if len(data) > 0 {
				separator = 1
			}
			if separator > maxBytes-encodedSize {
				overflow = true
				return false
			}
			remaining := maxBytes - encodedSize - separator
			_, fits, err := boundedJSONSize(values, remaining)
			if err != nil {
				encodeErr = err
				return false
			}
			if !fits {
				overflow = true
				return false
			}
			encodedRow, err := json.Marshal(values)
			if err != nil {
				encodeErr = err
				return false
			}
			if len(encodedRow) > remaining {
				overflow = true
				return false
			}
			data = append(data, json.RawMessage(encodedRow))
			encodedSize += separator + len(encodedRow)
			return true
		},
	)
	if err := ctx.Err(); err != nil {
		return inventoryCanceledResponse(err)
	}
	if encodeErr != nil {
		return funcapi.InternalErrorResponse("encode Redfish inventory response: %v", encodeErr)
	}
	if overflow {
		return inventoryOversizeResponse(totalRows, maxBytes)
	}
	if !found || totalRows == 0 {
		return funcapi.ErrorResponse(
			400,
			"Redfish inventory query failed: the selected job, host, and resource kind do not identify a current slice",
		)
	}

	payload["data"] = data
	return boundedInventoryResponseAt(payload, totalRows, maxBytes)
}

func inventoryResponsePayload(
	columns map[string]any,
	data []json.RawMessage,
	selectors inventorySelectors,
) map[string]any {
	return map[string]any{
		"v":                   3,
		"update_every":        60,
		"status":              200,
		"type":                "table",
		"has_history":         false,
		"help":                "Current standard Redfish hardware inventory and status",
		"accepted_params":     []string{"__job", "host", "resource_kind"},
		"required_params":     selectors.requiredParams,
		"columns":             columns,
		"data":                data,
		"default_sort_column": "sort_key",
	}
}

func boundedInventoryResponse(payload map[string]any, rows int) *funcapi.FunctionResponse {
	return boundedInventoryResponseAt(payload, rows, maxInventoryResponseBytes)
}

func boundedInventoryResponseAt(
	payload map[string]any,
	rows int,
	maxBytes int,
) *funcapi.FunctionResponse {
	_, fits, err := boundedJSONSize(payload, maxBytes)
	if err != nil {
		return funcapi.InternalErrorResponse("encode Redfish inventory response: %v", err)
	}
	if !fits {
		return inventoryOversizeResponse(rows, maxBytes)
	}
	return funcapi.RawResponse(payload)
}

func inventoryOversizeResponse(rows, maxBytes int) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(
		413,
		"Redfish inventory result is too large: %d rows exceed the %d-byte encoded response limit",
		rows,
		maxBytes,
	)
}

func inventoryCanceledResponse(err error) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(503, "Redfish inventory query canceled: %v", err)
}

func boundedJSONSize(value any, maxBytes int) (int, bool, error) {
	if maxBytes < 0 {
		return 0, false, nil
	}
	size, err := jsonValueSize(reflect.ValueOf(value), maxBytes)
	if err != nil {
		return 0, false, err
	}
	if size > maxBytes {
		return size, false, nil
	}
	return size, true, nil
}

func jsonValueSize(value reflect.Value, remaining int) (int, error) {
	if remaining < 0 {
		return remaining + 1, nil
	}
	if !value.IsValid() {
		return jsonLeafSize(nil, remaining)
	}
	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			return jsonLeafSize(nil, remaining)
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return jsonLeafSize(nil, remaining)
		}
		return jsonValueSize(value.Elem(), remaining)
	}
	if value.CanInterface() {
		switch value.Interface().(type) {
		case json.Number, json.RawMessage, []byte:
			return jsonLeafSize(value.Interface(), remaining)
		}
	}

	switch value.Kind() {
	case reflect.Map:
		if value.IsNil() {
			return jsonLeafSize(nil, remaining)
		}
		if value.Type().Key().Kind() != reflect.String {
			return jsonLeafSize(value.Interface(), remaining)
		}
		size := 2
		if size > remaining {
			return size, nil
		}
		iterator := value.MapRange()
		first := true
		for iterator.Next() {
			if !first {
				size++
			}
			first = false
			if size > remaining {
				return size, nil
			}
			keySize, err := jsonLeafSize(iterator.Key().String(), remaining-size)
			if err != nil {
				return 0, err
			}
			size += keySize + 1
			if size > remaining {
				return size, nil
			}
			childSize, err := jsonValueSize(iterator.Value(), remaining-size)
			if err != nil {
				return 0, err
			}
			size += childSize
			if size > remaining {
				return size, nil
			}
		}
		return size, nil
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return jsonLeafSize(nil, remaining)
		}
		size := 2
		if size > remaining {
			return size, nil
		}
		for index := 0; index < value.Len(); index++ {
			if index > 0 {
				size++
			}
			if size > remaining {
				return size, nil
			}
			childSize, err := jsonValueSize(value.Index(index), remaining-size)
			if err != nil {
				return 0, err
			}
			size += childSize
			if size > remaining {
				return size, nil
			}
		}
		return size, nil
	default:
		return jsonLeafSize(value.Interface(), remaining)
	}
}

func jsonLeafSize(value any, remaining int) (int, error) {
	if value == nil {
		return boundedLeafSize(4, remaining), nil
	}
	if raw, ok := value.(json.RawMessage); ok {
		if !json.Valid(raw) {
			return 0, errors.New("invalid raw JSON value")
		}
		if len(raw) > remaining {
			return remaining + 1, nil
		}
		return len(raw), nil
	}
	if text, ok := value.(string); ok {
		return boundedJSONStringSize(text, remaining), nil
	}
	if number, ok := value.(json.Number); ok {
		if len(number) > remaining {
			return remaining + 1, nil
		}
		encoded, err := json.Marshal(number)
		if err != nil {
			return 0, err
		}
		return boundedLeafSize(len(encoded), remaining), nil
	}
	if bytes, ok := value.([]byte); ok {
		// JSON encodes []byte as a quoted base64 string.
		if len(bytes) > (remaining-2)/4*3+2 {
			return remaining + 1, nil
		}
		encodedSize := 2 + base64.StdEncoding.EncodedLen(len(bytes))
		if encodedSize > remaining {
			return remaining + 1, nil
		}
		encoded, err := json.Marshal(bytes)
		if err != nil {
			return 0, err
		}
		return boundedLeafSize(len(encoded), remaining), nil
	}
	switch value := value.(type) {
	case bool:
		if value {
			return boundedLeafSize(4, remaining), nil
		}
		return boundedLeafSize(5, remaining), nil
	case int:
		return boundedLeafSize(len(strconv.FormatInt(int64(value), 10)), remaining), nil
	case int8:
		return boundedLeafSize(len(strconv.FormatInt(int64(value), 10)), remaining), nil
	case int16:
		return boundedLeafSize(len(strconv.FormatInt(int64(value), 10)), remaining), nil
	case int32:
		return boundedLeafSize(len(strconv.FormatInt(int64(value), 10)), remaining), nil
	case int64:
		return boundedLeafSize(len(strconv.FormatInt(value, 10)), remaining), nil
	case uint:
		return boundedLeafSize(len(strconv.FormatUint(uint64(value), 10)), remaining), nil
	case uint8:
		return boundedLeafSize(len(strconv.FormatUint(uint64(value), 10)), remaining), nil
	case uint16:
		return boundedLeafSize(len(strconv.FormatUint(uint64(value), 10)), remaining), nil
	case uint32:
		return boundedLeafSize(len(strconv.FormatUint(uint64(value), 10)), remaining), nil
	case uint64:
		return boundedLeafSize(len(strconv.FormatUint(value, 10)), remaining), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	if len(encoded) > remaining {
		return remaining + 1, nil
	}
	return len(encoded), nil
}

func boundedLeafSize(size, remaining int) int {
	if size > remaining {
		return remaining + 1
	}
	return size
}

func boundedJSONStringSize(value string, remaining int) int {
	if remaining < 2 {
		return remaining + 1
	}
	size := 2
	add := func(bytes int) bool {
		if bytes > remaining-size {
			size = remaining + 1
			return false
		}
		size += bytes
		return true
	}
	for index := 0; index < len(value); {
		current := value[index]
		if current < utf8.RuneSelf {
			index++
			switch {
			case current == '\\' || current == '"' || current == '\b' || current == '\f' ||
				current == '\n' || current == '\r' || current == '\t':
				if !add(2) {
					return size
				}
			case current == '<' || current == '>' || current == '&' || current < 0x20:
				if !add(6) {
					return size
				}
			default:
				if !add(1) {
					return size
				}
			}
			continue
		}
		r, width := utf8.DecodeRuneInString(value[index:])
		index += width
		switch {
		case (r == utf8.RuneError && width == 1) || r == '\u2028' || r == '\u2029':
			if !add(6) {
				return size
			}
		default:
			if !add(width) {
				return size
			}
		}
	}
	return size
}

func parseInventorySelections(payload []byte) (map[string]string, error) {
	var request struct {
		Selections map[string][]string `json:"selections"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return nil, fmt.Errorf("invalid JSON request: %w", err)
	}
	result := make(map[string]string, 3)
	for _, id := range []string{"__job", "host", "resource_kind"} {
		values := request.Selections[id]
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			return nil, fmt.Errorf("selector %q requires exactly one value", id)
		}
		result[id] = values[0]
	}
	return result, nil
}

func validateInventorySelections(selectors inventorySelectors, selected map[string]string) error {
	for _, selector := range []struct {
		id     string
		values []string
	}{
		{id: "__job", values: selectors.jobs},
		{id: "host", values: selectors.hosts},
		{id: "resource_kind", values: selectors.resourceKinds},
	} {
		value := selected[selector.id]
		if _, ok := slices.BinarySearch(selector.values, value); !ok {
			return fmt.Errorf("selector %q does not contain option %q", selector.id, value)
		}
	}
	return nil
}

func buildInventorySelectors(
	ctx context.Context,
	deps Deps,
	maxBytes int,
) (inventorySelectors, int, bool, error) {
	var jobs []string
	type hostLabel struct {
		name       string
		conflicted bool
	}
	hostLabels := make(map[string]hostLabel)
	kinds := make(map[string]struct{})
	lowerBound := 0
	addBytes := func(addition int) bool {
		if addition < 0 || addition > maxBytes-lowerBound {
			return false
		}
		lowerBound += addition
		return true
	}
	addOption := func(value string) bool {
		// Every option emits {"id":<value>,"name":<value>} at minimum.
		if !addBytes(len(`{"id":,"name":}`)) {
			return false
		}
		for range 2 {
			remaining := maxBytes - lowerBound
			size := boundedJSONStringSize(value, remaining)
			if size > remaining || !addBytes(size) {
				return false
			}
		}
		return true
	}
	const minimumEncodedJobOptionBytes = 21 // One non-empty ID/name option, before separators.
	maxJobs := maxBytes / minimumEncodedJobOptionBytes
	complete := deps.VisitInventoryCatalog(
		ctx,
		maxJobs,
		func(job string) bool {
			if ctx.Err() != nil {
				return false
			}
			if job == "" || !addOption(job) {
				return false
			}
			jobs = append(jobs, job)
			return true
		},
		func(uri, name string) bool {
			if ctx.Err() != nil {
				return false
			}
			if uri == "" {
				return true
			}
			current, exists := hostLabels[uri]
			if !exists {
				if !addOption(uri) {
					return false
				}
				hostLabels[uri] = hostLabel{name: name}
				return true
			}
			if !current.conflicted && current.name != name {
				current.conflicted = true
				current.name = ""
				hostLabels[uri] = current
			}
			return true
		},
		func(kind string) bool {
			if ctx.Err() != nil {
				return false
			}
			if kind == "" {
				return true
			}
			if _, exists := kinds[kind]; exists {
				return true
			}
			if !addOption(kind) {
				return false
			}
			kinds[kind] = struct{}{}
			return true
		},
	)
	if err := ctx.Err(); err != nil {
		return inventorySelectors{}, len(jobs), false, err
	}
	if !complete {
		return inventorySelectors{}, len(jobs), false, nil
	}
	if err := sortInventoryValues(ctx, jobs, strings.Compare); err != nil {
		return inventorySelectors{}, len(jobs), false, err
	}
	hostOptions := make([]selectorOption, 0, min(len(hostLabels), 1_024))
	hostOptionBytes := 0
	addHostOption := func(option selectorOption) bool {
		size, fits, err := boundedJSONSize(
			map[string]any{"id": option.id, "name": option.name},
			maxBytes-hostOptionBytes,
		)
		if err != nil || !fits {
			return false
		}
		hostOptionBytes += size
		hostOptions = append(hostOptions, option)
		return true
	}
	for uri, label := range hostLabels {
		if err := ctx.Err(); err != nil {
			return inventorySelectors{}, len(jobs), false, err
		}
		if label.conflicted || label.name == "" {
			if !addHostOption(selectorOption{id: uri, name: uri}) {
				return inventorySelectors{}, len(jobs), false, nil
			}
			continue
		}
		if len(uri)+3 > maxBytes || len(label.name) > maxBytes-len(uri)-3 {
			return inventorySelectors{}, len(jobs), false, nil
		}
		name := label.name + " (" + uri + ")"
		if !addHostOption(selectorOption{id: uri, name: name}) {
			return inventorySelectors{}, len(jobs), false, nil
		}
	}
	if err := sortInventoryValues(ctx, hostOptions, func(a, b selectorOption) int {
		return strings.Compare(a.id, b.id)
	}); err != nil {
		return inventorySelectors{}, len(jobs), false, err
	}
	kindValues := make([]string, 0, min(len(kinds), 1_024))
	for kind := range kinds {
		if err := ctx.Err(); err != nil {
			return inventorySelectors{}, len(jobs), false, err
		}
		kindValues = append(kindValues, kind)
	}
	if err := sortInventoryValues(ctx, kindValues, strings.Compare); err != nil {
		return inventorySelectors{}, len(jobs), false, err
	}
	required, fits, err := encodeInventoryRequiredParams(
		ctx,
		maxBytes,
		jobs,
		hostOptions,
		kindValues,
	)
	if err != nil || !fits {
		return inventorySelectors{}, len(jobs), false, err
	}
	hostValues := make([]string, 0, len(hostOptions))
	for _, option := range hostOptions {
		if err := ctx.Err(); err != nil {
			return inventorySelectors{}, len(jobs), false, err
		}
		hostValues = append(hostValues, option.id)
	}
	return inventorySelectors{
		jobs:           jobs,
		hosts:          hostValues,
		resourceKinds:  kindValues,
		requiredParams: required,
	}, len(jobs), true, nil
}

func encodeInventoryRequiredParams(
	ctx context.Context,
	maxBytes int,
	jobs []string,
	hosts []selectorOption,
	kinds []string,
) (json.RawMessage, bool, error) {
	var encoded []byte
	var ok bool
	if encoded, ok = appendBoundedJSON(encoded, []byte{'['}, maxBytes); !ok {
		return nil, false, nil
	}
	params := []struct {
		id     string
		name   string
		help   string
		count  int
		option func(int) selectorOption
	}{
		{
			"__job", "Redfish Job", "Select one Redfish endpoint job", len(jobs),
			func(index int) selectorOption { return selectorOption{id: jobs[index], name: jobs[index]} },
		},
		{
			"host", "Host", "Select one ComputerSystem or service scope", len(hosts),
			func(index int) selectorOption { return hosts[index] },
		},
		{
			"resource_kind", "Resource Kind", "Select one Redfish resource kind", len(kinds),
			func(index int) selectorOption { return selectorOption{id: kinds[index], name: kinds[index]} },
		},
	}
	for paramIndex, param := range params {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if paramIndex > 0 {
			if encoded, ok = appendBoundedJSON(encoded, []byte{','}, maxBytes); !ok {
				return nil, false, nil
			}
		}
		if encoded, ok = appendBoundedJSON(encoded, []byte(`{"id":`), maxBytes); !ok {
			return nil, false, nil
		}
		var err error
		if encoded, ok, err = appendBoundedJSONString(encoded, param.id, maxBytes); err != nil || !ok {
			return nil, false, err
		}
		if encoded, ok = appendBoundedJSON(encoded, []byte(`,"name":`), maxBytes); !ok {
			return nil, false, nil
		}
		if encoded, ok, err = appendBoundedJSONString(encoded, param.name, maxBytes); err != nil || !ok {
			return nil, false, err
		}
		if encoded, ok = appendBoundedJSON(encoded, []byte(`,"type":"select","options":[`), maxBytes); !ok {
			return nil, false, nil
		}
		for optionIndex := range param.count {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			option := param.option(optionIndex)
			if optionIndex > 0 {
				if encoded, ok = appendBoundedJSON(encoded, []byte{','}, maxBytes); !ok {
					return nil, false, nil
				}
			}
			if encoded, ok = appendBoundedJSON(encoded, []byte(`{"id":`), maxBytes); !ok {
				return nil, false, nil
			}
			if encoded, ok, err = appendBoundedJSONString(encoded, option.id, maxBytes); err != nil || !ok {
				return nil, false, err
			}
			if encoded, ok = appendBoundedJSON(encoded, []byte(`,"name":`), maxBytes); !ok {
				return nil, false, nil
			}
			if encoded, ok, err = appendBoundedJSONString(encoded, option.name, maxBytes); err != nil || !ok {
				return nil, false, err
			}
			if optionIndex == 0 {
				if encoded, ok = appendBoundedJSON(encoded, []byte(`,"defaultSelected":true`), maxBytes); !ok {
					return nil, false, nil
				}
			}
			if encoded, ok = appendBoundedJSON(encoded, []byte{'}'}, maxBytes); !ok {
				return nil, false, nil
			}
		}
		if encoded, ok = appendBoundedJSON(encoded, []byte(`],"help":`), maxBytes); !ok {
			return nil, false, nil
		}
		if encoded, ok, err = appendBoundedJSONString(encoded, param.help, maxBytes); err != nil || !ok {
			return nil, false, err
		}
		if encoded, ok = appendBoundedJSON(encoded, []byte{'}'}, maxBytes); !ok {
			return nil, false, nil
		}
	}
	if encoded, ok = appendBoundedJSON(encoded, []byte{']'}, maxBytes); !ok {
		return nil, false, nil
	}
	return json.RawMessage(encoded), true, nil
}

func sortInventoryValues[T any](
	ctx context.Context,
	values []T,
	compare func(a, b T) int,
) error {
	const checkEvery = 1_024
	siftDown := func(root, end int) {
		for {
			child := 2*root + 1
			if child >= end {
				return
			}
			if child+1 < end && compare(values[child], values[child+1]) < 0 {
				child++
			}
			if compare(values[root], values[child]) >= 0 {
				return
			}
			values[root], values[child] = values[child], values[root]
			root = child
		}
	}
	for root := len(values)/2 - 1; root >= 0; root-- {
		if root%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		siftDown(root, len(values))
	}
	for end := len(values) - 1; end > 0; end-- {
		if end%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		values[0], values[end] = values[end], values[0]
		siftDown(0, end)
	}
	return ctx.Err()
}

func appendBoundedJSONString(dst []byte, value string, maxBytes int) ([]byte, bool, error) {
	if maxBytes < len(dst) || boundedJSONStringSize(value, maxBytes-len(dst)) > maxBytes-len(dst) {
		return dst, false, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false, err
	}
	result, ok := appendBoundedJSON(dst, encoded, maxBytes)
	return result, ok, nil
}

func appendBoundedJSON(dst, value []byte, maxBytes int) ([]byte, bool) {
	if maxBytes < len(dst) || len(value) > maxBytes-len(dst) {
		return dst, false
	}
	required := len(dst) + len(value)
	if required > cap(dst) {
		capacity := max(required, max(1, cap(dst))*2)
		capacity = min(capacity, maxBytes)
		grown := make([]byte, len(dst), capacity)
		copy(grown, dst)
		dst = grown
	}
	return append(dst, value...), true
}

type inventoryColumn struct {
	ID      string
	Members map[string]struct{}
	funcapi.ColumnMeta
}

var inventoryColumnRegistry = compileInventoryColumns()

func compileInventoryColumns() []inventoryColumn {
	contract := registry.MustCompile()
	result := make([]inventoryColumn, 0, len(contract.Columns))
	for _, source := range contract.Columns {
		column := inventoryColumn{
			ID:      source.ID,
			Members: source.Members,
			ColumnMeta: funcapi.ColumnMeta{
				Name: source.Name, Tooltip: source.Tooltip, Units: source.Units,
				Visible: source.Visible, Sortable: source.Sortable,
				Sticky: source.Sticky, UniqueKey: source.Unique,
				Sort:          funcapi.FieldSortAscending,
				Summary:       funcapi.FieldSummaryCount,
				Visualization: funcapi.FieldVisualValue,
			},
		}
		switch source.Type {
		case registry.ColumnEnum:
			column.Type = funcapi.FieldTypeString
			column.Transform = funcapi.FieldTransformText
			column.Visualization = funcapi.FieldVisualPill
			if source.Facet {
				column.Filter = funcapi.FieldFilterMultiselect
			}
		case registry.ColumnInteger:
			column.Type = funcapi.FieldTypeInteger
			column.Transform = funcapi.FieldTransformNumber
			column.Sort = funcapi.FieldSortDescending
			if source.Facet {
				column.Filter = funcapi.FieldFilterRange
			}
		case registry.ColumnFloat:
			column.Type = funcapi.FieldTypeFloat
			column.Transform = funcapi.FieldTransformNumber
			column.DecimalPoints = 3
			column.Sort = funcapi.FieldSortDescending
			if source.Facet {
				column.Filter = funcapi.FieldFilterRange
			}
		case registry.ColumnBoolean:
			column.Type = funcapi.FieldTypeBoolean
			column.Visualization = funcapi.FieldVisualPill
			if source.Facet {
				column.Filter = funcapi.FieldFilterMultiselect
			}
		case registry.ColumnTimestamp:
			column.Type = funcapi.FieldTypeTimestamp
			column.Transform = funcapi.FieldTransformDatetime
			column.Sort = funcapi.FieldSortDescending
			if source.Facet {
				column.Filter = funcapi.FieldFilterRange
			}
		default:
			column.Type = funcapi.FieldTypeString
			column.Transform = funcapi.FieldTransformText
			if source.Facet {
				column.Filter = funcapi.FieldFilterMultiselect
			}
			if source.Structured {
				column.Wrap = true
			}
		}
		if source.Additive {
			column.Summary = funcapi.FieldSummarySum
		}
		result = append(result, column)
	}
	return result
}

func inventoryColumns(kind string) map[string]any {
	return buildInventoryColumns(inventoryColumnsForKind(kind))
}

func buildInventoryColumns(columns []inventoryColumn) map[string]any {
	set := funcapi.Columns(columns, func(column inventoryColumn) funcapi.ColumnMeta {
		return column.ColumnMeta
	})
	return set.BuildColumns()
}

func inventoryColumnsForKind(kind string) []inventoryColumn {
	if kind == "" {
		return inventoryColumnRegistry
	}
	result := make([]inventoryColumn, 0, len(inventoryColumnRegistry))
	var reading []inventoryColumn
	for _, column := range inventoryColumnRegistry {
		if _, ok := column.Members["__reading__"]; ok {
			reading = append(reading, column)
			continue
		}
		if len(column.Members) == 0 {
			result = append(result, column)
			continue
		}
		if _, ok := column.Members[kind]; ok {
			result = append(result, column)
		}
	}
	return append(result, reading...)
}

func inventoryDataRow(row map[string]any, columns []inventoryColumn) []any {
	values := make([]any, 0, len(columns))
	for _, column := range columns {
		values = append(values, row[column.ID])
	}
	return values
}

func mapString(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return value
}
