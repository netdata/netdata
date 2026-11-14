// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/require"
)

// frame builds baseline fixture output independently of the live wire decoder.
type frame struct {
	Type   string           `json:"type"`
	Event  *synthetic.Event `json:"event,omitempty"`
	Result *reporterResult  `json:"result,omitempty"`
}

func emit(writer io.Writer, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	_, _ = fmt.Fprintln(writer, string(raw))
}

func success() reporterResult {
	return reporterResult{
		Status: synthetic.Success,
		Tests: &synthetic.TestCounts{
			Declared: 1,
			Passed:   1,
		},
		CaptureState: "disabled",
	}
}

// extensionLine encodes one optional-output frame; json.RawMessage data is
// passed through as raw JSON.
func extensionLine(feature string, data any) string {
	raw, err := json.Marshal(map[string]any{"type": "extension", "extension": map[string]any{"feature": feature, "data": data}})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func resultLine(result reporterResult) string {
	raw, err := json.Marshal(frame{Type: "result", Result: &result})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// consume feeds frames to a fresh sink for request and returns its snapshot.
// The stream itself must stay valid.
func consume(t *testing.T, request synthetic.Request, secrets map[string]string, lines ...string) sinkSnapshot {
	t.Helper()
	s := newSink(request, newRedactor(secrets))
	require.NoError(t, s.consume(strings.NewReader(strings.Join(lines, "\n"))))
	snapshot := s.snapshot()
	require.NoError(t, snapshot.err)
	return snapshot
}

// consumeWithResult ends the frames with result and checks that optional
// output did not change the core outcome.
func consumeWithResult(t *testing.T, request synthetic.Request, secrets map[string]string, result reporterResult, lines ...string) sinkSnapshot {
	t.Helper()
	snapshot := consume(t, request, secrets, append(lines, resultLine(result))...)
	require.NotNil(t, snapshot.result)
	require.Equal(t, result, *snapshot.result, "optional output must not rewrite the core result")
	return snapshot
}
