// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"fmt"
	"testing"
)

type recordingPublicationPort struct {
	nextID      uint64
	published   []PublicationRecord
	withdrawn   []PublicationHandle
	events      []string
	active      map[uint64]PublicationHandle
	badHandle   bool
	publishErr  error
	withdrawErr error
}

func newRecordingPublicationPort() *recordingPublicationPort {
	return &recordingPublicationPort{active: make(map[uint64]PublicationHandle)}
}

func (port *recordingPublicationPort) Publish(record PublicationRecord) (PublicationHandle, error) {
	if port.publishErr != nil {
		return PublicationHandle{}, port.publishErr
	}
	port.nextID++
	handle := PublicationHandle{
		ID: port.nextID, Epoch: 1, Generation: record.Generation, Name: record.Name,
	}
	if port.badHandle {
		handle.Generation++
	}
	port.published = append(port.published, record)
	port.events = append(port.events, "publish:"+record.Name)
	port.active[handle.ID] = handle
	return handle, nil
}

func (port *recordingPublicationPort) Withdraw(handle PublicationHandle) error {
	if port.withdrawErr != nil {
		return port.withdrawErr
	}
	if _, ok := port.active[handle.ID]; !ok {
		return errors.New("duplicate or unknown withdraw")
	}
	delete(port.active, handle.ID)
	port.withdrawn = append(port.withdrawn, handle)
	port.events = append(port.events, "withdraw:"+handle.Name)
	return nil
}

func publicationRecord(name string, generation uint64) PublicationRecord {
	return PublicationRecord{Name: name, Generation: generation, Timeout: 1, Access: "0x0000"}
}

