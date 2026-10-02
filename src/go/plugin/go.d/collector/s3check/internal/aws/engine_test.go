// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/testutil"
)

func TestCheckValidatesVersioningAndApplicableDeleteMarkerRuleWithoutMutation(t *testing.T) {
	source, destination, _ := newAWSClients()
	engine := newAWSEngine(t, source, destination, nil)

	require.NoError(t, engine.Check(context.Background()))
	assert.Equal(t, 1, source.Count("bucket_versioning"))
	assert.Equal(t, 1, destination.Count("bucket_versioning"))
	assert.Equal(t, 1, source.Count("bucket_replication"))
	assert.Zero(t, source.Count("put"))
	assert.Zero(t, destination.Count("put"))
}

func TestCheckUsesGeneratedOwnerNamespaceForRuleApplicability(t *testing.T) {
	t.Run("rejects narrower additional destination", func(t *testing.T) {
		source, destination, _ := newAWSClients()
		engine := newAWSEngine(t, source, destination, nil)
		namespace, err := engine.generator.Namespace()
		require.NoError(t, err)
		source.BucketReplicationFunc = replicationRules(
			s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "netdata-s3check/",
				DeleteMarkerReplication: true,
				Priority:                10,
			},
			s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "unowned-destination",
				Prefix:                  namespace,
				DeleteMarkerReplication: true,
				Priority:                20,
			},
		)

		require.Error(t, engine.Check(context.Background()))
	})

	t.Run("rejects fixed probe stem additional destination", func(t *testing.T) {
		source, destination, _ := newAWSClients()
		engine := newAWSEngine(t, source, destination, nil)
		namespace, err := engine.generator.Namespace()
		require.NoError(t, err)
		source.BucketReplicationFunc = replicationRules(
			s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "netdata-s3check/",
				DeleteMarkerReplication: true,
				Priority:                10,
			},
			s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "unowned-destination",
				Prefix:                  namespace + "probe-",
				DeleteMarkerReplication: true,
				Priority:                20,
			},
		)

		require.Error(t, engine.Check(context.Background()))
	})

	t.Run("accepts narrower configured destination", func(t *testing.T) {
		source, destination, _ := newAWSClients()
		engine := newAWSEngine(t, source, destination, nil)
		namespace, err := engine.generator.Namespace()
		require.NoError(t, err)
		source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "destination",
			Prefix:                  namespace,
			DeleteMarkerReplication: true,
			Priority:                10,
		})

		require.NoError(t, engine.Check(context.Background()))
	})
}

func TestCollectRevalidatesReplicationPolicyBeforeMutation(t *testing.T) {
	source, destination, _ := newAWSClients()
	engine := newAWSEngine(t, source, destination, nil)
	require.NoError(t, engine.Check(context.Background()))
	source.BucketReplicationFunc = replicationRules(
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "destination",
			Prefix:                  "netdata-s3check/",
			DeleteMarkerReplication: true,
			Priority:                10,
		},
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "unowned-destination",
			Prefix:                  "netdata-s3check/",
			DeleteMarkerReplication: true,
			Priority:                20,
		},
	)

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Nil(t, result.Probe)
	assert.Zero(t, source.Count("put"))
	engine.Cleanup(context.Background())
}

func TestCollectValidatesExactGeneratedKeyBeforeMutation(t *testing.T) {
	source, destination, _ := newAWSClients()
	engine := newAWSEngine(t, source, destination, nil)
	keyTime := time.Unix(100, 0)
	engine.generator.Now = func() time.Time { return keyTime }
	source.BucketReplicationFunc = replicationRules(
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "destination",
			Prefix:                  "netdata-s3check/",
			DeleteMarkerReplication: true,
			Priority:                10,
		},
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "unowned-destination",
			Prefix:                  engine.keyPrefix + "1",
			DeleteMarkerReplication: true,
			Priority:                20,
		},
	)

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Nil(t, result.Probe)
	assert.Zero(t, source.Count("put"))
	source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
		Enabled:                 true,
		DestinationBucket:       "destination",
		Prefix:                  "netdata-s3check/",
		DeleteMarkerReplication: true,
	})
	engine.Cleanup(context.Background())
}

func TestCollectValidatesExactOwnedKeysBeforeCleanup(t *testing.T) {
	source, destination, model := newAWSClients()
	model.failPutAfterMutation = true
	engine := newAWSEngineWithOptions(t, source, destination, nil, func(opts *Options) {
		opts.QueueCapacity = 1
	})

	engine.Collect(context.Background())
	key, _ := model.onlySourceObject(t)
	source.BucketReplicationFunc = replicationRules(
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "destination",
			Prefix:                  "netdata-s3check/",
			DeleteMarkerReplication: true,
			Priority:                10,
		},
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "unowned-destination",
			Prefix:                  key,
			DeleteMarkerReplication: true,
			Priority:                20,
		},
	)
	listsBefore := source.Count("list_versions")

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Equal(t, listsBefore, source.Count("list_versions"))
	source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
		Enabled:                 true,
		DestinationBucket:       "destination",
		Prefix:                  "netdata-s3check/",
		DeleteMarkerReplication: true,
	})
	engine.Cleanup(context.Background())
}

