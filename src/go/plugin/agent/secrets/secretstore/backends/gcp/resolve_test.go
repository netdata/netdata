// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPublishedStoreResolve_LogsDetailedResolution(t *testing.T) {
	s := &publishedStore{
		provider: &provider{
			apiClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "/v1/projects/my-project/secrets/my-secret/versions/latest:access", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`{"payload":{"data":"` +
						base64.StdEncoding.EncodeToString([]byte("secret-value")) + `"}}`)),
					Header: make(http.Header),
				}, nil
			})},
			metadataClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"access_token":"test-token"}`)),
					Header:     make(http.Header),
				}, nil
			})},
		},
		mode: "metadata",
	}

	out := captureLoggerOutput(t, func(log *logger.Logger) {
		ctx := logger.ContextWithLogger(context.Background(), log)
		value, err := s.Resolve(ctx, secretstore.ResolveRequest{
			StoreKey: "gcp-sm:gcp_prod",
			Operand:  "my-project/my-secret/latest",
			Original: "${store:gcp-sm:gcp_prod:my-project/my-secret/latest}",
		})
		require.NoError(t, err)
		assert.Equal(t, "secret-value", value)
	})

	assert.Contains(t, out, "resolved secret via gcp-sm secretstore 'gcp-sm:gcp_prod' project 'my-project' secret 'my-secret' version 'latest'")
	assert.NotContains(t, out, "secret-value")
}

func captureLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	fn(logger.NewWithWriter(&buf))
	return buf.String()
}
