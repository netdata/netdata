// SPDX-License-Identifier: GPL-3.0-or-later

package redfishruntime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

type testBackend struct {
	calls atomic.Int64
}

func (b *testBackend) Append(context.Context, []JournalEntry) (AppendResult, error) {
	b.calls.Add(1)
	return AppendResult{Committed: 1}, nil
}

func (b *testBackend) Contains(_ context.Context, keys []string) (map[string]bool, error) {
	result := make(map[string]bool, len(keys))
	for _, key := range keys {
		result[key] = false
	}
	return result, nil
}

func TestEndpointRegistrationRejectsDigestCollisionUntilLastOwnerLeaves(t *testing.T) {
	t.Parallel()

	runtime := New()
	removeFirst, err := runtime.RegisterEndpoint("short-key", "full-digest-a")
	require.NoError(t, err)
	removeSecond, err := runtime.RegisterEndpoint("short-key", "full-digest-a")
	require.NoError(t, err)

	_, err = runtime.RegisterEndpoint("short-key", "full-digest-b")
	require.ErrorContains(t, err, "collides")

	removeFirst()
	removeFirst()
	_, err = runtime.RegisterEndpoint("short-key", "full-digest-b")
	require.ErrorContains(t, err, "collides")

	removeSecond()
	removeDifferent, err := runtime.RegisterEndpoint("short-key", "full-digest-b")
	require.NoError(t, err)
	removeDifferent()
}

func TestBackendRegistrationDrainsLeases(t *testing.T) {
	runtime := New()
	backend := &testBackend{}
	registration, err := runtime.RegisterBackend("default", "key", t.TempDir(), backend)
	require.NoError(t, err)
	lease, ok := runtime.AcquireBackend("default")
	require.True(t, ok)

	closeCtx, cancelClose := context.WithCancel(context.Background())
	defer cancelClose()
	done := make(chan error, 1)
	go func() {
		done <- registration.Close(closeCtx)
	}()
	require.Eventually(t, func() bool {
		return !runtime.BackendAvailable("default")
	}, asyncTestTimeout, time.Millisecond)
	select {
	case <-done:
		t.Fatal("registration closed before its lease drained")
	default:
	}

	result, err := lease.Append(context.Background(), []JournalEntry{{Fields: map[string]string{"MESSAGE": "test"}}})
	require.NoError(t, err)
	require.Equal(t, 1, result.Committed)
	lease.Release()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(asyncTestTimeout):
		cancelClose()
		t.Fatal("backend registration did not finish after its lease drained")
	}
	require.EqualValues(t, 1, backend.calls.Load())
}

func TestProducerRegistrationCountsDesiredRoutes(t *testing.T) {
	t.Parallel()

	runtime := New()
	removeA, err := runtime.RegisterProducer("default", "endpoint-a")
	require.NoError(t, err)
	removeB, err := runtime.RegisterProducer("default", "endpoint-b")
	require.NoError(t, err)

	assert.Equal(t, 2, runtime.BackendProducerCount("default"))
	_, err = runtime.RegisterProducer("default", "endpoint-a")
	require.Error(t, err)

	removeA()
	removeA()
	assert.Equal(t, 1, runtime.BackendProducerCount("default"))
	removeB()
	assert.Zero(t, runtime.BackendProducerCount("default"))
}

func TestBackendRegistrationCloseCanRetryAfterTimeout(t *testing.T) {
	t.Parallel()

	runtime := New()
	registration, err := runtime.RegisterBackend("default", "key", t.TempDir(), &testBackend{})
	require.NoError(t, err)
	lease, ok := runtime.AcquireBackend("default")
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	require.ErrorIs(t, registration.Close(ctx), context.DeadlineExceeded)
	lease.Release()
	require.NoError(t, registration.Close(context.Background()))
}

func TestBackendRegistrationRemovesFunctionRootWhenRouteCloses(t *testing.T) {
	t.Parallel()

	runtime := New()
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	first, err := runtime.RegisterBackend("first", "first-key", firstRoot, &testBackend{})
	require.NoError(t, err)
	second, err := runtime.RegisterBackend("second", "second-key", secondRoot, &testBackend{})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, first.Close(context.Background()))
		require.NoError(t, second.Close(context.Background()))
	})

	require.Equal(t, map[string]string{
		"first":  firstRoot,
		"second": secondRoot,
	}, runtime.LogRoots())

	require.NoError(t, first.Close(context.Background()))
	require.Equal(t, map[string]string{"second": secondRoot}, runtime.LogRoots())
}

func TestInventorySnapshotsAreDeeplyImmutable(t *testing.T) {
	t.Parallel()

	runtime := New()
	value := true
	source := InventorySnapshot{
		Job: "endpoint",
		Rows: []map[string]any{{
			"host_uri":          "/redfish/v1/Systems/1",
			"resource_kind":     "fan",
			"sort_key":          "01",
			"failure_predicted": &value,
			"nested":            map[string]any{"values": []any{"a", "b"}},
		}},
	}
	require.NoError(t, runtime.PublishInventory(source))

	value = false
	source.Rows[0]["nested"].(map[string]any)["values"].([]any)[0] = "changed"
	first := collectInventorySlice(t, runtime, "endpoint", "/redfish/v1/Systems/1", "fan")
	require.True(t, *first[0]["failure_predicted"].(*bool))
	require.Equal(t, "a", first[0]["nested"].(map[string]any)["values"].([]any)[0])

	*first[0]["failure_predicted"].(*bool) = false
	first[0]["nested"].(map[string]any)["values"].([]any)[0] = "changed again"
	second := collectInventorySlice(t, runtime, "endpoint", "/redfish/v1/Systems/1", "fan")
	require.True(t, *second[0]["failure_predicted"].(*bool))
	require.Equal(t, "a", second[0]["nested"].(map[string]any)["values"].([]any)[0])
}

func TestInventorySnapshotRejectsUnsupportedMutableValuesAtomically(t *testing.T) {
	t.Parallel()

	runtime := New()
	valid := InventorySnapshot{
		Job: "endpoint",
		Rows: []map[string]any{{
			"host_uri": "/redfish/v1/Systems/1", "resource_kind": "fan", "sort_key": "01",
			"resource_key": "known-good",
		}},
	}
	require.NoError(t, runtime.PublishInventory(valid))

	invalid := valid
	invalid.Rows = []map[string]any{{
		"host_uri": "/redfish/v1/Systems/1", "resource_kind": "fan", "sort_key": "01",
		"resource_key": "replacement", "unsupported": map[string]string{"mutable": "value"},
	}}
	require.ErrorContains(t, runtime.PublishInventory(invalid), "unsupported mutable inventory value type")

	rows := collectInventorySlice(t, runtime, "endpoint", "/redfish/v1/Systems/1", "fan")
	require.Equal(t, "known-good", rows[0]["resource_key"])
}

func collectInventorySlice(
	t *testing.T,
	runtime *Runtime,
	job, host, kind string,
) []map[string]any {
	t.Helper()
	var rows []map[string]any
	total, found := runtime.VisitInventorySlice(
		context.Background(),
		job,
		host,
		kind,
		func(row map[string]any) bool {
			rows = append(rows, row)
			return true
		},
	)
	require.True(t, found)
	require.Equal(t, total, len(rows))
	return rows
}