func TestCleanupValidatesExactOwnedKeysBeforeShutdownMutation(t *testing.T) {
	source, destination, model := newAWSClients()
	engine := newAWSEngine(t, source, destination, nil)

	engine.Collect(context.Background())
	key, sourceObjectID := model.onlySourceObject(t)
	model.replicateObject(t, key, sourceObjectID)
	source.BucketReplicationFunc = replicationRules(
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "destination",
			Prefix:                  "netdata-s3check/",
			DeleteMarkerReplication: true,
			Priority:                10,
		},
		s3client.ReplicationRule{
			Enabled:                 true,
			DestinationBucket:       "unowned-destination",
			Prefix:                  key,
			DeleteMarkerReplication: true,
			Priority:                20,
		},
	)

	engine.Cleanup(context.Background())

	assert.Zero(t, source.Count("delete"))
	assert.True(t, model.hasVersions("source", key))
	assert.FileExists(t, engine.journal.Path())
}

func TestCheckRejectsUnsafeProviderConfiguration(t *testing.T) {
	tests := map[string]func(*testutil.S3, *testutil.S3){
		"source versioning disabled": func(source, _ *testutil.S3) {
			source.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
				return s3client.BucketVersioningResult{
					Status: s3client.VersioningDisabled,
				}, nil
			}
		},
		"destination versioning suspended": func(_, destination *testutil.S3) {
			destination.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
				return s3client.BucketVersioningResult{
					Status: s3client.VersioningSuspended,
				}, nil
			}
		},
		"MFA Delete enabled": func(source, _ *testutil.S3) {
			source.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
				return s3client.BucketVersioningResult{
					Status:    s3client.VersioningEnabled,
					MFADelete: true,
				}, nil
			}
		},
		"wrong destination": func(source, _ *testutil.S3) {
			source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "other",
				Prefix:                  "netdata-s3check/",
				DeleteMarkerReplication: true,
			})
		},
		"prefix does not contain probes": func(source, _ *testutil.S3) {
			source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "other/",
				DeleteMarkerReplication: true,
			})
		},
		"tag filtered": func(source, _ *testutil.S3) {
			source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
				Enabled:                 true,
				DestinationBucket:       "destination",
				Prefix:                  "netdata-s3check/",
				TagFiltered:             true,
				DeleteMarkerReplication: true,
			})
		},
		"delete markers disabled": func(source, _ *testutil.S3) {
			source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
				Enabled:           true,
				DestinationBucket: "destination",
				Prefix:            "netdata-s3check/",
			})
		},
		"additional applicable destination": func(source, _ *testutil.S3) {
			source.BucketReplicationFunc = replicationRules(
				s3client.ReplicationRule{
					Enabled:                 true,
					DestinationBucket:       "destination",
					Prefix:                  "netdata-s3check/",
					DeleteMarkerReplication: true,
					Priority:                10,
				},
				s3client.ReplicationRule{
					Enabled:                 true,
					DestinationBucket:       "unowned-destination",
					Prefix:                  "netdata-s3check/",
					DeleteMarkerReplication: true,
					Priority:                20,
				},
			)
		},
		"higher priority rule disables delete markers": func(source, _ *testutil.S3) {
			source.BucketReplicationFunc = replicationRules(
				s3client.ReplicationRule{
					Enabled:                 true,
					DestinationBucket:       "destination",
					Prefix:                  "netdata-s3check/",
					DeleteMarkerReplication: true,
					Priority:                10,
				},
				s3client.ReplicationRule{
					Enabled:           true,
					DestinationBucket: "destination",
					Prefix:            "netdata-s3check/",
					Priority:          20,
				},
			)
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			source, destination, _ := newAWSClients()
			mutate(source, destination)
			engine := newAWSEngine(t, source, destination, nil)

			err := engine.Check(context.Background())
			require.Error(t, err)
			assert.Zero(t, source.Count("put"))
		})
	}
}

