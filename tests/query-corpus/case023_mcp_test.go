// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

type c023MCPResponse struct {
	Document map[string]any
	Session  string
}

type c023MCPValidCall struct {
	label, group, expression string
	metric, dimension        string
	after, before            int64
	units                    string
	value, anomalyRate       float64
	annotations              int64
}

func c023MCPDecodeMessage(payload []byte, id int) (map[string]any, bool) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, false
	}
	got, ok := doc["id"].(float64)
	return doc, ok && got == float64(id)
}

func c023MCPDecodeJSONResponse(t *testing.T, body []byte, id int) map[string]any {
	t.Helper()
	doc, ok := c023MCPDecodeMessage(body, id)
	if !ok {
		t.Fatalf("MCP response is not the JSON-RPC response for id %d: %q", id, body)
	}
	return doc
}

func c023MCPDecodeSSEResponse(t *testing.T, body io.Reader, id int) map[string]any {
	t.Helper()

	// A valid Streamable HTTP response may keep its SSE connection open.
	// Stop as soon as this request's response event arrives.
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	var data []string
	decodeEvent := func() map[string]any {
		if len(data) == 0 {
			return nil
		}
		doc, ok := c023MCPDecodeMessage([]byte(strings.Join(data, "\n")), id)
		data = data[:0]
		if ok {
			return doc
		}
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if doc := decodeEvent(); doc != nil {
				return doc
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("MCP SSE response failed before JSON-RPC id %d arrived: %v", id, err)
	}
	if doc := decodeEvent(); doc != nil {
		return doc
	}
	t.Fatalf("MCP SSE response ended without JSON-RPC id %d", id)
	return nil
}

func c023MCPValidSessionID(session string) bool {
	if session == "" {
		return false
	}
	for _, c := range []byte(session) {
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

func c023MCPDo(t *testing.T, envelope map[string]any, session string) *http.Response {
	t.Helper()

	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, td.BaseURL+"/mcp", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func c023MCPPost(t *testing.T, id int, method string, params map[string]any, session string) c023MCPResponse {
	t.Helper()

	resp := c023MCPDo(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}, session)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatalf("MCP %s HTTP status = %d, want 200; body=%q", method, resp.StatusCode, body)
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("MCP %s Content-Type is invalid: %q: %v", method, resp.Header.Get("Content-Type"), err)
	}
	var doc map[string]any
	switch mediaType {
	case "application/json":
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 {
			t.Fatalf("MCP %s returned an empty body", method)
		}
		doc = c023MCPDecodeJSONResponse(t, body, id)
	case "text/event-stream":
		doc = c023MCPDecodeSSEResponse(t, resp.Body, id)
	default:
		t.Fatalf("MCP response Content-Type = %q, want application/json or text/event-stream", mediaType)
	}
	if got := doc["jsonrpc"]; got != "2.0" {
		t.Errorf("MCP %s jsonrpc = %v, want 2.0", method, got)
	}
	if got, ok := doc["id"].(float64); !ok || got != float64(id) {
		t.Errorf("MCP %s id = %v, want %d", method, doc["id"], id)
	}

	gotSession := resp.Header.Get("Mcp-Session-Id")
	if gotSession != "" && !c023MCPValidSessionID(gotSession) {
		t.Fatalf("MCP %s returned a non-visible-ASCII Mcp-Session-Id %q", method, gotSession)
	}
	if session != "" && gotSession != "" && gotSession != session {
		t.Errorf("MCP %s changed session id from %q to %q", method, session, gotSession)
	}
	if gotSession == "" {
		gotSession = session
	}

	return c023MCPResponse{Document: doc, Session: gotSession}
}

func c023MCPNotify(t *testing.T, method string, params map[string]any, session string) {
	t.Helper()

	resp := c023MCPDo(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}, session)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusAccepted || len(body) != 0 {
		t.Fatalf("MCP notification %s returned HTTP %d body=%q, want 202 with no body",
			method, resp.StatusCode, body)
	}
}

func c023MCPResult(t *testing.T, response map[string]any, method string) map[string]any {
	t.Helper()
	if errObj, has := response["error"]; has {
		t.Fatalf("MCP %s returned an error, want success: %v", method, errObj)
	}
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP %s result is missing or malformed: %v", method, response["result"])
	}
	return result
}

