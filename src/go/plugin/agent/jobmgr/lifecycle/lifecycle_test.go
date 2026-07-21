// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAdmissionAndUIDExactRelease(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	ordinary := ledger.RequestOrdinary(1, lane, OrdinaryBudgetBytes)
	require.Nil(t, ordinary.Rejected)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(len(grants), &grants)
	require.NoError(t, err)
	require.False(t, count != 1 || grants[0].Ref != ordinary.Ref)

	require.Error(t, ledger.CloseDrained(1))

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(ordinary.Ref)
	require.NoError(t, releaseOrdinaryErr)

	require.NoError(t, ledger.BeginCleanupOnly(1))

	require.NoError(t, ledger.CloseDrained(1))

	now := time.Unix(1_000, 0)
	uids := NewUIDLedger()

	require.NoError(t, uids.Admit("a", now))

	require.Error(t, uids.Admit("a", now))

	require.NoError(t, uids.Complete("a", true, now))

	require.Error(t, uids.Admit("a", now.Add(UIDTombstoneLifetime-time.Nanosecond)))

	require.NoError(t, uids.Admit("a", now.Add(UIDTombstoneLifetime)))

	require.NoError(t, uids.Complete("a", false, now.Add(UIDTombstoneLifetime)))

	more, err := uids.CloseBatch(UIDReturnBatch)
	require.False(t, err != nil || more)

	active, tombstones, closed := uids.Census()
	require.False(t, active != 0 || tombstones != 0 || !closed)
}

func TestAdmissionProcessBytesReduceOrdinaryCapacityUntilFinalClose(t *testing.T) {
	ledger := NewAdmissionLedger()
	const processBytes int64 = 4096

	require.NoError(t, ledger.ReserveProcessBytes(processBytes))

	require.NotNil(t, ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, OrdinaryBudgetBytes-processBytes+1).Rejected)

	result := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, OrdinaryBudgetBytes-processBytes)
	require.Nil(t, result.Rejected)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != result.Ref)

	require.Error(t, ledger.ReleaseProcessBytes(processBytes))

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(result.Ref)
	require.NoError(t, releaseOrdinaryErr)

	require.NoError(t, ledger.BeginCleanupOnly(1))

	require.NoError(t, ledger.CloseDrained(1))

	require.NoError(t, ledger.ReleaseProcessBytes(processBytes))

	census := ledger.Census()
	require.False(t, census.ProcessBytes != 0 || census.OrdinaryBytes != 0 || census.Phase != AdmissionClosed)
}

func TestAdmissionProcessBytesCanUnwindPristineConstruction(t *testing.T) {
	ledger := NewAdmissionLedger()
	const processBytes = int64(4096)

	require.NoError(t, ledger.ReserveProcessBytes(processBytes))

	require.NoError(t, ledger.ReleaseProcessBytes(processBytes))

	census := ledger.Census()
	require.False(t, census.ProcessBytes != 0 || census.ActiveRecords != 0 || census.Phase != AdmissionOrdinaryOpen || census.RunGeneration != 0)

	require.NoError(t, ledger.ReserveProcessBytes(processBytes))

	require.NoError(t, ledger.ReleaseProcessBytes(processBytes))
}

func TestAdmissionLongLivedTransferDetachesOperationRecords(t *testing.T) {
	ledger := NewAdmissionLedger()
	longLived := make([]AdmissionRef, 0, formerFixedPopulation)
	for index := range formerFixedPopulation {
		lane := AdmissionLaneRef{Slot: uint32(index + 1), Generation: 1}
		request := ledger.RequestOrdinary(1, lane, 2)
		require.Nil(t, request.Rejected)
		var grants [4]AdmissionGrant
		count, _, err := ledger.TakeGrants(1, &grants)
		require.False(t, err != nil || count != 1 || grants[0].Ref != request.Ref)

		require.NoError(t, ledger.transferLongLived(request.Ref, 1))

		_, releaseOrdinaryErr2 := ledger.ReleaseOrdinary(request.Ref)
		require.NoError(t, releaseOrdinaryErr2)

		longLived = append(longLived, request.Ref)
	}

	census := ledger.Census()
	require.False(t, census.ActiveRecords != 0 ||
		census.FreeRecords == 0 ||
		census.LongLivedRecords != formerFixedPopulation)

	last := ledger.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 1, Generation: 2},
		2,
	)
	require.Nil(t, last.Rejected)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != last.Ref)

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(last.Ref)
	require.NoError(t, releaseOrdinaryErr)

	for _, ref := range longLived {

		_, releaseLongLivedErr := ledger.releaseLongLived(ref, 1)
		require.NoError(t, releaseLongLivedErr)

	}

	require.NoError(t, ledger.BeginCleanupOnly(1))

	require.NoError(t, ledger.CloseDrained(1))
}

func TestAdmissionReopensOnlyFromAnExactlyDrainedRun(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	request := ledger.RequestOrdinary(1, lane, 1)
	require.Nil(t, request.Rejected)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != request.Ref)

	require.NoError(t, ledger.BeginCleanupOnly(1))

	require.Error(t, ledger.ReopenDrained(1, 2))

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(request.Ref)
	require.NoError(t, releaseOrdinaryErr)

	require.Error(t, ledger.ReopenDrained(1, 1))

	require.NoError(t, ledger.ReopenDrained(1, 2))

	result := ledger.RequestOrdinary(1, lane, 1)
	require.NotNil(t, result.Rejected)

	second := ledger.RequestOrdinary(2, AdmissionLaneRef{Slot: 1, Generation: 2}, 1)
	require.Nil(t, second.Rejected)

	require.NoError(t, ledger.CancelWaiting(second.Ref))

	require.NoError(t, ledger.BeginCleanupOnly(2))

	require.NoError(t, ledger.CloseDrained(2))
}

func TestAdmissionCarriesOneStableInputBodyAcrossRunRotation(t *testing.T) {
	ledger := NewAdmissionLedger()
	const capacity = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, capacity)
	require.NoError(t, err)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].InputBodyToken != token)

	_, commitInputBodyGrowthErr := ledger.CommitInputBodyGrowth(token, capacity)
	require.NoError(t, commitInputBodyGrowthErr)

	require.False(t, ledger.RunDrained(1))

	require.NoError(t, ledger.SuspendInputBody(1, 2, token))

	require.NoError(t, ledger.BeginCleanupOnly(1))

	require.True(t, ledger.RunDrained(1))

	require.Error(t, ledger.CloseDrained(1))

	require.NoError(t, ledger.ReopenDrained(1, 2))

	census := ledger.Census()
	require.False(t, census.RunGeneration != 2 || !census.InputBodyCarried || census.InputBodyBytes != capacity)

	require.NoError(t, ledger.AdoptInputBody(2, token))

	require.False(t, ledger.Census().InputBodyCarried)

	_, abortInputBodyErr := ledger.AbortInputBody(token)
	require.NoError(t, abortInputBodyErr)

	require.NoError(t, ledger.BeginCleanupOnly(2))

	require.NoError(t, ledger.CloseDrained(2))
}

