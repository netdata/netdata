// SPDX-License-Identifier: GPL-3.0-or-later

package historyfunc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusFunc func(context.Context) (journalstore.Status, error)

func (f statusFunc) Status(ctx context.Context) (journalstore.Status, error) { return f(ctx) }
func request() funcapi.RawMethodRequest {
	return funcapi.RawMethodRequest{
		Method:      method.ID,
		Permissions: "0x1b",
	}
}
func TestStatusPreservesIndependentFacts(t *testing.T) {
	now := time.Unix(1000, 0)
	h := New(statusFunc(func(context.Context) (journalstore.Status, error) {
		return journalstore.Status{
			Policy: &journalstore.RetentionPolicy{
				Days:     30,
				MaxBytes: 1 << 30,
			},
			ObservedAt:     now,
			InventoryError: "header unavailable",
			WriterError:    "writer failed",
			Cleanup: journalstore.CleanupStatus{
				AttemptedAt:      now,
				LastSuccessfulAt: now.Add(-time.Hour),
				Error:            "unlink failed",
			},
		}, nil
	}), func() string { return "invalid edited policy" })
	want := method.Table()
	want["data"] = [][]any{{
		30,
		int64(1 << 30),
		nil,
		nil,
		now.UnixMicro(),
		"header unavailable",
		now.UnixMicro(),
		now.Add(-time.Hour).UnixMicro(),
		"unlink failed",
		"writer failed",
		"invalid edited policy",
	}}
	assert.Equal(t, funcapi.RawResponse(want), h.HandleRaw(context.Background(), request()))
}
func TestUnknownAndEmptyStatusAreDistinct(t *testing.T) {
	for _, inventory := range []*journalstore.Inventory{nil, {Bytes: 0, Files: 0}} {
		h := New(statusFunc(func(context.Context) (journalstore.Status, error) {
			return journalstore.Status{
				Inventory: inventory,
			}, nil
		}), func() string { return "" })
		row := h.HandleRaw(context.Background(), request()).RawResponse["data"].([][]any)[0]
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
	h := New(statusFunc(func(context.Context) (journalstore.Status, error) {
		t.Fatal("unexpected storage I/O")
		return journalstore.Status{}, nil
	}), func() string { t.Fatal("unexpected reload state read"); return "" })
	cases := []struct {
		name string
		req  funcapi.RawMethodRequest
		want int
	}{
		{"info", funcapi.RawMethodRequest{
			Method: method.ID,
			Info:   true,
			Args:   []string{"info"},
		}, 200},
		{"missing permission", funcapi.RawMethodRequest{
			Method: method.ID,
		}, 403},
		{"insufficient permission", funcapi.RawMethodRequest{
			Method:      method.ID,
			Permissions: "0x1",
		}, 403},
		{"payload field", funcapi.RawMethodRequest{
			Method:      method.ID,
			Permissions: "0x1b",
			Payload:     []byte(`{"after":1}`),
		}, 400},
		{"payload not an object", funcapi.RawMethodRequest{
			Method:      method.ID,
			Permissions: "0x1b",
			Payload:     []byte(`[]`),
		}, 400},
		{"payload field on info", funcapi.RawMethodRequest{
			Method:  method.ID,
			Info:    true,
			Args:    []string{"info"},
			Payload: []byte(`{"site":"shop"}`),
		}, 400},
		{"time filter", funcapi.RawMethodRequest{
			Method:      method.ID,
			Permissions: "0x1b",
			Args:        []string{"after:1"},
		}, 400},
		{"unknown", funcapi.RawMethodRequest{
			Method: "other",
		}, 404},
	}
	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) { assert.Equal(t, tc.want, h.HandleRaw(context.Background(), tc.req).Status) },
		)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, 499, h.HandleRaw(ctx, request()).Status)
	declarations := Declarations()
	require.Len(t, declarations, 1)
	assert.False(t, declarations[0].HasHistory)
	assert.Empty(t, declarations[0].AcceptedParams)
	assert.True(t, declarations[0].RequireCloud)
	assert.True(t, declarations[0].RequireCommercial)
}
func TestCancellationReachesStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := New(statusFunc(func(got context.Context) (journalstore.Status, error) {
		assert.Same(t, ctx, got)
		cancel()
		return journalstore.Status{}, got.Err()
	}), func() string { return "" })
	assert.Equal(t, 499, h.HandleRaw(ctx, request()).Status)
}

func TestStorageErrorStatuses(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want *funcapi.FunctionResponse
	}{
		"cancelled": {err: context.Canceled, want: funcapi.ErrorResponse(499, "request cancelled")},
		"deadline":  {err: context.DeadlineExceeded, want: funcapi.ErrorResponse(504, "request timed out")},
		"storage":   {err: errors.New("journal closed"), want: funcapi.UnavailableResponse("journal closed")},
	} {
		t.Run(name, func(t *testing.T) {
			h := New(statusFunc(func(context.Context) (journalstore.Status, error) {
				return journalstore.Status{}, tc.err
			}), func() string { return "" })
			assert.Equal(t, tc.want, h.HandleRaw(context.Background(), request()))
		})
	}
}

func TestCloudTableRequests(t *testing.T) {
	h := New(statusFunc(func(context.Context) (journalstore.Status, error) {
		return journalstore.Status{}, nil
	}), func() string { return "" })
	// The table sends no query or selections to a Function without accepted parameters.
	body := []byte(`{"timeout":120000,"last":200}`)
	for name, req := range map[string]funcapi.RawMethodRequest{
		"info call": {
			Method: method.ID,
			Info:   true,
			Args:   []string{"info", "after:1700000000", "before:1700000900"},
		},
		"info with body": {
			Method:  method.ID,
			Info:    true,
			Args:    []string{"info"},
			Payload: body,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &funcapi.FunctionResponse{
				Status: 200,
				Help:   method.Help,
			}, h.HandleRaw(context.Background(), req))
		})
	}
	data := h.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method:      method.ID,
		Payload:     body,
		Permissions: "0x1b",
	})
	require.NotNil(t, data.RawResponse, data.Message)
	assert.Equal(t, 200, data.RawResponse["status"])
	assert.Len(t, data.RawResponse["data"], 1)
}

// The Cloud UI table posts a column filter as a selection named after the column; history status accepts none.
func TestColumnsDeclareNoFilters(t *testing.T) {
	for name, column := range method.Columns {
		assert.Equal(t, "none", column.(map[string]any)["filter"], name)
	}
}
