// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCollectionValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		handler http.HandlerFunc
		wantErr string
	}{
		"pagination loop": {
			handler: func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":              "/redfish/v1/Test",
					"@odata.type":            "#TestCollection.TestCollection",
					"Members@odata.count":    1,
					"Members":                []any{},
					"Members@odata.nextLink": r.URL.RequestURI(),
				})
			},
			wantErr: "pagination loop",
		},
		"count changes": {
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") == "2" {
					writeJSON(w, map[string]any{
						"@odata.id":           "/redfish/v1/Test",
						"@odata.type":         "#TestCollection.TestCollection",
						"Members@odata.count": 2,
						"Members":             []any{map[string]any{"@odata.id": "/redfish/v1/Systems/1"}},
					})
					return
				}
				writeJSON(w, map[string]any{
					"@odata.id":              "/redfish/v1/Test",
					"@odata.type":            "#TestCollection.TestCollection",
					"Members@odata.count":    1,
					"Members":                []any{},
					"Members@odata.nextLink": "/redfish/v1/Test?page=2",
				})
			},
			wantErr: "@odata.count changed",
		},
		"duplicate identity": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 2,
					"Members": []any{
						map[string]any{"@odata.id": "/redfish/v1/Systems/1"},
						map[string]any{"@odata.id": "/redfish/v1/Systems/1"},
					},
				})
			},
			wantErr: "duplicate member identity",
		},
		"final count mismatch": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 2,
					"Members":             []any{map[string]any{"@odata.id": "/redfish/v1/Systems/1"}},
				})
			},
			wantErr: "advertised 2",
		},
		"members absent": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 0,
				})
			},
			wantErr: "no Members array",
		},
		"members null": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, map[string]any{
					"@odata.id":           "/redfish/v1/Test",
					"@odata.type":         "#TestCollection.TestCollection",
					"Members@odata.count": 0,
					"Members":             nil,
				})
			},
			wantErr: "no Members array",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			client := newTestProtocolClient(t, testConfig(server.URL, "none"))
			_, complete, err := fetchTestCollection(client, context.Background(), "/redfish/v1/Test", nil)
			assert.False(t, complete)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestFetchCollectionAcceptsSameOriginRedirectIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/Test":
			http.Redirect(w, r, "/redfish/v1/RedirectedTest", http.StatusTemporaryRedirect)
		case "/redfish/v1/RedirectedTest":
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/RedirectedTest",
				"@odata.type":         "#TestCollection.TestCollection",
				"Members@odata.count": 0,
				"Members":             []any{},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	members, complete, err := fetchTestCollection(client, context.Background(), "/redfish/v1/Test", nil)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Empty(t, members)
}

func TestFetchCollectionContinuesAfterInvalidMember(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Test",
				"@odata.type":         "#TestCollection.TestCollection",
				"Members@odata.count": 3,
				"Members": []any{
					map[string]any{"@odata.id": "/redfish/v1/Test/1"},
					map[string]any{"Name": "missing identity"},
				},
				"Members@odata.nextLink": "/redfish/v1/Test?page=2",
			})
		case "2":
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Test",
				"@odata.type":         "#TestCollection.TestCollection",
				"Members@odata.count": 3,
				"Members": []any{
					map[string]any{"@odata.id": "/redfish/v1/Test/2"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	members, complete, err := fetchTestCollection(client,
		context.Background(),
		"/redfish/v1/Test",
		nil,
	)
	require.ErrorContains(t, err, "collection member has no @odata.id")
	require.False(t, complete)
	require.Equal(t, []redfishLink{
		{ODataID: "/redfish/v1/Test/1"},
		{ODataID: "/redfish/v1/Test/2"},
	}, members)
}

func TestFetchCollectionContinuesAfterDuplicateMember(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"@odata.id":           "/redfish/v1/Test",
			"@odata.type":         "#TestCollection.TestCollection",
			"Members@odata.count": 2,
			"Members": []any{
				map[string]any{"@odata.id": "/redfish/v1/Test/1"},
				map[string]any{"@odata.id": "/redfish/v1/Test/1"},
				map[string]any{"@odata.id": "/redfish/v1/Test/2"},
			},
		})
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	members, complete, err := fetchTestCollection(client,
		context.Background(),
		"/redfish/v1/Test",
		nil,
	)
	require.ErrorContains(t, err, "duplicate member identity")
	require.False(t, complete)
	require.Equal(t, []redfishLink{
		{ODataID: "/redfish/v1/Test/1"},
		{ODataID: "/redfish/v1/Test/2"},
	}, members)
}

