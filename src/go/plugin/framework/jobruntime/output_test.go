// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"errors"
	"io"
	"testing"
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
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error=%v want=%v", err, test.wantErr)
			}
			if (output.poison != nil) != test.wantPoison {
				t.Fatalf("poison=%v want=%v", output.poison, test.wantPoison)
			}
		})
	}
}

type recordingJobOutput struct {
	count  int
	err    error
	poison error
}

func (output *recordingJobOutput) Write([]byte) (int, error) {
	return output.count, output.err
}

func (output *recordingJobOutput) PoisonOutput(err error) {
	output.poison = err
}
