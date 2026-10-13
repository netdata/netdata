// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"bytes"
	"context"
	"errors"
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

func TestCheckIsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "journal")
	j := newTestJournal(t, root)
	client, _ := newLifecycleClient()
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)

	require.NoError(t, engine.Check(context.Background()))
	assert.Equal(t, 1, client.Count("bucket_versioning"))
	assert.Zero(t, client.Count("put"))
	_, err = os.Stat(root)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestCollectRevalidatesBucketVersioningBeforeMutation(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, _ := newLifecycleClient()
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)
	require.NoError(t, engine.Check(context.Background()))
	client.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		return s3client.BucketVersioningResult{
			Status: s3client.VersioningEnabled,
		}, nil
	}

	result := engine.Collect(context.Background())
	assert.Error(t, result.Err)
	assert.Nil(t, result.Probe)
	assert.Zero(t, client.Count("put"))
	engine.Cleanup(context.Background())
}

func TestCollectRetainsOwnershipWhenPutCreatesVersion(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{
			VersionID: "version-1",
		}, nil
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)

	result := engine.Collect(context.Background())
	assert.ErrorContains(t, result.Err, "versioning")
	assert.Equal(t, 1, result.Cleanup.Pending)
	assert.Zero(t, client.Count("delete"))
	var persisted state
	found, err := j.Load(&persisted)
	require.NoError(t, err)
	require.True(t, found)
	assert.Len(t, persisted.Entries, 1)
	assert.Equal(t, result.Probe, persisted.LastTerminal)
	engine.Cleanup(context.Background())
}

func TestCollectRevalidatesVersioningImmediatelyBeforeDelete(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, _ := newLifecycleClient()
	checks := 0
	client.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		checks++
		status := s3client.VersioningDisabled
		if checks > 1 {
			status = s3client.VersioningEnabled
		}
		return s3client.BucketVersioningResult{
			Status: status,
		}, nil
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)

	result := engine.Collect(context.Background())
	assert.ErrorContains(t, result.Err, "versioning")
	assert.Equal(t, 2, client.Count("bucket_versioning"))
	assert.Zero(t, client.Count("delete"))
	assert.Equal(t, 1, result.Cleanup.Pending)
	requirePersistedEntries(t, j, 1)
	engine.Cleanup(context.Background())
}

func TestCollectRetainsOwnershipWhenDeleteCreatesMarker(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.DeleteFunc = func(
		_ context.Context, bucket, key string, _ s3client.DeleteOptions,
	) (s3client.DeleteResult, error) {
		model.delete(bucket, key)
		return s3client.DeleteResult{
			VersionID:    "marker-1",
			DeleteMarker: true,
		}, nil
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)

	result := engine.Collect(context.Background())
	assert.ErrorContains(t, result.Err, "versioning")
	assert.Equal(t, 1, client.Count("get"), "unsafe delete must not be reconciled as unversioned absence")
	assert.Equal(t, 1, result.Cleanup.Pending)
	requirePersistedEntries(t, j, 1)
	engine.Cleanup(context.Background())
}

func TestCleanupRevalidatesBucketVersioningBeforeDelete(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
		QueueCapacity:  1,
		CleanupBatch:   1,
	})
	require.NoError(t, err)

	result := engine.Collect(context.Background())
	require.Equal(t, 1, result.Cleanup.Pending)
	client.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		return s3client.BucketVersioningResult{
			Status: s3client.VersioningEnabled,
		}, nil
	}
	deletesBefore := client.Count("delete")

	engine.Cleanup(context.Background())
	assert.Equal(t, deletesBefore, client.Count("delete"))
	var persisted state
	found, err := j.Load(&persisted)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Len(t, persisted.Entries, 1)
}

