// SPDX-License-Identifier: GPL-3.0-or-later
package faro

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoaderURLAndFailureContract(t *testing.T) {
	requireNode(t)
	payload, err := json.Marshal(map[string]string{
		"core": Bootstrap("shop", BootstrapOptions{
			MeasureRate: 1,
		}),
		"tracing": Bootstrap("shop", BootstrapOptions{
			MeasureRate: 1,
			Tracing:     true,
		}),
		"disabled": Bootstrap("shop", BootstrapOptions{}),
	})
	require.NoError(t, err)
	cmd := exec.Command("node", "testdata/loader.cjs")
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}