func TestAdmissionShutdownSettlesOnlyPendingInputBodyGrowth(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	ordinary := ledger.RequestOrdinary(1, lane, 1)
	require.Nil(t, ordinary.Rejected)
	const capacity = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, capacity)
	require.NoError(t, err)
	grant, waiting, err := ledger.TakeShutdownInputBodyGrant(1)
	require.False(t, err != nil || waiting)
	require.False(t, grant.Kind != ReservationInputBodyGrowth || grant.InputBodyToken != token || grant.Bytes != capacity)

	census := ledger.Census()
	require.False(t, census.OrdinaryWaiting != 1 || !census.InputBodyActive || census.InputBodyWaiting)

	require.Error(t, ledger.BeginCleanupOnly(1))

	require.NoError(t, ledger.CancelWaiting(ordinary.Ref))

	require.NoError(t, ledger.BeginCleanupOnly(1))

	takeShutdownInputBodyGrantRepeated, takeShutdownInputBodyGrantStillWaiting, takeShutdownInputBodyGrantErr := ledger.TakeShutdownInputBodyGrant(1)
	require.False(t, takeShutdownInputBodyGrantErr != nil || takeShutdownInputBodyGrantStillWaiting || takeShutdownInputBodyGrantRepeated.Kind != 0)

	_, commitInputBodyGrowthErr := ledger.CommitInputBodyGrowth(token, capacity)
	require.NoError(t, commitInputBodyGrowthErr)

	_, abortInputBodyErr := ledger.AbortInputBody(token)
	require.NoError(t, abortInputBodyErr)

	require.NoError(t, ledger.CloseDrained(1))
}

func TestUIDLedgerAdmissionAndCloseWorkAreBatched(t *testing.T) {
	now := time.Unix(2_000, 0)
	uids := NewUIDLedger()
	for index := range UIDReturnBatch + 1 {
		uid := fmt.Sprintf("u-%d", index)

		require.NoError(t, uids.Admit(uid, now))

		require.NoError(t, uids.Complete(uid, true, now))
	}

	require.NoError(t, uids.Admit("u-64", now.Add(UIDTombstoneLifetime)))

	active, tombstones, _ := uids.Census()
	require.False(t, active != 1 || tombstones != 1)

	require.NoError(t, uids.Complete("u-64", true, now.Add(UIDTombstoneLifetime)))

	more, err := uids.CloseBatch(1)
	require.False(t, err != nil || !more)
	more, err = uids.CloseBatch(UIDReturnBatch)
	require.False(t, err != nil || more)

	_, _, closed := uids.Census()
	require.True(t, closed)

	require.Error(t, uids.Admit("after-close", now))

	_, closeBatchErr := uids.CloseBatch(UIDReturnBatch)
	require.NoError(t, closeBatchErr)

}

func TestUIDLedgerGrowsAndCloseWorkRemainsBatched(t *testing.T) {
	now := time.Unix(3_000, 0)
	uids := NewUIDLedger()
	const population = formerFixedUIDPopulation + 1
	for index := range population {
		require.NoError(t, uids.Admit(fmt.Sprintf("active-%d", index), now))
	}
	for index := range population {
		require.NoError(t, uids.Complete(fmt.Sprintf("active-%d", index), true, now))
	}
	wantBatches := (population + UIDReturnBatch - 1) / UIDReturnBatch
	for batch := 1; batch <= wantBatches; batch++ {
		more, err := uids.CloseBatch(UIDReturnBatch)
		require.NoError(t, err)
		require.EqualValues(t, batch < wantBatches, more)
	}

	active, tombstones, closed := uids.Census()
	require.False(t, active != 0 || tombstones != 0 || !closed)
}

func TestAdmissionRadixDomainAndOldestFitting(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	for _, size := range []int64{0, OrdinaryBudgetBytes + 1, OrdinaryBudgetBytes + 2} {
		result := ledger.RequestOrdinary(1, lane, size)
		require.False(t, result.Rejected == nil || result.Ref.Valid())
	}
	for _, size := range []int64{1, 134_217_727, 134_217_728, OrdinaryBudgetBytes} {
		result := ledger.RequestOrdinary(1, lane, size)
		require.Nil(t, result.Rejected)

		require.NoError(t, ledger.CancelWaiting(result.Ref))
	}

	blocker := ledger.RequestOrdinary(1, lane, OrdinaryBudgetBytes-10)
	oldLarge := ledger.RequestOrdinary(1, lane, 11)
	firstSmall := ledger.RequestOrdinary(1, lane, 5)
	laterSmall := ledger.RequestOrdinary(1, lane, 4)
	for _, result := range []AdmissionRequestResult{blocker, oldLarge, firstSmall, laterSmall} {
		require.Nil(t, result.Rejected)
	}
	var grants [4]AdmissionGrant
	count, more, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || !more || grants[0].Ref != blocker.Ref)
	count, _, err = ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != firstSmall.Ref)

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(blocker.Ref)
	require.NoError(t, releaseOrdinaryErr)

	count, _, err = ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != oldLarge.Ref)
	count, _, err = ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != laterSmall.Ref)
	for _, ref := range []AdmissionRef{firstSmall.Ref, oldLarge.Ref, laterSmall.Ref} {

		_, releaseOrdinaryErr2 := ledger.ReleaseOrdinary(ref)
		require.NoError(t, releaseOrdinaryErr2)

	}
}

func TestAdmissionMoreReportsOnlyImmediatelyGrantableWork(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	full := ledger.RequestOrdinary(1, lane, OrdinaryBudgetBytes)
	waiting := ledger.RequestOrdinary(1, lane, 1)
	require.False(t, full.Rejected != nil || waiting.Rejected != nil)
	var grants [4]AdmissionGrant
	count, more, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != full.Ref || more)
	wake, err := ledger.ReleaseOrdinary(full.Ref)
	require.False(t, err != nil || !wake)
	count, more, err = ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != waiting.Ref || more)

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(waiting.Ref)
	require.NoError(t, releaseOrdinaryErr)

}