func publicationDigest(t testing.TB, records ...PublicationRecord) [32]byte {
	t.Helper()
	digest, err := DigestSortedPublications(records)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestFunctionPublicationDiff(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	a := publicationRecord("a", 1)
	b := publicationRecord("b", 1)
	if err := publication.ApplyInitialSnapshot(1, 1, publicationDigest(t, a, b), 2, []PublicationChange{
		{Name: "a", Record: &a}, {Name: "b", Record: &b},
	}); err != nil {
		t.Fatal(err)
	}
	if len(port.published) != 2 || len(port.withdrawn) != 0 {
		t.Fatalf("initial calls differ: publish=%d withdraw=%d", len(port.published), len(port.withdrawn))
	}
	if err := publication.Poll(1, 1, publicationDigest(t, a, b)); err != nil {
		t.Fatal(err)
	}
	if len(port.published) != 2 || len(port.withdrawn) != 0 {
		t.Fatal("unchanged poll performed external work")
	}

	a2 := publicationRecord("a", 2)
	if err := publication.ApplyTransition(1, 2, publicationDigest(t, a2), []PublicationChange{
		{Name: "a", Record: &a2}, {Name: "b"},
	}, func() error { return nil }, func() error { return nil }, func() error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(port.published) != 3 || len(port.withdrawn) != 2 || len(port.active) != 1 {
		t.Fatalf("changed calls differ: publish=%d withdraw=%d active=%d",
			len(port.published), len(port.withdrawn), len(port.active))
	}
	if got := port.events[len(port.events)-3:]; !equalPublicationEvents(
		got,
		[]string{"withdraw:a", "withdraw:b", "publish:a"},
	) {
		t.Fatalf("replacement ordering=%v", got)
	}
	if census := publication.Census(); census.Version != 2 || census.Published != 1 ||
		census.Dirty || census.Stopped {
		t.Fatalf("publication census differs: %+v", census)
	}
	if err := publication.Stop(1); err != nil {
		t.Fatal(err)
	}
	if len(port.active) != 0 || len(port.withdrawn) != 3 {
		t.Fatalf("stop did not withdraw exact live set: active=%d withdrawn=%d",
			len(port.active), len(port.withdrawn))
	}
	if err := publication.Stop(1); err != nil {
		t.Fatal(err)
	}
	if len(port.withdrawn) != 3 {
		t.Fatal("repeat stop duplicated an unregister")
	}
}

func equalPublicationEvents(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestFunctionPublicationNoRearm(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	record := publicationRecord("work", 1)
	digest := publicationDigest(t, record)
	if err := publication.ApplyInitialSnapshot(1, 1, digest, 1, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := publication.Stop(1); err != nil {
		t.Fatal(err)
	}
	if err := publication.ApplyTransition(1, 2, digest, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}, func() error { return nil }, func() error { return nil }, func() error {
		return nil
	}); err == nil {
		t.Fatal("publication rearmed after stop")
	}
	if err := publication.Poll(1, 1, digest); err == nil {
		t.Fatal("availability poll rearmed after stop")
	}
	if len(port.published) != 1 || len(port.withdrawn) != 1 || len(port.active) != 0 {
		t.Fatal("post-stop calls changed external publication state")
	}
}

func TestFunctionPublicationInitialSnapshotExceedsMutationQuantum(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	records := make(
		[]PublicationRecord,
		0,
		MaximumMutationPublicationChanges+3,
	)
	changes := make(
		[]PublicationChange,
		0,
		MaximumMutationPublicationChanges+3,
	)
	for index := 0; index < MaximumMutationPublicationChanges+3; index++ {
		record := publicationRecord(fmt.Sprintf("work-%03d", index), 1)
		records = append(records, record)
		changes = append(
			changes,
			PublicationChange{Name: record.Name, Record: &records[index]},
		)
	}
	if err := publication.ApplyInitialSnapshot(
		1,
		1,
		publicationDigest(t, records...),
		int64(len(records)),
		changes,
	); err != nil {
		t.Fatal(err)
	}
	if census := publication.Census(); census.Published != len(records) ||
		census.Version != 1 ||
		census.Dirty {
		t.Fatalf("initial snapshot census=%+v", census)
	}
}

func TestFunctionPublicationMutationCannotExceedQuantum(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.ApplyInitialSnapshot(
		1,
		1,
		publicationDigest(t),
		0,
		nil,
	); err != nil {
		t.Fatal(err)
	}
	changes := make(
		[]PublicationChange,
		0,
		MaximumMutationPublicationChanges+1,
	)
	for index := 0; index < MaximumMutationPublicationChanges+1; index++ {
		record := publicationRecord(fmt.Sprintf("work-%03d", index), 1)
		changes = append(
			changes,
			PublicationChange{Name: record.Name, Record: &record},
		)
	}
	committed := false
	if err := publication.ApplyTransition(
		1,
		2,
		[32]byte{},
		changes,
		func() error {
			committed = true
			return nil
		},
		func() error {
			committed = true
			return nil
		},
		func() error {
			committed = true
			return nil
		},
	); err == nil {
		t.Fatal("oversized steady mutation was accepted")
	}
	if committed || len(port.published) != 0 || len(port.active) != 0 {
		t.Fatalf(
			"oversized steady mutation changed state: committed=%v published=%d active=%d",
			committed,
			len(port.published),
			len(port.active),
		)
	}
}

func TestFunctionPublicationTransitionOrdersCatalogBetweenFrames(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	current := publicationRecord("work", 1)
	if err := publication.ApplyInitialSnapshot(
		1,
		1,
		publicationDigest(t, current),
		1,
		[]PublicationChange{{Name: current.Name, Record: &current}},
	); err != nil {
		t.Fatal(err)
	}
	next := publicationRecord("work", 2)
	if err := publication.ApplyTransition(
		1,
		2,
		publicationDigest(t, next),
		[]PublicationChange{{Name: next.Name, Record: &next}},
		func() error {
			port.events = append(port.events, "catalog:quiesce")
			return nil
		},
		func() error {
			port.events = append(port.events, "catalog:commit")
			return nil
		},
		func() error {
			port.events = append(port.events, "catalog:abort")
			return nil
		},
	); err != nil {
		t.Fatal(err)
	}
	if got := port.events; !equalPublicationEvents(got, []string{
		"publish:work",
		"catalog:quiesce",
		"withdraw:work",
		"catalog:commit",
		"publish:work",
	}) {
		t.Fatalf("transition order=%v", got)
	}
}

func TestFunctionPublicationWithdrawalFailureAbortsQuiescedCatalog(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	current := publicationRecord("work", 1)
	if err := publication.ApplyInitialSnapshot(
		1,
		1,
		publicationDigest(t, current),
		1,
		[]PublicationChange{{Name: current.Name, Record: &current}},
	); err != nil {
		t.Fatal(err)
	}
	port.withdrawErr = errors.New("withdraw failed")
	var quiesced, committed, aborted bool
	next := publicationRecord("work", 2)
	err = publication.ApplyTransition(
		1,
		2,
		publicationDigest(t, next),
		[]PublicationChange{{Name: next.Name, Record: &next}},
		func() error {
			quiesced = true
			return nil
		},
		func() error {
			committed = true
			return nil
		},
		func() error {
			aborted = true
			return nil
		},
	)
	if err == nil {
		t.Fatal("withdrawal failure was accepted")
	}
	if !quiesced || committed || !aborted {
		t.Fatalf(
			"transition callbacks: quiesced=%v committed=%v aborted=%v",
			quiesced,
			committed,
			aborted,
		)
	}
	if census := publication.Census(); census.Version != 1 ||
		census.Published != 1 || !census.Dirty {
		t.Fatalf("failed transition census=%+v", census)
	}
}

func TestFunctionPublicationMismatchedAcknowledgementPoisonsAndRetainsHandle(t *testing.T) {
	port := newRecordingPublicationPort()
	port.badHandle = true
	publication, err := NewPublication(1, port)
	if err != nil {
		t.Fatal(err)
	}
	record := publicationRecord("work", 1)
	if err := publication.ApplyInitialSnapshot(1, 1, publicationDigest(t, record), 1, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}); err == nil {
		t.Fatal("mismatched acknowledgement was accepted")
	}
	if census := publication.Census(); !census.Dirty || census.RetainedHandles != 1 ||
		census.Published != 0 {
		t.Fatalf("poisoned publication census differs: %+v", census)
	}
	if err := publication.Stop(1); err == nil {
		t.Fatal("dirty publication stop lost its terminal cause")
	}
	if len(port.active) != 0 || len(port.withdrawn) != 1 {
		t.Fatal("dirty publication did not withdraw retained handle")
	}
}

func BenchmarkBFunctionPublication(b *testing.B) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	if err != nil {
		b.Fatal(err)
	}
	record := publicationRecord("work", 1)
	digest := publicationDigest(b, record)
	if err := publication.ApplyInitialSnapshot(1, 1, digest, 1, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if err := publication.Poll(1, 1, digest); err != nil {
			b.Fatal(err)
		}
	}
}
