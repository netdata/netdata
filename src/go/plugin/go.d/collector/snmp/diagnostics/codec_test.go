// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArchiveEncoderReuse(t *testing.T) {
	for name, tc := range map[string]struct {
		first  io.Writer
		failed bool
	}{
		"completed file": {first: io.Discard},
		"failed file":    {first: failingArchiveWriter{}, failed: true},
		"missing writer": {failed: true},
	} {
		t.Run(name, func(t *testing.T) {
			encoder, err := newArchiveEncoder()
			require.NoError(t, err)
			err = encoder.write(tc.first, testDocument(1))
			if tc.failed {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			var encoded bytes.Buffer
			want := testDocument(2)
			require.NoError(t, encoder.write(&encoded, want))
			got, err := Read(&encoded, DefaultReadLimits())
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

type failingArchiveWriter struct{}

func (failingArchiveWriter) Write([]byte) (int, error) {
	return 0, errors.New("synthetic archive write failure")
}
