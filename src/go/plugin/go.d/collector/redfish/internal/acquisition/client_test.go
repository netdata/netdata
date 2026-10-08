// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

func TestProtocolClientCancellationMarksUnvisitedMembersUnknown(t *testing.T) {
	t.Parallel()

	blocked := make(chan struct{}, 2)
	server := testutil.NewResourceServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/Systems/1" {
			select {
			case blocked <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			<-r.Context().Done()
			return
		}
		testutil.ServeBaseResource(w, r.URL.Path, "#ComputerSystem.v1_20_0.ComputerSystem", r.URL.Path)
	}))
	defer server.Close()

	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	recorder := &requestRecordingTransport{
		base: client.http.Transport,
	}
	client.http.Transport = recorder
	members := []collectionMember{
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Systems/1",
		}},
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Systems/2",
		}},
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Systems/3",
		}},
	}
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			resources, err := client.fetchBaseMembers(ctx, "system", members, nil)
			assert.Equal(t, []baseResource{
				{Kind: "system", URI: "/redfish/v1/Systems/1", AcquisitionState: "unreadable"},
				{Kind: "system", URI: "/redfish/v1/Systems/2", AcquisitionState: "unknown"},
				{Kind: "system", URI: "/redfish/v1/Systems/3", AcquisitionState: "unknown"},
			}, resources)
			result <- err
		}()
		select {
		case <-blocked:
		case <-time.After(asyncTestTimeout):
			cancel()
			t.Fatal("timed out waiting for blocked Redfish member request")
		}
		cancel()
		select {
		case err := <-result:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(asyncTestTimeout):
			t.Fatal("timed out waiting for canceled Redfish member collection")
		}
	}
	assert.Equal(t, []string{"/redfish/v1/Systems/1", "/redfish/v1/Systems/1"}, recorder.paths())
}
