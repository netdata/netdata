// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestCheckReadsBothUnversionedBucketsWithoutMutation(t *testing.T) {
	root := t.TempDir()
	j := newCephJournal(t, root)
	source, destination, _ := newCephClients()
	engine, err := New(Options{
		Source:               source,
		Destination:          destination,
		SourceBucket:         "source",
		DestinationBucket:    "destination",
		Journal:              j,
		Generator:            newCephGenerator(j.OwnerID()),
		SourceRequestTimeout: time.Second, DestinationRequestTimeout: time.Second,
		WriteObjective:  10 * time.Minute,
		WriteTimeout:    20 * time.Minute,
		DeleteObjective: 5 * time.Minute,
		DeleteTimeout:   10 * time.Minute,
	})
	require.NoError(t, err)

	require.NoError(t, engine.Check(context.Background()))
	assert.Equal(t, 1, source.Count("bucket_versioning"))
	assert.Equal(t, 1, destination.Count("bucket_versioning"))
	assert.Zero(t, source.Count("put"))
	assert.Zero(t, destination.Count("put"))
}

func TestCollectRevalidatesBucketVersioningBeforeMutation(t *testing.T) {
	j := newCephJournal(t, t.TempDir())
	source, destination, _ := newCephClients()
	engine, err := New(Options{
		Source: source, Destination: destination,
		SourceBucket: "source", DestinationBucket: "destination",
		Journal: j, Generator: newCephGenerator(j.OwnerID()),
		SourceRequestTimeout: time.Second, DestinationRequestTimeout: time.Second,
		WriteObjective: time.Minute, WriteTimeout: 2 * time.Minute,
		DeleteObjective: time.Minute, DeleteTimeout: 2 * time.Minute,
	})
	require.NoError(t, err)
	require.NoError(t, engine.Check(context.Background()))
	destination.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		return s3client.BucketVersioningResult{Status: s3client.VersioningEnabled}, nil
	}

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Nil(t, result.Probe)
	assert.Zero(t, source.Count("put"))
	engine.Cleanup(context.Background())
}

func TestDirectionalProbePreservesExactKeyAndMeasuresSuccessfulEvents(t *testing.T) {
	j := newCephJournal(t, t.TempDir())
	source, destination, model := newCephClients()
	now := time.Unix(100, 0)
	engine, err := New(Options{
		Source:               source,
		Destination:          destination,
		SourceBucket:         "source",
		DestinationBucket:    "different-destination",
		Journal:              j,
		Generator:            newCephGenerator(j.OwnerID()),
		SourceRequestTimeout: time.Second, DestinationRequestTimeout: time.Second,
		WriteObjective:  15 * time.Second,
		WriteTimeout:    time.Minute,
		DeleteObjective: 10 * time.Second,
		DeleteTimeout:   time.Minute,
		Now:             func() time.Time { return now },
	})
	require.NoError(t, err)

	waitingWrite := engine.Collect(context.Background())
	require.NotNil(t, waitingWrite.Probe)
	assert.Equal(t, contract.StatusWaiting, waitingWrite.Probe.Status)
	assert.False(t, waitingWrite.Probe.PayloadCompared)
	require.Len(t, model.source, 1)
	var sourceKey string
	for key := range model.source {
		sourceKey = key
	}

	now = now.Add(10 * time.Second)
	model.replicateWrite(sourceKey)
	waitingDelete := engine.Collect(context.Background())
	require.NotNil(t, waitingDelete.Probe)
	assert.Equal(t, contract.StatusWaiting, waitingDelete.Probe.Status)
	assert.True(t, waitingDelete.Probe.PayloadCompared)
	assert.True(t, waitingDelete.Probe.WriteVisibility.Performed)
	assert.Equal(t, 10*time.Second, waitingDelete.Probe.WriteVisibility.Lag)
	assert.Contains(t, model.destination, sourceKey)

	now = now.Add(5 * time.Second)
	model.replicateDelete(sourceKey)
	success := engine.Collect(context.Background())
	require.NotNil(t, success.Probe)
	assert.Equal(t, contract.StatusSuccess, success.Probe.Status)
	assert.True(t, success.Probe.PayloadCompared)
	assert.True(t, success.Probe.DeleteVisibility.Performed)
	assert.Equal(t, 5*time.Second, success.Probe.DeleteVisibility.Lag)
	assert.Equal(t, sourceKey, model.lastDestinationReadKey)
	assert.Zero(t, success.Cleanup.Pending)
	engine.Cleanup(context.Background())
}