func TestBacklogRevalidatesVersioningImmediatelyBeforeDelete(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
		QueueCapacity:  1,
		CleanupBatch:   1,
	})
	require.NoError(t, err)
	first := engine.Collect(context.Background())
	require.Equal(t, 1, first.Cleanup.Pending)

	checks := 0
	client.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		checks++
		status := s3client.VersioningDisabled
		if checks > 1 {
			status = s3client.VersioningEnabled
		}
		return s3client.BucketVersioningResult{
			Status: status,
		}, nil
	}
	result := engine.Collect(context.Background())
	assert.ErrorContains(t, result.Err, "versioning")
	assert.Equal(t, 2, checks)
	assert.Zero(t, client.Count("delete"))
	assert.Equal(t, 1, result.Cleanup.Pending)
	requirePersistedEntries(t, j, 1)
	engine.Cleanup(context.Background())
}

func TestBacklogRetainsOwnershipWhenDeleteCreatesMarker(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
		QueueCapacity:  1,
		CleanupBatch:   1,
	})
	require.NoError(t, err)
	first := engine.Collect(context.Background())
	require.Equal(t, 1, first.Cleanup.Pending)

	client.DeleteFunc = func(
		_ context.Context, bucket, key string, _ s3client.DeleteOptions,
	) (s3client.DeleteResult, error) {
		model.delete(bucket, key)
		return s3client.DeleteResult{
			VersionID:    "marker-1",
			DeleteMarker: true,
		}, nil
	}
	getsBefore := client.Count("get")
	result := engine.Collect(context.Background())
	assert.ErrorContains(t, result.Err, "versioning")
	assert.Equal(t, getsBefore, client.Count("get"), "unsafe delete must not be reconciled as unversioned absence")
	assert.Equal(t, 1, result.Cleanup.Pending)
	requirePersistedEntries(t, j, 1)
	engine.Cleanup(context.Background())
}

func TestSuccessfulProbeDoesNotEnterQuarantine(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, _ := newLifecycleClient()
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)

	first := engine.Collect(context.Background())
	require.NotNil(t, first.Probe)
	assert.Equal(t, contract.StatusSuccess, first.Probe.Status)
	assert.True(t, first.Probe.PayloadCompared)
	assert.Zero(t, first.Cleanup.Pending)
	_, statErr := os.Stat(j.Path())
	assert.ErrorIs(t, statErr, os.ErrNotExist)

	second := engine.Collect(context.Background())
	require.NotNil(t, second.Probe)
	assert.Equal(t, contract.StatusSuccess, second.Probe.Status)
	assert.True(t, second.Probe.PayloadCompared)
	assert.Equal(t, 2, client.Count("put"))
	engine.Cleanup(context.Background())
}

func TestBackpressureNeverEvictsUnresolvedOwnership(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.PutFunc = func(_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	client.DeleteFunc = func(context.Context, string, string, s3client.DeleteOptions) (s3client.DeleteResult, error) {
		return s3client.DeleteResult{}, errors.New("cleanup unavailable")
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
		QueueCapacity:  2,
		CleanupBatch:   1,
	})
	require.NoError(t, err)

	first := engine.Collect(context.Background())
	second := engine.Collect(context.Background())
	third := engine.Collect(context.Background())

	require.NotNil(t, first.Probe)
	require.NotNil(t, second.Probe)
	assert.Equal(t, contract.StatusFailed, first.Probe.Status)
	assert.Equal(t, contract.StatusFailed, second.Probe.Status)
	assert.Nil(t, third.Probe)
	assert.True(t, third.Cleanup.Backpressure)
	assert.Equal(t, 2, third.Cleanup.Pending)
	assert.Equal(t, 2, client.Count("put"))

	var persisted state
	found, err := j.Load(&persisted)
	require.NoError(t, err)
	require.True(t, found)
	assert.Len(t, persisted.Entries, 2)
	engine.Cleanup(context.Background())
}

func TestJournalFailurePreservesBackpressureAndLastTerminal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "state")
	j := newTestJournal(t, root)
	client, model := newLifecycleClient()
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	client.DeleteFunc = func(context.Context, string, string, s3client.DeleteOptions) (s3client.DeleteResult, error) {
		return s3client.DeleteResult{}, errors.New("cleanup unavailable")
	}
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
		QueueCapacity:  1,
		CleanupBatch:   1,
	})
	require.NoError(t, err)

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

