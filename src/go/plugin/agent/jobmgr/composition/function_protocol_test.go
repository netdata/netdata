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
