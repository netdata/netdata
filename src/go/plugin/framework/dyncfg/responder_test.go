// SPDX-License-Identifier: GPL-3.0-or-later

// Characterization tests (MUST-NOT-FLIP): pins of the responder's wire
// contracts that both the serialized and the concurrent pipeline designs
// depend on.

package dyncfg

import (
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type writeRecorder struct {
	mu     sync.Mutex
	writes []string
}

func (w *writeRecorder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

func (w *writeRecorder) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.writes...)
}

func testResponderFn(uid string) Function {
	return NewFunction(functions.Function{UID: uid})
}

func TestResponder_CharacterizationContracts(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, r *Responder, rec *writeRecorder)
	}{
		"terminal gated by finalizer while CONFIG lines bypass": {
			run: func(t *testing.T, r *Responder, rec *writeRecorder) {
				// Simulate a tombstoned transaction: the finalizer refuses emission.
				r.SetTerminalFinalizer(func(_, _ string, _ func()) bool { return false })

				r.SendCodef(testResponderFn("tx-tombstoned"), 200, "late response")
				r.SendJSON(testResponderFn("tx-tombstoned"), `{"late":true}`)
				assert.Empty(t, rec.snapshot(), "terminal responses must be suppressed when the finalizer refuses emission")

				r.ConfigStatus("test:collector:mod:job", StatusRunning)
				r.ConfigDelete("test:collector:mod:gone")

				writes := rec.snapshot()
				require.Len(t, writes, 2, "CONFIG lines must bypass the terminal finalizer")
				assert.Contains(t, writes[0], "CONFIG test:collector:mod:job status running")
				assert.Contains(t, writes[1], "CONFIG test:collector:mod:gone delete")
			},
		},
		"terminal emits through finalizer closure": {
			run: func(t *testing.T, r *Responder, rec *writeRecorder) {
				var finalized []string
				r.SetTerminalFinalizer(func(uid, source string, emit func()) bool {
					finalized = append(finalized, uid+"|"+source)
					emit()
					return true
				})

				r.SendCodef(testResponderFn("tx1"), 200, "ok")

				require.Len(t, finalized, 1)
				assert.Equal(t, "tx1|dyncfg.responder.sendcodef", finalized[0])
				writes := rec.snapshot()
				require.Len(t, writes, 1)
				assert.Contains(t, writes[0], "FUNCTION_RESULT_BEGIN tx1 200")
			},
		},
		"protocol messages are single writes": {
			run: func(t *testing.T, r *Responder, rec *writeRecorder) {
				r.SendCodef(testResponderFn("tx1"), 200, "ok")

				writes := rec.snapshot()
				require.Len(t, writes, 1, "a FUNCTION_RESULT block must be one Write")
				assert.True(t, strings.HasPrefix(writes[0], "FUNCTION_RESULT_BEGIN tx1 200"))
				assert.True(t, strings.HasSuffix(writes[0], "FUNCTION_RESULT_END\n\n"))

				rec2 := &writeRecorder{}
				r2 := NewResponder(netdataapi.New(rec2))
				r2.ConfigStatus("id1", StatusRunning)
				writes2 := rec2.snapshot()
				require.Len(t, writes2, 1, "a CONFIG line must be one Write")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := &writeRecorder{}
			tc.run(t, NewResponder(netdataapi.New(rec)), rec)
		})
	}
}