func c023MCPCall(t *testing.T, id int, session string, arguments map[string]any) c023MCPResponse {
	t.Helper()
	return c023MCPPost(t, id, "tools/call", map[string]any{
		"name":      "query_metrics",
		"arguments": arguments,
	}, session)
}

func c023MCPQueryDocument(t *testing.T, response map[string]any) map[string]any {
	t.Helper()

	result := c023MCPResult(t, response, "tools/call")
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("MCP tools/call result.content has type %T and %d items, want exactly one text item",
			result["content"], len(content))
	}
	if raw, has := result["isError"]; has {
		isError, ok := raw.(bool)
		if !ok {
			t.Fatalf("MCP tools/call result.isError has type %T, want boolean", raw)
		}
		if isError {
			t.Fatal("MCP tools/call returned result.isError=true, want success")
		}
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("MCP tools/call result.content[0] is malformed: %v", content[0])
	}
	if got := item["type"]; got != "text" {
		t.Fatalf("MCP tools/call content type = %v, want text", got)
	}
	text, ok := item["text"].(string)
	if !ok || text == "" {
		t.Fatalf("MCP tools/call content text is empty or malformed: %v", item["text"])
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("MCP query_metrics content is not JSON: %v; text=%q", err, text)
	}
	return doc
}

func c023MCPResultRow(t *testing.T, doc map[string]any) ([]any, []any) {
	t.Helper()

	result, ok := doc["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP query result is missing or malformed: %T", doc["result"])
	}

	rows, ok := result["data"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("MCP query data has type %T and %d rows, want exactly 1",
			result["data"], len(rows))
	}
	row, ok := rows[0].([]any)
	if !ok || len(row) != 2 {
		t.Fatalf("MCP query row = %v, want exactly [time point]", rows[0])
	}
	point, ok := row[1].([]any)
	if !ok || len(point) != 3 {
		t.Fatalf("MCP query point = %v, want exactly [value anomaly_rate point_annotations]", row[1])
	}
	return row, point
}

func c023MCPAssertResultSchema(t *testing.T, doc map[string]any, dimension string) {
	t.Helper()

	result, ok := doc["result"].(map[string]any)
	if !ok {
		t.Fatalf("MCP query result is missing or malformed: %T", doc["result"])
	}
	labels, ok := result["labels"].([]any)
	if !ok || len(labels) != 2 || labels[0] != "time" || labels[1] != dimension {
		t.Fatalf("MCP query labels = %v, want exactly [time %s]", result["labels"], dimension)
	}
	pointSchema, ok := result["point_schema"].(map[string]any)
	if !ok || len(pointSchema) != 3 {
		t.Fatalf("MCP query point_schema has type %T and %d fields, want exactly 3",
			result["point_schema"], len(pointSchema))
	}
	for field, wantIndex := range map[string]float64{
		"value":                    0,
		"anomaly_rate_percent":     1,
		"point_annotations_bitmap": 2,
	} {
		index, ok := pointSchema[field].(float64)
		if !ok || index != wantIndex {
			t.Errorf("MCP query point_schema.%s = %v, want exactly %.0f",
				field, pointSchema[field], wantIndex)
		}
	}
	row, point := c023MCPResultRow(t, doc)
	if _, ok := row[0].(string); !ok {
		t.Errorf("MCP query row time has type %T, want string", row[0])
	}
	for i, field := range []string{"value", "anomaly_rate_percent", "point_annotations_bitmap"} {
		if _, ok := point[i].(float64); !ok {
			t.Errorf("MCP query point %s has type %T, want number", field, point[i])
		}
	}
}