func TestProbeOwnsIndependentVersionsAndCleansExactIdentities(t *testing.T) {
	source, destination, model := newAWSClients()
	now := time.Unix(100, 0)
	engine := newAWSEngine(t, source, destination, &now)

	wantObject := engine.Collect(context.Background())
	require.NotNil(t, wantObject.Probe)
	assert.Equal(t, contract.StatusWaiting, wantObject.Probe.Status)
	assert.False(t, wantObject.Probe.PayloadCompared)
	key, sourceObjectID := model.onlySourceObject(t)
	require.True(t, model.lastPutConditional)

	now = now.Add(10 * time.Second)
	destinationObjectID := model.replicateObject(t, key, sourceObjectID)
	wantMarker := engine.Collect(context.Background())
	require.NotNil(t, wantMarker.Probe)
	assert.Equal(t, contract.StatusWaiting, wantMarker.Probe.Status)
	assert.True(t, wantMarker.Probe.PayloadCompared)
	assert.Equal(t, 10*time.Second, wantMarker.Probe.WriteVisibility.Lag)
	sourceMarkerID := model.onlySourceMarker(t, key)
	require.NotEmpty(t, model.lastLogicalDeleteIfMatch)

	now = now.Add(5 * time.Second)
	destinationMarkerID := model.replicateMarker(t, key, sourceMarkerID)
	success := engine.Collect(context.Background())
	require.NotNil(t, success.Probe)
	assert.Equal(t, contract.StatusSuccess, success.Probe.Status)
	assert.True(t, success.Probe.PayloadCompared)
	assert.Equal(t, 10*time.Second, success.Probe.WriteVisibility.Lag)
	assert.Equal(t, 5*time.Second, success.Probe.DeleteVisibility.Lag)
	assert.Zero(t, success.Cleanup.Pending)
	assert.False(t, model.hasVersions("source", key))
	assert.False(t, model.hasVersions("destination", key))

	assertExactVersionDeleted(t, source, key, sourceObjectID)
	assertExactVersionDeleted(t, source, key, sourceMarkerID)
	assertExactVersionDeleted(t, destination, key, destinationObjectID)
	assertExactVersionDeleted(t, destination, key, destinationMarkerID)
	engine.Cleanup(context.Background())
}

func TestAmbiguousPutReconcilesExactSourceVersion(t *testing.T) {
	source, destination, model := newAWSClients()
	model.failPutAfterMutation = true
	engine := newAWSEngineWithOptions(t, source, destination, nil, func(opts *Options) {
		opts.QueueCapacity = 1
	})

	failed := engine.Collect(context.Background())
	require.NotNil(t, failed.Probe)
	assert.Equal(t, contract.StatusFailed, failed.Probe.Status)
	assert.Equal(t, contract.ReasonRequest, failed.Probe.Reason)
	key, sourceObjectID := model.onlySourceObject(t)

	waiting := engine.Collect(context.Background())
	assert.Nil(t, waiting.Probe)
	assert.Equal(t, 1, waiting.Cleanup.Pending)
	assert.Equal(t, 1, source.Count("put"))
	assert.GreaterOrEqual(t, source.Count("list_versions"), 1)

	model.replicateObject(t, key, sourceObjectID)
	engine.Collect(context.Background())
	assert.NotEmpty(t, model.onlySourceMarker(t, key))
	engine.Cleanup(context.Background())
}

func TestAmbiguousLogicalDeleteReconcilesExactMarker(t *testing.T) {
	source, destination, model := newAWSClients()
	engine := newAWSEngine(t, source, destination, nil)

	engine.Collect(context.Background())
	key, sourceObjectID := model.onlySourceObject(t)
	model.replicateObject(t, key, sourceObjectID)
	model.failDeleteAfterMutation = true

	failed := engine.Collect(context.Background())
	require.NotNil(t, failed.Probe)
	assert.Equal(t, contract.StatusFailed, failed.Probe.Status)
	sourceMarkerID := model.onlySourceMarker(t, key)

	waiting := engine.Collect(context.Background())
	require.NotNil(t, waiting.Probe)
	assert.Equal(t, contract.StatusWaiting, waiting.Probe.Status)
	assert.GreaterOrEqual(t, source.Count("list_versions"), 1)

	model.replicateMarker(t, key, sourceMarkerID)
	success := engine.Collect(context.Background())
	require.NotNil(t, success.Probe)
	assert.Equal(t, contract.StatusSuccess, success.Probe.Status)
	engine.Cleanup(context.Background())
}

func TestLateMarkerRetainsOwnershipAndBackpressuresInsteadOfDeletingVersions(t *testing.T) {
	source, destination, model := newAWSClients()
	now := time.Unix(100, 0)
	engine := newAWSEngineWithOptions(t, source, destination, &now, func(opts *Options) {
		opts.QueueCapacity = 1
		opts.DeleteTimeout = time.Minute
	})

	engine.Collect(context.Background())
	key, sourceObjectID := model.onlySourceObject(t)
	model.replicateObject(t, key, sourceObjectID)
	engine.Collect(context.Background())

	now = now.Add(time.Minute + time.Second)
	timedOut := engine.Collect(context.Background())
	require.NotNil(t, timedOut.Probe)
	assert.Equal(t, contract.ReasonDeleteTimeout, timedOut.Probe.Reason)

	blocked := engine.Collect(context.Background())
	assert.True(t, blocked.Cleanup.Backpressure)
	assert.Equal(t, 1, blocked.Cleanup.Pending)
	assert.Equal(t, 1, source.Count("put"))
	assertNoExactVersionDeletes(t, source)
	assertNoExactVersionDeletes(t, destination)
	engine.Cleanup(context.Background())
}

