// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const payloadStartLine = `FUNCTION_PAYLOAD tx1 1 "fn1 arg1" 0xFFFF "method=api,role=test" application/json`

func TestInputParser_ParseEvent_Cancel(t *testing.T) {
	p := newInputParser()

	ev, err := p.parseEvent("FUNCTION_CANCEL tx1")
	require.NoError(t, err)
	assert.Equal(t, inputEventCancel, ev.kind)
	assert.Equal(t, "tx1", ev.uid)
	assert.False(t, ev.preAdmission)
}

func TestInputParser_ParseEvent_MalformedCancel(t *testing.T) {
	p := newInputParser()

	_, err := p.parseEvent("FUNCTION_CANCEL tx1 extra")
	require.Error(t, err)
}

func TestInputParser_ParseEvent_Progress(t *testing.T) {
	p := newInputParser()

	ev, err := p.parseEvent("FUNCTION_PROGRESS tx1 10 100")
	require.NoError(t, err)
	assert.Equal(t, inputEventProgress, ev.kind)
	assert.Equal(t, "tx1", ev.uid)
}

func TestInputParser_ParseEvent_PayloadCancelDifferentUIDContinues(t *testing.T) {
	p := newInputParser()

	_, err := p.parseEvent(payloadStartLine)
	require.NoError(t, err)
	_, err = p.parseEvent("line1")
	require.NoError(t, err)

	ev, err := p.parseEvent("FUNCTION_CANCEL tx-other")
	require.NoError(t, err)
	assert.Equal(t, inputEventCancel, ev.kind)
	assert.Equal(t, "tx-other", ev.uid)
	assert.False(t, ev.preAdmission)
	assert.True(t, p.readingPayload)
	require.NotNil(t, p.currentFn)
	assert.Equal(t, "tx1", p.currentFn.UID)

	_, err = p.parseEvent("line2")
	require.NoError(t, err)
	ev, err = p.parseEvent("FUNCTION_PAYLOAD_END")
	require.NoError(t, err)
	assert.Equal(t, inputEventCall, ev.kind)
	require.NotNil(t, ev.fn)
	assert.Equal(t, []byte("line1\nline2"), ev.fn.Payload)
}

func TestInputParser_ParseEvent_PayloadCancelSameUIDPreAdmission(t *testing.T) {
	p := newInputParser()

	_, err := p.parseEvent(payloadStartLine)
	require.NoError(t, err)
	_, err = p.parseEvent("line1")
	require.NoError(t, err)

	ev, err := p.parseEvent("FUNCTION_CANCEL tx1")
	require.NoError(t, err)
	assert.Equal(t, inputEventCancel, ev.kind)
	assert.Equal(t, "tx1", ev.uid)
	assert.True(t, ev.preAdmission)
	assert.False(t, p.readingPayload)
	assert.Nil(t, p.currentFn)
	assert.Equal(t, 0, p.payloadBuf.Len())
}

func TestInputParser_ParseEvent_PayloadMalformedCancelDoesNotMutateState(t *testing.T) {
	p := newInputParser()

	_, err := p.parseEvent(payloadStartLine)
	require.NoError(t, err)
	_, err = p.parseEvent("line1")
	require.NoError(t, err)

	_, err = p.parseEvent("FUNCTION_CANCEL tx1 extra")
	require.Error(t, err)
	assert.True(t, p.readingPayload)
	require.NotNil(t, p.currentFn)
	assert.Equal(t, "tx1", p.currentFn.UID)

	_, err = p.parseEvent("line2")
	require.NoError(t, err)
	ev, err := p.parseEvent("FUNCTION_PAYLOAD_END")
	require.NoError(t, err)
	assert.Equal(t, inputEventCall, ev.kind)
	require.NotNil(t, ev.fn)
	assert.Equal(t, []byte("line1\nline2"), ev.fn.Payload)
}

func TestInputParser_Parse_Wrapper(t *testing.T) {
	p := newInputParser()

	fn, err := p.parse(`FUNCTION tx1 1 "fn1 arg1" 0xFFFF "method=api,role=test"`)
	require.NoError(t, err)
	require.NotNil(t, fn)
	assert.Equal(t, "tx1", fn.UID)

	fn, err = p.parse("FUNCTION_CANCEL tx1")
	require.NoError(t, err)
	assert.Nil(t, fn)

	fn, err = p.parse("FUNCTION_PROGRESS tx1 10 100")
	require.NoError(t, err)
	assert.Nil(t, fn)
}
