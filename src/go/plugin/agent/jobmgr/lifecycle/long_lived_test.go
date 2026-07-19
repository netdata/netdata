// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestPipelinePermitConservesLifecycleFacetsWithoutAdmissionCharge(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan(
		[]string{"file", "service-discovery"},
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := grantLongLivedTestAdmission(t, admission, plan.Bytes())
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, owner, plan)
	if err != nil {
		t.Fatal(err)
	}
	if census := admission.Census(); census.OrdinaryBytes != 1 ||
		census.LongLivedRecords != 0 ||
		census.LongLivedBytes != 0 {
		t.Fatalf("admission after transfer=%+v", census)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{
		Active: 1, Pipelines: 1,
		GReserved: 3, ExternalReserved: 1,
	}) {
		t.Fatalf("permit after transfer=%+v", census)
	}
	slot := supervisor.longLived.slots[permit.ref.Slot]
	if slot.admission != nil || slot.admissionRef.Valid() || slot.bytes != 0 {
		t.Fatalf(
			"charge-free Pipeline retained admission authority: admission=%p ref=%+v bytes=%d",
			slot.admission,
			slot.admissionRef,
			slot.bytes,
		)
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
	if _, err := supervisor.StartInheritedWithPermitKey(
		context.Background(),
		owner,
		InheritedPipelineProvider,
		"unknown",
		permit,
		func(context.Context) error { return nil },
	); err == nil {
		t.Fatal("unknown provider key was accepted")
	}
	if census := supervisor.LongLivedCensus(); census.GReserved != 3 ||
		census.GActive != 0 {
		t.Fatalf("unknown key changed facets=%+v", census)
	}

	refs := make(map[string]InheritedTaskRef)
	child, err := supervisor.StartInheritedWithPermit(
		context.Background(),
		owner,
		InheritedPipelineSupervisor,
		permit,
		func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	refs["supervisor"] = child
	for name := range map[string]struct{}{"file": {}, "service-discovery": {}} {
		child, err := supervisor.StartInheritedWithPermitKey(
			context.Background(),
			owner,
			InheritedPipelineProvider,
			name,
			permit,
			func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		refs[name] = child
	}
	if _, err := supervisor.StartInheritedWithPermitKey(
		context.Background(),
		owner,
		InheritedPipelineProvider,
		"file",
		permit,
		func(context.Context) error { return nil },
	); err == nil {
		t.Fatal("duplicate provider key was accepted")
	}
	if census := supervisor.LongLivedCensus(); census.GReserved != 0 || census.GActive != 3 || census.ExternalReserved != 0 || census.ExternalActive != 1 {
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
	if census := admission.Census(); census.OrdinaryBytes != 1 ||
		census.LongLivedRecords != 0 || census.LongLivedBytes != 0 {
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

func TestPipelineLongLivedPlanProviderKeys(t *testing.T) {
	tests := map[string]struct {
		keys      []string
		wantBytes int64
		wantErr   bool
	}{
		"one provider": {
			keys:      []string{"file"},
			wantBytes: 0,
		},
		"several providers": {
			keys:      []string{"service-discovery", "file", "dummy"},
			wantBytes: 0,
		},
		"empty": {
			wantErr: true,
		},
		"blank": {
			keys:    []string{""},
			wantErr: true,
		},
		"whitespace": {
			keys:    []string{" file"},
			wantErr: true,
		},
		"duplicate": {
			keys:    []string{"file", "file"},
			wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plan, err := NewPipelineLongLivedPlan(test.keys)
			if test.wantErr {
				if err == nil {
					t.Fatalf("plan=%+v, want error", plan)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Bytes() != test.wantBytes {
				t.Fatalf(
					"bytes=%d, want %d",
					plan.Bytes(),
					test.wantBytes,
				)
			}
		})
	}
}

func TestLongLivedResourcePlansChargeDeclaredBytes(t *testing.T) {
	tests := map[string]struct {
		newPlan func(int64) (LongLivedPlan, error)
	}{
		"job": {
			newPlan: NewJobLongLivedPlan,
		},
		"secret store": {
			newPlan: NewSecretStoreLongLivedPlan,
		},
		"secret store replacement": {
			newPlan: NewSecretStoreReplacementLongLivedPlan,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			const retainedBytes = int64(73)
			plan, err := test.newPlan(retainedBytes)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Bytes() != retainedBytes {
				t.Fatalf(
					"retained bytes=%d, want %d",
					plan.Bytes(),
					retainedBytes,
				)
			}
			if plan, err := test.newPlan(0); err == nil {
				t.Fatalf("zero-byte resource plan was accepted: %+v", plan)
			}
		})
	}
}

func TestZeroChargePipelinePermitRequiresLiveAdmissionAuthority(t *testing.T) {
	tests := map[string]struct {
		ref     func(*testing.T, *AdmissionLedger) AdmissionRef
		cleanup func(*testing.T, *AdmissionLedger, AdmissionRef)
	}{
		"fabricated reference": {
			ref: func(*testing.T, *AdmissionLedger) AdmissionRef {
				return AdmissionRef{Slot: inputBodyRecordSlot + 1, Generation: 1}
			},
		},
		"waiting reference": {
			ref: func(t *testing.T, admission *AdmissionLedger) AdmissionRef {
				t.Helper()
				requested := admission.RequestOrdinary(
					1,
					AdmissionLaneRef{Slot: 1, Generation: 1},
					1,
				)
				if requested.Rejected != nil {
					t.Fatal(requested.Rejected)
				}
				return requested.Ref
			},
			cleanup: func(
				t *testing.T,
				admission *AdmissionLedger,
				ref AdmissionRef,
			) {
				t.Helper()
				if err := admission.CancelWaiting(ref); err != nil {
					t.Fatal(err)
				}
			},
		},
		"stale reference": {
			ref: func(t *testing.T, admission *AdmissionLedger) AdmissionRef {
				t.Helper()
				ref := grantLongLivedTestAdmission(t, admission, 0)
				if _, err := admission.ReleaseOrdinary(ref); err != nil {
					t.Fatal(err)
				}
				return ref
			},
		},
		"already transferred reference": {
			ref: func(t *testing.T, admission *AdmissionLedger) AdmissionRef {
				t.Helper()
				ref := grantLongLivedTestAdmission(t, admission, 1)
				if err := admission.transferLongLived(ref, 1); err != nil {
					t.Fatal(err)
				}
				return ref
			},
			cleanup: func(
				t *testing.T,
				admission *AdmissionLedger,
				ref AdmissionRef,
			) {
				t.Helper()
				if _, err := admission.releaseLongLived(ref, 1); err != nil {
					t.Fatal(err)
				}
				if _, err := admission.ReleaseOrdinary(ref); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			admission := NewAdmissionLedger()
			supervisor := newLongLivedTestSupervisor(t)
			plan, err := NewPipelineLongLivedPlan([]string{"provider"})
			if err != nil {
				t.Fatal(err)
			}
			ref := test.ref(t, admission)
			if _, err := supervisor.IssueLongLivedPermit(
				admission,
				ref,
				ResourceIdentity{ID: "pipeline", Generation: 1},
				plan,
			); err == nil {
				t.Fatal("zero-charge Pipeline accepted invalid admission authority")
			}
			if test.cleanup != nil {
				test.cleanup(t, admission, ref)
			}
			if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
				t.Fatalf("failed issue retained lifecycle ownership: %+v", census)
			}
			if census := admission.Census(); census.ActiveRecords != 0 ||
				census.OrdinaryBytes != 0 {
				t.Fatalf("failed issue retained admission: %+v", census)
			}
		})
	}
}

func TestPipelinePermitReleasesDisabledProviderClaim(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan([]string{"disabled", "enabled"})
	if err != nil {
		t.Fatal(err)
	}
	admissionRef := grantLongLivedTestAdmission(
		t,
		admission,
		plan.Bytes(),
	)
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	permit, err := supervisor.IssueLongLivedPermit(
		admission,
		admissionRef,
		owner,
		plan,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.ReleaseUnusedInherited(
		InheritedPipelineProvider,
		"disabled",
	); err != nil {
		t.Fatal(err)
	}
	if err := permit.ReleaseUnusedInherited(
		InheritedPipelineProvider,
		"disabled",
	); err == nil {
		t.Fatal("disabled provider claim released twice")
	}
	if census := supervisor.LongLivedCensus(); census.GReserved != 2 {
		t.Fatalf("permit after disabled release=%+v", census)
	}
	if err := permit.AbortUnused(); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
		t.Fatal(err)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("final permit census=%+v", census)
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.OrdinaryBytes != 0 {
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
	pipelinePlan, err := NewPipelineLongLivedPlan([]string{"provider"})
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
		ref := grantLongLivedTestAdmission(t, admission, plan.Bytes())
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
		census.LongLivedRecords != jobs ||
		census.OrdinaryBytes != int64(jobs)*jobPlan.Bytes() {
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

func TestSecretStoreSteadyPermitsHaveNoConfiguredStoreCountLimit(t *testing.T) {
	const stores = 9

	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewSecretStoreLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
	permits := make([]LongLivedPermit, 0, stores)
	for index := range stores {
		ref := grantLongLivedTestAdmission(t, admission, 2)
		permit, issueErr := supervisor.IssueLongLivedPermit(
			admission,
			ref,
			ResourceIdentity{
				ID:         fmt.Sprintf("secret-store-%02d", index),
				Generation: 1,
			},
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
	if census := supervisor.LongLivedCensus(); census.Active != stores ||
		census.SecretStores != stores ||
		census.Bytes != stores*plan.Bytes() {
		t.Fatalf("steady Store permit census=%+v", census)
	}
	for _, permit := range permits {
		if err := permit.AbortUnused(); err != nil {
			t.Fatal(err)
		}
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("final Store permit census=%+v", census)
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
		byteCount+1,
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