func TestReconciliationIsBoundedAndRetainsStateOnPaginationOverflow(t *testing.T) {
	source, destination, model := newAWSClients()
	model.failPutAfterMutation = true
	source.ListVersionsFunc = func(context.Context, string, string, string, string, int32) (s3client.VersionPage, error) {
		return s3client.VersionPage{
			Versions: []s3client.Version{
				{Kind: s3client.VersionObject, Key: "different-key", VersionID: "v"},
			},
			Truncated:           true,
			NextKeyMarker:       "next",
			NextVersionIDMarker: "next-version",
		}, nil
	}
	engine := newAWSEngineWithOptions(t, source, destination, nil, func(opts *Options) {
		opts.QueueCapacity = 1
	})

	engine.Collect(context.Background())
	result := engine.Collect(context.Background())
	assert.Nil(t, result.Probe)
	assert.LessOrEqual(t, source.Count("list_versions"), maxListPages)
	assert.Equal(t, 1, result.Cleanup.Pending)
	engine.Cleanup(context.Background())
}

func TestRestartRetainsPutIntentUntilCrossCycleAbsenceProof(t *testing.T) {
	source, destination, _ := newAWSClients()
	now := time.Unix(100, 0)
	engine := newAWSEngineWithOptions(t, source, destination, &now, func(opts *Options) {
		opts.QueueCapacity = 1
		opts.UpdateEvery = time.Minute
		opts.WriteTimeout = 2 * time.Minute
	})
	object, err := engine.generator.Next()
	require.NoError(t, err)
	locked, err := engine.journal.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	require.NoError(t, engine.journal.Save(state{
		Entries: []entry{{
			Key: object.Key, Digest: object.Digest, CreatedAt: now, Phase: phasePutIntent,
		}},
		ActiveKey: object.Key,
	}))
	engine.journal.Unlock()

	crashRecovered := engine.Collect(context.Background())
	require.NotNil(t, crashRecovered.Probe)
	assert.Equal(t, contract.StatusFailed, crashRecovered.Probe.Status)
	assert.Equal(t, 1, crashRecovered.Cleanup.Pending)
	assert.Zero(t, source.Count("put"))

	firstAbsence := engine.Collect(context.Background())
	assert.Equal(t, 1, firstAbsence.Cleanup.Pending)
	assert.Zero(t, source.Count("put"))

	now = now.Add(2*time.Minute + time.Second)
	cleared := engine.Collect(context.Background())
	assert.Equal(t, 1, source.Count("put"), "new mutation starts only after delayed source/destination absence proof")
	assert.NotNil(t, cleared.Probe)
	engine.Cleanup(context.Background())
}

func TestDeleteLagStartsAfterSuccessfulMarkerCreation(t *testing.T) {
	source, destination, model := newAWSClients()
	now := time.Unix(100, 0)
	originalDelete := source.DeleteFunc
	source.DeleteFunc = func(
		ctx context.Context, bucket, key string, opts s3client.DeleteOptions,
	) (s3client.DeleteResult, error) {
		if opts.VersionID == "" {
			now = now.Add(8 * time.Second)
		}
		return originalDelete(ctx, bucket, key, opts)
	}
	engine := newAWSEngine(t, source, destination, &now)

	engine.Collect(context.Background())
	key, sourceObjectID := model.onlySourceObject(t)
	model.replicateObject(t, key, sourceObjectID)
	engine.Collect(context.Background())
	sourceMarkerID := model.onlySourceMarker(t, key)

	now = now.Add(5 * time.Second)
	model.replicateMarker(t, key, sourceMarkerID)
	result := engine.Collect(context.Background())
	require.NotNil(t, result.Probe)
	assert.Equal(t, 5*time.Second, result.Probe.DeleteVisibility.Lag)
	engine.Cleanup(context.Background())
}

func TestRetiredReconciliationReportsProviderOperationFailure(t *testing.T) {
	source, destination, model := newAWSClients()
	model.failPutAfterMutation = true
	engine := newAWSEngineWithOptions(t, source, destination, nil, func(opts *Options) {
		opts.QueueCapacity = 1
	})

	engine.Collect(context.Background())
	wantErr := errors.New("list unavailable")
	source.ListVersionsFunc = func(context.Context, string, string, string, string, int32) (s3client.VersionPage, error) {
		return s3client.VersionPage{}, wantErr
	}
	result := engine.Collect(context.Background())

	assert.Equal(t, 1, result.Cleanup.Pending)
	var failed *contract.OperationResult
	for index := range result.Operations {
		if errors.Is(result.Operations[index].Err, wantErr) {
			failed = &result.Operations[index]
			break
		}
	}
	require.NotNil(t, failed)
	assert.Equal(t, contract.ReasonRequest, failed.Reason)
	engine.Cleanup(context.Background())
}