func TestProtocolClientUsesAdvertisedExpandedCollectionMembers(t *testing.T) {
	t.Parallel()

	var collectionRequests, memberRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/Systems":
			collectionRequests.Add(1)
			if got := r.URL.Query().Get("$expand"); got != "." {
				t.Errorf("$expand = %q, want %q", got, ".")
				http.Error(w, "unexpected expansion", http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Systems",
				"@odata.type":         "#ComputerSystemCollection.ComputerSystemCollection",
				"Members@odata.count": 1,
				"Members": []any{map[string]any{
					"@odata.id":   "/redfish/v1/Systems/1",
					"@odata.type": "#ComputerSystem.v1_20_0.ComputerSystem",
					"Id":          "1",
					"Name":        "System 1",
					"ExactTotal":  json.Number("9007199254740993"),
				}},
			})
		case "/redfish/v1/Systems/1":
			memberRequests.Add(1)
			http.Error(w, "expanded member should avoid this request", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	client.setExpansionValue(".")
	members, complete, err := client.fetchCollectionMembers(
		context.Background(),
		"/redfish/v1/Systems",
		"system",
		nil,
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Len(t, members, 1)
	assert.NotNil(t, members[0].Data)
	assert.Equal(t, json.Number("9007199254740993"), members[0].Data["ExactTotal"])
	assert.Equal(t, int64(1), collectionRequests.Load())
	assert.Zero(t, memberRequests.Load())
}

func TestCollectionExpansionFailureClassification(t *testing.T) {
	t.Parallel()

	for name, test := range map[string]struct {
		err  error
		want bool
	}{
		"bad request": {
			err: statusError{
				status: http.StatusBadRequest,
				class:  "protocol",
			},
			want: true,
		},
		"not implemented": {
			err: statusError{
				status: http.StatusNotImplemented,
				class:  "protocol",
			},
			want: true,
		},
		"service unavailable": {
			err: statusError{
				status: http.StatusServiceUnavailable,
				class:  "protocol",
			},
		},
		"authentication": {
			err: statusError{
				status: http.StatusUnauthorized,
				class:  "auth",
			},
		},
		"transport": {
			err: transportError{
				temporary: true,
			},
		},
		"timeout": {
			err: transportError{
				timeout: true,
			},
		},
		"caller deadline": {
			err: context.DeadlineExceeded,
		},
		"structural": {
			err:  errors.New("expanded collection member has no schema type"),
			want: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, persistentCollectionExpansionFailure(test.err))
		})
	}
}

func TestProtocolClientRestartsPaginationAfterCancellation(t *testing.T) {
	t.Parallel()

	var firstPageRequests, secondPageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/redfish/v1/Test" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			if secondPageRequests.Add(1) == 1 {
				<-r.Context().Done()
				return
			}
			writeJSON(w, map[string]any{
				"@odata.id":           "/redfish/v1/Test",
				"@odata.type":         "#ResourceCollection.ResourceCollection",
				"Members@odata.count": 2,
				"Members":             []any{map[string]any{"@odata.id": "/redfish/v1/Test/2"}},
			})
			return
		}
		memberID := "/redfish/v1/Test/1"
		if firstPageRequests.Add(1) > 1 {
			memberID = "/redfish/v1/Test/3"
		}
		writeJSON(w, map[string]any{
			"@odata.id":              "/redfish/v1/Test",
			"@odata.type":            "#ResourceCollection.ResourceCollection",
			"Members@odata.count":    2,
			"Members":                []any{map[string]any{"@odata.id": memberID}},
			"Members@odata.nextLink": "/redfish/v1/Test?page=2",
		})
	}))
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	members, complete, err := fetchTestCollection(client, ctx, "/redfish/v1/Test", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, complete)
	require.Len(t, members, 1)

	members, complete, err = fetchTestCollection(client, context.Background(), "/redfish/v1/Test", nil)
	require.NoError(t, err)
	assert.True(t, complete)
	require.Len(t, members, 2)
	assert.Equal(t, "/redfish/v1/Test/3", members[0].ODataID)
	assert.Equal(t, "/redfish/v1/Test/2", members[1].ODataID)
	assert.Equal(t, int64(2), firstPageRequests.Load())
	assert.Equal(t, int64(2), secondPageRequests.Load())
}
