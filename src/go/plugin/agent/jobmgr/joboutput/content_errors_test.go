// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mysql"
	_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	_ "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/x509check"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No secret references are used: the collectors must produce safe errors before
// the optional lifecycle secret redaction is involved.
func TestDynCfgFileErrorsDoNotExposeContent(t *testing.T) {
	const marker = "private_file_marker"
	for name, test := range map[string]struct {
		module  string
		content []byte
		config  func(string) map[string]any
		message string
	}{
		"MySQL INI": {
			module: "mysql", content: []byte(marker + "\n"),
			config:  func(path string) map[string]any { return map[string]any{"my.cnf": path} },
			message: "invalid INI syntax",
		},
		"MySQL derived DSN": {
			module: "mysql", content: []byte("[client]\ndatabase=%" + marker + "\n"),
			config:  func(path string) map[string]any { return map[string]any{"my.cnf": path} },
			message: "invalid DSN",
		},
		"Prometheus exposition": {
			module: "prometheus", content: []byte("test_metric " + marker + "\n"),
			config: func(path string) map[string]any {
				return map[string]any{"url": "file://" + path, "profiles": map[string]any{"mode": "none"}}
			},
			message: "invalid exposition format",
		},
		"X509 certificate": {
			module: "x509check", content: collecttest.CertificateWithInvalidURI(t, marker),
			config:  func(path string) map[string]any { return map[string]any{"source": "file://" + path} },
			message: "invalid X.509 certificate",
		},
	} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "input")
			require.NoError(t, os.WriteFile(filename, test.content, 0600))
			controller, _, _, _, _ := newDynCfgJobTestHarness(t)
			creator, ok := collectorapi.DefaultRegistry.Lookup(test.module)
			require.True(t, ok)
			controller.modules[test.module] = creator
			payload, err := json.Marshal(test.config(filename))
			require.NoError(t, err)
			result, err := controller.Handle(t.Context(), DynCfgJobRequest{
				Args:         []string{"go.d:collector:" + test.module, "test", "fixture"},
				Payload:      payload,
				ContentType:  "application/json",
				CallerSource: "user=test",
				HasPayload:   true,
			})
			require.NoError(t, err)
			var output bytes.Buffer
			frames, err := lifecycle.NewFrameOwner(&output)
			require.NoError(t, err)
			frame, err := lifecycle.PrepareFrame("fixture", result, time.Now().Add(time.Minute).Unix())
			require.NoError(t, err)
			require.NoError(t, frames.Commit(frame))
			assert.Contains(t, output.String(), test.message)
			assert.NotContains(t, output.String(), marker)
			assert.NotContains(t, output.String(), "redacted")
		})
	}
}
