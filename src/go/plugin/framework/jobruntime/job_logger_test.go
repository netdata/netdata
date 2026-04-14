// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
)

func TestJobLoggerAttrs(t *testing.T) {
	var buf bytes.Buffer

	logger.NewWithWriter(&buf).
		With(jobLoggerAttrs("module", "job", "stock/file reader")...).
		Info("captured")

	out := buf.String()
	assert.Contains(t, out, "collector=module")
	assert.Contains(t, out, "job=job")
	assert.Contains(t, out, "config_source=\"stock/file reader\"")
}
