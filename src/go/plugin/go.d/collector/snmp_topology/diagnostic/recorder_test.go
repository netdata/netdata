// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSemanticCapability = CapabilityKey{Name: "semantic_replay", Revision: 1}

func newTestRecorder(t *testing.T, sink CaptureSink, capacity uint64) *Recorder {
	t.Helper()
	recorder, err := NewRecorder(RecorderConfig{
		QueueCapacity:    capacity,
		MaxMembers:       16,
		MaxRetainedBytes: 1 << 20,
		Sink:             sink,
	})
	require.NoError(t, err)
	return recorder
}

func TestRecorder_SealsHandleAndBuildsPortableGraph(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 1)
	txn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txn.DefineSection("semantic_inputs", StateSuccess, 1))
	handle, err := txn.AddOwned(
		"semantic_inputs",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		testLeaf{ID: "leaf-a", Tags: map[string]string{"role": "switch"}},
		64,
	)
	require.NoError(t, err)
	_, _, state := handle.Resolve()
	assert.Equal(t, HandlePending, state)
	_, err = json.Marshal(handle)
	require.ErrorContains(t, err, "not serializable")

	require.NoError(t, txn.Commit(StateSuccess))
	require.ErrorIs(t, txn.Commit(StateSuccess), ErrTransactionClosed)
	recorder.Close()

	ref, err, state := handle.Resolve()
	require.NoError(t, err)
	assert.Equal(t, HandleSealed, state)
	assert.Equal(t, "semantic_leaf", ref.Kind)

	results := sink.Results()
	require.Len(t, results, 1)
	result := results[0]
	require.NoError(t, result.Err)
	require.NoError(t, result.Manifest.Validate())
	assert.Equal(t, uint64(64), result.RetainedBytes)
	assert.NotContains(t, string(mustCanonicalBytes(t, result.Manifest)), "member-handle")

	registry := NewRegistry()
	require.NoError(t, registry.Register(testSemanticCapability, Closure{
		RootType: MemberType{Kind: KindCapabilityRoot, Schema: SchemaV1},
		Decode: map[MemberType]DecodeMemberFunc{
			{Kind: KindCapabilityRoot, Schema: SchemaV1}: DecodeCapabilityRoot(testSemanticCapability),
			{Kind: "semantic_leaf", Schema: SchemaV1}:    DecodeLeaf[testLeaf](),
		},
	}))
	report, err := registry.ValidateCapability(result.Manifest, result.Members, testSemanticCapability, testReaderLimits())
	require.NoError(t, err)
	assert.True(t, report.Replayable)
}

func TestRecorder_DeduplicatesInventoryWhilePreservingRepeatedReferences(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 1)
	txn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txn.DefineSection("semantic_inputs", StateSuccess, 2))
	value := testLeaf{ID: "leaf-a"}
	handleA, err := txn.AddOwned("semantic_inputs", MemberType{Kind: "semantic_leaf", Schema: SchemaV1}, value, 32)
	require.NoError(t, err)
	handleB, err := txn.AddOwned("semantic_inputs", MemberType{Kind: "semantic_leaf", Schema: SchemaV1}, value, 32)
	require.NoError(t, err)
	require.NoError(t, txn.Commit(StateSuccess))
	recorder.Close()

	result := sink.Results()[0]
	require.NoError(t, result.Manifest.Validate())
	assert.Len(t, result.Manifest.Members, 2, "the root and one deduplicated leaf are inventoried")
	refA, err, state := handleA.Resolve()
	require.NoError(t, err)
	assert.Equal(t, HandleSealed, state)
	refB, err, state := handleB.Resolve()
	require.NoError(t, err)
	assert.Equal(t, HandleSealed, state)
	assert.Equal(t, refA, refB)

	rootRef := result.Manifest.Roots[0].Root
	var root CapabilityRootV1
	require.NoError(t, DecodeCanonical(result.Members[rootRef.Key()], testReaderLimits(), &root))
	require.Len(t, root.Sections[0].Members, 2)
	assert.Equal(t, root.Sections[0].Members[0], root.Sections[0].Members[1])
}