func c023MCPAssertTimestamp(t *testing.T, doc map[string]any, timestamp int64) {
	t.Helper()

	row, _ := c023MCPResultRow(t, doc)

	timeString, ok := row[0].(string)
	if !ok {
		t.Fatalf("MCP query row time has type %T, want RFC3339 string", row[0])
	}
	if _, err := time.Parse(time.RFC3339, timeString); err != nil {
		t.Fatalf("MCP query row time %q is not RFC3339: %v", timeString, err)
	}
	wantTime := time.Unix(timestamp, 0).UTC().Format(time.RFC3339)
	if timeString != wantTime {
		t.Errorf("MCP query row time = %q, want exactly %q", timeString, wantTime)
	}
}

func c023MCPAssertPointField(t *testing.T, doc map[string]any, index int, field string, want, tolerance float64) {
	t.Helper()

	_, point := c023MCPResultRow(t, doc)
	value, ok := point[index].(float64)
	if !ok {
		t.Fatalf("MCP query point %s has type %T, want number", field, point[index])
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value-want) > tolerance {
		t.Errorf("MCP query point %s = %v, want exactly %v", field, value, want)
	}
}

func c023MCPAssertEcho(t *testing.T, doc map[string]any, group, expression string) {
	t.Helper()

	request := queryObject(t, doc, "request", "query request")
	aggregations := queryObject(t, request, "aggregations", "query request.aggregations")
	timeAggregation := queryObject(t, aggregations, "time", "query request.aggregations.time")
	wantGroup := group
	if group == "countif" {
		wantGroup = "percentage-of-samples"
	}
	if got := timeAggregation["time_group"]; got != wantGroup {
		t.Errorf("MCP %s canonical time_group echo = %v, want %q", group, got, wantGroup)
	}
	if got := timeAggregation["time_group_options"]; got != expression {
		t.Errorf("MCP %s time_group_options echo = %v, want %q", group, got, expression)
	}
}

func c023MCPAssertOptionsError(t *testing.T, response map[string]any, group string) {
	t.Helper()

	if result, has := response["result"]; has {
		if object, ok := result.(map[string]any); ok {
			t.Errorf("MCP accepted invalid time_group_options for %s: result keys=%v", group, keys(object))
		} else {
			t.Errorf("MCP accepted invalid time_group_options for %s: result type=%T", group, result)
		}
		return
	}
	errorObject, ok := response["error"].(map[string]any)
	if !ok {
		t.Errorf("MCP invalid time_group_options for %s returned no structured error: %v", group, response)
		return
	}
	if got, ok := errorObject["code"].(float64); !ok || got != -32602 {
		t.Errorf("MCP %s error.code = %v, want -32602 Invalid Params", group, errorObject["code"])
	}
	message, ok := errorObject["message"].(string)
	if !ok || message == "" {
		t.Errorf("MCP %s error.message is empty or malformed: %v", group, errorObject["message"])
	}

	data, ok := errorObject["data"].(map[string]any)
	if !ok {
		t.Errorf("MCP %s error.data is missing or malformed: %v", group, errorObject["data"])
		return
	}
	if got := data["status"]; got != "error" {
		t.Errorf("MCP %s error.data.status = %v, want error", group, got)
	}
	if got := data["code"]; got != "INVALID_PARAMS" {
		t.Errorf("MCP %s error.data.code = %v, want INVALID_PARAMS", group, got)
	}
	if got, ok := data["codeNumeric"].(float64); !ok || got != 2 {
		t.Errorf("MCP %s error.data.codeNumeric = %v, want 2", group, data["codeNumeric"])
	}
	if got, ok := data["message"].(string); !ok || got == "" {
		t.Errorf("MCP %s error.data.message is empty or malformed: %v", group, data["message"])
	}
}

