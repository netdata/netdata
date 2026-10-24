// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/require"
)

type testClientConfig struct {
	Options
	Timeout confopt.Duration
}

func testConfig(rawURL, auth string) testClientConfig {
	cfg := testClientConfig{
		Options: Options{
			URL:                   rawURL,
			AuthMethod:            auth,
			Username:              "user",
			Password:              "test-password",
			MaxConcurrentRequests: 3,
			Collect:               "*",
		},
		Timeout: confopt.Duration(15 * time.Second),
	}
	if auth == "none" {
		cfg.Username, cfg.Password = "", ""
	}
	return cfg
}
func newTestProtocolClient(t *testing.T, cfg testClientConfig) *Client {
	t.Helper()
	httpClient, err := web.NewHTTPClient(t.Context(), web.ClientConfig{
		Timeout: cfg.Timeout,
	})
	require.NoError(t, err)
	t.Cleanup(httpClient.CloseIdleConnections)
	client, err := New(cfg.Options, httpClient)
	require.NoError(t, err)
	return client
}
func fetchTestCollection(
	c *Client,
	ctx context.Context,
	ref string,
	stats *wireStats,
) ([]redfishLink, bool, error) {
	members, complete, err := c.fetchCollectionMembers(ctx, ref, stats)
	result := make([]redfishLink, len(members))
	for i, member := range members {
		result[i] = member.Ref
	}
	return result, complete, err
}

type requestRecordingTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	path []string
}

func (t *requestRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.path = append(t.path, req.URL.Path)
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}

func (t *requestRecordingTransport) paths() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.path...)
}

func (t *requestRecordingTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// Resource unit tests supply only the resource under test; SDK connections also
// read the ServiceRoot. Lifecycle tests use their own full endpoint fixtures.

func newTestResourceClient(t *testing.T, cfg testClientConfig) *Client {
	t.Helper()
	c := newTestProtocolClient(t, cfg)
	require.NoError(t, c.initializeAuthentication(t.Context(), nil))
	return c
}