func TestCleanupJournalFailureReportsRuntimeError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions are required")
	}
	source, destination, model := newAWSClients()
	model.failPutAfterMutation = true
	engine := newAWSEngineWithOptions(t, source, destination, nil, func(opts *Options) {
		opts.QueueCapacity = 1
	})

	failed := engine.Collect(context.Background())
	require.NotNil(t, failed.Probe)
	require.Equal(t, contract.StatusFailed, failed.Probe.Status)
	root := filepath.Dir(engine.journal.Path())
	backup := root + "-backup"
	require.NoError(t, os.Rename(root, backup))
	require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o600))
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = os.Remove(root)
			_ = os.Rename(backup, root)
		}
		engine.Cleanup(context.Background())
	})

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Equal(t, 1, result.Cleanup.Pending)
	require.NoError(t, os.Remove(root))
	require.NoError(t, os.Rename(backup, root))
	restored = true
}

func TestCollectStopsAfterBacklogJournalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming an open journal directory is not portable to Windows")
	}
	source, destination, _ := newAWSClients()
	now := time.Unix(100, 0)
	engine := newAWSEngine(t, source, destination, &now)
	prepareActiveAndBacklog(t, engine, &now)

	root := filepath.Dir(engine.journal.Path())
	backup := root + "-backup"
	require.NoError(t, os.Rename(root, backup))
	require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o600))
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = os.Remove(root)
			_ = os.Rename(backup, root)
		}
		engine.Cleanup(context.Background())
	})
	destinationGetsBefore := destination.Count("get")

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Equal(
		t,
		destinationGetsBefore+1,
		destination.Count("get"),
		"only the backlog read before the persistence failure may run; the active probe must not advance",
	)

	require.NoError(t, os.Remove(root))
	require.NoError(t, os.Rename(backup, root))
	restored = true
}

func TestCleanupStopsAfterActiveRetirementJournalFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming an open journal directory is not portable to Windows")
	}
	source, destination, _ := newAWSClients()
	now := time.Unix(100, 0)
	engine := newAWSEngine(t, source, destination, &now)
	prepareActiveAndBacklog(t, engine, &now)

	root := filepath.Dir(engine.journal.Path())
	backup := root + "-backup"
	require.NoError(t, os.Rename(root, backup))
	require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o600))
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = os.Remove(root)
			_ = os.Rename(backup, root)
		}
	})
	destinationGetsBefore := destination.Count("get")

	engine.Cleanup(context.Background())
	assert.Equal(
		t,
		destinationGetsBefore,
		destination.Count("get"),
		"backlog must not advance after active retirement fails",
	)

	require.NoError(t, os.Remove(root))
	require.NoError(t, os.Rename(backup, root))
	restored = true
}

func TestPublicationUncertaintyPreservesCleanupResultOnLaterCollections(t *testing.T) {
	source, destination, model := newAWSClients()
	engine := newAWSEngineWithOptions(t, source, destination, nil, func(opts *Options) {
		opts.QueueCapacity = 1
		opts.CleanupBatch = 1
	})
	t.Cleanup(func() { engine.Cleanup(context.Background()) })

	engine.Collect(context.Background())
	key, sourceObjectID := model.onlySourceObject(t)
	model.replicateObject(t, key, sourceObjectID)
	engine.Collect(context.Background())
	sourceMarkerID := model.onlySourceMarker(t, key)
	model.replicateMarker(t, key, sourceMarkerID)

	originalDelete := destination.DeleteFunc
	failExactDelete := true
	destination.DeleteFunc = func(
		ctx context.Context,
		bucket, objectKey string,
		opts s3client.DeleteOptions,
	) (s3client.DeleteResult, error) {
		if opts.VersionID != "" && failExactDelete {
			failExactDelete = false
			return s3client.DeleteResult{}, errors.New("cleanup unavailable")
		}
		return originalDelete(ctx, bucket, objectKey, opts)
	}

	failedCleanup := engine.Collect(context.Background())
	require.NotNil(t, failedCleanup.Probe)
	require.Equal(t, contract.ReasonCleanup, failedCleanup.Probe.Reason)
	require.Len(t, engine.state.Entries, 1)
	require.Empty(t, engine.state.ActiveKey)
	destination.DeleteFunc = originalDelete

	tmpPath := engine.journal.Path() + ".tmp"
	require.NoError(t, os.Mkdir(tmpPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(tmpPath, "blocker"), []byte("block"), 0o600))

	publicationFailure := engine.Collect(context.Background())
	require.ErrorIs(t, publicationFailure.Err, journal.ErrPublicationUncertain)
	require.Equal(t, 1, publicationFailure.Cleanup.Pending)
	require.True(t, publicationFailure.Cleanup.Backpressure)

	later := engine.Collect(context.Background())
	require.ErrorIs(t, later.Err, journal.ErrPublicationUncertain)
	assert.Equal(t, 1, later.Cleanup.Pending)
	assert.True(t, later.Cleanup.Backpressure)
}

