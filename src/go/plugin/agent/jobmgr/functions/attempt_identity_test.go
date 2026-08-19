// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestFunctionAttemptIdentityPreservesSemanticRelationships(t *testing.T) {
	agent := modulePlanAttemptIdentity("module")
	require.Len(t, agent.Key, sha256.Size)
	require.Equal(t, "module", agent.Resource)
	require.Equal(t, agent.Key, modulePlanAttemptIdentity("module").Key)

	job := jobFunctionAttemptKey(1, "module_job")
	require.Len(t, job, sha256.Size)
	require.NotEqual(t, agent.Key, job)
	require.NotEqual(t, job, jobFunctionAttemptKey(2, "module_job"))
	require.NotEqual(t, job, jobFunctionAttemptKey(1, "module_other"))

	long := modulePlanAttemptIdentity(strings.Repeat("m", 257))
	require.Equal(t, "collector module", long.Resource)
}

func TestJobFunctionInvocationAcceptsLongPublicJobName(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})

	const module = "module"
	name := strings.Repeat("a", 4090)
	require.NoError(t, dyncfg.JobNameRuleStrict(name))
	job := &controllerTestJob{
		fullName: module + "_" + name,
		module:   module,
		name:     name,
		running:  true,
	}
	require.Greater(t, len(job.FullName()), 4096)
	stager := &JobStager{
		modules: collectorapi.Registry{
			module: {
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &controllerTestHandler{}
				},
			},
		},
		shared: map[string][]funcapi.FunctionConfig{
			module: {{ID: "method"}},
		},
		attempts: attempts,
		epoch:    1,
	}
	staged, err := stager.StageJob(job)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, staged.CloseAndDrain(context.Background()))
	})

	calls := 0
	_, err = staged.bundle.invoke(t.Context(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls)
}
