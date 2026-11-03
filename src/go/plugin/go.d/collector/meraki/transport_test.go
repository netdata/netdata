// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingTransport struct {
	starts []time.Time
	closed int
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.starts = append(r.starts, time.Now())
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`[]`)),
		Request:    req,
	}
	if req.URL.Host == "api.meraki.com" {
		resp.StatusCode = 307
		resp.Header.Set("Location", "https://n123.meraki.com"+req.URL.RequestURI())
	}
	if req.URL.Query().Get("startingAfter") == "" && req.URL.Host != "api.meraki.com" {
		resp.Header.Set("Link", `<?perPage=3&startingAfter=next>; rel="next"`)
	}
	return resp, nil
}
func (r *recordingTransport) CloseIdleConnections() { r.closed++ }

func TestPacingCoversRedirectsPagesAndCancellation(t *testing.T) {
	base := &recordingTransport{}
	transport := &pacedTransport{
		base: base,
	}
	hc := &http.Client{
		Transport: transport,
	}
	client, err := merakiapi.New(merakiapi.Config{
		APIKey: "test-key",
	}, hc)
	require.NoError(t, err)
	_, err = client.ListDevices(t.Context(), "org")
	require.NoError(t, err)
	require.Len(t, base.starts, 3)
	for i := 1; i < len(base.starts); i++ {
		assert.GreaterOrEqual(t, base.starts[i].Sub(base.starts[i-1]), requestSpacing-5*time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	transport.mu.Lock()
	transport.lastStart = time.Now()
	transport.mu.Unlock()
	require.ErrorIs(t, client.ProbeOrganization(ctx, "org"), context.DeadlineExceeded)
	require.Len(t, base.starts, 3, "canceled pacing must not send another request")
	hc.CloseIdleConnections()
	assert.Equal(t, 1, base.closed)
}
