// SPDX-License-Identifier: GPL-3.0-or-later
package historyfunc

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusFunc func(context.Context) (journal.Status, error)

func (f statusFunc) Status(ctx context.Context) (journal.Status, error) { return f(ctx) }
func request() funcapi.RawMethodRequest {
	return funcapi.RawMethodRequest{
		Method:      method,
		Permissions: "0x1b",
	}
}
func TestStatusPreservesIndependentFacts(t *testing.T) {
	now := time.Unix(1000, 0)
	h := New(statusFunc(func(context.Context) (journal.Status, error) {
		return journal.Status{
			Policy: &journal.RetentionPolicy{
				Days:     30,
				MaxBytes: 1 << 30,
			},
			ObservedAt:     now,
			InventoryError: "header unavailable",
			WriterError:    "writer failed",
			Cleanup: journal.CleanupStatus{
				AttemptedAt:      now,
				LastSuccessfulAt: now.Add(-time.Hour),
				Error:            "unlink failed",
			},
		}, nil
	}), func() string { return "invalid edited policy" })
	response := h.HandleRaw(context.Background(), request())
	require.Equal(t, 200, response.Status)
	assert.Nil(t, response.RawResponse)
	rows := response.Data.([][]any)
	require.Len(t, rows, 1)
	assert.Equal(t, []any{30, int64(1 << 30), nil, nil, now.UnixMicro(), "header unavailable", now.UnixMicro(), now.Add(-time.Hour).UnixMicro(), "unlink failed", "writer failed", "invalid edited policy"}, rows[0])
}
func TestUnknownAndEmptyStatusAreDistinct(t *testing.T) {
	for _, inventory := range []*journal.Inventory{nil, {Bytes: 0, Files: 0}} {
		h := New(statusFunc(func(context.Context) (journal.Status, error) {
			return journal.Status{
				Inventory: inventory,
			}, nil
		}), nil)
		row := h.HandleRaw(context.Background(), request()).Data.([][]any)[0]
		assert.Nil(t, row[0])
		assert.Nil(t, row[1])
		if inventory == nil {
			assert.Nil(t, row[2])
			assert.Nil(t, row[3])
		} else {
			assert.Equal(t, uint64(0), row[2])
			assert.Equal(t, 0, row[3])
		}
		for _, value := range row[4:] {
			assert.Nil(t, value)
		}
	}
}
func TestMetadataAndRejectedRequestsDoNotReadStorage(t *testing.T) {
	h := New(statusFunc(func(context.Context) (journal.Status, error) {
		t.Fatal("unexpected storage I/O")
		return journal.Status{}, nil
	}), func() string { t.Fatal("unexpected reload state read"); return "" })
	cases := []struct {
		name string
		req  funcapi.RawMethodRequest
		want int
	}{
		{"info", funcapi.RawMethodRequest{
			Method: method,
			Info:   true,
			Args:   []string{"info"},
		}, 200},
		{"missing permission", funcapi.RawMethodRequest{
			Method: method,
		}, 403},
		{"insufficient permission", funcapi.RawMethodRequest{
			Method:      method,
			Permissions: "0x1",
		}, 403},
		{"payload", funcapi.RawMethodRequest{
			Method:      method,
			Permissions: "0x1b",
			Payload:     []byte("{}"),
		}, 400},
		{"time filter", funcapi.RawMethodRequest{
			Method:      method,
			Permissions: "0x1b",
			Args:        []string{"after:1"},
		}, 400},
		{"unknown", funcapi.RawMethodRequest{
			Method: "other",
		}, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, h.HandleRaw(context.Background(), tc.req).Status) })
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, 499, h.HandleRaw(ctx, request()).Status)
	assert.Equal(t, 503, New(nil, nil).HandleRaw(context.Background(), request()).Status)
	declarations := Declarations()
	require.Len(t, declarations, 1)
	assert.False(t, declarations[0].HasHistory)
	assert.Empty(t, declarations[0].AcceptedParams)
}
func TestCancellationReachesStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := New(statusFunc(func(got context.Context) (journal.Status, error) {
		assert.Same(t, ctx, got)
		cancel()
		return journal.Status{}, got.Err()
	}), nil)
	assert.Equal(t, 499, h.HandleRaw(ctx, request()).Status)
}
