// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"errors"
	"testing"
)

func TestPreparedFunctionFrameTransfersExactlyOnce(t *testing.T) {
	result, err := NewSealedResult(200, "application/json", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := PrepareFrame("u-linear", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	alias := frame
	var output bytes.Buffer
	owner, err := NewFrameOwner(&output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Commit(frame); err != nil {
		t.Fatal(err)
	}
	if err := owner.Commit(alias); !errors.Is(err, ErrPreparedFrameConsumed) {
		t.Fatalf("duplicate transfer error=%v, want ErrPreparedFrameConsumed", err)
	}
	if got := bytes.Count(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN")); got != 1 {
		t.Fatalf("writes=%d, want 1", got)
	}
}

func TestPreparedProtocolFrameCommitOrAbortIsLinear(t *testing.T) {
	tests := map[string]struct {
		commit bool
	}{
		"commit": {commit: true},
		"abort":  {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := PrepareProtocolFrame([]byte("HOST_DEFINE node\n\n"))
			if err != nil {
				t.Fatal(err)
			}
			alias := frame
			if !test.commit {
				if err := frame.Abort(); err != nil {
					t.Fatal(err)
				}
				owner, err := NewFrameOwner(&bytes.Buffer{}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := owner.CommitPreparedProtocolFrame(alias); !errors.Is(err, ErrPreparedFrameConsumed) {
					t.Fatalf("post-abort transfer error=%v, want ErrPreparedFrameConsumed", err)
				}
				return
			}

			var output bytes.Buffer
			owner, err := NewFrameOwner(&output, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.CommitPreparedProtocolFrame(frame); err != nil {
				t.Fatal(err)
			}
			if err := alias.Abort(); !errors.Is(err, ErrPreparedFrameConsumed) {
				t.Fatalf("post-commit abort error=%v, want ErrPreparedFrameConsumed", err)
			}
			if got := output.String(); got != "HOST_DEFINE node\n\n" {
				t.Fatalf("output=%q", got)
			}
		})
	}
}
