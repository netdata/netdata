// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCollectionAcceptsSameOriginRedirectIdentity(t *testing.T) {
	t.Parallel()

	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	members, complete, err := fetchTestCollection(client, context.Background(), "/redfish/v1/Test", nil)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Empty(t, members)
}

func TestFetchCollectionContinuesAfterInvalidMember(t *testing.T) {
	t.Parallel()

	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	members, complete, err := fetchTestCollection(client,
		context.Background(),
		"/redfish/v1/Test",
		nil,
	)
	require.ErrorContains(t, err, "collection member has no usable @odata.id")
	require.False(t, complete)
	require.Equal(t, []redfishLink{
		{ODataID: "/redfish/v1/Test/1"},
		{ODataID: "/redfish/v1/Test/2"},
	}, members)
}

func TestFetchCollectionContinuesAfterDuplicateMember(t *testing.T) {
	t.Parallel()

	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	members, complete, err := fetchTestCollection(client,
		context.Background(),
		"/redfish/v1/Test",
		nil,
	)
	require.NoError(t, err)
	require.True(t, complete)
	require.Equal(t, []redfishLink{
		{ODataID: "/redfish/v1/Test/1"},
		{ODataID: "/redfish/v1/Test/2"},
	}, members)
}

func TestProtocolClientRestartsPaginationAfterCancellation(t *testing.T) {
	t.Parallel()

	var firstPageRequests, secondPageRequests atomic.Int64
	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	client := newTestResourceClient(t, testConfig(server.URL, "none"))
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

func TestCollectionCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		complete   bool
		members    int
	}{
		{"empty without count", `{"Members":[]}`, true, 0},
		{"missing members", `{}`, false, 0},
		{"null members", `{"Members":null}`, false, 0},
		{"count is not a snapshot", `{"Members@odata.count":3,"Members":[{"@odata.id":"/redfish/v1/Test/1"}]}`, true, 1},
		{"invalid siblings", `{"Members":[null,5,{"@odata.id":false},{"@odata.id":"/redfish/v1/Test/1"}]}`, false, 1},
		{"loop", `{"Members":[{"@odata.id":"/redfish/v1/Test/1"}],"Members@odata.nextLink":"/redfish/v1/Test"}`, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client := newTestResourceClient(t, testConfig(server.URL, "none"))
			members, complete, err := fetchTestCollection(client, t.Context(), "/redfish/v1/Test", nil)
			assert.Equal(t, tc.complete, complete)
			assert.Len(t, members, tc.members)
			if tc.complete {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
