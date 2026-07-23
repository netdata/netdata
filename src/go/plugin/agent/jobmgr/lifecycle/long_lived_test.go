// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPipelinePermitConservesOwnershipFacets(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan([]string{"file", "service-discovery"})
	require.NoError(t, err)
	owner := ResourceIdentity{
		ID:         "pipeline",
		Generation: 1,
	}
	permit, err := supervisor.IssueLongLivedPermit(owner, plan)
	require.NoError(t, err)

	require.Equal(t, LongLivedCensus{
		Active: 1,
	}, supervisor.LongLivedCensus())
	require.Error(t, permit.Return())

	wrongOwner := permit
	wrongOwner.owner = ResourceIdentity{
		ID:         "other",
		Generation: 1,
	}
	require.Error(t, wrongOwner.ActivateExternal())
	require.NoError(t, permit.ActivateExternal())
	require.Error(t, permit.ActivateExternal())

	_, err = supervisor.StartInheritedWithPermitKey(
		context.Background(), owner, InheritedPipelineProvider, "unknown", permit,
		func(context.Context) error { return nil },
	)
	require.Error(t, err)

	refs := make(map[string]InheritedTaskRef)
	child, err := supervisor.StartInheritedWithPermit(
		context.Background(), owner, InheritedPipelineSupervisor, permit,
		func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	)
	require.NoError(t, err)
	refs["supervisor"] = child
	for name := range map[string]struct{}{"file": {}, "service-discovery": {}} {
		child, err := supervisor.StartInheritedWithPermitKey(
			context.Background(), owner, InheritedPipelineProvider, name, permit,
			func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
		)
		require.NoError(t, err)
		refs[name] = child
	}
	require.Equal(t, LongLivedCensus{
		Active: 1,
	}, supervisor.LongLivedCensus())

	require.NoError(t, permit.ReleaseExternal())
	for _, child := range refs {
		require.NoError(t, supervisor.CancelInherited(child, owner))
		joined, err := supervisor.JoinInherited(context.Background(), child, owner)
		require.NoError(t, err)
		require.True(t, joined)
		require.NoError(t, supervisor.ReleaseInherited(child, owner))
	}
	require.NoError(t, permit.Return())
	require.Error(t, permit.Return())
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestPipelineLongLivedPlanProviderKeys(t *testing.T) {
	tests := map[string]struct {
		keys    []string
		wantErr bool
	}{
		"one provider":      {keys: []string{"file"}},
		"several providers": {keys: []string{"service-discovery", "file", "dummy"}},
		"empty":             {wantErr: true},
		"blank":             {keys: []string{""}, wantErr: true},
		"whitespace":        {keys: []string{" file"}, wantErr: true},
		"duplicate":         {keys: []string{"file", "file"}, wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plan, err := NewPipelineLongLivedPlan(test.keys)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, plan.Validate())
			require.Equal(t, LongLivedPipeline, plan.Class())
		})
	}
}

func TestLongLivedPlansDescribeResourceOwnership(t *testing.T) {
	tests := map[string]struct {
		plan  LongLivedPlan
		class LongLivedClass
	}{
		"job":          {plan: NewJobLongLivedPlan(), class: LongLivedJob},
		"secret store": {plan: NewSecretStoreLongLivedPlan(), class: LongLivedSecretStore},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, test.plan.Validate())
			require.Equal(t, test.class, test.plan.Class())
		})
	}
}