func prepareActiveAndBacklog(t *testing.T, engine *Engine, now *time.Time) {
	t.Helper()
	waiting := engine.Collect(context.Background())
	require.NotNil(t, waiting.Probe)
	require.Equal(t, contract.StatusWaiting, waiting.Probe.Status)

	*now = now.Add(engine.writeTimeout + time.Second)
	timedOut := engine.Collect(context.Background())
	require.NotNil(t, timedOut.Probe)
	require.Equal(t, contract.ReasonVisibilityTimeout, timedOut.Probe.Reason)

	next := engine.Collect(context.Background())
	require.NotNil(t, next.Probe)
	require.Equal(t, contract.StatusWaiting, next.Probe.Status)
	require.Len(t, engine.state.Entries, 2)
	require.NotNil(t, engine.active())
}

func TestExactCleanupRequiresDurablePhaseBeforeDeletingVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming an open journal directory is not portable to Windows")
	}
	source, destination, model := newAWSClients()
	engine := newAWSEngine(t, source, destination, nil)

	engine.Collect(context.Background())
	key, sourceObjectID := model.onlySourceObject(t)
	model.replicateObject(t, key, sourceObjectID)
	engine.Collect(context.Background())
	sourceMarkerID := model.onlySourceMarker(t, key)
	model.replicateMarker(t, key, sourceMarkerID)

	var durable state
	found, err := engine.journal.Load(&durable)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, key, durable.ActiveKey)
	require.Equal(t, phaseWaitMarker, durable.Entries[0].Phase)
	require.Empty(t, durable.Entries[0].DestinationMarkerID)

	root := filepath.Dir(engine.journal.Path())
	backup := root + "-backup"
	require.NoError(t, os.Rename(root, backup))
	require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o600))
	restored := false
	t.Cleanup(func() {
		if !restored {
			_ = os.Remove(root)
			_ = os.Rename(backup, root)
		}
	})

	firstFailure := engine.Collect(context.Background())
	require.Error(t, firstFailure.Err)
	secondFailure := engine.Collect(context.Background())
	require.Error(t, secondFailure.Err)
	assertNoExactVersionDeletes(t, source)
	assertNoExactVersionDeletes(t, destination)
	engine.Cleanup(context.Background())
	assertNoExactVersionDeletes(t, source)
	assertNoExactVersionDeletes(t, destination)

	require.NoError(t, os.Remove(root))
	require.NoError(t, os.Rename(backup, root))
	restored = true
	successorJournal, err := journal.New(
		root,
		"agent",
		"job",
		journal.Fingerprint(
			"aws_replication", "", "source", "netdata-s3check/", "", "destination", "exact-key",
		),
	)
	require.NoError(t, err)
	successor := newAWSEngineWithJournal(t, source, destination, nil, successorJournal, nil)
	t.Cleanup(func() { successor.Cleanup(context.Background()) })

	recovered := successor.Collect(context.Background())
	require.NotNil(t, recovered.Probe)
	assert.Equal(t, contract.StatusSuccess, recovered.Probe.Status)
	assert.False(t, model.hasVersions("source", key))
	assert.False(t, model.hasVersions("destination", key))
}

func TestValidateEntryPhaseRejectsMutationWithoutRequiredProof(t *testing.T) {
	err := validateEntryPhase(entry{
		Phase:               phaseDeleteIntent,
		SourceObjectID:      "source-version",
		DestinationObjectID: "destination-version",
		ObjectSeen:          true,
		VisibleAt:           new(time.Unix(100, 0)),
	})
	assert.ErrorContains(t, err, "source object identity")
}

func newAWSEngine(t *testing.T, source, destination *testutil.S3, now *time.Time) *Engine {
	t.Helper()
	return newAWSEngineWithOptions(t, source, destination, now, nil)
}

func newAWSEngineWithOptions(
	t *testing.T,
	source, destination *testutil.S3,
	now *time.Time,
	modify func(*Options),
) *Engine {
	t.Helper()
	j, err := journal.New(
		t.TempDir(),
		"agent",
		"job",
		journal.Fingerprint(
			"aws_replication", "", "source", "netdata-s3check/", "", "destination", "exact-key",
		),
	)
	require.NoError(t, err)
	return newAWSEngineWithJournal(t, source, destination, now, j, modify)
}

