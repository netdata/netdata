// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	parserTestPermissions = "0xFFFF"
	parserTestSource      = "method=api,role=test"
	testPayloadStartLine  = `FUNCTION_PAYLOAD tx1 1 "fn1 arg1" ` + parserTestPermissions + ` "` + parserTestSource + `" application/json`
	testFunctionLine      = `FUNCTION tx2 1 "fn2 arg1" ` + parserTestPermissions + ` "` + parserTestSource + `"`
)

func TestInputParser_ParseEvent(t *testing.T) {
	tests := map[string]struct {
		lines       []string
		wantErr     bool
		assertEvent func(t *testing.T, events []inputEvent)
		assertState func(t *testing.T, p *inputParser)
	}{
		"cancel in normal mode": {
			lines: []string{"FUNCTION_CANCEL tx1"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 1)
				assert.Equal(t, inputEventCancel, events[0].kind)
				assert.Equal(t, "tx1", events[0].uid)
				assert.False(t, events[0].preAdmission)
			},
		},
		"progress in normal mode": {
			lines: []string{"FUNCTION_PROGRESS tx1 10 100"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 1)
				assert.Equal(t, inputEventProgress, events[0].kind)
				assert.Equal(t, "tx1", events[0].uid)
			},
		},
		"malformed cancel with extra token": {
			lines:   []string{"FUNCTION_CANCEL tx1 extra"},
			wantErr: true,
		},
		"malformed cancel with missing uid": {
			lines:   []string{"FUNCTION_CANCEL"},
			wantErr: true,
		},
		"payload cancel with different uid keeps payload": {
			lines: []string{testPayloadStartLine, "line1", "FUNCTION_CANCEL tx-other", "line2", "FUNCTION_PAYLOAD_END"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 2)
				assert.Equal(t, inputEventCancel, events[0].kind)
				assert.Equal(t, "tx-other", events[0].uid)
				assert.False(t, events[0].preAdmission)

				assert.Equal(t, inputEventCall, events[1].kind)
				require.NotNil(t, events[1].fn)
				assert.Equal(t, "tx1", events[1].fn.UID)
				assert.Equal(t, []byte("line1\nline2"), events[1].fn.Payload)
			},
			assertState: func(t *testing.T, p *inputParser) {
				assert.False(t, p.readingPayload)
				assert.Nil(t, p.currentFn)
				assert.Equal(t, 0, p.payloadBuf.Len())
			},
		},
		"payload cancel with same uid is pre-admission": {
			lines: []string{testPayloadStartLine, "line1", "FUNCTION_CANCEL tx1"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 1)
				assert.Equal(t, inputEventCancel, events[0].kind)
				assert.Equal(t, "tx1", events[0].uid)
				assert.True(t, events[0].preAdmission)
			},
			assertState: func(t *testing.T, p *inputParser) {
				assert.False(t, p.readingPayload)
				assert.Nil(t, p.currentFn)
				assert.Equal(t, 0, p.payloadBuf.Len())
			},
		},
		"malformed cancel during payload keeps parser state": {
			lines:   []string{testPayloadStartLine, "line1", "FUNCTION_CANCEL tx1 extra"},
			wantErr: true,
			assertState: func(t *testing.T, p *inputParser) {
				assert.True(t, p.readingPayload)
				require.NotNil(t, p.currentFn)
				assert.Equal(t, "tx1", p.currentFn.UID)
				assert.Equal(t, "line1", p.payloadBuf.String())
			},
		},
		"progress during payload keeps payload": {
			lines: []string{testPayloadStartLine, "line1", "FUNCTION_PROGRESS tx1 10 100", "line2", "FUNCTION_PAYLOAD_END"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 2)
				assert.Equal(t, inputEventProgress, events[0].kind)
				assert.Equal(t, "tx1", events[0].uid)

				assert.Equal(t, inputEventCall, events[1].kind)
				require.NotNil(t, events[1].fn)
				assert.Equal(t, []byte("line1\nline2"), events[1].fn.Payload)
			},
		},
		"payload data line with FUNCTION prefix text is preserved": {
			lines: []string{testPayloadStartLine, "line1", "FUNCTIONALITY=true", "line2", "FUNCTION_PAYLOAD_END"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 1)
				assert.Equal(t, inputEventCall, events[0].kind)
				require.NotNil(t, events[0].fn)
				assert.Equal(t, []byte("line1\nFUNCTIONALITY=true\nline2"), events[0].fn.Payload)
			},
		},
		"unexpected control line during payload aborts partial payload": {
			lines: []string{testPayloadStartLine, "line1", testFunctionLine},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 1)
				assert.Equal(t, inputEventCall, events[0].kind)
				require.NotNil(t, events[0].fn)
				assert.Equal(t, "tx2", events[0].fn.UID)
				assert.Nil(t, events[0].fn.Payload)
			},
			assertState: func(t *testing.T, p *inputParser) {
				assert.False(t, p.readingPayload)
				assert.Nil(t, p.currentFn)
			},
		},
		"quit during payload aborts payload and emits quit": {
			lines: []string{testPayloadStartLine, "line1", "QUIT"},
			assertEvent: func(t *testing.T, events []inputEvent) {
				require.Len(t, events, 1)
				assert.Equal(t, inputEventQuit, events[0].kind)
			},
			assertState: func(t *testing.T, p *inputParser) {
				assert.False(t, p.readingPayload)
				assert.Nil(t, p.currentFn)
				assert.Equal(t, 0, p.payloadBuf.Len())
			},
		},
		"unknown FUNCTION_ control during payload aborts payload and errors": {
			lines:   []string{testPayloadStartLine, "line1", "FUNCTION_UNKNOWN tx1"},
			wantErr: true,
			assertState: func(t *testing.T, p *inputParser) {
				assert.False(t, p.readingPayload)
				assert.Nil(t, p.currentFn)
				assert.Equal(t, 0, p.payloadBuf.Len())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := newInputParser()
			events := make([]inputEvent, 0, len(tc.lines))
			var parseErr error

			for _, line := range tc.lines {
				ev, err := p.parseEvent(line)
				if err != nil {
					parseErr = err
					break
				}
				if ev.kind != inputEventNone {
					events = append(events, ev)
				}
			}

			if tc.wantErr {
				require.Error(t, parseErr)
			} else {
				require.NoError(t, parseErr)
			}
			if !tc.wantErr && tc.assertEvent != nil {
				tc.assertEvent(t, events)
			}
			if tc.assertState != nil {
				tc.assertState(t, p)
			}
		})
	}
}

func TestInputParser_Parse_Wrapper(t *testing.T) {
	tests := map[string]struct {
		line   string
		wantFn bool
		wantID string
	}{
		"function line returns function": {
			line:   `FUNCTION tx1 1 "fn1 arg1" ` + parserTestPermissions + ` "` + parserTestSource + `"`,
			wantFn: true,
			wantID: "tx1",
		},
		"cancel line returns nil function": {
			line: "FUNCTION_CANCEL tx1",
		},
		"progress line returns nil function": {
			line: "FUNCTION_PROGRESS tx1 10 100",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p := newInputParser()
			fn, err := p.parse(tc.line)
			require.NoError(t, err)
			if tc.wantFn {
				require.NotNil(t, fn)
				assert.Equal(t, tc.wantID, fn.UID)
				return
			}
			assert.Nil(t, fn)
		})
	}
}
