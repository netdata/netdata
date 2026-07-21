// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelinePermitConservesLifecycleFacetsWithoutAdmissionCharge(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan(
		[]string{"file", "service-discovery"},
	)
	require.NoError(t, err)
	ref := grantLongLivedTestAdmission(t, admission, plan.Bytes())
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, owner, plan)
	require.NoError(t, err)

	census := admission.Census()
	require.False(t, census.OrdinaryBytes != 1 || census.LongLivedRecords != 0 || census.LongLivedBytes != 0)

	require.EqualValues(t, LongLivedCensus{
		Active: 1, Pipelines: 1,
		GReserved: 3, ExternalReserved: 1,
	}, supervisor.LongLivedCensus(),
	)

	slot := supervisor.longLived.slots[permit.ref.Slot]
	require.False(t, slot.admission != nil || slot.admissionRef.Valid() || slot.bytes != 0)

	require.Error(t, permit.Return())

	wrongOwner := permit
	wrongOwner.owner = ResourceIdentity{ID: "other", Generation: 1}

	require.Error(t, wrongOwner.ActivateExternal(LongLivedEProvider))

	require.NoError(t, permit.ActivateExternal(LongLivedEProvider))

	require.Error(t, permit.ActivateExternal(LongLivedEProvider))

	_, startInheritedWithPermitKeyErr := supervisor.StartInheritedWithPermitKey(
		context.Background(),
		owner,
		InheritedPipelineProvider,
		"unknown",
		permit,
		func(context.Context) error { return nil },
	)
	require.Error(t, startInheritedWithPermitKeyErr)

	longLivedCensus := supervisor.LongLivedCensus()
	require.False(t, longLivedCensus.GReserved != 3 || longLivedCensus.GActive != 0)

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
	require.NoError(t, err)
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
		require.NoError(t, err)
		refs[name] = child
	}

	_, startInheritedWithPermitKeyErr2 := supervisor.StartInheritedWithPermitKey(
		context.Background(),
		owner,
		InheritedPipelineProvider,
		"file",
		permit,
		func(context.Context) error { return nil },
	)
	require.Error(t, startInheritedWithPermitKeyErr2)

	longLivedCensus2 := supervisor.LongLivedCensus()
	require.False(t, longLivedCensus2.GReserved != 0 || longLivedCensus2.GActive != 3 || longLivedCensus2.ExternalReserved != 0 || longLivedCensus2.ExternalActive != 1)

	require.NoError(t, permit.ReleaseExternal(LongLivedEProvider))

	for _, child := range refs {
		require.NoError(t, supervisor.CancelInherited(child, owner))

		joinInheritedJoined, joinInheritedErr := supervisor.JoinInherited(context.Background(), child, owner)
		require.False(t, joinInheritedErr != nil || !joinInheritedJoined)

		require.NoError(t, supervisor.ReleaseInherited(child, owner))
	}

	require.NoError(t, permit.ReleaseBytes())

	admissionCensus := admission.Census()
	require.False(t, admissionCensus.OrdinaryBytes != 1 ||
		admissionCensus.LongLivedRecords != 0 || admissionCensus.LongLivedBytes != 0)

	longLivedCensus3 := supervisor.LongLivedCensus()
	require.False(t, longLivedCensus3.Active != 1 || longLivedCensus3.Bytes != 0 || longLivedCensus3.GActive != 0 || longLivedCensus3.ExternalActive != 0)

	require.NoError(t, permit.Return())

	require.Error(t, permit.Return())

	require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(ref)
	require.NoError(t, releaseOrdinaryErr)

	admissionCensus2 := admission.Census()
	require.False(t, admissionCensus2.ActiveRecords != 0 || admissionCensus2.OrdinaryBytes != 0)

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
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, test.wantBytes, plan.Bytes())
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
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			const retainedBytes = int64(73)
			plan, err := test.newPlan(retainedBytes)
			require.NoError(t, err)
			require.EqualValues(t, retainedBytes, plan.Bytes())

			newPlan2, newPlanErr := test.newPlan(0)
			require.Error(t, newPlanErr, "plan=%+v", newPlan2)

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
				require.Nil(t, requested.Rejected)
				return requested.Ref
			},
			cleanup: func(
				t *testing.T,
				admission *AdmissionLedger,
				ref AdmissionRef,
			) {
				t.Helper()

				require.NoError(t, admission.CancelWaiting(ref))
			},
		},
		"stale reference": {
			ref: func(t *testing.T, admission *AdmissionLedger) AdmissionRef {
				t.Helper()
				ref := grantLongLivedTestAdmission(t, admission, 0)

				_, err := admission.ReleaseOrdinary(ref)
				require.NoError(t, err)

				return ref
			},
		},
		"already transferred reference": {
			ref: func(t *testing.T, admission *AdmissionLedger) AdmissionRef {
				t.Helper()
				ref := grantLongLivedTestAdmission(t, admission, 1)

				require.NoError(t, admission.transferLongLived(ref, 1))

				return ref
			},
			cleanup: func(
				t *testing.T,
				admission *AdmissionLedger,
				ref AdmissionRef,
			) {
				t.Helper()

				_, err := admission.releaseLongLived(ref, 1)
				require.NoError(t, err)

				_, releaseOrdinaryErr := admission.ReleaseOrdinary(ref)
				require.NoError(t, releaseOrdinaryErr)

			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			admission := NewAdmissionLedger()
			supervisor := newLongLivedTestSupervisor(t)
			plan, err := NewPipelineLongLivedPlan([]string{"provider"})
			require.NoError(t, err)
			ref := test.ref(t, admission)

			_, issueLongLivedPermitErr := supervisor.IssueLongLivedPermit(
				admission,
				ref,
				ResourceIdentity{ID: "pipeline", Generation: 1},
				plan,
			)
			require.Error(t, issueLongLivedPermitErr)

			if test.cleanup != nil {
				test.cleanup(t, admission, ref)
			}

			require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())

			census := admission.Census()
			require.False(t, census.ActiveRecords != 0 || census.OrdinaryBytes != 0)
		})
	}
}