func TestAdmissionOrdinaryGrowthRetainsOneRecordAndWaitsByDelta(t *testing.T) {
	ledger := NewAdmissionLedger()
	first := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, 100)
	second := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 2, Generation: 1}, OrdinaryBudgetBytes-100)
	require.False(t, first.Rejected != nil || second.Rejected != nil)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(2, &grants)
	require.False(t, err != nil || count != 2)
	ready, _, err := ledger.ResizeOrdinary(first.Ref, 101)
	require.False(t, err != nil || ready)

	census := ledger.Census()
	require.False(t, census.ActiveRecords != 2 || census.OrdinaryGranted != 2 || census.OrdinaryWaiting != 1 || census.OrdinaryBytes != OrdinaryBudgetBytes)

	ready, wake, err := ledger.ResizeOrdinary(second.Ref, OrdinaryBudgetBytes-101)
	require.False(t, err != nil || !ready || !wake)
	count, _, err = ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != first.Ref || grants[0].Kind != ReservationOrdinaryGrowth || grants[0].Bytes != 101)

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(first.Ref)
	require.NoError(t, releaseOrdinaryErr)

	_, releaseOrdinaryErr2 := ledger.ReleaseOrdinary(second.Ref)
	require.NoError(t, releaseOrdinaryErr2)

}

func TestAdmissionInputBodyGrowthTransfersIntoOperationRecord(t *testing.T) {
	ledger := NewAdmissionLedger()
	const initial = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, initial)
	require.NoError(t, err)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(4, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Kind != ReservationInputBodyGrowth || grants[0].InputBodyToken != token || grants[0].Bytes != initial)

	_, commitInputBodyGrowthErr := ledger.CommitInputBodyGrowth(token, initial)
	require.NoError(t, commitInputBodyGrowthErr)

	const grown = int64(128 * 1024)

	requestInputBodyGrowthNext, requestInputBodyGrowthErr := ledger.RequestInputBodyGrowth(1, token, grown)
	require.False(t, requestInputBodyGrowthErr != nil || requestInputBodyGrowthNext != token)

	count, _, err = ledger.TakeGrants(4, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Kind != ReservationInputBodyGrowth || grants[0].Bytes != initial+grown)

	census := ledger.Census()
	require.False(t, census.OrdinaryBytes != initial+grown || census.InputBodyBytes != initial+grown)

	_, commitInputBodyGrowthErr2 := ledger.CommitInputBodyGrowth(token, grown)
	require.NoError(t, commitInputBodyGrowthErr2)

	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	transferred := ledger.TransferInputBody(1, token, lane, grown+512, grown)
	require.False(t, transferred.Rejected != nil || !transferred.Ref.Valid())

	ledgerCensus := ledger.Census()
	require.False(t, ledgerCensus.InputBodyActive || ledgerCensus.InputBodyBytes != 0 || ledgerCensus.ActiveRecords != 1 || ledgerCensus.OrdinaryBytes != grown)

	count, _, err = ledger.TakeGrants(4, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Kind != ReservationOrdinary || grants[0].Ref != transferred.Ref || grants[0].Bytes != grown+512)

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(transferred.Ref)
	require.NoError(t, releaseOrdinaryErr)

	ledgerCensus2 := ledger.Census()
	require.False(t, ledgerCensus2.ActiveRecords != 0 || ledgerCensus2.OrdinaryBytes != 0 || ledgerCensus2.InputBodyActive)

}

func TestAdmissionCancelTransferredInputBodyWaitingReleasesHeldBytes(t *testing.T) {
	ledger := NewAdmissionLedger()
	const capacity = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, capacity)
	require.NoError(t, err)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1)

	_, commitInputBodyGrowthErr := ledger.CommitInputBodyGrowth(token, capacity)
	require.NoError(t, commitInputBodyGrowthErr)

	transferred := ledger.TransferInputBody(
		1, token, AdmissionLaneRef{Slot: 1, Generation: 1}, capacity+512, capacity,
	)
	require.Nil(t, transferred.Rejected)

	require.NoError(t, ledger.CancelWaiting(transferred.Ref))

	census := ledger.Census()
	require.False(t, census.ActiveRecords != 0 || census.OrdinaryBytes != 0 || census.InputBodyActive)
}

func TestAdmissionLongLivedRecordDetachesFromCompletedLaneGeneration(t *testing.T) {
	ledger := NewAdmissionLedger()
	first := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, 100)
	require.Nil(t, first.Rejected)
	var grants [4]AdmissionGrant

	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != first.Ref)

	require.NoError(t, ledger.transferLongLived(first.Ref, 40))

	_, releaseOrdinaryErr := ledger.ReleaseOrdinary(first.Ref)
	require.NoError(t, releaseOrdinaryErr)

	second := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 2}, 60)
	require.Nil(t, second.Rejected)

	takeGrantsCount, _, takeGrantsErr := ledger.TakeGrants(1, &grants)
	require.False(t, takeGrantsErr != nil || takeGrantsCount != 1 || grants[0].Ref != second.Ref)

	_, releaseOrdinaryErr2 := ledger.ReleaseOrdinary(second.Ref)
	require.NoError(t, releaseOrdinaryErr2)

	_, releaseLongLivedErr := ledger.releaseLongLived(first.Ref, 40)
	require.NoError(t, releaseLongLivedErr)

	census := ledger.Census()
	require.False(t, census.ActiveRecords != 0 || census.OrdinaryGranted != 0 || census.OrdinaryBytes != 0 || census.LongLivedRecords != 0 || census.LongLivedBytes != 0)
}