func TestRecorder_DerivedReferenceCarriesTransitivePortableInventory(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 3)

	leafTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, leafTxn.DefineSection("leaf", StateSuccess, 1))
	leafHandle, err := leafTxn.AddOwned(
		"leaf",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		testLeaf{ID: "leaf-a"},
		32,
	)
	require.NoError(t, err)
	require.NoError(t, leafTxn.Commit(StateSuccess))

	derivedTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.DefineSection("derived", StateSuccess, 1))
	derivedHandle, err := derivedTxn.AddDerivedOwned(
		"derived",
		MemberType{Kind: "derived_leaf", Schema: SchemaV1},
		[]MemberHandle{leafHandle},
		func(refs []ContentRef) (any, error) {
			require.Len(t, refs, 1)
			return referencedTestLeaf{ID: "derived-a", Child: refs[0]}, nil
		},
		64,
	)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.Commit(StateSuccess))

	referenceTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, referenceTxn.DefineSection("reference", StateSuccess, 1))
	require.NoError(t, referenceTxn.AddReference("reference", derivedHandle))
	require.NoError(t, referenceTxn.Commit(StateSuccess))
	recorder.Close()

	results := sink.Results()
	require.Len(t, results, 3)
	result := results[2]
	require.NoError(t, result.Err)
	require.NoError(t, result.Manifest.Validate())
	assert.Len(t, result.Manifest.Members, 3, "root, derived member, and transitive leaf must be inventoried")

	leafRef, err, state := leafHandle.Resolve()
	require.NoError(t, err)
	require.Equal(t, HandleSealed, state)
	derivedRef, err, state := derivedHandle.Resolve()
	require.NoError(t, err)
	require.Equal(t, HandleSealed, state)
	assert.Contains(t, result.Manifest.Members, leafRef)
	assert.Contains(t, result.Manifest.Members, derivedRef)

	rootRef := result.Manifest.Roots[0].Root
	var root CapabilityRootV1
	require.NoError(t, DecodeCanonical(result.Members[rootRef.Key()], testReaderLimits(), &root))
	require.Equal(t, []ContentRef{derivedRef}, root.Sections[0].Members)
}

func TestRecorder_OptionalDependencyPreservesResultWhenEvidenceFails(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 2)
	failedTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, failedTxn.DefineSection("leaf", StateSuccess, 1))
	failed, err := failedTxn.AddOwned(
		"leaf",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		func() {},
		32,
	)
	require.NoError(t, err)
	require.NoError(t, failedTxn.Commit(StateSuccess))

	derivedTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.DefineSection("derived", StateSuccess, 1))
	derived, err := derivedTxn.AddOptionalDerivedOwned(
		"derived",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		[]MemberHandle{failed},
		func(resolved []MemberResolution) (any, error) {
			require.Len(t, resolved, 1)
			require.Equal(t, HandleFailed, resolved[0].State)
			return testLeaf{ID: "capture-unavailable"}, nil
		},
		32,
	)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.Commit(StateSuccess))
	recorder.Close()

	_, handleErr, state := failed.Resolve()
	assert.Equal(t, HandleFailed, state)
	require.Error(t, handleErr)
	_, handleErr, state = derived.Resolve()
	assert.Equal(t, HandleSealed, state)
	require.NoError(t, handleErr)
	require.Len(t, sink.Results(), 2)
	require.NoError(t, sink.Results()[1].Err)
}

func TestCaptureTransaction_RejectsCrossRecorderHandles(t *testing.T) {
	t.Parallel()

	sinkA := &MemorySink{}
	recorderA := newTestRecorder(t, sinkA, 1)
	txnA, err := recorderA.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txnA.DefineSection("leaf", StateSuccess, 1))
	handle, err := txnA.AddOwned(
		"leaf",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		testLeaf{ID: "leaf-a"},
		32,
	)
	require.NoError(t, err)
	require.NoError(t, txnA.Commit(StateSuccess))
	recorderA.Close()

	sinkB := &MemorySink{}
	recorderB := newTestRecorder(t, sinkB, 1)
	txnB, err := recorderB.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txnB.DefineSection("reference", StateSuccess, 1))
	refErr := txnB.AddReference("reference", handle)
	require.NoError(t, txnB.Commit(StateSuccess))
	recorderB.Close()

	require.ErrorContains(t, refErr, "different recorder")
}