func TestPipelinePermitReleasesDisabledProviderClaim(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan([]string{"disabled", "enabled"})
	require.NoError(t, err)
	admissionRef := grantLongLivedTestAdmission(t, admission, plan.Bytes())
	owner := ResourceIdentity{ID: "pipeline", Generation: 1}
	permit, err := supervisor.IssueLongLivedPermit(
		admission,
		admissionRef,
		owner,
		plan,
	)
	require.NoError(t, err)

	require.NoError(t, permit.ReleaseUnusedInherited(InheritedPipelineProvider, "disabled"))

	require.Error(t, permit.ReleaseUnusedInherited(InheritedPipelineProvider, "disabled"))

	require.EqualValues(t, 2, supervisor.LongLivedCensus().GReserved)

	require.NoError(t, permit.AbortUnused())

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, releaseOrdinaryErr)

	require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())

	census := admission.Census()
	require.False(t, census.ActiveRecords != 0 || census.OrdinaryBytes != 0)
}

func TestLongLivedPermitSurvivesOperationAdmissionRelease(t *testing.T) {
	admission := NewAdmissionLedger()
	ref := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewJobLongLivedPlan(40)
	require.NoError(t, err)
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, ResourceIdentity{ID: "job", Generation: 1}, plan)
	require.NoError(t, err)

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(ref)
	require.NoError(t, releaseOrdinaryErr)

	census := admission.Census()
	require.False(t, census.ActiveRecords != 0 ||
		census.FreeRecords == 0 ||
		census.OrdinaryGranted != 0 ||
		census.OrdinaryBytes != plan.Bytes() ||
		census.LongLivedBytes != plan.Bytes())

	require.NoError(t, permit.ReleaseExternal(LongLivedEJobResources))

	require.NoError(t, permit.ReleaseBytes())

	require.NoError(t, permit.Return())

	admissionCensus := admission.Census()
	require.False(t, admissionCensus.ActiveRecords != 0 || admissionCensus.OrdinaryBytes != 0 || admissionCensus.LongLivedRecords != 0)

}

