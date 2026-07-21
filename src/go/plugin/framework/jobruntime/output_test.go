// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitJobOutput(t *testing.T) {
	sentinel := errors.New("write failed")
	tests := map[string]struct {
		count      int
		writeErr   error
		wantErr    error
		wantPoison bool
	}{
		"whole write": {
			count: 4,
		},
		"short write": {
			count: 3, wantErr: io.ErrShortWrite, wantPoison: true,
		},
		"write error": {
			writeErr: sentinel, wantErr: sentinel, wantPoison: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			output := &recordingJobOutput{count: test.count, err: test.writeErr}
			err := commitJobOutput(output, []byte("data"))
			require.ErrorIs(t, err, test.wantErr)
			assert.Equal(t, test.wantPoison, output.poison != nil)
		})
	}
}

func TestCommitJobOutputTransaction(t *testing.T) {
	writeErr := errors.New("write failed")
	commitErr := errors.New("commit failed")
	tests := map[string]struct {
		payload    []byte
		count      int
		writeErr   error
		commitErr  error
		wantErr    error
		wantEvents []string
		wantPoison bool
	}{
		"whole write commits state": {
			payload: []byte("data"), count: 4,
			wantEvents: []string{"write", "commit"},
		},
		"empty output still commits state": {
			wantEvents: []string{"commit"},
		},
		"write error aborts state": {
			payload: []byte("data"), writeErr: writeErr, wantErr: writeErr,
			wantEvents: []string{"write", "abort"}, wantPoison: true,
		},
		"commit error aborts and poisons written output": {
			payload: []byte("data"), count: 4,
			commitErr: commitErr, wantErr: commitErr,
			wantEvents: []string{"write", "commit", "abort"}, wantPoison: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var events []string
			output := &recordingJobOutput{
				count: test.count, err: test.writeErr, events: &events,
			}
			err := commitJobOutputTransaction(
				output,
				test.payload,
				&recordingOutputState{
					events: &events, commitErr: test.commitErr,
				},
			)
			require.ErrorIs(t, err, test.wantErr)
			assert.Equal(t, test.wantEvents, events)
			assert.Equal(t, test.wantPoison, output.poison != nil)
		})
	}
}

type recordingOutputState struct {
	events    *[]string
	commitErr error
}

func (state *recordingOutputState) Commit() error {
	*state.events = append(*state.events, "commit")
	return state.commitErr
}

func (state *recordingOutputState) Abort() error {
	*state.events = append(*state.events, "abort")
	return nil
}

type recordingJobOutput struct {
	count  int
	err    error
	poison error
	events *[]string
}

func (output *recordingJobOutput) Write([]byte) (int, error) {
	if output.events != nil {
		*output.events = append(*output.events, "write")
	}
	return output.count, output.err
}

func (output *recordingJobOutput) PoisonOutput(err error) {
	output.poison = err
}