func TestPipelinePermitReleasesDisabledProviderClaim(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	plan, err := NewPipelineLongLivedPlan([]string{"disabled", "enabled"})
	require.NoError(t, err)
	permit, err := supervisor.IssueLongLivedPermit(ResourceIdentity{
		ID:         "pipeline",
		Generation: 1,
	}, plan)
	require.NoError(t, err)

	require.NoError(t, permit.ReleaseUnusedInherited(InheritedPipelineProvider, "disabled"))
	require.Error(t, permit.ReleaseUnusedInherited(InheritedPipelineProvider, "disabled"))
	require.Len(t, supervisor.longLived.slots[permit.ref].gClaims, 2)
	require.NoError(t, permit.AbortUnused())
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestLongLivedPermitDomainsGrowBeyondFormerJobLimit(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	const jobs = formerFixedPopulation + 1
	permits := make([]LongLivedPermit, 0, jobs)
	for index := range jobs {
		permit, err := supervisor.IssueLongLivedPermit(
			ResourceIdentity{
				ID:         fmt.Sprintf("job-%03d", index),
				Generation: 1,
			},
			NewJobLongLivedPlan(),
		)
		require.NoError(t, err)
		permits = append(permits, permit)
	}
	require.Equal(t, jobs, supervisor.LongLivedCensus().Active)
	for _, permit := range permits {
		require.NoError(t, permit.AbortUnused())
	}
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestLongLivedPermitRejectsDuplicateOwnerAndAllowsMultiplePipelines(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	pipeline, err := NewPipelineLongLivedPlan([]string{"provider"})
	require.NoError(t, err)
	owner := ResourceIdentity{
		ID:         "pipeline",
		Generation: 1,
	}
	permit, err := supervisor.IssueLongLivedPermit(owner, pipeline)
	require.NoError(t, err)

	_, err = supervisor.IssueLongLivedPermit(owner, NewJobLongLivedPlan())
	require.Error(t, err)
	second, err := supervisor.IssueLongLivedPermit(ResourceIdentity{
		ID:         "other-pipeline",
		Generation: 1,
	}, pipeline)
	require.NoError(t, err)
	require.NoError(t, second.AbortUnused())
	require.NoError(t, permit.AbortUnused())
}

func TestLongLivedPermitRemainsLiveAfterIssuanceIsSealed(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	permit, err := supervisor.IssueLongLivedPermit(ResourceIdentity{
		ID:         "job",
		Generation: 1,
	}, NewJobLongLivedPlan())
	require.NoError(t, err)
	require.NoError(t, supervisor.SealInherited())
	require.NoError(t, permit.ValidateLive())
	require.NoError(t, permit.ActivateExternal())
	require.NoError(t, permit.ReleaseExternal())
	require.NoError(t, permit.Return())
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestSecretStorePermitsHaveNoConfiguredCountLimit(t *testing.T) {
	const stores = 9
	supervisor := newLongLivedTestSupervisor(t)
	permits := make([]LongLivedPermit, 0, stores)
	for index := range stores {
		permit, err := supervisor.IssueLongLivedPermit(
			ResourceIdentity{
				ID:         fmt.Sprintf("secret-store-%02d", index),
				Generation: 1,
			},
			NewSecretStoreLongLivedPlan(),
		)
		require.NoError(t, err)
		permits = append(permits, permit)
	}
	census := supervisor.LongLivedCensus()
	require.Equal(t, stores, census.Active)
	require.Equal(t, stores, census.SecretStores)
	for _, permit := range permits {
		require.NoError(t, permit.AbortUnused())
	}
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func TestLongLivedPermitReturnWaitsForExternalRelease(t *testing.T) {
	supervisor := newLongLivedTestSupervisor(t)
	permit, err := supervisor.IssueLongLivedPermit(
		ResourceIdentity{
			ID:         "secret-store",
			Generation: 1,
		},
		NewSecretStoreLongLivedPlan(),
	)
	require.NoError(t, err)
	require.Error(t, permit.Return())
	require.NoError(t, permit.ActivateExternal())
	require.Error(t, permit.Return())
	require.NoError(t, permit.ReleaseExternal())
	require.NoError(t, permit.Return())
	require.Equal(t, LongLivedCensus{}, supervisor.LongLivedCensus())
}

func newLongLivedTestSupervisor(t *testing.T) *TaskSupervisor {
	t.Helper()
	frame, err := NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	supervisor, err := NewTaskSupervisor(frame)
	require.NoError(t, err)
	return supervisor
}