func c023MCPPushCadenceFixture(t *testing.T, host, context string) (int64, int64) {
	t.Helper()

	const (
		firstSamples  = 5
		secondSamples = 10
		secondEvery   = 10
	)
	base := int64(fixture.T0 + 1000)
	conn := connect(t, host, guid(342), stream.CapsLive)

	first := fixture.Chart{
		ID: context, Title: "MCP cadence", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	for i := 1; i <= firstSamples; i++ {
		first.Dimensions[0].Points = append(first.Dimensions[0].Points, fixture.Point{
			T: base + int64(i), Collected: "1", Flags: stream.FlagNotAnomalous,
		})
	}
	first.Define(conn)
	first.PushLive(conn)

	second := fixture.Chart{
		ID: context, Title: "MCP cadence", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: secondEvery,
		Dimensions: []fixture.Dimension{{ID: "value"}},
	}
	transition := base + firstSamples
	for i := 1; i <= secondSamples; i++ {
		second.Dimensions[0].Points = append(second.Dimensions[0].Points, fixture.Point{
			T: transition + int64(i*secondEvery), Collected: "0", Flags: stream.FlagNotAnomalous,
		})
	}
	second.Define(conn)
	second.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention(host, context, first.FirstT(), second.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	return base, second.LastT()
}

func TestCase023MCPQueryMetricsContract(t *testing.T) {
	for _, contract := range []string{
		"CASE-023/mcp-protocol-lifecycle",
		"CASE-023/mcp-query-tool-schema",
		"CASE-023/mcp-valid-result-schema",
		"CASE-023/mcp-valid-query-units",
		"CASE-023/mcp-valid-query-echo",
		"CASE-023/mcp-valid-query-timestamps",
		"CASE-023/mcp-valid-query-values",
		"CASE-023/mcp-valid-query-anomaly-rates",
		"CASE-023/mcp-valid-query-annotations",
		"CASE-023/mcp-default-zero-options",
		"CASE-023/mcp-invalid-options",
	} {
		registerContract(t, contract)
	}

	const (
		host            = "c023-mcp"
		cadenceHost     = "c023-mcp-cadence"
		standardContext = "fixture.c023_mcp"
		cadenceContext  = "fixture.c023_mcp_cadence"
	)
	standard := c023FleetFixture(standardContext)
	for i := range standard.Dimensions {
		points := standard.Dimensions[i].Points
		points[len(points)-1].Flags = stream.FlagReset
	}
	pushLiveBurst(t, host, guid(334), standard)
	if _, err := td.WaitRetention(
		host, standardContext, standard.FirstT(), standard.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	cadenceAfter, cadenceBefore := c023MCPPushCadenceFixture(t, cadenceHost, cadenceContext)

	groups := []struct {
		name, units string
	}{
		{name: "percentage-of-samples", units: "%"},
		{name: "countif", units: "%"},
		{name: "percentage-of-time", units: "%"},
		{name: "number-of-flaps", units: "flaps"},
		{name: "number-of-times", units: "events"},
	}

	var initialize *c023MCPResponse
	session := func(t *testing.T) c023MCPResponse {
		t.Helper()
		if initialize == nil {
			response := c023MCPPost(t, 1, "initialize", map[string]any{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name":    "query-corpus",
					"version": "1",
				},
			}, "")
			c023MCPNotify(t, "notifications/initialized", map[string]any{}, response.Session)
			initialize = &response
		}
		return *initialize
	}

	t.Run("protocol-lifecycle", func(t *testing.T) {
		trackContract(t, "CASE-023/mcp-protocol-lifecycle")

		initialize := session(t)

		initializeResult := c023MCPResult(t, initialize.Document, "initialize")
		if got := initializeResult["protocolVersion"]; got != "2025-03-26" {
			t.Errorf("initialize protocolVersion = %v, want 2025-03-26", got)
		}
		serverInfo := queryObject(t, initializeResult, "serverInfo", "initialize.result.serverInfo")
		for _, key := range []string{"name", "version"} {
			if value, ok := serverInfo[key].(string); !ok || value == "" {
				t.Errorf("initialize serverInfo.%s is empty or malformed: %v", key, serverInfo[key])
			}
		}
		capabilities := queryObject(t, initializeResult, "capabilities", "initialize.result.capabilities")
		_ = queryObject(t, capabilities, "tools", "initialize.result.capabilities.tools")
	})

	t.Run("query-tool-schema", func(t *testing.T) {
		trackContract(t, "CASE-023/mcp-query-tool-schema")

		toolsList := c023MCPPost(t, 2, "tools/list", map[string]any{}, session(t).Session)
		toolsResult := c023MCPResult(t, toolsList.Document, "tools/list")
		tools, ok := toolsResult["tools"].([]any)
		if !ok || len(tools) == 0 {
			t.Fatalf("tools/list returned no tools: %v", toolsResult["tools"])
		}
		var queryMetrics map[string]any
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if ok && tool["name"] == "query_metrics" {
				queryMetrics = tool
				break
			}
		}
		if queryMetrics == nil {
			t.Fatal("tools/list does not expose query_metrics")
		}
		inputSchema := queryObject(t, queryMetrics, "inputSchema", "query_metrics.inputSchema")
		properties := queryObject(t, inputSchema, "properties", "query_metrics.inputSchema.properties")
		timeGroup := queryObject(t, properties, "time_group", "query_metrics.inputSchema.properties.time_group")
		enum, ok := timeGroup["enum"].([]any)
		if !ok || len(enum) == 0 {
			t.Fatalf("query_metrics time_group enum is missing or empty: %v", timeGroup["enum"])
		}
		enumSet := make(map[string]bool, len(enum))
		for _, raw := range enum {
			value, ok := raw.(string)
			if !ok || value == "" {
				t.Fatalf("query_metrics time_group enum contains an empty or non-string value: %v", raw)
			}
			enumSet[value] = true
		}
		for _, group := range groups {
			if !enumSet[group.name] {
				t.Errorf("query_metrics time_group enum does not contain %q", group.name)
			}
		}
		timeGroupOptions := queryObject(
			t, properties, "time_group_options",
			"query_metrics.inputSchema.properties.time_group_options",
		)
		if got := timeGroupOptions["type"]; got != "string" {
			t.Errorf("query_metrics time_group_options type = %v, want string", got)
		}
		description, ok := timeGroupOptions["description"].(string)
		if !ok || description == "" {
			t.Fatalf("query_metrics time_group_options description is empty or malformed: %v", timeGroupOptions["description"])
		}
		for _, group := range groups {
			if !strings.Contains(description, group.name) {
				t.Errorf("query_metrics time_group_options description does not name %q", group.name)
			}
		}
		for _, phrase := range []string{"omitted", "blank", "zero"} {
			if !strings.Contains(description, phrase) {
				t.Errorf("query_metrics time_group_options description does not document %q", phrase)
			}
		}
	})

	baseArguments := func(metric, dimension string, after, before int64) map[string]any {
		return map[string]any{
			"metric":            metric,
			"dimensions":        []string{dimension},
			"after":             after,
			"before":            before,
			"points":            1,
			"tier":              0,
			"group_by":          []string{"dimension"},
			"aggregation":       "average",
			"cardinality_limit": 10,
			"options":           "unaligned debug",
		}
	}

	validCalls := []c023MCPValidCall{
		{
			label: "numeric", group: "percentage-of-samples", expression: "==1",
			metric: cadenceContext, dimension: "value", after: cadenceAfter, before: cadenceBefore,
			units: "%", value: 100.0 * 5.0 / 15.0,
		},
		{
			label: "numeric", group: "countif", expression: "==1",
			metric: cadenceContext, dimension: "value", after: cadenceAfter, before: cadenceBefore,
			units: "%", value: 100.0 * 5.0 / 15.0,
		},
		{
			label: "numeric", group: "percentage-of-time", expression: "==1",
			metric: cadenceContext, dimension: "value", after: cadenceAfter, before: cadenceBefore,
			units: "%", value: 100.0 * 5.0 / 105.0,
		},
		{
			label: "numeric", group: "number-of-flaps", expression: "==1",
			metric: standardContext, dimension: "bool", after: fixture.T0, before: standard.LastT(),
			units: "flaps", value: 2, anomalyRate: 100.0 / 12.0, annotations: fixture.PAReset,
		},
		{
			label: "numeric", group: "number-of-times", expression: ">20",
			metric: standardContext, dimension: "counter", after: fixture.T0, before: standard.LastT(),
			units: "events", value: 6, anomalyRate: 100.0 / 12.0, annotations: fixture.PAReset,
		},
		{
			label: "gap", group: "percentage-of-samples", expression: "==gap",
			metric: standardContext, dimension: "sparse", after: fixture.T0, before: standard.LastT(),
			units: "%", value: 100.0 * 4.0 / 12.0, anomalyRate: 12.5, annotations: fixture.PAReset,
		},
		{
			label: "gap", group: "countif", expression: "!=gap",
			metric: standardContext, dimension: "sparse", after: fixture.T0, before: standard.LastT(),
			units: "%", value: 100.0 * 8.0 / 12.0, anomalyRate: 12.5, annotations: fixture.PAReset,
		},
		{
			label: "gap", group: "percentage-of-time", expression: "==gap",
			metric: standardContext, dimension: "sparse", after: fixture.T0, before: standard.LastT(),
			units: "%", value: 100.0 * 4.0 / 12.0, anomalyRate: 12.5, annotations: fixture.PAReset,
		},
		{
			label: "gap", group: "number-of-flaps", expression: "==gap",
			metric: standardContext, dimension: "sparse", after: fixture.T0, before: standard.LastT(),
			units: "flaps", value: 1, anomalyRate: 12.5, annotations: fixture.PAReset,
		},
		{
			label: "gap", group: "number-of-times", expression: "==gap",
			metric: standardContext, dimension: "sparse", after: fixture.T0, before: standard.LastT(),
			units: "events", value: 4, anomalyRate: 12.5, annotations: fixture.PAReset,
		},
	}

	runValidCalls := func(t *testing.T, requestID int, check func(*testing.T, c023MCPValidCall, map[string]any)) {
		t.Helper()
		for _, call := range validCalls {
			call := call
			t.Run(call.group+"/"+call.label, func(t *testing.T) {
				arguments := baseArguments(call.metric, call.dimension, call.after, call.before)
				arguments["time_group"] = call.group
				arguments["time_group_options"] = call.expression
				response := c023MCPCall(t, requestID, session(t).Session, arguments)
				requestID++
				doc := c023MCPQueryDocument(t, response.Document)
				check(t, call, doc)
			})
		}
	}

	validContracts := []struct {
		name, contract string
		requestID      int
		check          func(*testing.T, c023MCPValidCall, map[string]any)
	}{
		{
			name: "valid-result-schema", contract: "CASE-023/mcp-valid-result-schema", requestID: 10,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				c023MCPAssertResultSchema(t, doc, call.dimension)
			},
		},
		{
			name: "valid-query-units", contract: "CASE-023/mcp-valid-query-units", requestID: 30,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				view := queryObject(t, doc, "view", "query view")
				if got := queryStrictOneUnit(t, view["units"], "query view.units"); got != call.units {
					t.Errorf("%s view.units = %q, want %q", call.group, got, call.units)
				}
				if got := queryStrictDimensionUnit(t, view, "query view"); got != call.units {
					t.Errorf("%s view.dimensions.units[0] = %q, want %q", call.group, got, call.units)
				}
			},
		},
		{
			name: "valid-query-echo", contract: "CASE-023/mcp-valid-query-echo", requestID: 50,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				c023MCPAssertEcho(t, doc, call.group, call.expression)
			},
		},
		{
			name: "valid-query-timestamps", contract: "CASE-023/mcp-valid-query-timestamps", requestID: 70,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				c023MCPAssertTimestamp(t, doc, call.before)
			},
		},
		{
			name: "valid-query-values", contract: "CASE-023/mcp-valid-query-values", requestID: 90,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				c023MCPAssertPointField(t, doc, 0, "value", call.value, printTol)
			},
		},
		{
			name: "valid-query-anomaly-rates", contract: "CASE-023/mcp-valid-query-anomaly-rates", requestID: 110,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				c023MCPAssertPointField(t, doc, 1, "anomaly_rate_percent", call.anomalyRate, printTol)
			},
		},
		{
			name: "valid-query-annotations", contract: "CASE-023/mcp-valid-query-annotations", requestID: 130,
			check: func(t *testing.T, call c023MCPValidCall, doc map[string]any) {
				c023MCPAssertPointField(t, doc, 2, "point_annotations_bitmap", float64(call.annotations), 0)
			},
		},
	}
	for _, contract := range validContracts {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			trackContract(t, contract.contract)
			runValidCalls(t, contract.requestID, contract.check)
		})
	}

	t.Run("default-zero-options", func(t *testing.T) {
		trackContract(t, "CASE-023/mcp-default-zero-options")
		requestID := 200

		for _, group := range groups {
			group := group
			dimension := "counter"
			equalZero, greaterZero := 0.0, 100.0
			switch group.name {
			case "number-of-times":
				greaterZero = 12
			case "number-of-flaps":
				dimension = "bool"
				equalZero, greaterZero = 3, 2
			}

			for _, defaultCase := range []struct {
				name        string
				value       string
				set         bool
				greaterZero bool
			}{
				{name: "missing"},
				{name: "empty", value: "", set: true},
				{name: "whitespace", value: "   ", set: true},
				{name: "operator-only", value: ">", set: true, greaterZero: true},
				{name: "operator-whitespace", value: ">   ", set: true, greaterZero: true},
			} {
				defaultCase := defaultCase
				t.Run(group.name+"/"+defaultCase.name, func(t *testing.T) {
					arguments := baseArguments(
						standardContext, dimension, fixture.T0, standard.LastT())
					arguments["time_group"] = group.name
					if defaultCase.set {
						arguments["time_group_options"] = defaultCase.value
					}
					response := c023MCPCall(t, requestID, session(t).Session, arguments)
					requestID++
					doc := c023MCPQueryDocument(t, response.Document)
					want := equalZero
					if defaultCase.greaterZero {
						want = greaterZero
					}
					c023MCPAssertPointField(t, doc, 0, "value", want, printTol)
				})
			}
		}
	})

	t.Run("invalid-options", func(t *testing.T) {
		trackContract(t, "CASE-023/mcp-invalid-options")
		requestID := 300
		for _, group := range groups {
			invalidOptions := []struct {
				name  string
				value any
			}{
				{name: "non-string", value: 0},
				{name: "null", value: nil},
				{name: "malformed", value: "abc"},
				{name: "positive-infinity", value: "+Inf"},
				{name: "negative-infinity", value: "-Inf"},
				{name: "overflow-to-infinity", value: "1e309"},
			}
			for _, invalid := range invalidOptions {
				t.Run(group.name+"/"+invalid.name+"-options", func(t *testing.T) {
					arguments := baseArguments(
						standardContext, "bool", fixture.T0, standard.LastT())
					arguments["time_group"] = group.name
					arguments["time_group_options"] = invalid.value
					response := c023MCPCall(t, requestID, session(t).Session, arguments)
					requestID++
					c023MCPAssertOptionsError(t, response.Document, group.name)
				})
			}
		}
	})
}