func TestAdmissionResizesOrdinaryRemainderWithLongLivedTransfer(t *testing.T) {
	tests := map[string]struct {
		resized int64
	}{
		"grow response bytes":   {resized: 120},
		"shrink response bytes": {resized: 80},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ledger := NewAdmissionLedger()
			request := ledger.RequestOrdinary(
				1,
				AdmissionLaneRef{Slot: 1, Generation: 1},
				100,
			)
			require.Nil(t, request.Rejected)
			var grants [4]AdmissionGrant

			takeGrantsCount, _, takeGrantsErr := ledger.TakeGrants(1, &grants)
			require.False(t, takeGrantsErr != nil || takeGrantsCount != 1)

			require.NoError(t, ledger.transferLongLived(request.Ref, 40))

			ready, _, err := ledger.ResizeOrdinary(request.Ref, test.resized)
			require.False(t, err != nil || !ready)

			census := ledger.Census()
			require.False(t, census.OrdinaryBytes != test.resized || census.LongLivedBytes != 40 || census.LongLivedRecords != 1)

			_, releaseOrdinaryErr := ledger.ReleaseOrdinary(request.Ref)
			require.NoError(t, releaseOrdinaryErr)

			_, releaseLongLivedErr := ledger.releaseLongLived(request.Ref, 40)
			require.NoError(t, releaseLongLivedErr)

			ledgerCensus := ledger.Census()
			require.False(t, ledgerCensus.ActiveRecords != 0 ||
				ledgerCensus.OrdinaryBytes != 0 ||
				ledgerCensus.LongLivedBytes != 0)

		})
	}
}

func TestAdmissionInputBodyAbortReleasesWaitingAndGrantedCapacity(t *testing.T) {
	for _, afterGrant := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-grant-%t", afterGrant), func(t *testing.T) {
			ledger := NewAdmissionLedger()
			token, err := ledger.RequestInputBodyGrowth(1, 0, 64*1024)
			require.NoError(t, err)
			if afterGrant {
				var grants [4]AdmissionGrant

				takeGrantsCount, _, takeGrantsErr := ledger.TakeGrants(4, &grants)
				require.False(t, takeGrantsErr != nil || takeGrantsCount != 1)

			}

			_, abortInputBodyErr := ledger.AbortInputBody(token)
			require.NoError(t, abortInputBodyErr)

			census := ledger.Census()
			require.False(t, census.InputBodyActive || census.OrdinaryWaiting != 0 || census.OrdinaryBytes != 0)
		})
	}
}

func TestTaskSupervisorDynamicPopulationAndGenerationCheckedReuse(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	release := make(chan struct{})
	const population = TaskStartServiceQuantum*2 + 1
	for range population {
		_, err := supervisor.Enqueue(TaskClassGenericFunction, TaskPlan{
			Source: SourceFunction,
			Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
				<-release
				return NewSealedResult(200, "application/json", []byte(`{}`))
			}),
		})
		require.NoError(t, err)
	}
	refs := make([]TaskRef, 0, population)
	for {
		var started [TaskStartServiceQuantum]TaskStart
		count, more, err := supervisor.Dispatch(context.Background(), TaskStartServiceQuantum, &started)
		require.NoError(t, err)
		require.False(t, count > TaskStartServiceQuantum)
		for _, start := range started[:count] {
			refs = append(refs, start.Task)
		}
		if !more {
			break
		}
	}
	require.False(t, len(refs) != population || supervisor.Active() != population)
	close(release)
	for range refs {
		completion := <-supervisor.CompletionCh()
		require.EqualValues(t, 1, completion.Sequence)

		require.NoError(t, supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}))

		ack := <-supervisor.AcknowledgementCh()
		require.False(t, ack.Sequence != 2 || ack.Err != nil)

		require.NoError(t, supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}))

		ack = <-supervisor.AcknowledgementCh()
		require.False(t, ack.Sequence != 3 || ack.Kind != TaskActionTerminate || ack.Err != nil)

		require.NoError(t, supervisor.Release(ack.Ref))
	}
	previous := make(map[uint32]uint64, len(refs))
	for _, ref := range refs {
		previous[ref.Slot] = ref.Generation
	}
	pending, err := supervisor.Enqueue(
		TaskClassGenericFunction,
		TaskPlan{Source: SourceFunction, Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		})},
	)
	require.NoError(t, err)
	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
	require.False(t, err != nil || count != 1 || more || started[0].Request != pending)
	ref := started[0].Task

	generation, ok := previous[ref.Slot]
	require.False(t, !ok || ref.Generation != generation+1)

	stale := TaskRef{
		Slot:       ref.Slot,
		Generation: previous[ref.Slot],
	}

	require.Error(t, supervisor.Cancel(stale))

	require.Error(t, supervisor.SendAction(TaskAction{
		Ref: stale, Sequence: 2, Kind: TaskActionDispose,
	}),
	)

	require.Error(t, supervisor.Release(stale))

	completion := <-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}))

	ack := <-supervisor.AcknowledgementCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}))

	ack = <-supervisor.AcknowledgementCh()

	require.NoError(t, supervisor.Release(ack.Ref))
}

func TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	release := make(chan struct{})
	refs := make([]TaskRef, 0, RetainedTimeoutFailStopThreshold+1)
	for range RetainedTimeoutFailStopThreshold + 1 {
		_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
			Source: SourceFunction,
			Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
				<-release
				return NewSealedResult(200, "application/json", []byte(`{}`))
			}),
		})
		refs = append(refs, ref)
	}
	for index, ref := range refs {
		saturated, err := supervisor.MarkRetainedTimeout(ref)
		require.NoError(t, err)
		wantSaturated := index == RetainedTimeoutFailStopThreshold-1
		require.EqualValues(t, wantSaturated, saturated)
		count, latched := supervisor.RetainedTimeouts()
		wantLatched := index >= RetainedTimeoutFailStopThreshold-1
		require.False(t, count != index+1 || latched != wantLatched)
	}
	for index, ref := range refs {
		cleared, err := supervisor.ClearRetainedTimeout(ref)
		require.False(t, err != nil || !cleared)
		count, latched := supervisor.RetainedTimeouts()
		require.False(t, count != len(refs)-index-1 || !latched)
	}
	close(release)
	for range refs {
		completion := <-supervisor.CompletionCh()

		require.NoError(t, supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}))
	}
	acknowledged := make([]TaskRef, 0, len(refs))
	for range refs {
		ack := <-supervisor.AcknowledgementCh()
		require.False(t, ack.Sequence != 2 || ack.Kind != TaskActionDispose || ack.Err != nil)
		acknowledged = append(acknowledged, ack.Ref)
	}
	for _, ref := range acknowledged {
		require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))
	}
	for range refs {
		ack := <-supervisor.AcknowledgementCh()

		require.NoError(t, supervisor.Release(ack.Ref))
	}
	require.EqualValues(t, 0, supervisor.Active())
}

