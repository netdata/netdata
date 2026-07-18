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
)

func TestAdmissionAndUIDExactRelease(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	ordinary := ledger.RequestOrdinary(1, lane, OrdinaryBudgetBytes)
	if ordinary.Rejected != nil {
		t.Fatal(ordinary.Rejected)
	}
	cleanup := ledger.RequestCleanup(1, lane, CleanupBudgetBytes)
	if cleanup.Rejected != nil {
		t.Fatal(cleanup.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(len(grants), &grants)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || grants[0].Ref != cleanup.Ref || grants[1].Ref != ordinary.Ref {
		t.Fatalf("admission grant order differs: %#v", grants[:count])
	}
	if err := ledger.CloseDrained(1); err == nil {
		t.Fatal("ledger closed with retained grants")
	}
	if _, err := ledger.ReleaseCleanup(cleanup.Ref, FrameOutcomeCommitted); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ReleaseOrdinary(ordinary.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CloseDrained(1); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_000, 0)
	uids := NewUIDLedger()
	if err := uids.Admit("a", now); err != nil {
		t.Fatal(err)
	}
	if err := uids.Admit("a", now); err == nil {
		t.Fatal("duplicate UID admitted")
	}
	if err := uids.Complete("a", true, now); err != nil {
		t.Fatal(err)
	}
	if err := uids.Admit("a", now.Add(UIDTombstoneLifetime-time.Nanosecond)); err == nil {
		t.Fatal("tombstoned UID admitted")
	}
	if err := uids.Admit("a", now.Add(UIDTombstoneLifetime)); err != nil {
		t.Fatalf("exact-expiry UID was not reclaimed: %v", err)
	}
	if err := uids.Complete("a", false, now.Add(UIDTombstoneLifetime)); err != nil {
		t.Fatal(err)
	}
	more, err := uids.CloseBatch(UIDReturnBatch)
	if err != nil || more {
		t.Fatalf("UID close differs: more=%v err=%v", more, err)
	}
	if active, tombstones, closed := uids.Census(); active != 0 || tombstones != 0 || !closed {
		t.Fatalf("UID final census differs: active=%d tombstones=%d closed=%v", active, tombstones, closed)
	}
}

func TestAdmissionProcessBytesReduceOrdinaryCapacityUntilFinalClose(t *testing.T) {
	ledger := NewAdmissionLedger()
	const processBytes int64 = 4096
	if err := ledger.ReserveProcessBytes(processBytes); err != nil {
		t.Fatal(err)
	}
	if result := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, OrdinaryBudgetBytes-processBytes+1); result.Rejected == nil {
		t.Fatal("ordinary request exceeded process-adjusted capacity")
	}
	result := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, OrdinaryBudgetBytes-processBytes)
	if result.Rejected != nil {
		t.Fatal(result.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != result.Ref {
		t.Fatalf("process-adjusted grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if err := ledger.ReleaseProcessBytes(processBytes); err == nil {
		t.Fatal("process bytes released before final Admission close")
	}
	if _, err := ledger.ReleaseOrdinary(result.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ReleaseProcessBytes(processBytes); err != nil {
		t.Fatal(err)
	}
	if census := ledger.Census(); census.ProcessBytes != 0 || census.OrdinaryBytes != 0 || census.Phase != "closed" {
		t.Fatalf("released process reservation differs: %+v", census)
	}
}

func TestAdmissionProcessBytesCanUnwindPristineConstruction(t *testing.T) {
	ledger := NewAdmissionLedger()
	const processBytes = int64(4096)
	if err := ledger.ReserveProcessBytes(processBytes); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ReleaseProcessBytes(processBytes); err != nil {
		t.Fatalf("pristine construction reservation did not unwind: %v", err)
	}
	if census := ledger.Census(); census.ProcessBytes != 0 || census.ActiveRecords != 0 || census.Phase != "ordinary-open" || census.RunGeneration != 0 {
		t.Fatalf("pristine construction unwind left state: %+v", census)
	}
	if err := ledger.ReserveProcessBytes(processBytes); err != nil {
		t.Fatalf("unwound construction reservation was not reusable: %v", err)
	}
	if err := ledger.ReleaseProcessBytes(processBytes); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionLongLivedTransferDetachesOperationRecords(t *testing.T) {
	ledger := NewAdmissionLedger()
	longLived := make([]AdmissionRef, 0, formerFixedPopulation)
	for index := 0; index < formerFixedPopulation; index++ {
		lane := AdmissionLaneRef{Slot: uint32(index + 1), Generation: 1}
		request := ledger.RequestOrdinary(1, lane, 2)
		if request.Rejected != nil {
			t.Fatal(request.Rejected)
		}
		var grants [4]AdmissionGrant
		count, _, err := ledger.TakeGrants(1, &grants)
		if err != nil || count != 1 || grants[0].Ref != request.Ref {
			t.Fatalf("long-lived grant %d differs: count=%d grant=%+v err=%v", index, count, grants[0], err)
		}
		if err := ledger.transferLongLived(request.Ref, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := ledger.ReleaseOrdinary(request.Ref); err != nil {
			t.Fatal(err)
		}
		longLived = append(longLived, request.Ref)
	}
	if census := ledger.Census(); census.ActiveRecords != 0 ||
		census.FreeRecords == 0 ||
		census.LongLivedRecords != formerFixedPopulation {
		t.Fatalf("detached long-lived census=%+v", census)
	}
	last := ledger.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 1, Generation: 2},
		2,
	)
	if last.Rejected != nil {
		t.Fatal(last.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != last.Ref {
		t.Fatalf("headroom grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if _, err := ledger.ReleaseOrdinary(last.Ref); err != nil {
		t.Fatal(err)
	}
	for _, ref := range longLived {
		if _, err := ledger.releaseLongLived(ref, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionReopensOnlyFromAnExactlyDrainedRun(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	request := ledger.RequestOrdinary(1, lane, 1)
	if request.Rejected != nil {
		t.Fatal(request.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != request.Ref {
		t.Fatalf("grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ReopenDrained(1, 2); err == nil {
		t.Fatal("admission reopened with retained run state")
	}
	if _, err := ledger.ReleaseOrdinary(request.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.ReopenDrained(1, 1); err == nil {
		t.Fatal("admission reopened without a fresh generation")
	}
	if err := ledger.ReopenDrained(1, 2); err != nil {
		t.Fatal(err)
	}
	if result := ledger.RequestOrdinary(1, lane, 1); result.Rejected == nil {
		t.Fatal("stale run generation admitted after reopen")
	}
	second := ledger.RequestOrdinary(2, AdmissionLaneRef{Slot: 1, Generation: 2}, 1)
	if second.Rejected != nil {
		t.Fatal(second.Rejected)
	}
	if err := ledger.CancelWaiting(second.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(2); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CloseDrained(2); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionCarriesOneStableInputBodyAcrossRunRotation(t *testing.T) {
	ledger := NewAdmissionLedger()
	const capacity = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, capacity)
	if err != nil {
		t.Fatal(err)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].InputBodyToken != token {
		t.Fatalf("input grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if _, err := ledger.CommitInputBodyGrowth(token, capacity); err != nil {
		t.Fatal(err)
	}
	if ledger.RunDrained(1) {
		t.Fatal("uncarried input body counted as a drained run")
	}
	if err := ledger.SuspendInputBody(1, 2, token); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if !ledger.RunDrained(1) {
		t.Fatal("stable carried input body did not permit old-run quiescence")
	}
	if err := ledger.CloseDrained(1); err == nil {
		t.Fatal("final close accepted a carried input body")
	}
	if err := ledger.ReopenDrained(1, 2); err != nil {
		t.Fatal(err)
	}
	if census := ledger.Census(); census.RunGeneration != 2 || !census.InputBodyCarried || census.InputBodyBytes != capacity {
		t.Fatalf("reopened carried body census differs: %+v", census)
	}
	if err := ledger.AdoptInputBody(2, token); err != nil {
		t.Fatal(err)
	}
	if census := ledger.Census(); census.InputBodyCarried {
		t.Fatalf("adopted input body remained carried: %+v", census)
	}
	if _, err := ledger.AbortInputBody(token); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(2); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CloseDrained(2); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionShutdownSettlesOnlyPendingInputBodyGrowth(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	ordinary := ledger.RequestOrdinary(1, lane, 1)
	if ordinary.Rejected != nil {
		t.Fatal(ordinary.Rejected)
	}
	const capacity = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, capacity)
	if err != nil {
		t.Fatal(err)
	}
	grant, waiting, err := ledger.TakeShutdownInputBodyGrant(1)
	if err != nil || waiting {
		t.Fatalf("shutdown input grant differs: waiting=%v err=%v", waiting, err)
	}
	if grant.Kind != ReservationInputBodyGrowth || grant.InputBodyToken != token || grant.Bytes != capacity {
		t.Fatalf("shutdown input grant differs: %+v", grant)
	}
	if census := ledger.Census(); census.OrdinaryWaiting != 1 || !census.InputBodyActive || census.InputBodyWaiting {
		t.Fatalf("shutdown input grant affected unrelated ordinary work: %+v", census)
	}
	if err := ledger.BeginCleanupOnly(1); err == nil {
		t.Fatal("cleanup-only transition ignored unrelated ordinary waiter")
	}
	if err := ledger.CancelWaiting(ordinary.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if repeated, stillWaiting, err := ledger.TakeShutdownInputBodyGrant(1); err != nil || stillWaiting || repeated.Kind != 0 {
		t.Fatalf("repeated shutdown input service differs: grant=%+v waiting=%v err=%v", repeated, stillWaiting, err)
	}
	if _, err := ledger.CommitInputBodyGrowth(token, capacity); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.AbortInputBody(token); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
}

func TestUIDLedgerAdmissionAndCloseWorkAreBatched(t *testing.T) {
	now := time.Unix(2_000, 0)
	uids := NewUIDLedger()
	for index := 0; index < UIDReturnBatch+1; index++ {
		uid := fmt.Sprintf("u-%d", index)
		if err := uids.Admit(uid, now); err != nil {
			t.Fatal(err)
		}
		if err := uids.Complete(uid, true, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := uids.Admit("u-64", now.Add(UIDTombstoneLifetime)); err != nil {
		t.Fatal(err)
	}
	if active, tombstones, _ := uids.Census(); active != 1 || tombstones != 1 {
		t.Fatalf("direct-expiry admission batch differs: active=%d tombstones=%d", active, tombstones)
	}
	if err := uids.Complete("u-64", true, now.Add(UIDTombstoneLifetime)); err != nil {
		t.Fatal(err)
	}
	more, err := uids.CloseBatch(1)
	if err != nil || !more {
		t.Fatalf("first UID close batch differs: more=%v err=%v", more, err)
	}
	more, err = uids.CloseBatch(UIDReturnBatch)
	if err != nil || more {
		t.Fatalf("final UID close batch differs: more=%v err=%v", more, err)
	}
	if _, _, closed := uids.Census(); !closed {
		t.Fatal("UID ledger did not close")
	}
	if err := uids.Admit("after-close", now); err == nil {
		t.Fatal("UID admitted after close")
	}
	if _, err := uids.CloseBatch(UIDReturnBatch); err != nil {
		t.Fatal(err)
	}
}

func TestUIDLedgerGrowsAndCloseWorkRemainsBatched(t *testing.T) {
	now := time.Unix(3_000, 0)
	uids := NewUIDLedger()
	const population = formerFixedUIDPopulation + 1
	for index := 0; index < population; index++ {
		if err := uids.Admit(fmt.Sprintf("active-%d", index), now); err != nil {
			t.Fatalf("admit %d: %v", index, err)
		}
	}
	for index := 0; index < population; index++ {
		if err := uids.Complete(fmt.Sprintf("active-%d", index), true, now); err != nil {
			t.Fatalf("complete %d: %v", index, err)
		}
	}
	wantBatches := (population + UIDReturnBatch - 1) / UIDReturnBatch
	for batch := 1; batch <= wantBatches; batch++ {
		more, err := uids.CloseBatch(UIDReturnBatch)
		if err != nil {
			t.Fatalf("close batch %d: %v", batch, err)
		}
		if more != (batch < wantBatches) {
			t.Fatalf("close batch %d returned more=%v", batch, more)
		}
	}
	if active, tombstones, closed := uids.Census(); active != 0 || tombstones != 0 || !closed {
		t.Fatalf("capacity release census differs: active=%d tombstones=%d closed=%v", active, tombstones, closed)
	}
}

func TestAdmissionRadixDomainAndOldestFitting(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	for _, bytes := range []int64{0, OrdinaryBudgetBytes + 1, 268_435_455} {
		if result := ledger.RequestOrdinary(1, lane, bytes); result.Rejected == nil || result.Ref.Valid() {
			t.Fatalf("out-of-domain request %d was retained: %#v", bytes, result)
		}
	}
	for _, bytes := range []int64{1, 134_217_727, 134_217_728, OrdinaryBudgetBytes} {
		result := ledger.RequestOrdinary(1, lane, bytes)
		if result.Rejected != nil {
			t.Fatalf("boundary request %d: %v", bytes, result.Rejected)
		}
		if err := ledger.CancelWaiting(result.Ref); err != nil {
			t.Fatal(err)
		}
	}

	blocker := ledger.RequestOrdinary(1, lane, OrdinaryBudgetBytes-10)
	oldLarge := ledger.RequestOrdinary(1, lane, 11)
	firstSmall := ledger.RequestOrdinary(1, lane, 5)
	laterSmall := ledger.RequestOrdinary(1, lane, 4)
	for _, result := range []AdmissionRequestResult{blocker, oldLarge, firstSmall, laterSmall} {
		if result.Rejected != nil {
			t.Fatal(result.Rejected)
		}
	}
	var grants [4]AdmissionGrant
	count, more, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || !more || grants[0].Ref != blocker.Ref {
		t.Fatalf("blocker grant differs: count=%d more=%v grant=%#v err=%v", count, more, grants[0], err)
	}
	count, _, err = ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != firstSmall.Ref {
		t.Fatalf("oldest fitting grant differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	if _, err := ledger.ReleaseOrdinary(blocker.Ref); err != nil {
		t.Fatal(err)
	}
	count, _, err = ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != oldLarge.Ref {
		t.Fatalf("newly fitting old request was bypassed: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	count, _, err = ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != laterSmall.Ref {
		t.Fatalf("remaining small grant differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	for _, ref := range []AdmissionRef{firstSmall.Ref, oldLarge.Ref, laterSmall.Ref} {
		if _, err := ledger.ReleaseOrdinary(ref); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAdmissionMoreReportsOnlyImmediatelyGrantableWork(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	full := ledger.RequestOrdinary(1, lane, OrdinaryBudgetBytes)
	waiting := ledger.RequestOrdinary(1, lane, 1)
	if full.Rejected != nil || waiting.Rejected != nil {
		t.Fatalf("requests differ: full=%v waiting=%v", full.Rejected, waiting.Rejected)
	}
	var grants [4]AdmissionGrant
	count, more, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != full.Ref || more {
		t.Fatalf("saturated grant differs: count=%d more=%v grant=%#v err=%v", count, more, grants[0], err)
	}
	wake, err := ledger.ReleaseOrdinary(full.Ref)
	if err != nil || !wake {
		t.Fatalf("release did not expose the waiter: wake=%v err=%v", wake, err)
	}
	count, more, err = ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != waiting.Ref || more {
		t.Fatalf("waiter grant differs: count=%d more=%v grant=%#v err=%v", count, more, grants[0], err)
	}
	if _, err := ledger.ReleaseOrdinary(waiting.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionCleanupFIFOAndDirectCancellation(t *testing.T) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	firstCleanup := ledger.RequestCleanup(1, lane, CleanupBudgetBytes)
	secondCleanup := ledger.RequestCleanup(1, lane, 1)
	ordinary := ledger.RequestOrdinary(1, lane, 1)
	cancelled := ledger.RequestOrdinary(1, lane, 2)
	for _, result := range []AdmissionRequestResult{firstCleanup, secondCleanup, ordinary, cancelled} {
		if result.Rejected != nil {
			t.Fatal(result.Rejected)
		}
	}
	if err := ledger.CancelWaiting(cancelled.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.CancelWaiting(cancelled.Ref); err == nil {
		t.Fatal("stale direct cancellation succeeded")
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(len(grants), &grants)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || grants[0].Ref != firstCleanup.Ref || grants[1].Ref != ordinary.Ref {
		t.Fatalf("cleanup priority/single grant differs: %#v", grants[:count])
	}
	if _, err := ledger.ReleaseCleanup(firstCleanup.Ref, FrameOutcomeCommitted); err != nil {
		t.Fatal(err)
	}
	count, _, err = ledger.TakeGrants(len(grants), &grants)
	if err != nil || count != 1 || grants[0].Ref != secondCleanup.Ref {
		t.Fatalf("cleanup FIFO successor differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	if _, err := ledger.ReleaseCleanup(secondCleanup.Ref, FrameOutcomeSafeAbort); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ReleaseOrdinary(ordinary.Ref); err != nil {
		t.Fatal(err)
	}
	if err := ledger.BeginCleanupOnly(1); err != nil {
		t.Fatal(err)
	}
	if result := ledger.RequestOrdinary(1, lane, 1); result.Rejected == nil {
		t.Fatal("ordinary request entered cleanup-only phase")
	}
	if err := ledger.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionOrdinaryGrowthRetainsOneRecordAndWaitsByDelta(t *testing.T) {
	ledger := NewAdmissionLedger()
	first := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, 100)
	second := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 2, Generation: 1}, OrdinaryBudgetBytes-100)
	if first.Rejected != nil || second.Rejected != nil {
		t.Fatalf("initial admission differs: first=%v second=%v", first.Rejected, second.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(2, &grants)
	if err != nil || count != 2 {
		t.Fatalf("initial grants differ: count=%d err=%v", count, err)
	}
	ready, _, err := ledger.ResizeOrdinary(first.Ref, 101)
	if err != nil || ready {
		t.Fatalf("saturated growth did not wait: ready=%v err=%v", ready, err)
	}
	if census := ledger.Census(); census.ActiveRecords != 2 || census.OrdinaryGranted != 2 || census.OrdinaryWaiting != 1 || census.OrdinaryBytes != OrdinaryBudgetBytes {
		t.Fatalf("growth-wait census differs: %#v", census)
	}
	ready, wake, err := ledger.ResizeOrdinary(second.Ref, OrdinaryBudgetBytes-101)
	if err != nil || !ready || !wake {
		t.Fatalf("shrink did not expose growth: ready=%v wake=%v err=%v", ready, wake, err)
	}
	count, _, err = ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != first.Ref || grants[0].Kind != ReservationOrdinaryGrowth || grants[0].Bytes != 101 {
		t.Fatalf("growth grant differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	if _, err := ledger.ReleaseOrdinary(first.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ReleaseOrdinary(second.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionInputBodyGrowthTransfersIntoOperationRecord(t *testing.T) {
	ledger := NewAdmissionLedger()
	const initial = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, initial)
	if err != nil {
		t.Fatal(err)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(4, &grants)
	if err != nil || count != 1 || grants[0].Kind != ReservationInputBodyGrowth || grants[0].InputBodyToken != token || grants[0].Bytes != initial {
		t.Fatalf("initial input grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if _, err := ledger.CommitInputBodyGrowth(token, initial); err != nil {
		t.Fatal(err)
	}

	const grown = int64(128 * 1024)
	if next, err := ledger.RequestInputBodyGrowth(1, token, grown); err != nil || next != token {
		t.Fatalf("growth request differs: token=%d err=%v", next, err)
	}
	count, _, err = ledger.TakeGrants(4, &grants)
	if err != nil || count != 1 || grants[0].Kind != ReservationInputBodyGrowth || grants[0].Bytes != initial+grown {
		t.Fatalf("replacement grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if census := ledger.Census(); census.OrdinaryBytes != initial+grown || census.InputBodyBytes != initial+grown {
		t.Fatalf("replacement census differs before copy commit: %+v", census)
	}
	if _, err := ledger.CommitInputBodyGrowth(token, grown); err != nil {
		t.Fatal(err)
	}

	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	transferred := ledger.TransferInputBody(1, token, lane, grown+512, grown)
	if transferred.Rejected != nil || !transferred.Ref.Valid() {
		t.Fatalf("input transfer rejected: %+v", transferred)
	}
	if census := ledger.Census(); census.InputBodyActive || census.InputBodyBytes != 0 || census.ActiveRecords != 1 || census.OrdinaryBytes != grown {
		t.Fatalf("transfer census differs before operation grant: %+v", census)
	}
	count, _, err = ledger.TakeGrants(4, &grants)
	if err != nil || count != 1 || grants[0].Kind != ReservationOrdinary || grants[0].Ref != transferred.Ref || grants[0].Bytes != grown+512 {
		t.Fatalf("operation grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if _, err := ledger.ReleaseOrdinary(transferred.Ref); err != nil {
		t.Fatal(err)
	}
	if census := ledger.Census(); census.ActiveRecords != 0 || census.OrdinaryBytes != 0 || census.InputBodyActive {
		t.Fatalf("released transfer retained state: %+v", census)
	}
}

func TestAdmissionCancelTransferredInputBodyWaitingReleasesHeldBytes(t *testing.T) {
	ledger := NewAdmissionLedger()
	const capacity = int64(64 * 1024)
	token, err := ledger.RequestInputBodyGrowth(1, 0, capacity)
	if err != nil {
		t.Fatal(err)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 {
		t.Fatalf("input grant differs: count=%d err=%v", count, err)
	}
	if _, err := ledger.CommitInputBodyGrowth(token, capacity); err != nil {
		t.Fatal(err)
	}
	transferred := ledger.TransferInputBody(
		1, token, AdmissionLaneRef{Slot: 1, Generation: 1}, capacity+512, capacity,
	)
	if transferred.Rejected != nil {
		t.Fatal(transferred.Rejected)
	}
	if err := ledger.CancelWaiting(transferred.Ref); err != nil {
		t.Fatal(err)
	}
	if census := ledger.Census(); census.ActiveRecords != 0 || census.OrdinaryBytes != 0 || census.InputBodyActive {
		t.Fatalf("cancelled transfer retained state: %#v", census)
	}
}

func TestAdmissionLongLivedRecordDetachesFromCompletedLaneGeneration(t *testing.T) {
	ledger := NewAdmissionLedger()
	first := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 1}, 100)
	if first.Rejected != nil {
		t.Fatal(first.Rejected)
	}
	var grants [4]AdmissionGrant
	if count, _, err := ledger.TakeGrants(1, &grants); err != nil || count != 1 || grants[0].Ref != first.Ref {
		t.Fatalf("first grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if err := ledger.transferLongLived(first.Ref, 40); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ReleaseOrdinary(first.Ref); err != nil {
		t.Fatal(err)
	}
	second := ledger.RequestOrdinary(1, AdmissionLaneRef{Slot: 1, Generation: 2}, 60)
	if second.Rejected != nil {
		t.Fatalf("next lane generation was blocked by detached long-lived ownership: %v", second.Rejected)
	}
	if count, _, err := ledger.TakeGrants(1, &grants); err != nil || count != 1 || grants[0].Ref != second.Ref {
		t.Fatalf("second grant differs: count=%d grant=%+v err=%v", count, grants[0], err)
	}
	if _, err := ledger.ReleaseOrdinary(second.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.releaseLongLived(first.Ref, 40); err != nil {
		t.Fatal(err)
	}
	if census := ledger.Census(); census.ActiveRecords != 0 || census.OrdinaryGranted != 0 || census.OrdinaryBytes != 0 || census.LongLivedRecords != 0 || census.LongLivedBytes != 0 {
		t.Fatalf("detached long-lived release census=%+v", census)
	}
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
			if request.Rejected != nil {
				t.Fatal(request.Rejected)
			}
			var grants [4]AdmissionGrant
			if count, _, err := ledger.TakeGrants(
				1,
				&grants,
			); err != nil || count != 1 {
				t.Fatalf("grant count=%d err=%v", count, err)
			}
			if err := ledger.transferLongLived(
				request.Ref,
				40,
			); err != nil {
				t.Fatal(err)
			}
			ready, _, err := ledger.ResizeOrdinary(
				request.Ref,
				test.resized,
			)
			if err != nil || !ready {
				t.Fatalf("resize ready=%v err=%v", ready, err)
			}
			if census := ledger.Census(); census.OrdinaryBytes != test.resized ||
				census.LongLivedBytes != 40 ||
				census.LongLivedRecords != 1 {
				t.Fatalf("resized census=%+v", census)
			}
			if _, err := ledger.ReleaseOrdinary(request.Ref); err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.releaseLongLived(
				request.Ref,
				40,
			); err != nil {
				t.Fatal(err)
			}
			if census := ledger.Census(); census.ActiveRecords != 0 ||
				census.OrdinaryBytes != 0 ||
				census.LongLivedBytes != 0 {
				t.Fatalf("released census=%+v", census)
			}
		})
	}
}

func TestAdmissionInputBodyAbortReleasesWaitingAndGrantedCapacity(t *testing.T) {
	for _, afterGrant := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-grant-%t", afterGrant), func(t *testing.T) {
			ledger := NewAdmissionLedger()
			token, err := ledger.RequestInputBodyGrowth(1, 0, 64*1024)
			if err != nil {
				t.Fatal(err)
			}
			if afterGrant {
				var grants [4]AdmissionGrant
				if count, _, err := ledger.TakeGrants(4, &grants); err != nil || count != 1 {
					t.Fatalf("grant differs: count=%d err=%v", count, err)
				}
			}
			if _, err := ledger.AbortInputBody(token); err != nil {
				t.Fatal(err)
			}
			if census := ledger.Census(); census.InputBodyActive || census.OrdinaryWaiting != 0 || census.OrdinaryBytes != 0 {
				t.Fatalf("aborted body retained state: %+v", census)
			}
		})
	}
}

func TestTaskSupervisorDynamicPopulationAndGenerationCheckedReuse(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
		if err != nil {
			t.Fatal(err)
		}
	}
	refs := make([]TaskRef, 0, population)
	for {
		var started [TaskStartServiceQuantum]TaskStart
		count, more, err := supervisor.Dispatch(
			context.Background(),
			TaskStartServiceQuantum,
			&started,
		)
		if err != nil {
			t.Fatal(err)
		}
		if count > TaskStartServiceQuantum {
			t.Fatalf("one dispatch started %d tasks", count)
		}
		for _, start := range started[:count] {
			refs = append(refs, start.Task)
		}
		if !more {
			break
		}
	}
	if len(refs) != population || supervisor.Active() != population {
		t.Fatalf(
			"dynamic population refs=%d active=%d want=%d",
			len(refs),
			supervisor.Active(),
			population,
		)
	}
	close(release)
	for range refs {
		completion := <-supervisor.CompletionCh()
		if completion.Sequence != 1 {
			t.Fatalf("initial phase sequence differs: %#v", completion)
		}
		if err := supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
			t.Fatal(err)
		}
		ack := <-supervisor.AcknowledgementCh()
		if ack.Sequence != 2 || ack.Err != nil {
			t.Fatalf("disposal acknowledgement differs: %#v", ack)
		}
		if err := supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
			t.Fatal(err)
		}
		ack = <-supervisor.AcknowledgementCh()
		if ack.Sequence != 3 || ack.Kind != TaskActionTerminate || ack.Err != nil {
			t.Fatalf("termination acknowledgement differs: %#v", ack)
		}
		if err := supervisor.Release(ack.Ref); err != nil {
			t.Fatal(err)
		}
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
	if err != nil {
		t.Fatal(err)
	}
	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
	if err != nil || count != 1 || more || started[0].Request != pending {
		t.Fatalf("pending task dispatch differs: started=%#v count=%d more=%v err=%v", started[0], count, more, err)
	}
	ref := started[0].Task
	if generation, ok := previous[ref.Slot]; !ok || ref.Generation != generation+1 {
		t.Fatalf("slot reuse differs: old=%#v new=%#v", refs[0], ref)
	}
	completion := <-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if err := supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	ack = <-supervisor.AcknowledgementCh()
	if err := supervisor.Release(ack.Ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorRetainedTimeoutCountAndSaturationLatch(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
		if err != nil {
			t.Fatal(err)
		}
		wantSaturated := index == RetainedTimeoutFailStopThreshold-1
		if saturated != wantSaturated {
			t.Fatalf("retained timeout %d saturation=%v, want %v", index+1, saturated, wantSaturated)
		}
		count, latched := supervisor.RetainedTimeouts()
		wantLatched := index >= RetainedTimeoutFailStopThreshold-1
		if count != index+1 || latched != wantLatched {
			t.Fatalf("retained timeout %d census=(%d,%v), want (%d,%v)", index+1, count, latched, index+1, wantLatched)
		}
	}
	for index, ref := range refs {
		cleared, err := supervisor.ClearRetainedTimeout(ref)
		if err != nil || !cleared {
			t.Fatalf("clear retained timeout %d: cleared=%v err=%v", index+1, cleared, err)
		}
		count, latched := supervisor.RetainedTimeouts()
		if count != len(refs)-index-1 || !latched {
			t.Fatalf("cleared timeout %d census=(%d,%v), want (%d,true)", index+1, count, latched, len(refs)-index-1)
		}
	}
	close(release)
	for range refs {
		completion := <-supervisor.CompletionCh()
		if err := supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
			t.Fatal(err)
		}
	}
	acknowledged := make([]TaskRef, 0, len(refs))
	for range refs {
		ack := <-supervisor.AcknowledgementCh()
		if ack.Sequence != 2 || ack.Kind != TaskActionDispose || ack.Err != nil {
			t.Fatalf("retained-timeout disposal acknowledgement differs: %#v", ack)
		}
		acknowledged = append(acknowledged, ack.Ref)
	}
	for _, ref := range acknowledged {
		if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
			t.Fatal(err)
		}
	}
	for range refs {
		ack := <-supervisor.AcknowledgementCh()
		if err := supervisor.Release(ack.Ref); err != nil {
			t.Fatal(err)
		}
	}
	if supervisor.Active() != 0 {
		t.Fatalf("retained-timeout test left %d active tasks", supervisor.Active())
	}
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
			if err != nil {
				t.Fatal(err)
			}
			supervisor, err := NewTaskSupervisor(frame)
			if err != nil {
				t.Fatal(err)
			}
			_, ref := enqueueAndDispatchTask(t, supervisor, plan)
			completion := <-supervisor.CompletionCh()
			if completion.Ref != ref || !errors.Is(completion.Err, ErrTaskPanic) ||
				!strings.Contains(completion.Err.Error(), "fixture panic") {
				t.Fatalf("panic completion differs: %#v", completion)
			}
			if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
				t.Fatal(err)
			}
			ack := <-supervisor.AcknowledgementCh()
			if ack.Ref != ref || ack.Sequence != 2 || ack.Kind != TaskActionDispose || ack.Err != nil {
				t.Fatalf("panic disposal acknowledgement differs: %#v", ack)
			}
			if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
				t.Fatal(err)
			}
			ack = <-supervisor.AcknowledgementCh()
			if ack.Ref != ref || ack.Sequence != 3 || ack.Kind != TaskActionTerminate || ack.Err != nil {
				t.Fatalf("panic termination acknowledgement differs: %#v", ack)
			}
			if err := supervisor.Release(ref); err != nil {
				t.Fatal(err)
			}
			if supervisor.Active() != 0 {
				t.Fatalf("panic task retained active slot: %d", supervisor.Active())
			}
		})
	}
}

type taskRunnerFunc func(context.Context) (TaskOutcome, error)

func (fn taskRunnerFunc) RunTask(ctx context.Context) (TaskOutcome, error) {
	return fn(ctx)
}

func TestTaskSupervisorPreservesAuthoritativeCancellationCause(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := supervisor.CancelWithCause(ref, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	if got := <-observed; !got.ok || !got.deadline.Equal(deadline) || !errors.Is(got.err, context.DeadlineExceeded) || !errors.Is(got.cause, context.DeadlineExceeded) {
		t.Fatalf("TaskChild cancellation context deadline=%s ok=%v err=%v cause=%v", got.deadline, got.ok, got.err, got.cause)
	}
	completion := <-supervisor.CompletionCh()
	if completion.Ref != ref || !errors.Is(completion.Err, context.DeadlineExceeded) {
		t.Fatalf("deadline completion differs: %#v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	ack = <-supervisor.AcknowledgementCh()
	if ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorChecksPhaseSequenceAndPublishesOwnedResult(t *testing.T) {
	var output bytes.Buffer
	frame, err := NewFrameOwner(&output)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"value":"original"}`)
	sealed, err := NewSealedResult(200, "application/json", payload)
	if err != nil {
		t.Fatal(err)
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction, MaxPhaseTransitions: 4,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return sealed, nil
		}),
	})
	completion := <-supervisor.CompletionCh()
	if completion.Ref != ref || completion.Sequence != 1 || completion.Err != nil {
		t.Fatalf("initial publication differs: %#v", completion)
	}
	payload[10] = 'X'
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionEncodeWrite, UID: "u1", Expiry: 1}); err == nil {
		t.Fatal("wrong phase sequence was accepted")
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionEncodeWrite, UID: "u1", Expiry: 1}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if ack.Ref != ref || ack.Sequence != 2 || ack.Kind != TaskActionEncodeWrite || ack.Err != nil {
		t.Fatalf("encode/write acknowledgement differs: %#v", ack)
	}
	if !bytes.Contains(output.Bytes(), []byte(`{"value":"original"}`)) || bytes.Contains(output.Bytes(), []byte(`{"value":"Xriginal"}`)) {
		t.Fatalf("published result retained callback alias: %q", output.Bytes())
	}
	if err := supervisor.Release(ref); err == nil {
		t.Fatal("slot released before explicit termination")
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	ack = <-supervisor.AcknowledgementCh()
	if ack.Ref != ref || ack.Sequence != 3 || ack.Kind != TaskActionTerminate || ack.Err != nil {
		t.Fatalf("termination acknowledgement differs: %#v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorPreflightsResultEnvelopeBeforeAction(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	_, ref := enqueueAndDispatchTask(t, supervisor, TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return result, nil
		}),
	})
	completion := <-supervisor.CompletionCh()
	if completion.Err != nil {
		t.Fatal(completion.Err)
	}
	_, baseEnvelope, err := functionFrameSize("u", 200, "application/json", 1, result.payloadBytes)
	if err != nil {
		t.Fatal(err)
	}
	oversizedUID := strings.Repeat("u", 2+FunctionEnvelopeBytes-baseEnvelope)
	if _, err := supervisor.PreflightResult(ref, oversizedUID, 1); !errors.Is(err, ErrFunctionResultTooLarge) {
		t.Fatalf("oversized result envelope preflight differs: %v", err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatal(ack.Err)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorRunsCleanupBeforeExplicitTermination(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Sequence != 2 || ack.Err != nil {
		t.Fatalf("disposal acknowledgement differs: %#v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 3, Kind: TaskActionCleanup}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Sequence != 3 || ack.Kind != TaskActionCleanup || ack.Err != nil {
		t.Fatalf("cleanup acknowledgement differs: %#v", ack)
	}
	select {
	case <-cleaned:
	default:
		t.Fatal("cleanup phase did not execute")
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Sequence != 4 || ack.Kind != TaskActionTerminate || ack.Err != nil {
		t.Fatalf("termination acknowledgement differs: %#v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorDispatchRotatesPendingClasses(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil || count != TaskStartServiceQuantum || more {
		t.Fatalf("class-fair dispatch differs: count=%d more=%v err=%v", count, more, err)
	}
	want := []TaskRequestRef{j1, f1, j2, f2}
	for index, ref := range want {
		if started[index].Request != ref {
			t.Fatalf("class-fair dispatch order differs at %d: got=%#v want=%#v", index, started[index].Request, ref)
		}
	}
	close(release)
	for range TaskStartServiceQuantum {
		completion := <-supervisor.CompletionCh()
		if err := supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
			t.Fatal(err)
		}
		ack := <-supervisor.AcknowledgementCh()
		if err := supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
			t.Fatal(err)
		}
		ack = <-supervisor.AcknowledgementCh()
		if err := supervisor.Release(ack.Ref); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTaskSupervisorRejectsInvalidSchedulingClass(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
			if _, err := supervisor.Enqueue(class, plan); err == nil {
				t.Fatal("invalid scheduling class was accepted")
			}
			if supervisor.Pending() != 0 {
				t.Fatalf(
					"invalid class retained %d pending tasks",
					supervisor.Pending(),
				)
			}
		})
	}
}

func TestTaskSupervisorFrameworkControlStartsWithManyActiveGenericTasks(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
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
		request, err := supervisor.Enqueue(
			TaskClassGenericFunction,
			blockingPlan,
		)
		if err != nil {
			t.Fatal(err)
		}
		var started [TaskStartServiceQuantum]TaskStart
		count, _, err := supervisor.Dispatch(
			context.Background(),
			1,
			&started,
		)
		if err != nil || count != 1 || started[0].Request != request {
			t.Fatalf(
				"blocking dispatch differs: count=%d start=%+v err=%v",
				count,
				started[0],
				err,
			)
		}
	}
	readyPlan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		}),
	}
	generic, err := supervisor.Enqueue(TaskClassGenericFunction, readyPlan)
	if err != nil {
		t.Fatal(err)
	}
	control, err := supervisor.Enqueue(TaskClassFrameworkControl, readyPlan)
	if err != nil {
		t.Fatal(err)
	}
	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(
		context.Background(),
		1,
		&started,
	)
	if err != nil || count != 1 || !more {
		t.Fatalf(
			"control dispatch differs: count=%d more=%v err=%v",
			count,
			more,
			err,
		)
	}
	if started[0].Request != control {
		t.Fatalf(
			"started request=%+v, want control=%+v before generic=%+v",
			started[0].Request,
			control,
			generic,
		)
	}
	if supervisor.Active() != activeGeneric+1 {
		t.Fatalf("active=%d want=%d", supervisor.Active(), activeGeneric+1)
	}

	close(release)
	started = [TaskStartServiceQuantum]TaskStart{}
	count, more, err = supervisor.Dispatch(
		context.Background(),
		1,
		&started,
	)
	if err != nil || count != 1 || more || started[0].Request != generic {
		t.Fatalf(
			"remaining generic dispatch differs: count=%d more=%v start=%+v err=%v",
			count,
			more,
			started[0],
			err,
		)
	}
	for range activeGeneric + 2 {
		completion := <-supervisor.CompletionCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose,
		}); err != nil {
			t.Fatal(err)
		}
		ack := <-supervisor.AcknowledgementCh()
		if ack.Err != nil {
			t.Fatal(ack.Err)
		}
		terminateAndReleaseTask(t, supervisor, ack.Ref, 3)
	}
	if supervisor.Active() != 0 || supervisor.Pending() != 0 {
		t.Fatalf(
			"terminal task census active=%d pending=%d",
			supervisor.Active(),
			supervisor.Pending(),
		)
	}
}

func TestTaskSupervisorDirectlyCancelsPendingRequest(t *testing.T) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
	plan := func(source Source) TaskPlan {
		return TaskPlan{Source: source, Work: FrameTaskWork(func(context.Context) (SealedResult, error) {
			return NewSealedResult(200, "application/json", []byte(`{}`))
		})}
	}
	cancelled, err := supervisor.Enqueue(
		TaskClassFrameworkControl,
		plan(SourceJobManager),
	)
	if err != nil {
		t.Fatal(err)
	}
	survivor, err := supervisor.Enqueue(
		TaskClassGenericFunction,
		plan(SourceFunction),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := supervisor.CancelPending(cancelled); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.CancelPending(cancelled); err == nil {
		t.Fatal("stale pending-task cancellation was accepted")
	}
	var started [TaskStartServiceQuantum]TaskStart
	count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
	if err != nil || count != 1 || more || started[0].Request != survivor {
		t.Fatalf("pending cancellation survivor differs: started=%#v count=%d more=%v err=%v", started[0], count, more, err)
	}
	completion := <-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: completion.Ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if err := supervisor.SendAction(TaskAction{Ref: ack.Ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	ack = <-supervisor.AcknowledgementCh()
	if err := supervisor.Release(ack.Ref); err != nil {
		t.Fatal(err)
	}
}

func enqueueAndDispatchTask(t *testing.T, supervisor *TaskSupervisor, plan TaskPlan) (TaskRequestRef, TaskRef) {
	t.Helper()
	class := TaskClassFrameworkControl
	if plan.Source == SourceFunction {
		class = TaskClassGenericFunction
	}
	request, err := supervisor.Enqueue(class, plan)
	if err != nil {
		t.Fatal(err)
	}
	var started [TaskStartServiceQuantum]TaskStart
	count, _, err := supervisor.Dispatch(context.Background(), 1, &started)
	if err != nil || count != 1 || started[0].Request != request {
		t.Fatalf("task dispatch differs: started=%#v count=%d err=%v", started[0], count, err)
	}
	return request, started[0].Task
}

func TestFrameOwnerControlReservationPrecedesLaterOrdinaryFrame(t *testing.T) {
	writer := newStepWriter()
	controlReady := make(chan struct{}, 1)
	owner, err := NewFrameOwner(writer)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.BindControlReady(func() { controlReady <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	result, err := NewSealedResult(200, "application/json", []byte(`{"status":200}`))
	if err != nil {
		t.Fatal(err)
	}
	first, err := PrepareFrame("u1", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PrepareFrame("u2", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- owner.Commit(first) }()
	if got := <-writer.offered; !bytes.Contains(got, []byte("FUNCTION_RESULT_BEGIN u1 ")) {
		t.Fatalf("first frame differs: %q", got)
	}
	if err := owner.TryCommitControl(ControlFramePlan{UID: "uc", Status: ControlDeadline, Expiry: 1}); !errors.Is(err, ErrFrameOwnerBusy) {
		t.Fatalf("busy control result differs: %v", err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- owner.Commit(second) }()
	select {
	case got := <-writer.offered:
		t.Fatalf("later ordinary frame bypassed pending control: %q", got)
	case <-time.After(20 * time.Millisecond):
	}
	writer.release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	<-controlReady
	controlDone := make(chan error, 1)
	go func() {
		controlDone <- owner.TryCommitControl(ControlFramePlan{UID: "uc", Status: ControlDeadline, Expiry: 1})
	}()
	if got := <-writer.offered; !bytes.Contains(got, []byte("FUNCTION_RESULT_BEGIN uc 504 ")) {
		t.Fatalf("control frame differs: %q", got)
	}
	writer.release <- struct{}{}
	if err := <-controlDone; err != nil {
		t.Fatal(err)
	}
	if got := <-writer.offered; !bytes.Contains(got, []byte("FUNCTION_RESULT_BEGIN u2 ")) {
		t.Fatalf("second frame differs: %q", got)
	}
	writer.release <- struct{}{}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestFrameOwnerLateControlReadyBindingReplaysPendingWake(t *testing.T) {
	writer := newStepWriter()
	owner, err := NewFrameOwner(writer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := PrepareFrame("ordinary", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	committed := make(chan error, 1)
	go func() { committed <- owner.Commit(frame) }()
	<-writer.offered
	if err := owner.TryCommitControl(ControlFramePlan{
		UID: "control", Status: ControlDeadline, Expiry: 1,
	}); !errors.Is(err, ErrFrameOwnerBusy) {
		t.Fatalf("busy control result differs: %v", err)
	}
	writer.release <- struct{}{}
	if err := <-committed; err != nil {
		t.Fatal(err)
	}

	ready := make(chan struct{}, 1)
	if err := owner.BindControlReady(func() { ready <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	default:
		t.Fatal("late binding lost pending control wake")
	}
	if err := owner.BindControlReady(func() {}); err == nil {
		t.Fatal("duplicate control-ready binding succeeded")
	}
}

func TestFrameOwnerShortWritePoisonsAndRetains(t *testing.T) {
	owner, err := NewFrameOwner(shortWriter{})
	if err != nil {
		t.Fatal(err)
	}
	poisoned := make(chan error, 1)
	if err := owner.BindPoisoned(func(err error) { poisoned <- err }); err != nil {
		t.Fatal(err)
	}
	result, err := NewSealedResult(200, "application/json", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	frame, err := PrepareFrame("u", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Commit(frame); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write result differs: %v", err)
	}
	select {
	case err := <-poisoned:
		if !errors.Is(err, ErrFrameOwnerPoisoned) || !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("poison notification differs: %v", err)
		}
	default:
		t.Fatal("short write did not publish poison")
	}
	census := owner.Census()
	if !census.Poisoned || census.RetainedBytes == 0 {
		t.Fatalf("poison census differs: %#v", census)
	}
	next, err := PrepareFrame("next", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Commit(next); !errors.Is(err, ErrFrameOwnerPoisoned) {
		t.Fatalf("post-poison commit differs: %v", err)
	}
}

func TestFrameOwnerRunNotificationLeaseCanMoveAfterExactRelease(t *testing.T) {
	owner, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	notify := func() {}
	poison := func(error) {}
	if err := owner.BindRunNotifications(1, notify, poison); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		run func() error
	}{
		"duplicate live bind": {
			run: func() error {
				return owner.BindRunNotifications(2, notify, poison)
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
			if err := test.run(); err == nil {
				t.Fatal("invalid notification lease transition succeeded")
			}
		})
	}
	if err := owner.ReleaseRunNotifications(1); err != nil {
		t.Fatal(err)
	}
	if err := owner.BindRunNotifications(2, notify, poison); err != nil {
		t.Fatal(err)
	}
	if err := owner.ReleaseRunNotifications(2); err != nil {
		t.Fatal(err)
	}
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
		if err != nil || result.status != int(status) || result.contentType != "application/json" || string(result.payload) != payload {
			t.Fatalf("control result differs for %d: result=%#v err=%v", status, result, err)
		}
	}
	if _, err := NewControlResult(418); err == nil {
		t.Fatal("unknown control result was accepted")
	}
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
			if (err == nil) != test.valid {
				t.Fatalf("Function payload boundary differs for %d: %v", test.length, err)
			}
		})
	}
	payload := bytes.Repeat([]byte{'x'}, 1024*1024)
	result, err := NewSealedResult(200, "application/json", payload)
	if err != nil {
		t.Fatalf("1 MiB Function result was limited by the control reserve: %v", err)
	}
	owner, err := NewFrameOwner(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := PrepareFrame("u-large", result, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Commit(frame); err != nil {
		t.Fatal(err)
	}
	if census := owner.Census(); census.Commits != 1 || census.RetainedBytes != 0 {
		t.Fatalf("large Function frame census differs: %#v", census)
	}
}

func TestFunctionFrameSizePreflightsBeforeAppend(t *testing.T) {
	baseFrame, baseEnvelope, err := functionFrameSize("u", 200, "application/json", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	exactUID := strings.Repeat("u", 1+FunctionEnvelopeBytes-baseEnvelope)
	exactFrame, exactEnvelope, err := functionFrameSize(exactUID, 200, "application/json", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if exactEnvelope != FunctionEnvelopeBytes || exactFrame != baseFrame+len(exactUID)-1 {
		t.Fatalf("exact envelope sizing differs: frame=%d envelope=%d", exactFrame, exactEnvelope)
	}
	seed := []byte("unchanged")
	encoded, err := encodeResult(seed, exactUID+"u", 200, "application/json", 1, nil, MaximumFunctionFrameBytes, FunctionEnvelopeBytes, FunctionPayloadBytes)
	if !errors.Is(err, ErrFunctionResultTooLarge) || !bytes.Equal(encoded, seed) {
		t.Fatalf("oversized envelope appended a partial prefix: bytes=%q err=%v", encoded, err)
	}

	payload := []byte(`{"status":200}`)
	wantSize, _, err := functionFrameSize("u-size", 200, "application/json", 1, len(payload))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = encodeResult(nil, "u-size", 200, "application/json", 1, payload, MaximumFunctionFrameBytes, FunctionEnvelopeBytes, FunctionPayloadBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != wantSize {
		t.Fatalf("Size/Append identity differs: size=%d appended=%d", wantSize, len(encoded))
	}
}

type stepWriter struct {
	offered chan []byte
	release chan struct{}
	mu      sync.Mutex
}

func newStepWriter() *stepWriter {
	return &stepWriter{offered: make(chan []byte), release: make(chan struct{})}
}

func (writer *stepWriter) Write(payload []byte) (int, error) {
	copy := append([]byte(nil), payload...)
	writer.offered <- copy
	<-writer.release
	return len(payload), nil
}

type shortWriter struct{}

func (shortWriter) Write(payload []byte) (int, error) {
	return len(payload) - 1, nil
}