func TestWriteTimeoutMovesProbeToCleanupWithoutBlockingNewActiveSlot(t *testing.T) {
	j := newCephJournal(t, t.TempDir())
	source, destination, _ := newCephClients()
	now := time.Unix(100, 0)
	engine, err := New(Options{
		Source:               source,
		Destination:          destination,
		SourceBucket:         "source",
		DestinationBucket:    "destination",
		Journal:              j,
		Generator:            newCephGenerator(j.OwnerID()),
		SourceRequestTimeout: time.Second, DestinationRequestTimeout: time.Second,
		WriteObjective:  time.Minute,
		WriteTimeout:    2 * time.Minute,
		DeleteObjective: time.Minute,
		DeleteTimeout:   2 * time.Minute,
		QueueCapacity:   3,
		Now:             func() time.Time { return now },
	})
	require.NoError(t, err)

	first := engine.Collect(context.Background())
	require.NotNil(t, first.Probe)
	assert.Equal(t, contract.StatusWaiting, first.Probe.Status)
	assert.Equal(t, 1, source.Count("put"))

	now = now.Add(2*time.Minute + time.Second)
	timedOut := engine.Collect(context.Background())
	require.NotNil(t, timedOut.Probe)
	assert.Equal(t, contract.StatusFailed, timedOut.Probe.Status)
	assert.Equal(t, contract.ReasonVisibilityTimeout, timedOut.Probe.Reason)

	next := engine.Collect(context.Background())
	require.NotNil(t, next.Probe)
	assert.Equal(t, contract.StatusWaiting, next.Probe.Status)
	assert.Equal(t, 2, source.Count("put"))
	assert.GreaterOrEqual(t, next.Cleanup.Pending, 1)
	engine.Cleanup(context.Background())
}

func TestJournalFailurePreservesBackpressureAndLastTerminal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "state")
	j := newCephJournal(t, root)
	source, destination, _ := newCephClients()
	now := time.Unix(100, 0)
	engine, err := New(Options{
		Source: source, Destination: destination,
		SourceBucket: "source", DestinationBucket: "destination",
		Journal: j, Generator: newCephGenerator(j.OwnerID()),
		SourceRequestTimeout: time.Second, DestinationRequestTimeout: time.Second,
		WriteObjective: time.Minute, WriteTimeout: 2 * time.Minute,
		DeleteObjective: time.Minute, DeleteTimeout: 2 * time.Minute,
		QueueCapacity: 1, CleanupBatch: 1, Now: func() time.Time { return now },
	})
	require.NoError(t, err)

	engine.Collect(context.Background())
	now = now.Add(2*time.Minute + time.Second)
	terminal := engine.Collect(context.Background())
	require.NotNil(t, terminal.LastTerminal)
	require.NoError(t, os.Rename(root, root+".moved"))
	require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o600))

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Nil(t, result.Probe)
	assert.Equal(t, 1, result.Cleanup.Pending)
	assert.True(t, result.Cleanup.Backpressure)
	assert.Equal(t, terminal.LastTerminal, result.LastTerminal)
	engine.Cleanup(context.Background())
}

func newCephJournal(t *testing.T, root string) *journal.Journal {
	t.Helper()
	j, err := journal.New(
		root,
		"agent",
		"job",
		journal.Fingerprint(
			"ceph_multisite",
			"https://source.example",
			"source",
			"netdata-s3check/",
			"https://destination.example",
			"destination",
			"exact-key",
		),
	)
	require.NoError(t, err)
	return j
}

func newCephGenerator(ownerID string) probe.Generator {
	return probe.Generator{
		Prefix:  "netdata-s3check/",
		OwnerID: ownerID,
		Now:     time.Now,
		Random:  bytes.NewReader(bytes.Repeat([]byte{2}, 64*(16+probe.PayloadBytes))),
	}
}