func TestTaskSupervisorContainsPanicAndReleasesSlot(t *testing.T) {
	tests := map[string]TaskPlan{
		"function work": {
			Source: SourceFunction,
			Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
				panic("fixture panic")
			}),
		},
		"reusable runner": {
			Source: SourceFunction,
			Runner: taskRunnerFunc(func(context.Context) (TaskOutcome, error) {
				panic("fixture panic")
			}),
		},
	}
	for name, plan := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := NewFrameOwner(io.Discard)
			require.NoError(t, err)
			supervisor, err := NewTaskSupervisor(frame)
			require.NoError(t, err)
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)
			completion := <-supervisor.CompletionCh()
			require.False(t, completion.Ref != ref || !errors.Is(completion.Err, ErrTaskPanic) ||
				!strings.Contains(completion.Err.Error(), "fixture panic"))

			require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

			ack := <-supervisor.AcknowledgementCh()
			require.False(t, ack.Ref != ref || ack.Sequence != 2 || ack.Kind != TaskActionDispose || ack.Err != nil)

			require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

			ack = <-supervisor.AcknowledgementCh()
			require.False(t, ack.Ref != ref || ack.Sequence != 3 || ack.Kind != TaskActionTerminate || ack.Err != nil)

			require.NoError(t, supervisor.Release(ref))

			require.EqualValues(t, 0, supervisor.Active())
		})
	}
}

type taskRunnerFunc func(context.Context) (TaskOutcome, error)

func (fn taskRunnerFunc) RunTask(ctx context.Context) (TaskOutcome, error) {
	return fn(ctx)
}

func TestTaskSupervisorPreservesAuthoritativeCancellationCause(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	deadline := time.Unix(100, 0)
	type cancellationObservation struct {
		deadline time.Time
		ok       bool
		err      error
		cause    error
	}
	observed := make(chan cancellationObservation, 1)
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction, Deadline: deadline,
		Work: func(ctx context.Context) (TaskOutcome, error) {
			observedDeadline, ok := ctx.Deadline()
			<-ctx.Done()
			cause := context.Cause(ctx)
			observed <- cancellationObservation{deadline: observedDeadline, ok: ok, err: ctx.Err(), cause: cause}
			return TaskOutcome{}, cause
		},
	})

	require.NoError(t, supervisor.CancelWithCause(
		ref,
		fmt.Errorf("wrapped deadline: %w", context.DeadlineExceeded),
	))

	got := <-observed
	require.False(t, !got.ok || !got.deadline.Equal(deadline) || !errors.Is(got.err, context.DeadlineExceeded) || !errors.Is(got.cause, context.DeadlineExceeded))
	require.Equal(t, context.DeadlineExceeded, got.cause)

	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != ref || !errors.Is(completion.Err, context.DeadlineExceeded))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	ack := <-supervisor.AcknowledgementCh()
	require.Nil(t, ack.Err)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	ack = <-supervisor.AcknowledgementCh()
	require.Nil(t, ack.Err)

	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorDeliversCanonicalPendingCancellation(t *testing.T) {
	deadline := time.Unix(4102444800, 0)
	tests := map[string]struct {
		cause           error
		want            error
		deadline        time.Time
		setAfterEnqueue bool
	}{
		"initial wrapped cancellation": {
			cause: fmt.Errorf("wrapped cancellation: %w", context.Canceled),
			want:  context.Canceled,
		},
		"initial wrapped deadline": {
			cause:    fmt.Errorf("wrapped deadline: %w", context.DeadlineExceeded),
			want:     context.DeadlineExceeded,
			deadline: deadline,
		},
		"pending wrapped cancellation": {
			cause:           fmt.Errorf("wrapped cancellation: %w", context.Canceled),
			want:            context.Canceled,
			setAfterEnqueue: true,
		},
		"pending wrapped deadline": {
			cause:           fmt.Errorf("wrapped deadline: %w", context.DeadlineExceeded),
			want:            context.DeadlineExceeded,
			deadline:        deadline,
			setAfterEnqueue: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := NewFrameOwner(io.Discard)
			require.NoError(t, err)
			supervisor, err := NewTaskSupervisor(frame)
			require.NoError(t, err)
			observed := make(chan error, 1)
			plan := TaskPlan{
				Source:   SourceFunction,
				Deadline: test.deadline,
				Work: func(ctx context.Context) (TaskOutcome, error) {
					<-ctx.Done()
					cause := context.Cause(ctx)
					observed <- cause
					return TaskOutcome{}, cause
				},
			}
			if !test.setAfterEnqueue {
				plan.InitialCancellation = test.cause
			}
			ref, err := supervisor.Enqueue(TaskClassGenericFunction, plan)
			require.NoError(t, err)
			if test.setAfterEnqueue {
				require.NoError(t, supervisor.SetPendingCancellation(ref, test.cause))
			}
			var started [TaskStartServiceQuantum]TaskStart
			count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
			require.NoError(t, err)
			require.Equal(t, 1, count)
			require.False(t, more)
			require.Equal(t, ref, started[0].Request)

			require.Equal(t, test.want, <-observed)
			completion := <-supervisor.CompletionCh()
			require.Equal(t, started[0].Task, completion.Ref)
			require.ErrorIs(t, completion.Err, test.want)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose,
			}))
			ack := <-supervisor.AcknowledgementCh()
			require.NoError(t, ack.Err)

			require.NoError(t, supervisor.SendAction(TaskAction{
				Ref: completion.Ref, Sequence: 3, Kind: TaskActionTerminate,
			}))
			ack = <-supervisor.AcknowledgementCh()
			require.NoError(t, ack.Err)
			require.NoError(t, supervisor.Release(completion.Ref))
		})
	}
}

func TestTaskSupervisorRejectsAmbiguousPendingCancellation(t *testing.T) {
	tests := map[string]error{
		"joined cancellation causes": errors.Join(
			context.Canceled,
			context.DeadlineExceeded,
		),
		"unrecognized cancellation cause": errors.New("test cancellation"),
	}
	for name, cause := range tests {
		t.Run(name, func(t *testing.T) {
			frame, err := NewFrameOwner(io.Discard)
			require.NoError(t, err)
			supervisor, err := NewTaskSupervisor(frame)
			require.NoError(t, err)
			plan := TaskPlan{
				Source:              SourceFunction,
				InitialCancellation: cause,
				Work: func(context.Context) (TaskOutcome, error) {
					return NoValueOutcome(), nil
				},
			}
			_, err = supervisor.Enqueue(TaskClassGenericFunction, plan)
			require.Error(t, err)

			plan.InitialCancellation = nil
			ref, err := supervisor.Enqueue(TaskClassGenericFunction, plan)
			require.NoError(t, err)
			require.Error(t, supervisor.SetPendingCancellation(ref, cause))
			require.NoError(t, supervisor.CancelPending(ref))
		})
	}
}