func newAWSEngineWithJournal(
	t *testing.T,
	source, destination *testutil.S3,
	now *time.Time,
	j *journal.Journal,
	modify func(*Options),
) *Engine {
	t.Helper()
	opts := Options{
		Source:            source,
		Destination:       destination,
		SourceBucket:      "source",
		DestinationBucket: "destination",
		ProbePrefix:       "netdata-s3check/",
		Journal:           j,
		Generator: probe.Generator{
			Prefix:  "netdata-s3check/",
			OwnerID: j.OwnerID(),
			Now:     time.Now,
			Random:  bytes.NewReader(bytes.Repeat([]byte{3}, 64*(16+probe.PayloadBytes))),
		},
		SourceRequestTimeout:      time.Second,
		DestinationRequestTimeout: time.Second,
		UpdateEvery:               time.Minute,
		WriteObjective:            15 * time.Second,
		WriteTimeout:              time.Minute,
		DeleteObjective:           10 * time.Second,
		DeleteTimeout:             time.Minute,
	}
	if now != nil {
		opts.Now = func() time.Time { return *now }
	}
	if modify != nil {
		modify(&opts)
	}
	engine, err := New(opts)
	require.NoError(t, err)
	return engine
}

func replicationRules(
	rules ...s3client.ReplicationRule,
) func(context.Context, string) ([]s3client.ReplicationRule, error) {
	return func(context.Context, string) ([]s3client.ReplicationRule, error) { return rules, nil }
}

func assertExactVersionDeleted(t *testing.T, client *testutil.S3, key, versionID string) {
	t.Helper()
	for _, call := range client.Calls {
		if call.Operation == "delete" && call.Key == key && call.VersionID == versionID {
			return
		}
	}
	assert.Failf(t, "missing exact deletion", "key=%q version=%q calls=%+v", key, versionID, client.Calls)
}

func assertNoExactVersionDeletes(t *testing.T, client *testutil.S3) {
	t.Helper()
	for _, call := range client.Calls {
		if call.Operation == "delete" {
			assert.Empty(t, call.VersionID)
		}
	}
}

type awsVersion struct {
	kind    s3client.VersionKind
	id      string
	etag    string
	payload []byte
	latest  bool
}

type awsModel struct {
	mu sync.Mutex

	source      map[string][]awsVersion
	destination map[string][]awsVersion
	nextID      int

	failPutAfterMutation     bool
	failDeleteAfterMutation  bool
	lastPutConditional       bool
	lastLogicalDeleteIfMatch string
}

func (m *awsModel) onlySourceObject(t *testing.T) (string, string) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	var key, id string
	for candidate, versions := range m.source {
		for _, version := range versions {
			if version.kind == s3client.VersionObject {
				require.Empty(t, id)
				key, id = candidate, version.id
			}
		}
	}
	require.NotEmpty(t, id)
	return key, id
}

func (m *awsModel) onlySourceMarker(t *testing.T, key string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	var id string
	for _, version := range m.source[key] {
		if version.kind == s3client.VersionDeleteMarker {
			require.Empty(t, id)
			id = version.id
		}
	}
	require.NotEmpty(t, id)
	return id
}

func (m *awsModel) replicateObject(t *testing.T, key, sourceID string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	version := m.findLocked(t, m.source, key, sourceID)
	require.Equal(t, s3client.VersionObject, version.kind)
	return m.addLocked(m.destination, key, s3client.VersionObject, version.payload, version.etag)
}

func (m *awsModel) replicateMarker(t *testing.T, key, sourceID string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	version := m.findLocked(t, m.source, key, sourceID)
	require.Equal(t, s3client.VersionDeleteMarker, version.kind)
	return m.addLocked(m.destination, key, s3client.VersionDeleteMarker, nil, "")
}

func (m *awsModel) hasVersions(bucket, key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bucketLocked(bucket)[key]) > 0
}

func (m *awsModel) addLocked(
	versions map[string][]awsVersion,
	key string,
	kind s3client.VersionKind,
	payload []byte,
	etag string,
) string {
	for index := range versions[key] {
		versions[key][index].latest = false
	}
	m.nextID++
	id := fmt.Sprintf("v-%d", m.nextID)
	versions[key] = append([]awsVersion{{
		kind: kind, id: id, etag: etag, payload: append([]byte(nil), payload...), latest: true,
	}}, versions[key]...)
	return id
}

func (m *awsModel) findLocked(
	t *testing.T,
	versions map[string][]awsVersion,
	key, id string,
) awsVersion {
	t.Helper()
	for _, version := range versions[key] {
		if version.id == id {
			return version
		}
	}
	require.FailNowf(t, "missing version", "key=%q id=%q", key, id)
	return awsVersion{}
}

func (m *awsModel) bucketLocked(bucket string) map[string][]awsVersion {
	if bucket == "source" {
		return m.source
	}
	return m.destination
}