func TestRecorder_RejectsTransitiveManifestAboveMemberLimit(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder, err := NewRecorder(RecorderConfig{
		QueueCapacity:    8,
		MaxMembers:       4,
		MaxRetainedBytes: 1 << 20,
		Sink:             sink,
	})
	require.NoError(t, err)

	dependencies := make([]MemberHandle, 0, 2)
	for attempt := uint64(1); attempt <= 2; attempt++ {
		capability := CapabilityKey{Name: "test_dependency", Revision: 1}
		txn, err := recorder.Begin(capability)
		require.NoError(t, err)
		require.NoError(t, txn.DefineSection("items", StateSuccess, 1))
		handle, err := txn.AddOwned(
			"items",
			MemberType{Kind: KindCaptureGap, Schema: SchemaV1},
			CaptureGapV1{CapabilityClass: string(CaptureClassOther), FirstAttempt: attempt, LastAttempt: attempt, Count: 1, Reason: "test"},
			128,
		)
		require.NoError(t, err)
		dependencies = append(dependencies, handle)
		require.NoError(t, txn.Commit(StateSuccess))
	}

	derivedTxn, err := recorder.Begin(CapabilityKey{Name: "test_derived", Revision: 1})
	require.NoError(t, err)
	require.NoError(t, derivedTxn.DefineSection("derived", StateSuccess, 1))
	derivedHandle, err := derivedTxn.AddDerivedOwned(
		"derived",
		MemberType{Kind: KindCaptureGap, Schema: SchemaV1},
		dependencies,
		func([]ContentRef) (any, error) {
			return CaptureGapV1{CapabilityClass: string(CaptureClassOther), FirstAttempt: 3, LastAttempt: 3, Count: 1, Reason: "test"}, nil
		},
		128,
	)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.Commit(StateSuccess))

	overflowTxn, err := recorder.Begin(CapabilityKey{Name: "test_overflow", Revision: 1})
	require.NoError(t, err)
	require.NoError(t, overflowTxn.DefineSection("reference", StateSuccess, 1))
	require.NoError(t, overflowTxn.AddReference("reference", derivedHandle))
	require.NoError(t, overflowTxn.DefineSection("owned", StateSuccess, 1))
	overflowHandle, err := overflowTxn.AddOwned(
		"owned",
		MemberType{Kind: KindCaptureGap, Schema: SchemaV1},
		CaptureGapV1{CapabilityClass: string(CaptureClassOther), FirstAttempt: 4, LastAttempt: 4, Count: 1, Reason: "test"},
		128,
	)
	require.NoError(t, err)
	require.NoError(t, overflowTxn.Commit(StateSuccess))
	recorder.Close()

	results := sink.Results()
	require.Len(t, results, 4)
	result := results[3]
	require.ErrorContains(t, result.Err, "capture member limit exceeded")
	require.LessOrEqual(t, len(result.Manifest.Members), 4)
	require.NoError(t, result.Manifest.Validate())
	_, handleErr, state := overflowHandle.Resolve()
	require.Equal(t, HandleFailed, state)
	require.ErrorContains(t, handleErr, "capture member limit exceeded")
}

type referencedTestLeaf struct {
	ID    string     `json:"id"`
	Child ContentRef `json:"child"`
}

func (v referencedTestLeaf) Validate() error {
	if v.ID == "" {
		return errors.New("id is required")
	}
	return v.Child.Validate()
}

