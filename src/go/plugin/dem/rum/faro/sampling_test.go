// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Like the console oracle, this executes the unchanged pinned CDN bundle.
// The browser surface and transport sink are fixtures; initialization,
// session persistence, sampling and transport filtering are the real SDK.
func TestPinnedSDKSamplingContract(t *testing.T) {
	if os.Getenv("FARO_SDK_BUNDLE") == "" {
		t.Skip("set FARO_SDK_BUNDLE to the pinned SDK IIFE to execute the upstream oracle")
	}
	bootstraps := map[string]string{}
	for name, rate := range map[string]float64{"zero": 0, "fraction": 0.25, "full": 1} {
		bootstraps[name] = Bootstrap("shop", "https://rum.example.org", BootstrapOptions{
			MeasureRate: rate,
		})
	}
	encoded, err := json.Marshal(bootstraps)
	require.NoError(t, err)
	cmd := exec.Command("node", "testdata/sampling-sdk.cjs")
	cmd.Stdin = strings.NewReader(string(encoded))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
}