func newAWSClients() (*testutil.S3, *testutil.S3, *awsModel) {
	model := &awsModel{
		source:      make(map[string][]awsVersion),
		destination: make(map[string][]awsVersion),
	}
	source := newAWSClient(model, "source")
	destination := newAWSClient(model, "destination")
	source.BucketReplicationFunc = replicationRules(s3client.ReplicationRule{
		Enabled:                 true,
		DestinationBucket:       "destination",
		Prefix:                  "netdata-s3check/",
		DeleteMarkerReplication: true,
	})
	destination.BucketReplicationFunc = func(context.Context, string) ([]s3client.ReplicationRule, error) {
		return nil, s3client.ErrReplicationConfigAbsent
	}
	return source, destination, model
}

func newAWSClient(model *awsModel, bucketName string) *testutil.S3 {
	client := &testutil.S3{}
	client.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		return s3client.BucketVersioningResult{
			Status: s3client.VersioningEnabled,
		}, nil
	}
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, opts s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.mu.Lock()
		defer model.mu.Unlock()
		if bucket != bucketName {
			return s3client.PutResult{}, fmt.Errorf("unexpected bucket %q", bucket)
		}
		model.lastPutConditional = opts.IfNoneMatch
		versions := model.bucketLocked(bucket)
		if opts.IfNoneMatch && len(versions[key]) > 0 && versions[key][0].kind == s3client.VersionObject {
			return s3client.PutResult{}, errors.New("precondition failed")
		}
		etag := fmt.Sprintf("etag-%d", model.nextID+1)
		id := model.addLocked(versions, key, s3client.VersionObject, payload, etag)
		if model.failPutAfterMutation {
			model.failPutAfterMutation = false
			return s3client.PutResult{}, errors.New("ambiguous PUT")
		}
		return s3client.PutResult{
			VersionID: id,
			ETag:      etag,
		}, nil
	}
	client.GetFunc = func(
		_ context.Context, bucket, key, versionID string, _ int64,
	) (s3client.GetResult, error) {
		model.mu.Lock()
		defer model.mu.Unlock()
		versions := model.bucketLocked(bucket)
		var found *awsVersion
		for index := range versions[key] {
			if versionID == "" && versions[key][index].latest || versions[key][index].id == versionID {
				found = &versions[key][index]
				break
			}
		}
		if found == nil || found.kind == s3client.VersionDeleteMarker {
			return s3client.GetResult{}, s3client.ErrObjectNotFound
		}
		return s3client.GetResult{
			Payload:   append([]byte(nil), found.payload...),
			VersionID: found.id,
			ETag:      found.etag,
		}, nil
	}
	client.ListCurrentFunc = func(context.Context, string, string, int32) (s3client.CurrentPage, error) {
		return s3client.CurrentPage{}, nil
	}
	client.ListVersionsFunc = func(
		_ context.Context, bucket, prefix, _, _ string, _ int32,
	) (s3client.VersionPage, error) {
		model.mu.Lock()
		defer model.mu.Unlock()
		var keys []string
		for key := range model.bucketLocked(bucket) {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		var page s3client.VersionPage
		for _, key := range keys {
			for _, version := range model.bucketLocked(bucket)[key] {
				page.Versions = append(page.Versions, s3client.Version{
					Kind:      version.kind,
					Key:       key,
					VersionID: version.id,
					IsLatest:  version.latest,
				})
			}
		}
		return page, nil
	}
	client.DeleteFunc = func(
		_ context.Context, bucket, key string, opts s3client.DeleteOptions,
	) (s3client.DeleteResult, error) {
		model.mu.Lock()
		defer model.mu.Unlock()
		versions := model.bucketLocked(bucket)
		if opts.VersionID != "" {
			for index, version := range versions[key] {
				if version.id == opts.VersionID {
					versions[key] = append(versions[key][:index], versions[key][index+1:]...)
					if len(versions[key]) > 0 {
						versions[key][0].latest = true
					} else {
						delete(versions, key)
					}
					return s3client.DeleteResult{}, nil
				}
			}
			return s3client.DeleteResult{}, nil
		}
		if len(versions[key]) == 0 || versions[key][0].kind != s3client.VersionObject {
			return s3client.DeleteResult{}, errors.New("precondition failed")
		}
		if opts.IfMatch == "" || versions[key][0].etag != opts.IfMatch {
			return s3client.DeleteResult{}, errors.New("precondition failed")
		}
		model.lastLogicalDeleteIfMatch = opts.IfMatch
		id := model.addLocked(versions, key, s3client.VersionDeleteMarker, nil, "")
		if model.failDeleteAfterMutation {
			model.failDeleteAfterMutation = false
			return s3client.DeleteResult{}, errors.New("ambiguous DELETE")
		}
		return s3client.DeleteResult{
			VersionID:    id,
			DeleteMarker: true,
		}, nil
	}
	return client
}