func TestLongLivedPermitDomainsGrowBeyondFormerJobLimit(t *testing.T) {
	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	pipelinePlan, err := NewPipelineLongLivedPlan([]string{"provider"})
	require.NoError(t, err)
	jobPlan, err := NewJobLongLivedPlan(1)
	require.NoError(t, err)

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
		require.NoError(t, issueErr)

		_, releaseErr := admission.ReleaseOrdinary(ref)
		require.NoError(t, releaseErr)

		census := admission.Census()
		require.False(t, census.ActiveRecords != 0 || census.FreeRecords == 0)

		permits = append(permits, permit)
	}

	issue(
		ResourceIdentity{ID: "discovery", Generation: 1},
		pipelinePlan,
	)
	for index := range jobs {
		issue(
			ResourceIdentity{
				ID:         fmt.Sprintf("job-%03d", index),
				Generation: 1,
			},
			jobPlan,
		)
	}

	census := admission.Census()
	require.False(t, census.ActiveRecords != 0 ||
		census.LongLivedRecords != jobs ||
		census.OrdinaryBytes != int64(jobs)*jobPlan.Bytes())

	require.EqualValues(t, jobs+1, supervisor.LongLivedCensus().Active)

	for _, permit := range permits {
		require.NoError(t, permit.AbortUnused())
	}

	admissionCensus := admission.Census()
	require.False(t, admissionCensus.ActiveRecords != 0 ||
		admissionCensus.LongLivedRecords != 0 ||
		admissionCensus.OrdinaryBytes != 0)

	require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestLongLivedPermitRemainsLiveAfterIssuanceIsSealed(t *testing.T) {
	admission := NewAdmissionLedger()
	ref := grantLongLivedTestAdmission(t, admission, 40)
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewJobLongLivedPlan(40)
	require.NoError(t, err)
	permit, err := supervisor.IssueLongLivedPermit(
		admission,
		ref,
		ResourceIdentity{ID: "job", Generation: 1},
		plan,
	)
	require.NoError(t, err)

	require.NoError(t, supervisor.SealInherited())
	require.NoError(t, permit.ValidateLive())
	require.NoError(t, permit.ActivateExternal(
		LongLivedEJobResources,
	))
	require.NoError(t, permit.ReleaseExternal(
		LongLivedEJobResources,
	))
	require.NoError(t, permit.AbortUnused())

	_, err = admission.ReleaseOrdinary(ref)
	require.NoError(t, err)
	require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestSecretStoreSteadyPermitsHaveNoConfiguredStoreCountLimit(t *testing.T) {
	const stores = 9

	admission := NewAdmissionLedger()
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewSecretStoreLongLivedPlan(1)
	require.NoError(t, err)
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
		require.NoError(t, issueErr)

		_, releaseErr := admission.ReleaseOrdinary(ref)
		require.NoError(t, releaseErr)

		permits = append(permits, permit)
	}

	census := supervisor.LongLivedCensus()
	require.False(t, census.Active != stores || census.SecretStores != stores || census.Bytes != stores*plan.Bytes())

	for _, permit := range permits {
		require.NoError(t, permit.AbortUnused())
	}

	require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestLongLivedSecretStorePermitRejectsByteReleaseBeforeExternalRelease(t *testing.T) {
	admission := NewAdmissionLedger()
	ref := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewSecretStoreLongLivedPlan(40)
	require.NoError(t, err)
	permit, err := supervisor.IssueLongLivedPermit(admission, ref, ResourceIdentity{ID: "secret-store", Generation: 1}, plan)
	require.NoError(t, err)
	assertUnchanged := func(wantAdmission AdmissionCensus, wantLongLived LongLivedCensus) {
		t.Helper()

		require.EqualValues(t, wantAdmission, admission.Census())

		require.EqualValues(t, wantLongLived, supervisor.LongLivedCensus())
	}
	wantAdmission := admission.Census()
	wantLongLived := supervisor.LongLivedCensus()

	require.Error(t, permit.ReleaseBytes())

	assertUnchanged(wantAdmission, wantLongLived)

	require.NoError(t, permit.ActivateExternal(LongLivedESecretStore))

	wantAdmission = admission.Census()
	wantLongLived = supervisor.LongLivedCensus()

	require.Error(t, permit.ReleaseBytes())

	assertUnchanged(wantAdmission, wantLongLived)

	require.NoError(t, permit.ReleaseExternal(LongLivedESecretStore))

	require.NoError(t, permit.ReleaseBytes())

	require.NoError(t, permit.Return())

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(ref)
	require.NoError(t, releaseOrdinaryErr)

	census := admission.Census()
	require.False(t, census.ActiveRecords != 0 || census.OrdinaryBytes != 0)

	require.EqualValues(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func grantLongLivedTestAdmission(t *testing.T, ledger *AdmissionLedger, byteCount int64) AdmissionRef {
	t.Helper()
	requested := ledger.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 1, Generation: 1},
		byteCount+1,
	)
	require.Nil(t, requested.Rejected)
	var grants [4]AdmissionGrant
	count, _, err := ledger.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != requested.Ref)
	return requested.Ref
}

