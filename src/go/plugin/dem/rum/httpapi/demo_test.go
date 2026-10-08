// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDemoBehindPathPrefix(t *testing.T) {
	cfg := testCfg()
	cfg.Receiver.PublicURL = "https://collector.example/edge"
	fixture := newFixture(cfg, newRecSink(), nil)
	proxy := httptest.NewServer(http.StripPrefix("/edge", fixture.Handler()))
	defer proxy.Close()
	pageURL := proxy.URL + "/edge/rum/demo?key=shop"
	response, err := proxy.Client().Get(pageURL)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	page, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	match := regexp.MustCompile(`src="([^"]+)"`).FindSubmatch(page)
	require.Len(t, match, 2)
	base, err := url.Parse(pageURL)
	require.NoError(t, err)
	script, err := url.Parse(string(match[1]))
	require.NoError(t, err)
	resolved := base.ResolveReference(script)
	assert.Equal(t, "/edge/rum/shop.js", resolved.Path)
	response, err = proxy.Client().Get(resolved.String())
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	js, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Contains(t, string(js), "getOwnPropertyDescriptor(Document.prototype, 'currentScript')")
	assert.NotContains(t, string(js), "https://collector.example/edge")
}

func TestBootstrapCompilesTracingOrigins(t *testing.T) {
	cfg := testCfg()
	cfg.Sites[0].Tracing = &config.Tracing{
		Enabled:     true,
		PropagateTo: []string{"https://[2001:db8::1]:8443", "https://api.example"},
	}
	require.Empty(t, config.ValidateSiteExtras(cfg.Sites[0]))
	_, server := newTestServer(t, cfg, newRecSink(), nil)
	response, err := server.Client().Get(server.URL + "/rum/shop.js")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	js, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	patterns, err := json.Marshal([]string{`^https://\[2001:db8::1\]:8443(/|$)`, `^https://api\.example(/|$)`})
	require.NoError(t, err)
	assert.Contains(t, string(js), `"tracing":`+string(patterns))
}
