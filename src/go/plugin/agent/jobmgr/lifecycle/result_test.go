// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClosedFunctionResultVariantsHaveExactSizeAppend(t *testing.T) {
	message, err := StringValue("ok")
	require.NoError(t, err)
	body, err := ObjectValue(
		ObjectField{Key: "z", Value: Uint64Value(2)},
		ObjectField{Key: "a", Value: message},
	)
	require.NoError(t, err)
	errorResult, err := NewErrorResult(503, "Service unavailable.")
	require.NoError(t, err)
	tableResult, err := NewTableResult(200, body)
	require.NoError(t, err)
	topologyResult, err := NewTopologyResult(200, body)
	require.NoError(t, err)
	for name, result := range map[string]FunctionResult{
		"error": errorResult, "table": tableResult, "topology": topologyResult,
	} {
		seed := []byte("seed:")
		appended := result.Append(bytes.Clone(seed))
		require.EqualValues(t, result.Size(), len(appended)-len(seed), "result=%s", name)
	}

	require.EqualValues(t, `{"a":"ok","z":2}`, string(tableResult.Append(nil)))

	require.EqualValues(t, `{"errorMessage":"Service unavailable.","status":503}`, string(errorResult.Append(nil)))
}

func TestClosedFunctionValuesRejectInvalidGraphs(t *testing.T) {

	_, finiteFloat64ValueErr := FiniteFloat64Value(math.NaN())
	require.Error(t, finiteFloat64ValueErr)

	_, finiteFloat64ValueErr2 := FiniteFloat64Value(math.Inf(1))
	require.Error(t, finiteFloat64ValueErr2)

	_, stringValueErr := StringValue(string([]byte{0xff}))
	require.Error(t, stringValueErr)

	_, objectValueErr := ObjectValue(
		ObjectField{Key: "same", Value: NullValue()},
		ObjectField{Key: "same", Value: BoolValue(true)},
	)
	require.Error(t, objectValueErr)

	value := NullValue()
	for depth := 1; depth <= MaximumFunctionValueDepth; depth++ {
		var err error
		value, err = ArrayValue(value)
		require.NoError(t, err)
	}

	_, newTableResultErr := NewTableResult(200, value)
	require.NoError(t, newTableResultErr)

	value, err := ArrayValue(value)
	require.NoError(t, err)

	_, newTableResultErr2 := NewTableResult(200, value)
	require.Error(t, newTableResultErr2)

}

func TestRepeatedStringValuePreflightsExactDeferredBoundary(t *testing.T) {
	for _, deferredBytes := range []int{
		FunctionPayloadBytes - 1,
		FunctionPayloadBytes,
		FunctionPayloadBytes + 1,
	} {
		pad, err := RepeatedStringValue(deferredBytes-1-len(`{"pad":""}`), 'A')
		require.NoError(t, err)
		body, err := ObjectValue(ObjectField{Key: "pad", Value: pad})
		require.NoError(t, err)
		size, err := valueJSONSize(body, 0)
		require.NoError(t, err)
		require.EqualValues(t, deferredBytes, size+1)
		err = validateResultPlanSize(200, "application/json", size)
		require.EqualValues(t, deferredBytes <= FunctionPayloadBytes, err == nil)
	}
}

func TestResultPlanSizeUsesCanonicalPayloadBoundary(t *testing.T) {
	tests := map[string]int{
		"negative":        -1,
		"empty":           0,
		"largest valid":   FunctionPayloadBytes - 1,
		"at limit":        FunctionPayloadBytes,
		"integer maximum": math.MaxInt,
	}
	for name, payloadBytes := range tests {
		t.Run(name, func(t *testing.T) {
			planErr := validateResultPlanSize(200, "application/json", payloadBytes)
			canonicalErr := validateFunctionPayloadSize(payloadBytes)

			if payloadBytes < 0 {
				require.Error(t, planErr)
				return
			}
			require.Equal(t, canonicalErr == nil, planErr == nil)
			if canonicalErr != nil {
				require.ErrorIs(t, planErr, ErrFunctionResultTooLarge)
			}
		})
	}
}

func TestRepeatedStringValueAppendGrowsDestination(t *testing.T) {
	pad, err := RepeatedStringValue(4*1024, 'A')
	require.NoError(t, err)
	body, err := ObjectValue(ObjectField{Key: "pad", Value: pad})
	require.NoError(t, err)
	result, err := NewTableResult(200, body)
	require.NoError(t, err)
	tests := map[string][]byte{
		"nil destination": nil,
		"small capacity":  make([]byte, 0, 8),
	}
	for name, seed := range tests {
		t.Run(name, func(t *testing.T) {
			appended := result.Append(seed)
			require.False(t, len(appended) != result.Size() || !bytes.HasPrefix(appended, []byte(`{"pad":"`)) ||
				!bytes.HasSuffix(appended, []byte(`"}`)))
		})
	}
}
