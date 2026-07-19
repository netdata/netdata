// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/stretchr/testify/require"
)

func TestPreparedVNodeFrameLinearTransfer(t *testing.T) {
	tests := map[string]struct {
		writer     interface{ Write([]byte) (int, error) }
		wantCommit int
		wantAbort  int
	}{
		"commit after full write": {
			writer: &bytes.Buffer{}, wantCommit: 1,
		},
		"abort after short write": {
			writer: shortJobProtocolWriter{}, wantAbort: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			commits := 0
			aborts := 0
			prepared, err := PrepareVNodeFrame(
				7,
				3,
				[]byte("HOST_DEFINE node\n\n"),
				func() error { commits++; return nil },
				func() error { aborts++; return nil },
			)
			require.NoError(t, err)
			alias := prepared
			owner, err := lifecycle.NewFrameOwner(test.writer)
			require.NoError(t, err)
			transferErr := prepared.Transfer(owner)
			require.False(t, test.wantCommit > 0 && transferErr != nil)
			require.False(t, test.wantAbort > 0 && transferErr == nil)
			require.False(t, commits != test.wantCommit || aborts != test.wantAbort)

			require.ErrorIs(t, alias.Abort(), ErrPreparedVNodeFrameConsumed)

			if test.wantAbort > 0 {
				census := owner.Census()
				require.False(t, !census.Poisoned || census.RetainedBytes == 0)
			}
		})
	}
}

func TestPreparedVNodeFrameHoldsFrameOwnershipThroughMetadataCommit(t *testing.T) {
	writer := &countingProtocolWriter{}
	owner, err := lifecycle.NewFrameOwner(writer)
	require.NoError(t, err)
	commitSawBusy := false
	prepared, err := PrepareVNodeFrame(
		1,
		1,
		[]byte("HOST_DEFINE node\n\n"),
		func() error {
			commitSawBusy = owner.Census().Busy
			return nil
		},
		func() error { return nil },
	)
	require.NoError(t, err)

	require.NoError(t, prepared.Transfer(owner))

	require.True(t, commitSawBusy)

	require.NoError(t, owner.CommitProtocolFrame([]byte("BEGIN chart\nEND\n\n")))

	writes := writer.Writes()
	require.EqualValues(t, 2, writes)
}

func TestPreparedVNodeFrameCommitFailurePoisonsOwner(t *testing.T) {
	owner, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	sentinel := errors.New("metadata commit failed")
	prepared, err := PrepareVNodeFrame(
		1,
		1,
		[]byte("HOST_DEFINE node\n\n"),
		func() error { return sentinel },
		func() error { return nil },
	)
	require.NoError(t, err)

	require.ErrorIs(t, prepared.Transfer(owner), sentinel)

	census := owner.Census()
	require.False(t, !census.Poisoned || census.RetainedBytes == 0)
}

func TestPreparedVNodeFrameDrivesRegistryTransaction(t *testing.T) {
	tests := map[string]struct {
		writer      interface{ Write([]byte) (int, error) }
		wantVisible bool
	}{
		"commit": {
			writer: &bytes.Buffer{}, wantVisible: true,
		},
		"abort": {
			writer: shortJobProtocolWriter{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			registry := vnoderegistry.New()
			pending, err := registry.PrepareMetadata(
				vnoderegistry.MetadataAuthority{ID: "job", Generation: 1},
				netdataapi.HostInfo{GUID: "node", Hostname: "host"},
			)
			require.NoError(t, err)
			revision := pending.Revision()
			reservation, err := pending.Transfer()
			require.NoError(t, err)
			var lease vnoderegistry.MetadataLease
			prepared, err := PrepareVNodeFrame(
				1,
				revision,
				[]byte("HOST_DEFINE node\n\n"),
				func() error {
					var commitErr error
					lease, commitErr = reservation.Commit()
					return commitErr
				},
				reservation.Abort,
			)
			require.NoError(t, err)

			_, ok := registry.Lookup("node")
			require.False(t, ok)

			owner, err := lifecycle.NewFrameOwner(test.writer)
			require.NoError(t, err)
			transferErr := prepared.Transfer(owner)
			require.False(t, test.wantVisible && transferErr != nil)
			require.False(t, !test.wantVisible && transferErr == nil)

			lookupInfo, lookup := registry.Lookup("node")
			require.EqualValues(t, test.wantVisible, lookup, "metadata=%+v", lookupInfo)

			if test.wantVisible {

				releaseMetadataRemoved, releaseMetadataErr := registry.ReleaseMetadata(lease)
				require.False(t, releaseMetadataErr != nil || !releaseMetadataRemoved)

			} else {
				next, err := registry.PrepareMetadata(
					vnoderegistry.MetadataAuthority{ID: "job", Generation: 2},
					netdataapi.HostInfo{GUID: "node", Hostname: "next"},
				)
				require.NoError(t, err)
				reservation, err := next.Transfer()
				require.NoError(t, err)

				require.NoError(t, reservation.Abort())
			}
		})
	}
}

type countingProtocolWriter struct {
	writes int
}

func (cpw *countingProtocolWriter) Write(payload []byte) (int, error) {
	cpw.writes++
	return len(payload), nil
}

func (cpw *countingProtocolWriter) Writes() int {
	return cpw.writes
}

func BenchmarkBVnodeFrame(b *testing.B) {
	owner, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		prepared, err := PrepareVNodeFrame(
			1,
			1,
			[]byte("HOST_DEFINE node\n\n"),
			func() error { return nil },
			func() error { return nil },
		)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if err := prepared.Transfer(owner); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}
