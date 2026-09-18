// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisteredLogsFunctionReadsWithoutMetricSnapshot(t *testing.T) {
	docs := testutil.LogDocuments(513)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if doc, ok := docs[r.URL.RequestURI()]; ok {
			testutil.WriteJSON(w, doc)
		} else {
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	collector := newFunctionCollector(t, server.URL, "logs-job")
	require.Nil(t, collector.functionSnapshot.Load())
	handler := collectorapi.DefaultRegistry["redfish"].MethodHandler(functionTestJob{collector}).(funcapi.RawMethodHandler)
	response := handler.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method: "logs",
		Info:   true,
	})
	require.Equal(t, 200, response.Status)
	require.Len(t, response.RequiredParams[0].Options, 1)
	response = handler.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method:  "logs",
		Payload: []byte(fmt.Sprintf(`{"selections":{"__job":["logs-job"],"service":[%q]},"last":200}`, testutil.LogServiceURI)),
	})
	require.Equal(t, 200, response.Status)
	require.Len(t, response.Data.([][]any), 513)
	assert.Nil(t, collector.functionSnapshot.Load(), "on-demand logs do not publish a metric snapshot")
	collector.Cleanup(t.Context())
	response = handler.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method: "logs",
		Info:   true,
	})
	assert.Equal(t, 503, response.Status, "cleanup removes query access")
}
