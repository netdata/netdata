// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
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

func (rpp *recordingPublicationPort) Publish(record PublicationRecord) (PublicationHandle, error) {
	if rpp.publishErr != nil {
		return PublicationHandle{}, rpp.publishErr
	}
	rpp.nextID++
	handle := PublicationHandle{
		ID: rpp.nextID, Epoch: 1, Generation: record.Generation, Name: record.Name,
	}
	if rpp.badHandle {
		handle.Generation++
	}
	rpp.published = append(rpp.published, record)
	rpp.events = append(rpp.events, "publish:"+record.Name)
	rpp.active[handle.ID] = handle
	return handle, nil
}

func (rpp *recordingPublicationPort) Withdraw(handle PublicationHandle) error {
	if rpp.withdrawErr != nil {
		return rpp.withdrawErr
	}
	if _, ok := rpp.active[handle.ID]; !ok {
		return errors.New("duplicate or unknown withdraw")
	}
	delete(rpp.active, handle.ID)
	rpp.withdrawn = append(rpp.withdrawn, handle)
	rpp.events = append(rpp.events, "withdraw:"+handle.Name)
	return nil
}

func publicationRecord(name string, generation uint64) PublicationRecord {
	return PublicationRecord{Name: name, Generation: generation, Timeout: 1, Access: "0x0000"}
}

func TestFunctionPublicationDiff(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	a := publicationRecord("a", 1)
	b := publicationRecord("b", 1)

	require.NoError(t, publication.ApplyInitialSnapshot(1, 1, 2, []PublicationChange{
		{Name: "a", Record: &a}, {Name: "b", Record: &b},
	}),
	)

	require.False(t, len(port.published) != 2 || len(port.withdrawn) != 0)

	a2 := publicationRecord("a", 2)

	require.NoError(t, publication.ApplyTransition(1, 2, []PublicationChange{
		{Name: "a", Record: &a2}, {Name: "b"},
	}, func() error { return nil }, func() error { return nil }, func() error {
		return nil
	}),
	)

	require.False(t, len(port.published) != 3 || len(port.withdrawn) != 2 || len(port.active) != 1)

	got := port.events[len(port.events)-3:]
	require.True(t, equalPublicationEvents(
		got,
		[]string{"withdraw:a", "withdraw:b", "publish:a"},
	))

	require.NoError(t, publication.Stop(1))

	require.False(t, len(port.active) != 0 || len(port.withdrawn) != 3)

	require.NoError(t, publication.Stop(1))

	require.EqualValues(t, 3, len(port.withdrawn))
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
	require.NoError(t, err)
	record := publicationRecord("work", 1)
	require.NoError(t, publication.ApplyInitialSnapshot(1, 1, 1, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}),
	)

	require.NoError(t, publication.Stop(1))

	require.Error(t, publication.ApplyTransition(1, 2, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}, func() error { return nil }, func() error { return nil }, func() error {
		return nil
	}),
	)

	require.False(t, len(port.published) != 1 || len(port.withdrawn) != 1 || len(port.active) != 0)
}

func TestFunctionPublicationInitialSnapshotExceedsMutationQuantum(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	records := make([]PublicationRecord, 0, MaximumMutationPublicationChanges+3)
	changes := make([]PublicationChange, 0, MaximumMutationPublicationChanges+3)
	for index := range MaximumMutationPublicationChanges + 3 {
		record := publicationRecord(fmt.Sprintf("work-%03d", index), 1)
		records = append(records, record)
		changes = append(
			changes,
			PublicationChange{Name: record.Name, Record: &records[index]},
		)
	}

	require.NoError(t, publication.ApplyInitialSnapshot(
		1,
		1,
		int64(len(records)),
		changes,
	),
	)

	require.EqualValues(t, len(records), len(port.active))
}

func TestFunctionPublicationMutationCannotExceedQuantum(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)

	require.NoError(t, publication.ApplyInitialSnapshot(1, 1, 0, nil))

	changes := make([]PublicationChange, 0, MaximumMutationPublicationChanges+1)
	for index := range MaximumMutationPublicationChanges + 1 {
		record := publicationRecord(fmt.Sprintf("work-%03d", index), 1)
		changes = append(
			changes,
			PublicationChange{Name: record.Name, Record: &record},
		)
	}
	committed := false

	require.Error(t, publication.ApplyTransition(
		1,
		2,
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
	),
	)

	require.False(t, committed || len(port.published) != 0 || len(port.active) != 0)
}

func TestFunctionPublicationTransitionOrdersCatalogBetweenFrames(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	current := publicationRecord("work", 1)

	require.NoError(t, publication.ApplyInitialSnapshot(
		1,
		1,
		1,
		[]PublicationChange{{Name: current.Name, Record: &current}},
	),
	)

	next := publicationRecord("work", 2)

	require.NoError(t, publication.ApplyTransition(
		1,
		2,
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
	),
	)

	got := port.events
	require.True(t, equalPublicationEvents(got, []string{
		"publish:work",
		"catalog:quiesce",
		"withdraw:work",
		"catalog:commit",
		"publish:work",
	}))
}

func TestFunctionPublicationWithdrawalFailureAbortsQuiescedCatalog(t *testing.T) {
	port := newRecordingPublicationPort()
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	current := publicationRecord("work", 1)

	require.NoError(t, publication.ApplyInitialSnapshot(
		1,
		1,
		1,
		[]PublicationChange{{Name: current.Name, Record: &current}},
	),
	)

	port.withdrawErr = errors.New("withdraw failed")
	var quiesced, committed, aborted bool
	next := publicationRecord("work", 2)
	err = publication.ApplyTransition(
		1,
		2,
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
	require.Error(t, err)
	require.False(t, !quiesced || committed || !aborted)

	require.EqualValues(t, 1, len(port.active))
}

func TestFunctionPublicationMismatchedAcknowledgementPoisonsAndRetainsHandle(t *testing.T) {
	port := newRecordingPublicationPort()
	port.badHandle = true
	publication, err := NewPublication(1, port)
	require.NoError(t, err)
	record := publicationRecord("work", 1)

	require.Error(t, publication.ApplyInitialSnapshot(1, 1, 1, []PublicationChange{{
		Name: record.Name, Record: &record,
	}}),
	)

	require.EqualValues(t, 1, len(port.active))

	require.Error(t, publication.Stop(1))

	require.False(t, len(port.active) != 0 || len(port.withdrawn) != 1)
}
