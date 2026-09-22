// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorInitRequiresPersistedAgentIdentityBeforeClientCreation(t *testing.T) {
	c := New()
	c.Config = validConfig(contract.ModeLifecycle)
	c.registryUniqueID = func() string { return "" }
	created := false
	c.newS3Client = func(context.Context, s3client.Config) (s3client.Client, error) {
		created = true
		return nil, errors.New("must not be called")
	}

	assert.ErrorContains(t, c.Init(context.Background()), "persisted Agent registry identity")
	assert.False(t, created)
}

func TestResolveJournalRootPrefersRuntimeVarLib(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "runtime-varlib")
	got := resolveJournalRoot(runtimeDir, "/compiled/varlib", "/default/varlib")
	assert.Equal(t, filepath.Join(runtimeDir, "s3check"), got)
}

func TestCollectorCheckIsReadOnlyForEveryMode(t *testing.T) {
	for _, mode := range []contract.Mode{
		contract.ModeLifecycle,
		contract.ModeCephMultisite,
		contract.ModeAWSReplication,
	} {
		t.Run(string(mode), func(t *testing.T) {
			root := t.TempDir() + "/journal"
			clients := checkClients(mode)
			c := New()
			c.Config = validConfig(mode)
			c.registryUniqueID = func() string { return "agent-id" }
			c.journalRoot = root
			c.newS3Client = clientSequence(clients...)

			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			for _, client := range clients {
				assert.Zero(t, client.Count("put"))
			}
			_, err := os.Stat(root)
			assert.ErrorIs(t, err, os.ErrNotExist)
			c.Cleanup(context.Background())
		})
	}
}

func TestCollectorClosesSourceWhenDestinationCreationFails(t *testing.T) {
	source := checkClient(s3client.VersioningDisabled)
	c := New()
	c.Config = validConfig(contract.ModeCephMultisite)
	c.registryUniqueID = func() string { return "agent-id" }
	c.journalRoot = t.TempDir()
	calls := 0
	c.newS3Client = func(context.Context, s3client.Config) (s3client.Client, error) {
		calls++
		if calls == 1 {
			return source, nil
		}
		return nil, errors.New("destination unavailable")
	}

	assert.ErrorContains(t, c.Init(context.Background()), "create destination S3 client")
	assert.Equal(t, 1, source.Closed)
}

func TestCollectorUsesIndependentLogicalCallTimeouts(t *testing.T) {
	source := checkClient(s3client.VersioningDisabled)
	destination := checkClient(s3client.VersioningDisabled)
	var sourceDeadline, destinationDeadline time.Duration
	source.BucketVersioningFunc = func(ctx context.Context, _ string) (s3client.BucketVersioningResult, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		sourceDeadline = time.Until(deadline)
		return s3client.BucketVersioningResult{
			Status: s3client.VersioningDisabled,
		}, nil
	}
	destination.BucketVersioningFunc = func(ctx context.Context, _ string) (s3client.BucketVersioningResult, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		destinationDeadline = time.Until(deadline)
		return s3client.BucketVersioningResult{
			Status: s3client.VersioningDisabled,
		}, nil
	}

	c := New()
	c.Config = validConfig(contract.ModeCephMultisite)
	c.ModeCephMultisite.Source.Timeout = confopt.LongDuration(100 * time.Millisecond)
	c.ModeCephMultisite.Destination.Timeout = confopt.LongDuration(500 * time.Millisecond)
	c.registryUniqueID = func() string { return "agent-id" }
	c.journalRoot = t.TempDir()
	c.newS3Client = clientSequence(source, destination)

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	assert.Less(t, sourceDeadline, 250*time.Millisecond)
	assert.Greater(t, destinationDeadline, 350*time.Millisecond)
	c.Cleanup(context.Background())
}

func TestCollectorLogsRawOperationFailure(t *testing.T) {
	wantErr := errors.New("provider access denied sentinel")
	client := checkClient(s3client.VersioningDisabled)
	client.PutFunc = func(context.Context, string, string, []byte, s3client.PutOptions) (s3client.PutResult, error) {
		return s3client.PutResult{}, wantErr
	}

	var log bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&log)
	c.Config = validConfig(contract.ModeLifecycle)
	c.registryUniqueID = func() string { return "agent-id" }
	c.journalRoot = t.TempDir()
	c.newS3Client = clientSequence(client)

	require.NoError(t, c.Init(context.Background()))
	cc := mustCycleController(t, c.store)
	cc.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	cc.CommitCycleSuccess()
	assert.Contains(t, log.String(), wantErr.Error())
	c.Cleanup(context.Background())
}

func checkClients(mode contract.Mode) []*testutil.S3 {
	switch mode {
	case contract.ModeLifecycle:
		return []*testutil.S3{checkClient(s3client.VersioningDisabled)}
	case contract.ModeCephMultisite:
		return []*testutil.S3{
			checkClient(s3client.VersioningDisabled),
			checkClient(s3client.VersioningDisabled),
		}
	case contract.ModeAWSReplication:
		source := checkClient(s3client.VersioningEnabled)
		source.BucketReplicationFunc = func(context.Context, string) ([]s3client.ReplicationRule, error) {
			return []s3client.ReplicationRule{{
				Enabled: true, DestinationBucket: "destination-bucket", Prefix: defaultPrefix,
				DeleteMarkerReplication: true,
			}}, nil
		}
		return []*testutil.S3{source, checkClient(s3client.VersioningEnabled)}
	default:
		panic("unsupported test mode")
	}
}

func checkClient(versioning s3client.VersioningStatus) *testutil.S3 {
	return &testutil.S3{
		BucketVersioningFunc: func(context.Context, string) (s3client.BucketVersioningResult, error) {
			return s3client.BucketVersioningResult{
				Status: versioning,
			}, nil
		},
		BucketReplicationFunc: func(context.Context, string) ([]s3client.ReplicationRule, error) { return nil, nil },
		PutFunc: func(context.Context, string, string, []byte, s3client.PutOptions) (s3client.PutResult, error) {
			return s3client.PutResult{}, nil
		},
		GetFunc: func(context.Context, string, string, string, int64) (s3client.GetResult, error) {
			return s3client.GetResult{}, s3client.ErrObjectNotFound
		},
		ListCurrentFunc: func(context.Context, string, string, int32) (s3client.CurrentPage, error) {
			return s3client.CurrentPage{}, nil
		},
		ListVersionsFunc: func(context.Context, string, string, string, string, int32) (s3client.VersionPage, error) {
			return s3client.VersionPage{}, nil
		},
		DeleteFunc: func(context.Context, string, string, s3client.DeleteOptions) (s3client.DeleteResult, error) {
			return s3client.DeleteResult{}, nil
		},
	}
}

func clientSequence(clients ...*testutil.S3) s3ClientFactory {
	index := 0
	return func(context.Context, s3client.Config) (s3client.Client, error) {
		if index >= len(clients) {
			return nil, errors.New("unexpected S3 client creation")
		}
		client := clients[index]
		index++
		return client, nil
	}
}
