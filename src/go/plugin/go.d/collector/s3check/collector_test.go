// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	"errors"
	"os"
	"testing"

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
			return s3client.BucketVersioningResult{Status: versioning}, nil
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