func TestRecorder_AbortAndLateSealFailureAreExplicit(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value     any
		terminate func(*CaptureTransaction) error
		wantErr   string
	}{
		"abort": {
			value: testLeaf{ID: "leaf-a"},
			terminate: func(txn *CaptureTransaction) error {
				return txn.Abort(errors.New("collection cancelled"))
			},
			wantErr: "capture aborted",
		},
		"late seal failure": {
			value: func() {},
			terminate: func(txn *CaptureTransaction) error {
				return txn.Commit(StateSuccess)
			},
			wantErr: "unsupported type",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sink := &MemorySink{}
			recorder := newTestRecorder(t, sink, 1)
			txn, err := recorder.Begin(testSemanticCapability)
			require.NoError(t, err)
			require.NoError(t, txn.DefineSection("semantic_inputs", StateSuccess, 1))
			handle, err := txn.AddOwned(
				"semantic_inputs",
				MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
				tc.value,
				64,
			)
			require.NoError(t, err)
			require.NoError(t, tc.terminate(txn))
			recorder.Close()

			_, handleErr, state := handle.Resolve()
			assert.Equal(t, HandleFailed, state)
			require.Error(t, handleErr)
			result := sink.Results()[0]
			require.ErrorContains(t, result.Err, tc.wantErr)
			require.NoError(t, result.Manifest.Validate())
			assert.Equal(t, StateIncomplete, result.Manifest.Roots[0].State)
		})
	}
}

type blockingCaptureSink struct {
	entered chan CaptureResult
	release chan struct{}
}

func (s *blockingCaptureSink) Store(result CaptureResult) {
	s.entered <- result
	<-s.release
}

func TestRecorder_BeginReservesNonBlockingTerminalCapacityAndCoalescesGaps(t *testing.T) {
	t.Parallel()

	sink := &blockingCaptureSink{
		entered: make(chan CaptureResult),
		release: make(chan struct{}),
	}
	recorder := newTestRecorder(t, sink, 2)
	txnA, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	txnB, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	_, err = recorder.Begin(testSemanticCapability)
	require.ErrorIs(t, err, ErrRecorderSaturated)

	require.NoError(t, txnA.DefineSection("capture", StateEmpty, 0))
	require.NoError(t, txnB.DefineSection("capture", StateEmpty, 0))
	assertReturnsPromptly(t, func() error { return txnA.Commit(StateEmpty) })
	assertReturnsPromptly(t, func() error { return txnB.Abort(errors.New("cancelled")) })

	closeDone := make(chan struct{})
	go func() {
		recorder.Close()
		close(closeDone)
	}()

	var results []CaptureResult
	for range 3 {
		select {
		case result := <-sink.entered:
			results = append(results, result)
			sink.release <- struct{}{}
		case <-time.After(2 * time.Second):
			t.Fatal("recorder did not deliver reserved jobs")
		}
	}
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("recorder did not close")
	}

	var gaps int
	for _, result := range results {
		if len(result.Manifest.Roots) == 1 && result.Manifest.Roots[0].Name == CapabilityCaptureAccounting {
			gaps++
			var root CapabilityRootV1
			rootRef := result.Manifest.Roots[0].Root
			require.NoError(t, DecodeCanonical(result.Members[rootRef.Key()], testReaderLimits(), &root))
			require.Equal(t, uint64(1), root.Sections[0].ExpectedRecords)
			registry := NewRegistry()
			require.NoError(t, registry.Register(CaptureAccountingCapabilityV1(), CaptureAccountingClosureV1()))
			report, err := registry.ValidateCapability(
				result.Manifest,
				result.Members,
				CaptureAccountingCapabilityV1(),
				testReaderLimits(),
			)
			require.NoError(t, err)
			assert.True(t, report.Completeness)
			assert.False(t, report.Replayable)
		}
	}
	assert.Equal(t, 1, gaps)
}

func TestRecorder_NoAnonymousFallbackRoot(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 1)
	txn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txn.Commit(StateSuccess))
	recorder.Close()

	result := sink.Results()[0]
	require.ErrorContains(t, result.Err, "no terminal sections")
	assert.Empty(t, result.Manifest.Roots)
}

