// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoundedFunctionProtocolCapture(t *testing.T) {
	capture := &boundedFunctionProtocolCapture{limit: 8}

	written, err := capture.write([]byte("123456"))
	require.NoError(t, err)
	assert.Equal(t, 6, written)
	assert.LessOrEqual(t, cap(capture.payload), capture.limit)

	written, err = capture.write([]byte("7890"))
	require.ErrorIs(t, err, errFunctionProtocolCaptureTooLarge)
	assert.Equal(t, 2, written)
	assert.Equal(t, "12345678", string(capture.payload))
	assert.LessOrEqual(t, cap(capture.payload), capture.limit)

	written, err = capture.write([]byte("ignored"))
	require.ErrorIs(t, err, errFunctionProtocolCaptureTooLarge)
	assert.Zero(t, written)
	assert.Equal(t, "12345678", string(capture.payload))
}

func TestFunctionProtocolCaptureReportsIgnoredWriteOverflow(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	capture, err := newFunctionProtocolCapture(frames)
	require.NoError(t, err)
	capture.captureLimit = 64

	_, _, err = capture.invoke("overflow", func() {
		_, _ = capture.Write(bytes.Repeat([]byte{'x'}, 65))
	})
	require.ErrorIs(t, err, errFunctionProtocolCaptureTooLarge)
	assert.Nil(t, capture.active)
}

func TestSplitFunctionProtocol(t *testing.T) {
	const (
		resultWithPayload = "FUNCTION_RESULT_BEGIN uid 200 application/json 0\n{\"ok\":true}\nFUNCTION_RESULT_END\n\n"
		emptyResult       = "FUNCTION_RESULT_BEGIN uid 200 application/json 0\nFUNCTION_RESULT_END\n\n"
	)
	tests := map[string]struct {
		output            string
		wantPayload       string
		wantNotifications string
		wantError         string
	}{
		"non-empty payload": {
			output:      resultWithPayload,
			wantPayload: `{"ok":true}`,
		},
		"empty payload": {
			output: emptyResult,
		},
		"marker text before result": {
			output: "CONFIG example create running job path dyncfg 'source FUNCTION_RESULT_BEGIN uid 500 text/plain 0' commands\n\n" +
				resultWithPayload,
			wantPayload:       `{"ok":true}`,
			wantNotifications: "CONFIG example create running job path dyncfg 'source FUNCTION_RESULT_BEGIN uid 500 text/plain 0' commands\n\n",
		},
		"marker text after result": {
			output: resultWithPayload +
				"CONFIG example create running job path dyncfg 'source FUNCTION_RESULT_BEGIN another 500 text/plain 0' commands\n\n",
			wantPayload:       `{"ok":true}`,
			wantNotifications: "CONFIG example create running job path dyncfg 'source FUNCTION_RESULT_BEGIN another 500 text/plain 0' commands\n\n",
		},
		"two result lines": {
			output:    resultWithPayload + resultWithPayload,
			wantError: "Function handler produced multiple results",
		},
		"only embedded marker text": {
			output:    "CONFIG example create running job path dyncfg 'source FUNCTION_RESULT_BEGIN uid 500 text/plain 0' commands\n\n",
			wantError: "Function handler produced no terminal result",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var notifications bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&notifications)
			require.NoError(t, err)

			result, cleanup, err := splitFunctionProtocol("uid", []byte(test.output), frames)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				assert.Nil(t, cleanup)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cleanup)
			require.NoError(t, cleanup())
			assert.Equal(t, test.wantNotifications, notifications.String())

			var encoded bytes.Buffer
			resultFrames, err := lifecycle.NewFrameOwner(&encoded)
			require.NoError(t, err)
			frame, err := lifecycle.PrepareFrame("result", result, 1)
			require.NoError(t, err)
			require.NoError(t, resultFrames.Commit(frame))
			want := "FUNCTION_RESULT_BEGIN result 200 application/json 1\n"
			if test.wantPayload != "" {
				want += test.wantPayload + "\n"
			}
			want += "FUNCTION_RESULT_END\n\n"
			assert.Equal(t, want, encoded.String())
		})
	}
}