func TestAmbiguousPutNeedsLaterAbsenceConfirmationThenResumes(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	client.PutFunc = func(_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	client.DeleteFunc = func(_ context.Context, bucket, key string, _ s3client.DeleteOptions) (s3client.DeleteResult, error) {
		model.delete(bucket, key)
		return s3client.DeleteResult{}, nil
	}
	now := time.Unix(100, 0)
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
		QueueCapacity:  2,
		CleanupBatch:   2,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)

	_ = engine.Collect(context.Background())
	_ = engine.Collect(context.Background())
	atCapacity := engine.Collect(context.Background())
	assert.True(t, atCapacity.Cleanup.Backpressure)
	assert.Equal(t, 2, atCapacity.Cleanup.Pending)

	now = now.Add(time.Minute + time.Second)
	client.PutFunc = func(_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, nil
	}
	resumed := engine.Collect(context.Background())
	assert.False(t, resumed.Cleanup.Backpressure)
	assert.Equal(t, 3, client.Count("put"))
	engine.Cleanup(context.Background())
}

func TestAmbiguousPutRetainsOwnershipForRequestUncertaintyWindow(t *testing.T) {
	j := newTestJournal(t, t.TempDir())
	client, model := newLifecycleClient()
	var key string
	var payload []byte
	client.PutFunc = func(
		_ context.Context, bucket, objectKey string, objectPayload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		if key == "" {
			key = objectKey
			payload = append([]byte(nil), objectPayload...)
		}
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	client.DeleteFunc = func(
		_ context.Context, bucket, objectKey string, _ s3client.DeleteOptions,
	) (s3client.DeleteResult, error) {
		model.delete(bucket, objectKey)
		return s3client.DeleteResult{}, nil
	}
	now := time.Unix(100, 0)
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Minute,
		UpdateEvery:    time.Second,
		QueueCapacity:  1,
		CleanupBatch:   1,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)
	t.Cleanup(func() { engine.Cleanup(context.Background()) })

	first := engine.Collect(context.Background())
	require.NotNil(t, first.Probe)
	require.Equal(t, contract.StatusFailed, first.Probe.Status)
	require.NotEmpty(t, key)
	firstAbsence := engine.Collect(context.Background())
	require.Equal(t, 1, firstAbsence.Cleanup.Pending)

	now = now.Add(2 * time.Second)
	beforeLatePut := engine.Collect(context.Background())
	assert.Equal(t, 1, beforeLatePut.Cleanup.Pending)
	assert.Equal(t, 1, client.Count("put"))

	model.put("bucket", key, payload)
	afterLatePut := engine.Collect(context.Background())
	_, exists := model.get("bucket", key)
	assert.False(t, exists)
	assert.Equal(t, 1, afterLatePut.Cleanup.Pending)
	assert.Equal(t, 1, client.Count("put"))
}

func TestSameOwnerTakeoverReloadsIncumbentOwnership(t *testing.T) {
	root := t.TempDir()
	firstJournal := newTestJournal(t, root)
	secondJournal := newTestJournal(t, root)
	client, model := newLifecycleClient()
	client.PutFunc = func(
		_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions,
	) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, errors.New("ambiguous put")
	}
	client.DeleteFunc = func(context.Context, string, string, s3client.DeleteOptions) (s3client.DeleteResult, error) {
		return s3client.DeleteResult{}, errors.New("cleanup unavailable")
	}
	newEngine := func(j *journal.Journal) *Engine {
		engine, err := New(Options{
			Client:         client,
			Bucket:         "bucket",
			Journal:        j,
			Generator:      newGenerator(j.OwnerID()),
			RequestTimeout: time.Second,
			UpdateEvery:    time.Minute,
			QueueCapacity:  1,
			CleanupBatch:   1,
		})
		require.NoError(t, err)
		return engine
	}
	incumbent := newEngine(firstJournal)
	successor := newEngine(secondJournal)

	first := incumbent.Collect(context.Background())
	require.NotNil(t, first.Probe)
	blocked := successor.Collect(context.Background())
	require.NotNil(t, blocked.Probe)
	assert.Equal(t, contract.ReasonOwnership, blocked.Probe.Reason)
	assert.Equal(t, 1, client.Count("put"))

	incumbent.Cleanup(context.Background())
	takenOver := successor.Collect(context.Background())
	assert.Equal(t, 1, takenOver.Cleanup.Pending)
	assert.True(t, takenOver.Cleanup.Backpressure)
	assert.Equal(
		t,
		1,
		client.Count("put"),
		"successor must not overwrite incumbent state with its stale constructor snapshot",
	)
	successor.Cleanup(context.Background())
}

