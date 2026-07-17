// SPDX-License-Identifier: GPL-3.0-or-later

package vnoderegistry

import (
	"errors"
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

func TestMetadataLeaseCommitAbort(t *testing.T) {
	registry := New()
	prepared, err := registry.PrepareMetadata(
		MetadataAuthority{ID: "job", Generation: 1},
		netdataapi.HostInfo{GUID: "guid", Hostname: "first"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup("guid"); ok {
		t.Fatal("prepared metadata became visible before transfer and commit")
	}
	reservation, err := prepared.Transfer()
	if err != nil {
		t.Fatal(err)
	}
	lease, err := reservation.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if info, ok := registry.Lookup("guid"); !ok || info.Hostname != "first" {
		t.Fatalf("committed metadata=%+v ok=%v", info, ok)
	}

	aborted, err := registry.PrepareMetadata(
		MetadataAuthority{ID: "job", Generation: 2},
		netdataapi.HostInfo{GUID: "guid", Hostname: "second"},
	)
	if err != nil {
		t.Fatal(err)
	}
	abortedReservation, err := aborted.Transfer()
	if err != nil {
		t.Fatal(err)
	}
	if err := abortedReservation.Abort(); err != nil {
		t.Fatal(err)
	}
	if info, ok := registry.Lookup("guid"); !ok || info.Hostname != "first" {
		t.Fatalf("abort changed metadata=%+v ok=%v", info, ok)
	}
	if removed, err := registry.ReleaseMetadata(lease); err != nil || !removed {
		t.Fatalf("release removed=%v err=%v", removed, err)
	}
	if _, ok := registry.Lookup("guid"); ok {
		t.Fatal("last metadata lease retained registry entry")
	}
	if _, err := registry.ReleaseMetadata(lease); !errors.Is(err, ErrMetadataStaleLease) {
		t.Fatalf("duplicate release error=%v, want ErrMetadataStaleLease", err)
	}
}

func TestPreparedMetadataAbortBeforeTransfer(t *testing.T) {
	registry := New()
	prepared, err := registry.PrepareMetadata(
		MetadataAuthority{ID: "job", Generation: 1},
		netdataapi.HostInfo{GUID: "guid", Hostname: "first"},
	)
	if err != nil {
		t.Fatal(err)
	}
	alias := prepared
	if err := prepared.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err := alias.Transfer(); !errors.Is(err, ErrMetadataReservationConsumed) {
		t.Fatalf("post-abort transfer error=%v", err)
	}
	next, err := registry.PrepareMetadata(
		MetadataAuthority{ID: "job", Generation: 2},
		netdataapi.HostInfo{GUID: "guid", Hostname: "second"},
	)
	if err != nil {
		t.Fatalf("abandoned preparation retained GUID reservation: %v", err)
	}
	if err := next.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataGenerationAndOwnerFacets(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T, *Registry)
	}{
		"released generation remains stale": {
			run: func(t *testing.T, registry *Registry) {
				prepared, err := registry.PrepareMetadata(
					MetadataAuthority{ID: "job", Generation: 2},
					netdataapi.HostInfo{GUID: "guid", Hostname: "host"},
				)
				if err != nil {
					t.Fatal(err)
				}
				reservation, err := prepared.Transfer()
				if err != nil {
					t.Fatal(err)
				}
				lease, err := reservation.Commit()
				if err != nil {
					t.Fatal(err)
				}
				if removed, err := registry.ReleaseMetadata(lease); err != nil || !removed {
					t.Fatalf("release removed=%v err=%v", removed, err)
				}
				if _, err := registry.PrepareMetadata(
					MetadataAuthority{ID: "job", Generation: 1},
					netdataapi.HostInfo{GUID: "guid", Hostname: "stale"},
				); !errors.Is(err, ErrMetadataStaleGeneration) {
					t.Fatalf("stale generation error=%v", err)
				}
			},
		},
		"legacy release preserves metadata lease": {
			run: func(t *testing.T, registry *Registry) {
				info := netdataapi.HostInfo{GUID: "guid", Hostname: "host"}
				if _, err := registry.Register("job", info); err != nil {
					t.Fatal(err)
				}
				prepared, err := registry.PrepareMetadata(
					MetadataAuthority{ID: "job", Generation: 1},
					info,
				)
				if err != nil {
					t.Fatal(err)
				}
				reservation, err := prepared.Transfer()
				if err != nil {
					t.Fatal(err)
				}
				lease, err := reservation.Commit()
				if err != nil {
					t.Fatal(err)
				}
				if removed := registry.Release("job", "guid"); removed {
					t.Fatal("legacy release removed metadata-owned entry")
				}
				if _, ok := registry.Lookup("guid"); !ok {
					t.Fatal("legacy release invalidated metadata lease")
				}
				if removed, err := registry.ReleaseMetadata(lease); err != nil || !removed {
					t.Fatalf("metadata release removed=%v err=%v", removed, err)
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			test.run(t, New())
		})
	}
}

func TestMetadataHistoryIsBounded(t *testing.T) {
	registry := New()
	for index := 0; index < MaximumMetadataHistoryRecords; index++ {
		registry.metadataHistory[metadataHistoryKey{
			guid:  "guid",
			owner: Owner(strconv.Itoa(index)),
		}] = 1
	}
	if _, err := registry.PrepareMetadata(
		MetadataAuthority{ID: "overflow", Generation: 1},
		netdataapi.HostInfo{GUID: "other", Hostname: "host"},
	); !errors.Is(err, ErrMetadataHistoryCapacity) {
		t.Fatalf("capacity error=%v", err)
	}
}

func BenchmarkBMetadataLease(b *testing.B) {
	registry := New()
	info := netdataapi.HostInfo{GUID: "guid", Hostname: "host"}
	generation := uint64(0)
	b.ReportAllocs()
	for b.Loop() {
		generation++
		prepared, err := registry.PrepareMetadata(
			MetadataAuthority{ID: "job", Generation: generation},
			info,
		)
		if err != nil {
			b.Fatal(err)
		}
		reservation, err := prepared.Transfer()
		if err != nil {
			b.Fatal(err)
		}
		lease, err := reservation.Commit()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := registry.ReleaseMetadata(lease); err != nil {
			b.Fatal(err)
		}
	}
}
