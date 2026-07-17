// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"math"
	"testing"
)

func TestClosedFunctionResultVariantsHaveExactSizeAppend(t *testing.T) {
	message, err := StringValue("ok")
	if err != nil {
		t.Fatal(err)
	}
	body, err := ObjectValue(
		ObjectField{Key: "z", Value: Uint64Value(2)},
		ObjectField{Key: "a", Value: message},
	)
	if err != nil {
		t.Fatal(err)
	}
	errorResult, err := NewErrorResult(503, "Service unavailable.")
	if err != nil {
		t.Fatal(err)
	}
	tableResult, err := NewTableResult(200, body)
	if err != nil {
		t.Fatal(err)
	}
	topologyResult, err := NewTopologyResult(200, body)
	if err != nil {
		t.Fatal(err)
	}
	rawResult, err := NewCompleteRawEnvelope(200, ReviewedPerformanceJSON, []byte(`{"status":200}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]FunctionResult{
		"error": errorResult, "table": tableResult, "topology": topologyResult, "raw": rawResult,
	} {
		seed := []byte("seed:")
		appended := result.Append(bytes.Clone(seed))
		if len(appended)-len(seed) != result.Size() {
			t.Fatalf("%s Size/Append differs: size=%d appended=%d", name, result.Size(), len(appended)-len(seed))
		}
	}
	if got := string(tableResult.Append(nil)); got != `{"a":"ok","z":2}` {
		t.Fatalf("deterministic object order differs: %s", got)
	}
	if got := string(errorResult.Append(nil)); got != `{"errorMessage":"Service unavailable.","status":503}` {
		t.Fatalf("typed error bytes differ: %s", got)
	}
}

func TestClosedFunctionValuesRejectInvalidGraphs(t *testing.T) {
	if _, err := FiniteFloat64Value(math.NaN()); err == nil {
		t.Fatal("NaN Function value was accepted")
	}
	if _, err := FiniteFloat64Value(math.Inf(1)); err == nil {
		t.Fatal("infinite Function value was accepted")
	}
	if _, err := StringValue(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 Function value was accepted")
	}
	if _, err := ObjectValue(
		ObjectField{Key: "same", Value: NullValue()},
		ObjectField{Key: "same", Value: BoolValue(true)},
	); err == nil {
		t.Fatal("duplicate Function object key was accepted")
	}

	value := NullValue()
	for depth := 1; depth <= MaximumFunctionValueDepth; depth++ {
		var err error
		value, err = ArrayValue(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewTableResult(200, value); err != nil {
		t.Fatalf("depth %d was rejected: %v", MaximumFunctionValueDepth, err)
	}
	value, err := ArrayValue(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTableResult(200, value); err == nil {
		t.Fatalf("depth %d was accepted", MaximumFunctionValueDepth+1)
	}
}

func TestRepeatedStringValuePreflightsExactDeferredBoundary(t *testing.T) {
	for _, deferredBytes := range []int{
		FunctionPayloadBytes - 1,
		FunctionPayloadBytes,
		FunctionPayloadBytes + 1,
	} {
		pad, err := RepeatedStringValue(deferredBytes-1-len(`{"pad":""}`), 'A')
		if err != nil {
			t.Fatal(err)
		}
		body, err := ObjectValue(ObjectField{Key: "pad", Value: pad})
		if err != nil {
			t.Fatal(err)
		}
		size, err := valueJSONSize(body, 0)
		if err != nil {
			t.Fatal(err)
		}
		if size+1 != deferredBytes {
			t.Fatalf("deferred size %d computed as %d", deferredBytes, size+1)
		}
		err = validateResultPlanSize(200, "application/json", size)
		if (err == nil) != (deferredBytes <= FunctionPayloadBytes) {
			t.Fatalf("deferred boundary %d validation differs: %v", deferredBytes, err)
		}
	}
}

func TestReviewedRawEnvelopeCopiesAndValidatesBody(t *testing.T) {
	body := []byte(`{"status":200}`)
	result, err := NewCompleteRawEnvelope(200, ReviewedPerformanceJSON, body)
	if err != nil {
		t.Fatal(err)
	}
	body[2] = 'X'
	if got := string(result.Append(nil)); got != `{"status":200}` {
		t.Fatalf("accepted raw body retained caller alias: %s", got)
	}
	for _, invalid := range [][]byte{
		[]byte(`{"status": 200}`),
		[]byte(`{"status":`),
	} {
		if _, err := NewCompleteRawEnvelope(200, ReviewedPerformanceJSON, invalid); err == nil {
			t.Fatalf("invalid reviewed JSON was accepted: %q", invalid)
		}
	}
	if _, err := NewCompleteRawEnvelope(200, ReviewedDynCfgYAML, []byte("key: value\n")); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{
		[]byte("key: value\nFUNCTION_RESULT_END\n"),
		[]byte("key: &anchor value\nother: *anchor\n"),
		[]byte("key: !!str value\n"),
		[]byte("key: first\nkey: second\n"),
		[]byte("---\nkey: value\n---\nother: value\n"),
	} {
		if _, err := NewCompleteRawEnvelope(200, ReviewedDynCfgYAML, invalid); err == nil {
			t.Fatalf("unsafe reviewed YAML was accepted: %q", invalid)
		}
	}
	if _, err := NewCompleteRawEnvelope(200, 0, []byte(`{}`)); err == nil {
		t.Fatal("unknown raw body schema was accepted")
	}
}
