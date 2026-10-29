// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDiagnosticsDoNotExposeSource(t *testing.T) {
	const marker = "private_config_marker"
	for name, content := range map[string]string{
		"malformed line":    "# header\n" + marker + "\n",
		"raw boolean value": "statistics-cumulative: " + marker + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "unbound.conf")
			require.NoError(t, os.WriteFile(filename, []byte(content), 0600))
			var output bytes.Buffer
			c := New()
			c.Logger = logger.NewWithWriter(&output)
			c.ConfPath = filename
			assert.True(t, c.initConfig())
			assert.NotEmpty(t, output.String())
			assert.NotContains(t, output.String(), marker)
		})
	}
}