func TestTaskSupervisorChecksPhaseSequenceAndPublishesOwnedResult(t *testing.T) {
	var output bytes.Buffer
	frame, err := NewFrameOwner(&output)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	payload := []byte(`{"value":"original"}`)
	sealed, err := NewSealedResult(200, "application/json", payload)
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction, MaxPhaseTransitions: 4,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return sealed, nil
		}),
	})
	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != ref || completion.Sequence != 1 || completion.Err != nil)
	payload[10] = 'X'

	require.Error(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionEncodeWrite, UID: "u1", Expiry: 1}))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionEncodeWrite, UID: "u1", Expiry: 1}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Ref != ref || ack.Sequence != 2 || ack.Kind != TaskActionEncodeWrite || ack.Err != nil)
	require.False(t, !bytes.Contains(output.Bytes(), []byte(`{"value":"original"}`)) || bytes.Contains(output.Bytes(), []byte(`{"value":"Xriginal"}`)))

	require.Error(t, supervisor.Release(ref))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	ack = <-supervisor.AcknowledgementCh()
	require.False(t, ack.Ref != ref || ack.Sequence != 3 || ack.Kind != TaskActionTerminate || ack.Err != nil)

	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorPreflightsResultEnvelopeBeforeAction(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return result, nil
		}),
	})
	completion := <-supervisor.CompletionCh()
	require.Nil(t, completion.Err)
	_, baseEnvelope, err := functionFrameSize("u", 200, "application/json", 1, len(result.payload))
	require.NoError(t, err)
	oversizedUID := strings.Repeat("u", 2+FunctionEnvelopeBytes-baseEnvelope)

	_, preflightResultErr := supervisor.PreflightResult(ref, oversizedUID, 1)
	require.ErrorIs(t, preflightResultErr, ErrFunctionResultTooLarge)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorRunsCleanupBeforeExplicitTermination(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	cleaned := make(chan struct{}, 1)
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceJobManager, MaxPhaseTransitions: 4,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
		Cleanup: func() error {
			cleaned <- struct{}{}
			return nil
		},
	})
	completion := <-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Sequence != 2 || ack.Err != nil)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 3, Kind: TaskActionCleanup}))

	acknowledgementCh := <-supervisor.AcknowledgementCh()
	require.False(t, acknowledgementCh.Sequence != 3 || acknowledgementCh.Kind != TaskActionCleanup || acknowledgementCh.Err != nil)

	select {
	case <-cleaned:
	default:
		require.FailNow(t, "test failed", "cleanup phase did not execute")
	}

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))

	acknowledgementCh2 := <-supervisor.AcknowledgementCh()
	require.False(t, acknowledgementCh2.Sequence != 4 || acknowledgementCh2.Kind != TaskActionTerminate || acknowledgementCh2.Err != nil)

	require.NoError(t, supervisor.Release(ref))
}

func TestTaskSupervisorDispatchRotatesPendingClasses(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	release := make(chan struct{})
	plan := func(source Source) TaskPlan {
		return TaskPlan{Source: source, Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			<-release
			return NewSealedResult(200, "application/json", []byte(`{}`))
		})}
	}
	j1, _ := supervisor.Enqueue(TaskClassFrameworkControl, plan(SourceJobManager))
	j2, _ := supervisor.Enqueue(TaskClassFrameworkControl, plan(SourceJobManager))
	f1, _ := supervisor.Enqueue(TaskClassGenericFunction, plan(SourceFunction))
	f2, _ := supervisor.Enqueue(TaskClassGenericFunction, plan(SourceFunction))
	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), TaskStartServiceQuantum, &started)
	require.False(t, err != nil || count != TaskStartServiceQuantum || more)
	want := []TaskRequestRef{j1, f1, j2, f2}
	for index, ref := range want {
		require.EqualValues(t, ref, started[index].Request)
	}
	close(release)
	for range TaskStartServiceQuantum {
		completion := <-supervisor.CompletionCh()

		require.NoError(t, supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}))

		ack := <-supervisor.AcknowledgementCh()

		require.NoError(t, supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}))

		ack = <-supervisor.AcknowledgementCh()

		require.NoError(t, supervisor.Release(ack.Ref))
	}
}

func TestTaskSupervisorRejectsInvalidSchedulingClass(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	plan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
	}
	tests := map[string]TaskClass{
		"zero":    0,
		"unknown": 3,
	}
	for name, class := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := supervisor.Enqueue(class, plan)
			require.Error(t, err)

			require.EqualValues(t, 0, supervisor.Pending())
		})
	}
}

func TestTaskSupervisorFrameworkControlStartsWithManyActiveGenericTasks(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	release := make(chan struct{})
	blockingPlan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			<-release
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
	}
	const activeGeneric = TaskStartServiceQuantum * 2
	for range activeGeneric {
		request, err := supervisor.Enqueue(TaskClassGenericFunction, blockingPlan)
		require.NoError(t, err)
		var started [TaskStartServiceQuantum]TaskStart
		count, _, err := supervisor.Dispatch(context.Background(), 1, &started)
		require.False(t, err != nil || count != 1 || started[0].Request != request)
	}
	readyPlan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
	}
	generic, err := supervisor.Enqueue(TaskClassGenericFunction, readyPlan)
	require.NoError(t, err)
	control, err := supervisor.Enqueue(TaskClassFrameworkControl, readyPlan)
	require.NoError(t, err)
	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
	require.False(t, err != nil || count != 1 || !more)
	require.EqualValues(t, control, started[0].Request)
	require.EqualValues(t, activeGeneric+1, supervisor.Active())

	close(release)
	started = [TaskStartServiceQuantum]TaskStart{}
	count, more, err = supervisor.Dispatch(context.Background(), 1, &started)
	require.False(t, err != nil || count != 1 || more || started[0].Request != generic)
	for range activeGeneric + 2 {
		completion := <-supervisor.CompletionCh()

		require.NoError(t, supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose,
		}),
		)

		ack := <-supervisor.AcknowledgementCh()
		require.Nil(t, ack.Err)
		terminateAndReleaseTask(t, supervisor, ack.Ref, 3)
	}
	require.False(t, supervisor.Active() != 0 || supervisor.Pending() != 0)
}

