// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetGKEKubeClusterName(t *testing.T) {
	for _, failAt := range []string{"", "project/project-id", "instance/attributes/cluster-location", "instance/attributes/cluster-name"} {
		name := failAt
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) {
			values := map[string]string{
				"project/project-id":                   "project",
				"instance/attributes/cluster-location": "location",
				"instance/attributes/cluster-name":     "cluster",
			}
			var paths []string
			client := &http.Client{Transport: metadataTransport(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "GET", req.Method)
				assert.Equal(t, "metadata", req.URL.Host)
				assert.Equal(t, "Google", req.Header.Get("Metadata-Flavor"))
				path := strings.TrimPrefix(req.URL.Path, "/computeMetadata/v1/")
				value, ok := values[path]
				require.True(t, ok, "unexpected metadata path: %s", req.URL.Path)
				paths = append(paths, path)
				if path == failAt {
					return nil, errors.New("metadata unavailable")
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(value)),
					Request:    req,
				}, nil
			})}
			got, err := getGKEKubeClusterName(client)
			if failAt != "" {
				require.ErrorContains(t, err, "metadata unavailable")
				assert.Empty(t, got)
				assert.Equal(t, failAt, paths[len(paths)-1])
			} else {
				require.NoError(t, err)
				assert.Equal(t, "gke_project_location_cluster", got)
				assert.Equal(t, []string{
					"project/project-id", "instance/attributes/cluster-location", "instance/attributes/cluster-name",
				}, paths)
			}
		})
	}
}