type cephModel struct {
	mu sync.Mutex

	source                 map[string][]byte
	destination            map[string][]byte
	lastDestinationReadKey string
}

func (m *cephModel) replicateWrite(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destination[key] = append([]byte(nil), m.source[key]...)
}

func (m *cephModel) replicateDelete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.destination, key)
}

func newCephClients() (*testutil.S3, *testutil.S3, *cephModel) {
	model := &cephModel{source: make(map[string][]byte), destination: make(map[string][]byte)}
	source := &testutil.S3{}
	destination := &testutil.S3{}
	versioning := func(context.Context, string) (s3client.BucketVersioningResult, error) {
		return s3client.BucketVersioningResult{Status: s3client.VersioningDisabled}, nil
	}
	source.BucketVersioningFunc = versioning
	destination.BucketVersioningFunc = versioning
	source.BucketReplicationFunc = func(context.Context, string) ([]s3client.ReplicationRule, error) {
		return nil, s3client.ErrReplicationConfigAbsent
	}
	destination.BucketReplicationFunc = source.BucketReplicationFunc
	source.PutFunc = func(_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions) (s3client.PutResult, error) {
		if bucket != "source" {
			return s3client.PutResult{}, fmt.Errorf("unexpected source bucket %q", bucket)
		}
		model.mu.Lock()
		defer model.mu.Unlock()
		model.source[key] = append([]byte(nil), payload...)
		return s3client.PutResult{}, nil
	}
	destination.PutFunc = func(context.Context, string, string, []byte, s3client.PutOptions) (s3client.PutResult, error) {
		panic("destination PUT is not part of the Ceph probe")
	}
	source.GetFunc = func(_ context.Context, bucket, key, _ string, _ int64) (s3client.GetResult, error) {
		if bucket != "source" {
			return s3client.GetResult{}, fmt.Errorf("unexpected source bucket %q", bucket)
		}
		model.mu.Lock()
		defer model.mu.Unlock()
		payload, ok := model.source[key]
		if !ok {
			return s3client.GetResult{}, s3client.ErrObjectNotFound
		}
		return s3client.GetResult{Payload: append([]byte(nil), payload...)}, nil
	}
	destination.GetFunc = func(_ context.Context, bucket, key, _ string, _ int64) (s3client.GetResult, error) {
		if bucket != "different-destination" && bucket != "destination" {
			return s3client.GetResult{}, fmt.Errorf("unexpected destination bucket %q", bucket)
		}
		model.mu.Lock()
		defer model.mu.Unlock()
		model.lastDestinationReadKey = key
		payload, ok := model.destination[key]
		if !ok {
			return s3client.GetResult{}, s3client.ErrObjectNotFound
		}
		return s3client.GetResult{Payload: append([]byte(nil), payload...)}, nil
	}
	source.DeleteFunc = func(_ context.Context, bucket, key string, _ s3client.DeleteOptions) (s3client.DeleteResult, error) {
		if bucket != "source" {
			return s3client.DeleteResult{}, fmt.Errorf("unexpected source bucket %q", bucket)
		}
		model.mu.Lock()
		defer model.mu.Unlock()
		delete(model.source, key)
		return s3client.DeleteResult{}, nil
	}
	destination.DeleteFunc = func(_ context.Context, bucket, key string, _ s3client.DeleteOptions) (s3client.DeleteResult, error) {
		if bucket != "different-destination" && bucket != "destination" {
			return s3client.DeleteResult{}, fmt.Errorf("unexpected destination bucket %q", bucket)
		}
		model.mu.Lock()
		defer model.mu.Unlock()
		delete(model.destination, key)
		return s3client.DeleteResult{}, nil
	}
	emptyCurrent := func(context.Context, string, string, int32) (s3client.CurrentPage, error) {
		return s3client.CurrentPage{}, nil
	}
	emptyVersions := func(context.Context, string, string, string, string, int32) (s3client.VersionPage, error) {
		return s3client.VersionPage{}, nil
	}
	source.ListCurrentFunc = emptyCurrent
	destination.ListCurrentFunc = emptyCurrent
	source.ListVersionsFunc = emptyVersions
	destination.ListVersionsFunc = emptyVersions
	return source, destination, model
}