func TestTaskSupervisorDirectlyCancelsPendingRequest(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	plan := func(source Source) TaskPlan {
		return TaskPlan{Source: source, Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		})}
	}
	cancelled, err := supervisor.Enqueue(TaskClassFrameworkControl, plan(SourceJobManager))
	require.NoError(t, err)
	survivor, err := supervisor.Enqueue(TaskClassGenericFunction, plan(SourceFunction))
	require.NoError(t, err)

	require.NoError(t, supervisor.CancelPending(cancelled))

	require.Error(t, supervisor.CancelPending(cancelled))

	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
	require.False(t, err != nil || count != 1 || more || started[0].Request != survivor)
	completion := <-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}))

	ack := <-supervisor.AcknowledgementCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}))

	ack = <-supervisor.AcknowledgementCh()

	require.NoError(t, supervisor.Release(ack.Ref))
}

func enqueueAndDispatchTask(t *testing.T, supervisor *TaskSupervisor, plan TaskPlan) (TaskRequestRef, TaskRef) {
	t.Helper()
	class := TaskClassFrameworkControl
	if plan.Source == SourceFunction {
		class = TaskClassGenericFunction
	}
	request, err := supervisor.Enqueue(class, plan)
	require.NoError(t, err)
	var started [TaskStartServiceQuantum]TaskStart
	count, _, err := supervisor.Dispatch(context.Background(), 1, &started)
	require.False(t, err != nil || count != 1 || started[0].Request != request)
	return request, started[0].Task
}

func TestFrameOwnerControlReservationPrecedesLaterOrdinaryFrame(t *testing.T) {
	writer := newStepWriter()
	controlReady := make(chan struct{}, 1)
	owner, err := NewFrameOwner(writer)
	require.NoError(t, err)

	require.NoError(t, owner.BindRunNotifications(
		1,
		func() { controlReady <- struct{}{} },
		func(error) {},
		nil,
	))

	result, err := NewSealedResult(200, "application/json", []byte(`{"status":200}`))
	require.NoError(t, err)
	first, err := PrepareFrame("u1", result, 1)
	require.NoError(t, err)
	second, err := PrepareFrame("u2", result, 1)
	require.NoError(t, err)
	firstDone := make(chan error, 1)
	go func() { firstDone <- owner.Commit(first) }()

	require.True(t, bytes.Contains(<-writer.offered, []byte("FUNCTION_RESULT_BEGIN u1 ")))

	require.ErrorIs(t, owner.TryCommitControl(ControlFramePlan{UID: "uc", Status: ControlDeadline, Expiry: 1}), ErrFrameOwnerBusy)

	secondDone := make(chan error, 1)
	go func() { secondDone <- owner.Commit(second) }()
	select {
	case got := <-writer.offered:
		require.FailNowf(t, "test failed", "later ordinary frame bypassed pending control: %q", got)
	case <-time.After(20 * time.Millisecond):
	}
	writer.release <- struct{}{}

	require.NoError(t, <-firstDone)

	<-controlReady
	controlDone := make(chan error, 1)
	go func() {
		controlDone <- owner.TryCommitControl(ControlFramePlan{UID: "uc", Status: ControlDeadline, Expiry: 1})
	}()

	require.True(t, bytes.Contains(<-writer.offered, []byte("FUNCTION_RESULT_BEGIN uc 504 ")))

	writer.release <- struct{}{}

	require.NoError(t, <-controlDone)

	require.True(t, bytes.Contains(<-writer.offered, []byte("FUNCTION_RESULT_BEGIN u2 ")))

	writer.release <- struct{}{}

	require.NoError(t, <-secondDone)
}

func TestFrameOwnerLateControlReadyBindingReplaysPendingWake(t *testing.T) {
	writer := newStepWriter()
	owner, err := NewFrameOwner(writer)
	require.NoError(t, err)
	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	frame, err := PrepareFrame("ordinary", result, 1)
	require.NoError(t, err)
	committed := make(chan error, 1)
	go func() { committed <- owner.Commit(frame) }()
	<-writer.offered

	require.ErrorIs(t, owner.TryCommitControl(ControlFramePlan{
		UID: "control", Status: ControlDeadline, Expiry: 1,
	}), ErrFrameOwnerBusy)

	writer.release <- struct{}{}

	require.NoError(t, <-committed)

	ready := make(chan struct{}, 1)

	require.NoError(t, owner.BindRunNotifications(
		1,
		func() { ready <- struct{}{} },
		func(error) {},
		nil,
	))

	select {
	case <-ready:
	default:
		require.FailNow(t, "test failed", "late binding lost pending control wake")
	}

	require.Error(t, owner.BindRunNotifications(2, func() {}, func(error) {}, nil))
}

func TestFrameOwnerShortWritePoisonsAndRetains(t *testing.T) {
	owner, err := NewFrameOwner(shortWriter{})
	require.NoError(t, err)
	poisoned := make(chan error, 1)

	require.NoError(t, owner.BindRunNotifications(
		1,
		func() {},
		func(err error) { poisoned <- err },
		nil,
	))

	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	frame, err := PrepareFrame("u", result, 1)
	require.NoError(t, err)

	require.ErrorIs(t, owner.Commit(frame), io.ErrShortWrite)

	select {
	case err := <-poisoned:
		require.False(t, !errors.Is(err, ErrFrameOwnerPoisoned) || !errors.Is(err, io.ErrShortWrite))
	default:
		require.FailNow(t, "test failed", "short write did not publish poison")
	}
	census := owner.Census()
	require.False(t, !census.Poisoned || census.RetainedBytes == 0)
	next, err := PrepareFrame("next", result, 1)
	require.NoError(t, err)

	require.ErrorIs(t, owner.Commit(next), ErrFrameOwnerPoisoned)
}

func TestFrameOwnerLatePoisonBindingReplaysOriginalCause(t *testing.T) {
	cause := errors.New("write failed")
	owner, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	owner.Poison(cause)
	notified := make(chan error, 1)

	require.NoError(t, owner.BindRunNotifications(
		1,
		func() {},
		func(err error) { notified <- err },
		nil,
	))

	select {
	case err := <-notified:
		require.ErrorIs(t, err, ErrFrameOwnerPoisoned)
		require.ErrorIs(t, err, cause)
	default:
		require.FailNow(t, "late binding did not replay poison")
	}
}

