// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
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
			if err != nil {
				t.Fatal(err)
			}
			alias := prepared
			owner, err := lifecycle.NewFrameOwner(test.writer)
			if err != nil {
				t.Fatal(err)
			}
			transferErr := prepared.Transfer(owner)
			if test.wantCommit > 0 && transferErr != nil {
				t.Fatal(transferErr)
			}
			if test.wantAbort > 0 && transferErr == nil {
				t.Fatal("short write unexpectedly succeeded")
			}
			if commits != test.wantCommit || aborts != test.wantAbort {
				t.Fatalf("commits=%d aborts=%d", commits, aborts)
			}
			if err := alias.Abort(); !errors.Is(err, ErrPreparedVNodeFrameConsumed) {
				t.Fatalf("duplicate disposition error=%v", err)
			}
			if test.wantAbort > 0 {
				if census := owner.Census(); !census.Poisoned || census.RetainedBytes == 0 {
					t.Fatalf("frame census=%#v", census)
				}
			}
		})
	}
}

func TestPreparedVNodeFrameHoldsFrameOwnershipThroughMetadataCommit(t *testing.T) {
	writer := &countingProtocolWriter{}
	owner, err := lifecycle.NewFrameOwner(writer)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Transfer(owner); err != nil {
		t.Fatal(err)
	}
	if !commitSawBusy {
		t.Fatal("FrameOwner released serialization before metadata commit")
	}
	if err := owner.CommitProtocolFrame([]byte("BEGIN chart\nEND\n\n")); err != nil {
		t.Fatal(err)
	}
	if writes := writer.Writes(); writes != 2 {
		t.Fatalf("writes=%d want=2", writes)
	}
}

func TestPreparedVNodeFrameCommitFailurePoisonsOwner(t *testing.T) {
	owner, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("metadata commit failed")
	prepared, err := PrepareVNodeFrame(
		1,
		1,
		[]byte("HOST_DEFINE node\n\n"),
		func() error { return sentinel },
		func() error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Transfer(owner); !errors.Is(err, sentinel) {
		t.Fatalf("transfer error=%v want=%v", err, sentinel)
	}
	if census := owner.Census(); !census.Poisoned || census.RetainedBytes == 0 {
		t.Fatalf("frame census=%+v", census)
	}
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
			if err != nil {
				t.Fatal(err)
			}
			revision := pending.Revision()
			reservation, err := pending.Transfer()
			if err != nil {
				t.Fatal(err)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := registry.Lookup("node"); ok {
				t.Fatal("reservation visible before frame commit")
			}
			owner, err := lifecycle.NewFrameOwner(test.writer)
			if err != nil {
				t.Fatal(err)
			}
			transferErr := prepared.Transfer(owner)
			if test.wantVisible && transferErr != nil {
				t.Fatal(transferErr)
			}
			if !test.wantVisible && transferErr == nil {
				t.Fatal("short write unexpectedly succeeded")
			}
			if info, ok := registry.Lookup("node"); ok != test.wantVisible {
				t.Fatalf("metadata=%#v visible=%v want=%v", info, ok, test.wantVisible)
			}
			if test.wantVisible {
				if removed, err := registry.ReleaseMetadata(lease); err != nil || !removed {
					t.Fatalf("release removed=%v err=%v", removed, err)
				}
			} else {
				next, err := registry.PrepareMetadata(
					vnoderegistry.MetadataAuthority{ID: "job", Generation: 2},
					netdataapi.HostInfo{GUID: "node", Hostname: "next"},
				)
				if err != nil {
					t.Fatalf("failed frame retained reservation: %v", err)
				}
				reservation, err := next.Transfer()
				if err != nil {
					t.Fatal(err)
				}
				if err := reservation.Abort(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

type countingProtocolWriter struct {
	writes int
}

func (writer *countingProtocolWriter) Write(payload []byte) (int, error) {
	writer.writes++
	return len(payload), nil
}

func (writer *countingProtocolWriter) Writes() int {
	return writer.writes
}

func BenchmarkBVnodeFrame(b *testing.B) {
	owner, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		b.Fatal(err)
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
			b.Fatal(err)
		}
		if err := prepared.Transfer(owner); err != nil {
			b.Fatal(err)
		}
	}
}
