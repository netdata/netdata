// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestJobAttemptIdentityPreservesSemanticRelationships(t *testing.T) {
	const fullName = "module_job"
	candidate := jobAttemptIdentity(jobmgr.ProcessAttemptJob, fullName)
	runtime := jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, fullName)
	require.NotEqual(t, candidate.Namespace, runtime.Namespace)
	require.Equal(t, candidate.Key, runtime.Key)
	require.Len(t, candidate.Key, sha256.Size)
	require.Equal(t, fullName, candidate.Resource)

	config := factoryTestConfig(false)
	test := jobTestAttemptIdentity(configOperationTest, config)
	get := jobTestAttemptIdentity(configOperationGet, config)
	require.NotEqual(t, candidate.Key, test.Key)
	require.NotEqual(t, test.Key, get.Key)
	require.Len(t, test.Key, sha256.Size)

	long := jobAttemptIdentity(
		jobmgr.ProcessAttemptJob,
		strings.Repeat("a", jobmgr.MaximumProcessAttemptDiagnosticResourceBytes+1),
	)
	require.Equal(t, "collector job", long.Resource)
}

func TestConfigAttemptsAcceptPubliclyValidNamesAcrossContainmentBoundary(t *testing.T) {
	factory, _ := newFactoryTestHarness(t, collectorapi.Creator{}, nil)
	tests := map[string]string{
		"long":         strings.Repeat("a", 4090),
		"invalid UTF8": string([]byte{0xff}),
	}
	for name, jobName := range tests {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, dyncfg.JobNameRuleStrict(jobName))
			config := factoryTestConfig(false)
			config.SetName(jobName)
			for _, kind := range []configOperationKind{
				configOperationValidate,
				configOperationTest,
			} {
				stage, err := newPreparedConfigOperation(
					factory,
					config,
					kind,
					func(context.Context, confgroup.Config) ([]byte, error) {
						return []byte(`{"accepted":true}`), nil
					},
				)
				require.NoError(t, err)
				stage.Start()
				requireConfigOperationReady(t, stage)
				payload, err := stage.take()
				require.NoError(t, err)
				require.JSONEq(t, `{"accepted":true}`, string(payload))
				stage.Release()
			}
		})
	}
}

func TestCandidateAndRuntimeAttemptsAcceptLongPublicJobName(t *testing.T) {
	state := &factoryTestState{}
	creator := collectorapi.Creator{
		CreateV2: func() collectorapi.CollectorV2 {
			return &factoryTestV2{
				state:    state,
				store:    metrix.NewCollectorStore(),
				template: factoryTestChartTemplate,
			}
		},
	}
	factory, _ := newFactoryTestHarness(t, creator, nil)
	config := factoryTestConfig(false)
	config.SetName(strings.Repeat("a", 4090))
	require.NoError(t, dyncfg.JobNameRuleStrict(config.Name()))
	require.Greater(t, len(config.FullName()), 4096)

	identity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	prepared, failure, err := prepareFactoryTestCandidate(
		t.Context(),
		factory,
		config,
		identity,
		permit,
	)
	require.NoError(t, err)
	require.Nil(t, failure)

	resource, err := prepared.AcceptStart(t.Context(), identity.Generation)
	require.NoError(t, err)
	generation := resource.(*JobGeneration)
	require.NoError(t, generation.Publish())
	require.NoError(t, generation.reserveInstallation())
	require.NoError(t, generation.acknowledgeInstallation())
	require.NoError(t, generation.Stop(t.Context()))
	require.NoError(t, generation.Finalize())
	requireFactoryAttemptsIdle(t, factory)
	require.EqualValues(t, 1, state.collectorCleanup)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func requireConfigOperationReady(t *testing.T, stage *preparedConfigOperation) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-stage.Ready():
	case <-timer.C:
		require.FailNow(t, "test failed", "config operation did not become ready")
	}
}