func TestFrameOwnerTransactionalPreflightAbortsState(t *testing.T) {
	owner, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	commits := 0
	aborts := 0

	err = owner.CommitBorrowedProtocolTransaction(
		nil,
		protocolTestTransaction{
			commit: func() error {
				commits++
				return nil
			},
			abort: func() error {
				aborts++
				return nil
			},
		},
	)

	require.Error(t, err)
	require.Zero(t, commits)
	require.Equal(t, 1, aborts)
	require.False(t, owner.Census().Poisoned)
}

func TestFrameOwnerWriteFailureAbortsBeforePoisonNotification(t *testing.T) {
	abortErr := errors.New("abort failed")
	aborted := false
	owner, err := NewFrameOwner(shortWriter{})
	require.NoError(t, err)
	notified := make(chan error, 1)
	require.NoError(t, owner.BindRunNotifications(
		1,
		func() {},
		func(err error) {
			require.True(t, aborted)
			notified <- err
		},
		nil,
	))

	err = owner.CommitBorrowedProtocolTransaction(
		[]byte("frame"),
		protocolTestTransaction{
			commit: func() error {
				return nil
			},
			abort: func() error {
				aborted = true
				return abortErr
			},
		},
	)

	require.ErrorIs(t, err, io.ErrShortWrite)
	require.ErrorIs(t, err, abortErr)
	select {
	case poisonErr := <-notified:
		require.ErrorIs(t, poisonErr, io.ErrShortWrite)
		require.ErrorIs(t, poisonErr, abortErr)
	default:
		require.FailNow(t, "write failure did not notify poison")
	}
}

func TestFrameOwnerRunNotificationLeaseCanMoveAfterExactRelease(t *testing.T) {
	owner, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	notify := func() {}
	poison := func(error) {}

	require.NoError(t, owner.BindRunNotifications(1, notify, poison, nil))

	tests := map[string]struct {
		run func() error
	}{
		"duplicate live bind": {
			run: func() error {
				return owner.BindRunNotifications(2, notify, poison, nil)
			},
		},
		"stale release": {
			run: func() error {
				return owner.ReleaseRunNotifications(2)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Error(t, test.run())
		})
	}

	require.NoError(t, owner.ReleaseRunNotifications(1))

	require.NoError(t, owner.BindRunNotifications(2, notify, poison, nil))

	require.NoError(t, owner.ReleaseRunNotifications(2))
}

func TestClosedControlResults(t *testing.T) {
	for status, payload := range map[ControlStatus]string{
		ControlBadRequest:      `{"errorMessage":"Bad request.","status":400}`,
		ControlNotFound:        `{"errorMessage":"Not found.","status":404}`,
		ControlPayloadTooLarge: `{"errorMessage":"Payload too large.","status":413}`,
		ControlCancelled:       `{"errorMessage":"Request cancelled.","status":499}`,
		ControlInternal:        `{"errorMessage":"Internal error.","status":500}`,
		ControlUnavailable:     `{"errorMessage":"Service unavailable.","status":503}`,
		ControlDeadline:        `{"errorMessage":"Deadline exceeded.","status":504}`,
	} {
		result, err := NewControlResult(status)
		require.False(t, err != nil || result.status != int(status) || result.contentType != "application/json" || string(result.payload) != payload)
	}

	_, err := NewControlResult(418)
	require.Error(t, err)
}

func TestFunctionPayloadAndFrameCapacityAreSeparateFromControlReserve(t *testing.T) {
	tests := map[string]struct {
		length int
		valid  bool
	}{
		"below deferred boundary": {FunctionPayloadBytes - 2, true},
		"at deferred boundary":    {FunctionPayloadBytes - 1, true},
		"over deferred boundary":  {FunctionPayloadBytes, false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateFunctionPayloadSize(test.length)
			require.EqualValues(t, test.valid, err == nil)
		})
	}
	payload := bytes.Repeat([]byte{'x'}, 1024*1024)
	result, err := NewSealedResult(200, "application/json", payload)
	require.NoError(t, err)
	owner, err := NewFrameOwner(io.Discard)
	require.NoError(t, err)
	frame, err := PrepareFrame("u-large", result, 1)
	require.NoError(t, err)

	require.NoError(t, owner.Commit(frame))

	census := owner.Census()
	require.False(t, census.Commits != 1 || census.RetainedBytes != 0)
}

func TestFunctionFrameSizePreflightsBeforeAppend(t *testing.T) {
	baseFrame, baseEnvelope, err := functionFrameSize("u", 200, "application/json", 1, 0)
	require.NoError(t, err)
	exactUID := strings.Repeat("u", 1+FunctionEnvelopeBytes-baseEnvelope)
	exactFrame, exactEnvelope, err := functionFrameSize(exactUID, 200, "application/json", 1, 0)
	require.NoError(t, err)
	require.False(t, exactEnvelope != FunctionEnvelopeBytes || exactFrame != baseFrame+len(exactUID)-1)
	seed := []byte("unchanged")
	encoded, err := encodeResult(seed, exactUID+"u", 200, "application/json", 1, nil, MaximumFunctionFrameBytes, FunctionEnvelopeBytes, FunctionPayloadBytes)
	require.False(t, !errors.Is(err, ErrFunctionResultTooLarge) || !bytes.Equal(encoded, seed))

	payload := []byte(`{"status":200}`)
	wantSize, _, err := functionFrameSize("u-size", 200, "application/json", 1, len(payload))
	require.NoError(t, err)
	encoded, err = encodeResult(nil, "u-size", 200, "application/json", 1, payload, MaximumFunctionFrameBytes, FunctionEnvelopeBytes, FunctionPayloadBytes)
	require.NoError(t, err)
	require.EqualValues(t, wantSize, len(encoded))
}

type stepWriter struct {
	offered chan []byte
	release chan struct{}
	mu      sync.Mutex
}

func newStepWriter() *stepWriter {
	return &stepWriter{offered: make(chan []byte), release: make(chan struct{})}
}

func (sw *stepWriter) Write(payload []byte) (int, error) {
	payloadCopy := append([]byte(nil), payload...)
	sw.offered <- payloadCopy
	<-sw.release
	return len(payload), nil
}

type shortWriter struct{}

func (shortWriter) Write(payload []byte) (int, error) {
	return len(payload) - 1, nil
}

type protocolTestTransaction struct {
	commit func() error
	abort  func() error
}

func (transaction protocolTestTransaction) Commit() error {
	return transaction.commit()
}

func (transaction protocolTestTransaction) Abort() error {
	return transaction.abort()
}
