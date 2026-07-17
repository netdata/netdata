// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeFunctionRequest(t *testing.T) {
	line, err := encodeFunctionRequest("uid-1", "mysql:top-queries", []string{"info", "__job:local"}, 1500*time.Millisecond)
	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(line))
	reader.Comma = ' '
	fields, err := reader.Read()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"FUNCTION",
		"uid-1",
		"2",
		"mysql:top-queries info __job:local",
		functionPermissions,
		functionSource,
	}, fields)
}

func TestEncodeFunctionRequestRejectsInvalidTokens(t *testing.T) {
	tests := map[string]struct {
		function string
		args     []string
	}{
		"function whitespace": {
			function: "mysql:top queries",
		},
		"argument whitespace": {
			function: "mysql:top-queries",
			args:     []string{"two words"},
		},
		"argument line injection": {
			function: "mysql:top-queries",
			args:     []string{"info\nQUIT"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := encodeFunctionRequest(
				"uid-1",
				tc.function,
				tc.args,
				time.Second,
			)
			assert.Error(t, err)
		})
	}
}

func TestReadAgentProtocol(t *testing.T) {
	input := strings.NewReader(`CHART 'ignored'
FUNCTION GLOBAL "mysql:top-queries" 60 "help" "top" 0xFFFF 1 1

FUNCTION_RESULT_BEGIN other 200 application/json 1
{"ignored":true}
FUNCTION_RESULT_END

FUNCTION_RESULT_BEGIN functions-validation 200 application/json 2
{"status":200,
"data":[]}
FUNCTION_RESULT_END

`)
	events := protocolEvents{
		published: make(chan struct{}, 1),
		result:    make(chan functionResult, 1),
		readErr:   make(chan error, 1),
	}

	readAgentProtocol(input, "mysql:top-queries", functionUID, events)

	select {
	case <-events.published:
	default:
		t.Fatal("expected Function publication")
	}
	select {
	case result := <-events.result:
		assert.Equal(t, 200, result.status)
		assert.Equal(t, "application/json", result.contentType)
		assert.Equal(t, int64(2), result.expiry)
		assert.Equal(t, "{\"status\":200,\n\"data\":[]}", string(result.payload))
	default:
		t.Fatal("expected Function result")
	}
	assert.ErrorIs(t, <-events.readErr, io.EOF)
}

func TestReadAgentProtocolPayloadLimit(t *testing.T) {
	t.Run("exact multiline payload", func(t *testing.T) {
		events := readProtocolWithLimit(
			"FUNCTION_RESULT_BEGIN functions-validation 200 application/json 1\n"+
				"abc\n"+
				"d\n"+
				"FUNCTION_RESULT_END\n",
			5,
		)

		result := <-events.result
		assert.Equal(t, "abc\nd", string(result.payload))
		assert.ErrorIs(t, <-events.readErr, io.EOF)
	})

	for name, input := range map[string]string{
		"single line over limit":    "abcdef\n",
		"multiple lines over limit": "abc\nde\n",
	} {
		t.Run(name, func(t *testing.T) {
			events := readProtocolWithLimit(
				"FUNCTION_RESULT_BEGIN functions-validation 200 application/json 1\n"+
					input+
					"FUNCTION_RESULT_END\n",
				5,
			)

			select {
			case <-events.result:
				t.Fatal("oversized payload produced a result")
			default:
			}
			assert.ErrorIs(t, <-events.readErr, errFunctionResultTooLarge)
		})
	}

	t.Run("truncated frame", func(t *testing.T) {
		events := readProtocolWithLimit(
			"FUNCTION_RESULT_BEGIN functions-validation 200 application/json 1\n"+
				"abc\n",
			5,
		)
		assert.True(t, errors.Is(<-events.readErr, io.ErrUnexpectedEOF))
	})
}

func readProtocolWithLimit(input string, limit int) protocolEvents {
	events := protocolEvents{
		published: make(chan struct{}, 1),
		result:    make(chan functionResult, 1),
		readErr:   make(chan error, 1),
	}
	readAgentProtocolWithLimit(
		strings.NewReader(input),
		"mysql:top-queries",
		functionUID,
		limit,
		events,
	)
	return events
}

func TestParseFunctionResultHeader(t *testing.T) {
	header, ok, err := parseFunctionResultHeader(
		"FUNCTION_RESULT_BEGIN request-1 503 application/json 123",
	)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "request-1", header.uid)
	assert.Equal(t, 503, header.result.status)
	assert.Equal(t, "application/json", header.result.contentType)
	assert.Equal(t, int64(123), header.result.expiry)

	_, ok, err = parseFunctionResultHeader("CONFIG x create")
	require.NoError(t, err)
	assert.False(t, ok)

	_, _, err = parseFunctionResultHeader("FUNCTION_RESULT_BEGIN malformed")
	assert.Error(t, err)

	_, _, err = parseFunctionResultHeader(
		"FUNCTION_RESULT_BEGIN request-1 999 application/json 123",
	)
	assert.Error(t, err)
}

func TestPublishedFunction(t *testing.T) {
	assert.Equal(t, "mysql:top-queries",
		publishedFunction(`FUNCTION GLOBAL "mysql:top-queries" 60 "help" "top" 0xFFFF 1 1`))
	assert.Empty(t, publishedFunction(`FUNCTION_DEL GLOBAL "mysql:top-queries"`))
	assert.Empty(t, publishedFunction("FUNCTION malformed"))
}