func TestLongLivedByteReleaseSignalsNewAdmissionCapacity(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	ready := make(chan struct{}, 1)

	require.NoError(t, supervisor.BindAdmissionReady(func() { ready <- struct{}{} }))

	require.Error(t, supervisor.BindAdmissionReady(func() {}))

	admission := NewAdmissionLedger()
	ownerRef := grantLongLivedTestAdmission(t, admission, 100)
	plan, err := NewJobLongLivedPlan(40)
	require.NoError(t, err)
	permit, err := supervisor.IssueLongLivedPermit(
		admission, ownerRef, ResourceIdentity{ID: "owner", Generation: 1}, plan,
	)
	require.NoError(t, err)

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(ownerRef)
	require.NoError(t, releaseOrdinaryErr)

	blocker := admission.RequestOrdinary(
		1,
		AdmissionLaneRef{Slot: 2, Generation: 1},
		OrdinaryBudgetBytes-plan.Bytes(),
	)
	waiter := admission.RequestOrdinary(1, AdmissionLaneRef{Slot: 3, Generation: 1}, 1)
	require.False(t, blocker.Rejected != nil || waiter.Rejected != nil)
	var grants [4]AdmissionGrant
	count, _, err := admission.TakeGrants(2, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != blocker.Ref)

	require.NoError(t, permit.ReleaseExternal(LongLivedEJobResources))

	require.NoError(t, permit.ReleaseBytes())

	select {
	case <-ready:
	default:
		require.FailNow(t, "test failed", "long-lived byte release lost admission wake")
	}

	require.NoError(t, permit.Return())

	count, _, err = admission.TakeGrants(1, &grants)
	require.False(t, err != nil || count != 1 || grants[0].Ref != waiter.Ref)

	_, releaseOrdinaryErr2 := admission.ReleaseOrdinary(waiter.Ref)
	require.NoError(t, releaseOrdinaryErr2)

	_, releaseOrdinaryErr3 := admission.ReleaseOrdinary(blocker.Ref)
	require.NoError(t, releaseOrdinaryErr3)

}

func newLongLivedTestSupervisor(t *testing.T) *TaskSupervisor {
	t.Helper()
	frame, err := NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	return supervisor
}
