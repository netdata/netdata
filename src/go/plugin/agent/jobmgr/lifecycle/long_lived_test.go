// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestLongLivedPermitConservesAdmittedBGEFacets(t *testing.T) {
	admission := NewAdmissionLedger()
	ref := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, owner, plan)
	if err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.OrdinaryBytes != 100+TaskChildExecutionBytes ||
		census.LongLivedRecords != 1 ||
		census.LongLivedBytes != plan.Bytes() {
		t.Fatalf("admission after transfer=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{
		Active: 1, Pipelines: 1, Bytes: plan.Bytes(),
		GReserved: 2, ExternalReserved: 1,
	}) {
		t.Fatalf("permit after transfer=%+v", census)
	}
	if err := permit.Return(); err == nil {
		t.Fatal("permit returned with retained facets")
	}

	wrongOwner := permit
	wrongOwner.owner = ResourceIdentity{ID: "other", Generation: 1}
	if err := wrongOwner.ActivateExternal(LongLivedEProvider); err == nil {
		t.Fatal("cross-owner external activation succeeded")
	}
	if err := permit.ActivateExternal(LongLivedEProvider); err != nil {
		t.Fatal(err)
	}
	if err := permit.ActivateExternal(LongLivedEProvider); err == nil {
		t.Fatal("duplicate external activation succeeded")
	}

	roles := []InheritedTaskRole{InheritedPipelineSupervisor, InheritedPipelineProvider}
	refs := make([]InheritedTaskRef, 0, len(roles))
	for _, role := range roles {
		child, err := supervisor.StartInheritedWithPermit(context.Background(), owner, role, permit, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, child)
	}
	if census := supervisor.LongLivedCensus(); census.GReserved != 0 || census.GActive != 2 || census.ExternalReserved != 0 || census.ExternalActive != 1 {
		t.Fatalf("active facets=%+v", census)
	}
	if err := permit.ReleaseExternal(LongLivedEProvider); err != nil {
		t.Fatal(err)
	}
	for _, child := range refs {
		if err := supervisor.CancelInherited(child, owner); err != nil {
			t.Fatal(err)
		}
		if joined, err := supervisor.JoinInherited(context.Background(), child, owner); err != nil || !joined {
			t.Fatalf("joined=%v err=%v", joined, err)
		}
		if err := supervisor.ReleaseInherited(child, owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := permit.ReleaseBytes(); err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.OrdinaryBytes != 60 || census.LongLivedRecords != 0 || census.LongLivedBytes != 0 {
		t.Fatalf("admission after persistent release=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census.Active != 1 || census.Bytes != 0 || census.GActive != 0 || census.ExternalActive != 0 {
		t.Fatalf("permit before return=%+v", census)
	}
	if err := permit.Return(); err != nil {
		t.Fatal(err)
	}
	if err := permit.Return(); err == nil {
		t.Fatal("stale permit returned twice")
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("final permit census=%+v", census)
	}
	if _, err := admission.ReleaseOrdinary(ref); err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.OrdinaryBytes != 0 {
		t.Fatalf("final admission census=%+v", census)
	}
}

func TestLongLivedPermitSurvivesOperationAdmissionRelease(t *testing.T) {
	admission := NewAdmissionLedger()
	ref := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewJobLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, ResourceIdentity{ID: "job", Generation: 1}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(ref); err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.FreeRecords == 0 ||
		census.OrdinaryGranted != 0 ||
		census.OrdinaryBytes != plan.Bytes() ||
		census.LongLivedBytes != plan.Bytes() {
		t.Fatalf("persistent-only admission=%+v", census)
	}
	if err := permit.ReleaseExternal(LongLivedEJobResources); err != nil {
		t.Fatal(err)
	}
	if err := permit.ReleaseBytes(); err != nil {
		t.Fatal(err)
	}
	if err := permit.Return(); err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.OrdinaryBytes != 0 || census.LongLivedRecords != 0 {
		t.Fatalf("final admission=%+v", census)
	}
}

func TestLongLivedPermitDomainsGrowBeyondFormerJobLimit(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	pipelinePlan, err := NewPipelineLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
	jobPlan, err := NewJobLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}

	const jobs = formerFixedPopulation + 1
	permits := make([]LongLivedPermit, 0, jobs+1)
	issue := func(owner ResourceIdentity, plan LongLivedPlan) {
		t.Helper()
		ref := grantLongLivedTestAdmission(t, admission, 2)
		permit, issueErr := supervisor.IssueLongLivedPermit(
			admission,
			ref,
			owner,
			plan,
		)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		if _, releaseErr := admission.ReleaseOrdinary(ref); releaseErr != nil {
			t.Fatal(releaseErr)
		}
		if census := admission.Census(); census.ActiveRecords != 0 ||
			census.FreeRecords == 0 {
			t.Fatalf(
				"persistent resource retained operation admission: %+v",
				census,
			)
		}
		permits = append(permits, permit)
	}

	issue(
		ResourceIdentity{ID: "discovery", Generation: 1},
		pipelinePlan,
	)
	for index := 0; index < jobs; index++ {
		issue(
			ResourceIdentity{
				ID:         fmt.Sprintf("job-%03d", index),
				Generation: 1,
			},
			jobPlan,
		)
	}

	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.LongLivedRecords != jobs+1 ||
		census.OrdinaryBytes != int64(jobs+1)*jobPlan.Bytes() {
		t.Fatalf("separated admission census=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census.Active != jobs+1 {
		t.Fatalf("separated permit census=%+v", census)
	}

	for _, permit := range permits {
		if err := permit.AbortUnused(); err != nil {
			t.Fatal(err)
		}
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.LongLivedRecords != 0 ||
		census.OrdinaryBytes != 0 {
		t.Fatalf("final separated admission census=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("final separated permit census=%+v", census)
	}
}

func TestSecretStoreReplacementPermitsGrowBeyondFormerOverlapLimit(t *testing.T) {
	const replacements = 9

	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	steadyPlan, err := NewSecretStoreLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
	replacementPlan, err := NewSecretStoreReplacementLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}

	permits := make([]LongLivedPermit, 0, replacements+1)
	issue := func(owner ResourceIdentity, plan LongLivedPlan) {
		t.Helper()
		ref := grantLongLivedTestAdmission(t, admission, 2)
		permit, issueErr := supervisor.IssueLongLivedPermit(
			admission,
			ref,
			owner,
			plan,
		)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		if _, releaseErr := admission.ReleaseOrdinary(ref); releaseErr != nil {
			t.Fatal(releaseErr)
		}
		permits = append(permits, permit)
	}

	issue(
		ResourceIdentity{ID: "secret-store", Generation: 1},
		steadyPlan,
	)
	for index := 0; index < replacements; index++ {
		issue(
			ResourceIdentity{
				ID:         fmt.Sprintf("secret-store-replacement-%02d", index),
				Generation: 1,
			},
			replacementPlan,
		)
	}

	if census := supervisor.LongLivedCensus(); census.Active != replacements+1 ||
		census.SecretStores != replacements+1 ||
		census.Bytes != int64(replacements+1)*steadyPlan.Bytes() {
		t.Fatalf("replacement overlap census=%+v", census)
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.LongLivedRecords != replacements+1 ||
		census.OrdinaryBytes != int64(replacements+1)*steadyPlan.Bytes() {
		t.Fatalf("replacement overlap admission=%+v", census)
	}
	for _, permit := range permits {
		if err := permit.AbortUnused(); err != nil {
			t.Fatal(err)
		}
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.LongLivedRecords != 0 ||
		census.OrdinaryBytes != 0 {
		t.Fatalf("final replacement admission=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("final replacement census=%+v", census)
	}
}

func TestLongLivedSecretStorePermitRejectsByteReleaseBeforeExternalRelease(t *testing.T) {
	admission := NewAdmissionLedger()
	ref := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewSecretStoreLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, ResourceIdentity{ID: "secret-store", Generation: 1}, plan)
	if err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func(wantAdmission AdmissionCensus, wantLongLived LongLivedCensus) {
		t.Helper()
		if got := admission.Census(); got != wantAdmission {
			t.Fatalf("premature byte release changed Admission: got=%+v want=%+v", got, wantAdmission)
		}
		if got := supervisor.LongLivedCensus(); got != wantLongLived {
			t.Fatalf("premature byte release changed long-lived census: got=%+v want=%+v", got, wantLongLived)
		}
	}
	wantAdmission := admission.Census()
	wantLongLived := supervisor.LongLivedCensus()
	if err := permit.ReleaseBytes(); err == nil {
		t.Fatal("released SecretStore bytes while the external facet was reserved")
	}
	assertUnchanged(wantAdmission, wantLongLived)
	if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
		t.Fatal(err)
	}
	wantAdmission = admission.Census()
	wantLongLived = supervisor.LongLivedCensus()
	if err := permit.ReleaseBytes(); err == nil {
		t.Fatal("released SecretStore bytes while the external facet was active")
	}
	assertUnchanged(wantAdmission, wantLongLived)
	if err := permit.ReleaseExternal(LongLivedESecretStore); err != nil {
		t.Fatal(err)
	}
	if err := permit.ReleaseBytes(); err != nil {
		t.Fatal(err)
	}
	if err := permit.Return(); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(ref); err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.ActiveRecords != 0 || census.OrdinaryBytes != 0 {
		t.Fatalf("final Admission census=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("final long-lived census=%+v", census)
	}
}

func grantLongLivedTestAdmission(t *testing.T, ledger *AdmissionLedger, byteCount int64) AdmissionRef {
	t.Helper()
	requested := ledger.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 1, Generation: 1},
		byteCount+TaskChildExecutionBytes,
	)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf("grant count=%d grant=%+v err=%v", count, grants[0], err)
	}
	return requested.Ref
}

func TestLongLivedByteReleaseSignalsNewAdmissionCapacity(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	ready := make(chan struct{}, 1)
	if err := supervisor.BindAdmissionReady(func() { ready <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.BindAdmissionReady(func() {}); err == nil {
		t.Fatal("duplicate admission-ready binding succeeded")
	}
	admission := NewAdmissionLedger()
	ownerRef := grantLongLivedTestAdmission(t, admission, 100)
	plan, err := NewJobLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := supervisor.IssueLongLivedPermit(
		admission, ownerRef, ResourceIdentity{ID: "owner", Generation: 1}, plan,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(ownerRef); err != nil {
		t.Fatal(err)
	}
	blocker := admission.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 2, Generation: 1},
		OrdinaryBudgetBytes-plan.Bytes(),
	)
	waiter := admission.RequestOrdinary(1, AdmissionLaneRef{Slot: 3, Generation: 1}, 1)
	if blocker.Rejected != nil || waiter.Rejected != nil {
		t.Fatalf("saturation setup differs: blocker=%v waiter=%v", blocker.Rejected, waiter.Rejected)
	}
	var grants [4]AdmissionGrant
	count, _, err := admission.TakeGrants(2, &grants)
	if err != nil || count != 1 || grants[0].Ref != blocker.Ref {
		t.Fatalf("saturation grant differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	if err := permit.ReleaseExternal(LongLivedEJobResources); err != nil {
		t.Fatal(err)
	}
	if err := permit.ReleaseBytes(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	default:
		t.Fatal("long-lived byte release lost admission wake")
	}
	if err := permit.Return(); err != nil {
		t.Fatal(err)
	}
	count, _, err = admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != waiter.Ref {
		t.Fatalf("released-capacity grant differs: count=%d grant=%#v err=%v", count, grants[0], err)
	}
	if _, err := admission.ReleaseOrdinary(waiter.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(blocker.Ref); err != nil {
		t.Fatal(err)
	}
}

func newLongLivedTestSupervisor(t *testing.T) *TaskSupervisor {
	t.Helper()
	frame, err := NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		t.Fatal(err)
	}
	return supervisor
}
