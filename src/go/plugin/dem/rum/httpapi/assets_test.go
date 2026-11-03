// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetsHTTP(t *testing.T) {
	// Assets do not require configured sites or a live registry.
	handler := (&Server{}).Handler()
	for _, path := range []string{faro.SDKPath, faro.TracingPath, faro.NoticesPath} {
		t.Run(path, func(t *testing.T) {
			expected, ok := faro.LookupAsset(strings.TrimPrefix(path, "assets/"))
			require.True(t, ok)
			req := httptest.NewRequest(http.MethodGet, "/rum/"+path, nil)
			req.Header.Set("Origin", "https://unconfigured.example")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, expected.Content, rec.Body.String())
			assert.Equal(t, expected.ContentType, rec.Header().Get("Content-Type"))
			assert.Equal(t, expected.ETag, rec.Header().Get("ETag"))
			assert.Equal(t, strconv.Itoa(len(expected.Content)), rec.Header().Get("Content-Length"))
			assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
			assert.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
			assert.Equal(t, "cross-origin", rec.Header().Get("Cross-Origin-Resource-Policy"))
			assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
			assert.Equal(t, "<"+strings.TrimPrefix(faro.NoticesPath, "assets/")+">; rel=\"license\"", rec.Header().Get("Link"))
			for _, condition := range []string{expected.ETag, "W/" + expected.ETag, `"different", ` + expected.ETag, "*"} {
				req = httptest.NewRequest(http.MethodGet, "/rum/"+path, nil)
				req.Header.Set("If-None-Match", condition)
				rec = httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				assert.Equal(t, http.StatusNotModified, rec.Code)
				assert.Empty(t, rec.Body.String())
				assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
			}
			req = httptest.NewRequest(http.MethodHead, "/rum/"+path, nil)
			rec = httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Empty(t, rec.Body.String())
			assert.Equal(t, strconv.Itoa(len(expected.Content)), rec.Header().Get("Content-Length"))
		})
	}
}

func TestAssetsHTTPUnknown(t *testing.T) {
	handler := (&Server{}).Handler()
	for _, name := range []string{"faro-web-sdk.iife.js", "faro-web-sdk-0-unknown.js", "NOTICES.txt", "faro-web-sdk.iife.js.map", "manifest.json"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rum/assets/"+name, nil))
		assert.Equal(t, http.StatusNotFound, rec.Code, name)
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		assert.Empty(t, rec.Header().Get("ETag"))
	}
}