func newTestJournal(t *testing.T, root string) *journal.Journal {
	t.Helper()
	j, err := journal.New(
		root,
		"agent",
		"job",
		journal.Fingerprint("lifecycle", "endpoint", "bucket", "netdata-s3check/"),
	)
	require.NoError(t, err)
	return j
}

func requirePersistedEntries(t *testing.T, j *journal.Journal, count int) {
	t.Helper()
	var persisted state
	found, err := j.Load(&persisted)
	require.NoError(t, err)
	require.True(t, found)
	assert.Len(t, persisted.Entries, count)
}

func newGenerator(ownerID string) probe.Generator {
	return probe.Generator{
		Prefix:  "netdata-s3check/",
		OwnerID: ownerID,
		Now:     func() time.Time { return time.Now().UTC() },
		Random:  bytes.NewReader(bytes.Repeat([]byte{1}, 64*(16+probe.PayloadBytes))),
	}
}

type objectModel struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (m *objectModel) objectKey(bucket, key string) string { return bucket + "\x00" + key }

func (m *objectModel) put(bucket, key string, payload []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[m.objectKey(bucket, key)] = append([]byte(nil), payload...)
}

func (m *objectModel) get(bucket, key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	payload, ok := m.objects[m.objectKey(bucket, key)]
	return append([]byte(nil), payload...), ok
}

func (m *objectModel) delete(bucket, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, m.objectKey(bucket, key))
}

func (m *objectModel) keys(bucket, prefix string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for combined := range m.objects {
		objectBucket, key, _ := bytes.Cut([]byte(combined), []byte{0})
		if string(objectBucket) == bucket && bytes.HasPrefix(key, []byte(prefix)) {
			keys = append(keys, string(key))
		}
	}
	return keys
}

func newLifecycleClient() (*testutil.S3, *objectModel) {
	client := &testutil.S3{}
	model := &objectModel{
		objects: make(map[string][]byte),
	}
	client.BucketVersioningFunc = func(context.Context, string) (s3client.BucketVersioningResult, error) {
		return s3client.BucketVersioningResult{
			Status: s3client.VersioningDisabled,
		}, nil
	}
	client.BucketReplicationFunc = func(context.Context, string) ([]s3client.ReplicationRule, error) {
		return nil, s3client.ErrReplicationConfigAbsent
	}
	client.PutFunc = func(_ context.Context, bucket, key string, payload []byte, _ s3client.PutOptions) (s3client.PutResult, error) {
		model.put(bucket, key, payload)
		return s3client.PutResult{}, nil
	}
	client.GetFunc = func(_ context.Context, bucket, key, _ string, _ int64) (s3client.GetResult, error) {
		payload, ok := model.get(bucket, key)
		if !ok {
			return s3client.GetResult{}, s3client.ErrObjectNotFound
		}
		return s3client.GetResult{
			Payload: payload,
		}, nil
	}
	client.ListCurrentFunc = func(_ context.Context, bucket, prefix string, _ int32) (s3client.CurrentPage, error) {
		return s3client.CurrentPage{
			Keys: model.keys(bucket, prefix),
		}, nil
	}
	client.ListVersionsFunc = func(context.Context, string, string, string, string, int32) (s3client.VersionPage, error) {
		return s3client.VersionPage{}, nil
	}
	client.DeleteFunc = func(_ context.Context, bucket, key string, _ s3client.DeleteOptions) (s3client.DeleteResult, error) {
		model.delete(bucket, key)
		return s3client.DeleteResult{}, nil
	}
	return client, model
}