func TestRecorder_WorkerPanicReleasesAdmissionAndFailsHandle(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 2)
	leafTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, leafTxn.DefineSection("leaf", StateSuccess, 1))
	leaf, err := leafTxn.AddOwned(
		"leaf",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		testLeaf{ID: "leaf-a"},
		32,
	)
	require.NoError(t, err)
	require.NoError(t, leafTxn.Commit(StateSuccess))

	derivedTxn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.DefineSection("derived", StateSuccess, 1))
	derived, err := derivedTxn.AddDerivedOwned(
		"derived",
		MemberType{Kind: "derived_leaf", Schema: SchemaV1},
		[]MemberHandle{leaf},
		func([]ContentRef) (any, error) { panic("builder panic") },
		32,
	)
	require.NoError(t, err)
	require.NoError(t, derivedTxn.Commit(StateSuccess))
	recorder.Close()

	_, handleErr, state := derived.Resolve()
	assert.Equal(t, HandleFailed, state)
	require.ErrorContains(t, handleErr, "worker panicked")
	assert.Len(t, sink.Results(), 2)
}

type panicCaptureSink struct{}

func (panicCaptureSink) Store(CaptureResult) { panic("sink panic") }

func TestRecorder_SinkPanicReleasesAdmissionAndFailsHandle(t *testing.T) {
	t.Parallel()

	recorder := newTestRecorder(t, panicCaptureSink{}, 1)
	txn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txn.DefineSection("leaf", StateSuccess, 1))
	handle, err := txn.AddOwned(
		"leaf",
		MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
		testLeaf{ID: "leaf-a"},
		32,
	)
	require.NoError(t, err)
	require.NoError(t, txn.Commit(StateSuccess))
	recorder.Close()

	_, handleErr, state := handle.Resolve()
	assert.Equal(t, HandleFailed, state)
	require.ErrorContains(t, handleErr, "sink panicked")
}

func TestCaptureTransaction_RejectsMemberWithoutTransferringHandle(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder, err := NewRecorder(RecorderConfig{
		QueueCapacity:    1,
		MaxMembers:       2,
		MaxRetainedBytes: 8,
		Sink:             sink,
	})
	require.NoError(t, err)
	txn, err := recorder.Begin(testSemanticCapability)
	require.NoError(t, err)
	require.NoError(t, txn.DefineSection("capture", StateSuccess, 1))

	handle, err := txn.AddOwned("capture", MemberType{Kind: "semantic_leaf", Schema: SchemaV1}, testLeaf{ID: "leaf-a"}, 9)
	require.ErrorContains(t, err, "retained-byte limit")
	assert.Zero(t, handle.ID())
	require.NoError(t, txn.Commit(StateSuccess))
	recorder.Close()

	result := sink.Results()[0]
	require.ErrorContains(t, result.Err, "retained-byte limit")
	assert.Equal(t, StateIncomplete, result.Manifest.Roots[0].State)
}

func TestRecorder_ConcurrentTransactionsResolveExactlyOnce(t *testing.T) {
	t.Parallel()

	sink := &MemorySink{}
	recorder := newTestRecorder(t, sink, 32)
	const transactions = 32
	handles := make([]MemberHandle, transactions)
	var wg sync.WaitGroup
	for i := range transactions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			txn, err := recorder.Begin(testSemanticCapability)
			require.NoError(t, err)
			require.NoError(t, txn.DefineSection("semantic_inputs", StateSuccess, 1))
			handles[i], err = txn.AddOwned(
				"semantic_inputs",
				MemberType{Kind: "semantic_leaf", Schema: SchemaV1},
				testLeaf{ID: "leaf-a"},
				32,
			)
			require.NoError(t, err)
			require.NoError(t, txn.Commit(StateSuccess))
		}()
	}
	wg.Wait()
	recorder.Close()

	require.Len(t, sink.Results(), transactions)
	for _, handle := range handles {
		_, err, state := handle.Resolve()
		require.NoError(t, err)
		assert.Equal(t, HandleSealed, state)
	}
}

func assertReturnsPromptly(t *testing.T, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("terminal operation blocked")
	}
}

func mustCanonicalBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := CanonicalBytes(value)
	require.NoError(t, err)
	return data
}
