// SPDX-License-Identifier: GPL-3.0-or-later

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

type typedOutputRecorder struct {
	results  []Result
	creates  []netdataapi.ConfigOpts
	statuses []struct {
		id     string
		status Status
	}
	deletes []string
}

func (recorder *typedOutputRecorder) FunctionResult(result Result) {
	recorder.results = append(recorder.results, result)
}

func (recorder *typedOutputRecorder) ConfigCreate(opts netdataapi.ConfigOpts) {
	recorder.creates = append(recorder.creates, opts)
}

func (recorder *typedOutputRecorder) ConfigStatus(id string, status Status) {
	recorder.statuses = append(recorder.statuses, struct {
		id     string
		status Status
	}{id: id, status: status})
}

func (recorder *typedOutputRecorder) ConfigDelete(id string) {
	recorder.deletes = append(recorder.deletes, id)
}

func TestResponderEmitsTypedOutput(t *testing.T) {
	tests := map[string]struct {
		run   func(*Responder)
		check func(*testing.T, *typedOutputRecorder)
	}{
		"formatted code result": {
			run: func(responder *Responder) {
				responder.SendCodef(testResponderFn("tx1"), 400, "invalid %s", "config")
			},
			check: func(t *testing.T, recorder *typedOutputRecorder) {
				require.Equal(t, []Result{{
					UID:         "tx1",
					Code:        400,
					ContentType: "application/json",
					Payload:     `{"status":400,"errorMessage":"invalid config"}`,
				}}, recorder.results)
			},
		},
		"payload result": {
			run: func(responder *Responder) {
				responder.SendYAML(testResponderFn("tx2"), "enabled: true")
			},
			check: func(t *testing.T, recorder *typedOutputRecorder) {
				require.Equal(t, []Result{{
					UID:         "tx2",
					Code:        200,
					ContentType: "application/yaml",
					Payload:     "enabled: true",
				}}, recorder.results)
			},
		},
		"empty UID suppresses result": {
			run: func(responder *Responder) {
				responder.SendJSON(testResponderFn(""), `{}`)
			},
			check: func(t *testing.T, recorder *typedOutputRecorder) {
				assert.Empty(t, recorder.results)
			},
		},
		"configuration notifications": {
			run: func(responder *Responder) {
				responder.ConfigCreate(netdataapi.ConfigOpts{ID: "template"})
				responder.ConfigStatus("job", StatusRunning)
				responder.ConfigDelete("gone")
			},
			check: func(t *testing.T, recorder *typedOutputRecorder) {
				require.Equal(t, []netdataapi.ConfigOpts{{ID: "template"}}, recorder.creates)
				require.Len(t, recorder.statuses, 1)
				assert.Equal(t, "job", recorder.statuses[0].id)
				assert.Equal(t, StatusRunning, recorder.statuses[0].status)
				assert.Equal(t, []string{"gone"}, recorder.deletes)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := &typedOutputRecorder{}
			test.run(NewResponder(recorder))
			test.check(t, recorder)
		})
	}
}

func TestProtocolOutputWritesCompleteFrames(t *testing.T) {
	recorder := &writeRecorder{}
	output := NewProtocolOutput(recorder)

	output.FunctionResult(Result{
		UID: "tx1", Code: 200, ContentType: "application/json", Payload: `{}`,
	})
	output.ConfigStatus("id1", StatusRunning)

	writes := recorder.snapshot()
	require.Len(t, writes, 2)
	assert.True(t, strings.HasPrefix(writes[0], "FUNCTION_RESULT_BEGIN tx1 200 application/json "))
	assert.True(t, strings.HasSuffix(writes[0], "\n{}\nFUNCTION_RESULT_END\n\n"))
	assert.Equal(t, "CONFIG id1 status running\n\n", writes[1])
}
