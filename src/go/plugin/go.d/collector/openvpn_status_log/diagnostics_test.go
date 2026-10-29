// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorRejectedUserDiagnostics(t *testing.T) {
	const marker = "SYNTHETIC_SOURCE_MARKER"
	path := filepath.Join(t.TempDir(), "status.log")
	input := "OpenVPN CLIENT LIST\nCommon Name,Real Address,Bytes Received,Bytes Sent,Connected Since\n" + marker + " invalid,192.0.2.1:1234,5,6,Mon Jan 2 15:04:05 2006\nROUTING TABLE\n"
	require.NoError(t, os.WriteFile(path, []byte(input), 0600))
	var output bytes.Buffer
	collector := New()
	collector.Logger = logger.NewWithWriter(&output)
	collector.LogPath = path
	collector.PerUserStats = matcher.SimpleExpr{
		Includes: []string{"* *"},
	}
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	assert.Equal(t, int64(1), collector.Collect(context.Background())["clients"])
	assert.Contains(t, output.String(), "level=warn")
	assert.NotContains(t, output.String(), marker)
}
