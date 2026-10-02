// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

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

func TestNormalEnvelopeSnapshot(t *testing.T) {
	for name, tc := range map[string]struct {
		snapshot Snapshot
		invalid  bool
	}{
		"empty":               {},
		"producer scope":      {Snapshot{ProducerScopeID: "scope"}, true},
		"topology":            {Snapshot{Topology: &Sweep{}}, true},
		"aborted sweep":       {Snapshot{LastAborted: &Abort{}}, true},
		"lifecycle state":     {Snapshot{Lifecycle: Lifecycle{State: "available"}}, true},
		"lifecycle reason":    {Snapshot{Lifecycle: Lifecycle{Reason: "none"}}, true},
		"lifecycle sequence":  {Snapshot{Lifecycle: Lifecycle{Cut: LifecycleCut{Sequence: 1}}}, true},
		"lifecycle timestamp": {Snapshot{Lifecycle: Lifecycle{Cut: LifecycleCut{CapturedAt: time.Unix(1, 0).UTC()}}}, true},
		"lifecycle entries":   {Snapshot{Lifecycle: Lifecycle{Cut: LifecycleCut{Entries: []LifecycleEntry{{RegistrationID: 1}}}}}, true},
	} {
		t.Run(name, func(t *testing.T) {
			doc := Document{Format: Format, Version: Version, Kind: KindNormal, Normal: &NormalDevice{}, Snapshot: tc.snapshot}
			var encoded bytes.Buffer
			require.NoError(t, Write(&encoded, doc))
			_, err := Read(&encoded, DefaultReadLimits())
			if tc.invalid {
				require.ErrorContains(t, err, "invalid normal device document envelope")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestArchiveWrite(t *testing.T) {
	for name, tc := range map[string]struct {
		writer    io.Writer
		wantError string
	}{
		"nil writer":    {wantError: "nil writer"},
		"failed writer": {writer: failingArchiveWriter{}, wantError: "synthetic archive write failure"},
		"valid writer":  {writer: io.Discard},
	} {
		t.Run(name, func(t *testing.T) {
			err := Write(tc.writer, testDocument(1))
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
