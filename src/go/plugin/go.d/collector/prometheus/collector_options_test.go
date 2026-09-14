// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func TestWithProfileCatalogSelectsInjectedProfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "injected.yaml"), []byte(`
match: injected_metric
template:
  family: Injected
  context_namespace: injected
  metrics:
    - injected_metric
  charts:
    - title: Injected metric
      context: metric
      units: value
      dimensions:
        - selector: injected_metric
          name: value
`), 0o600))
	catalog, err := promprofiles.LoadFromDirs([]promprofiles.DirSpec{{Path: dir, IsStock: true}})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# TYPE injected_metric gauge\ninjected_metric 1\n"))
	}))
	defer srv.Close()

	collector := promcollector.NewWithOptions(promcollector.WithProfileCatalog(catalog))
	collector.URL = srv.URL
	collector.Profiles = promcollector.ProfilesConfig{
		Mode: "exact",
		ModeExact: &promcollector.ProfilesModeConfig{
			Entries: []promcollector.ProfileEntryConfig{{Name: "injected"}},
		},
	}

	require.NoError(t, collector.Init(context.Background()))
	defer collector.Cleanup(context.Background())
	require.NoError(t, collector.Check(context.Background()))
	require.Contains(t, collector.ChartTemplateYAML(), "context_namespace: injected")
}
