// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryInputConfinement(t *testing.T) {
	for name, tc := range map[string]struct {
		member   string
		internal bool
	}{
		"evidence directory outside root":         {member: "evidence"},
		"status outside root":                     {member: "status"},
		"run index outside root":                  {member: "runs"},
		"relative evidence directory inside root": {member: "evidence", internal: true},
	} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "bundle")
			state := filepath.Join(root, "06-state")
			require.NoError(t, os.MkdirAll(state, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(root, "MANIFEST.json"), []byte(`{}`), 0600))
			outside := filepath.Join(base, "outside")
			require.NoError(t, os.Mkdir(outside, 0700))
			link := func(target, name string) {
				t.Helper()
				err := os.Symlink(target, name)
				if runtime.GOOS == "windows" && os.IsPermission(err) {
					t.Skip("symlink creation requires Windows privileges")
				}
				require.NoError(t, err)
			}
			switch tc.member {
			case "evidence":
				target := outside
				linkTarget := filepath.Join("..", "..", "outside")
				if tc.internal {
					target = filepath.Join(root, "captured")
					linkTarget = filepath.Join("..", "captured")
				}
				require.NoError(t, os.MkdirAll(filepath.Join(target, "topology"), 0700))
				raw, err := os.ReadFile(replayableDiagnosticArchivePath())
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(target, "topology", "checkpoint-00000000000000000001.zst"), raw, 0600))
				link(linkTarget, filepath.Join(state, "snmp-diagnostics"))
				code, out, message := runBundleCommand(t, root, "validate")
				if tc.internal {
					require.Zero(t, code, message)
					assert.Contains(t, out, `"valid": true`)
				} else {
					require.Equal(t, 1, code, "outside evidence must not validate")
					assert.Empty(t, out)
					assert.NotEmpty(t, message)
				}
			case "status":
				require.NoError(t, os.WriteFile(filepath.Join(outside, "status.txt"), []byte("SYNTHETIC OUTSIDE STATUS"), 0600))
				link(filepath.Join(outside, "status.txt"), filepath.Join(state, "snmp-diagnostics-status.txt"))
				code, out, message := runBundleCommand(t, root, "list")
				require.Zero(t, code, message)
				var listing diagnosticListing
				require.NoError(t, json.Unmarshal([]byte(out), &listing))
				require.NotNil(t, listing.Bundle)
				assert.Nil(t, listing.Bundle.CollectionStatus)
				assert.NotEmpty(t, listing.Errors["collection_status"])
				assert.NotContains(t, out, "SYNTHETIC OUTSIDE STATUS")
			case "runs":
				normal := filepath.Join(state, "snmp-diagnostics", "normal")
				require.NoError(t, os.MkdirAll(filepath.Join(normal, currentRun), 0700))
				require.NoError(t, os.WriteFile(filepath.Join(normal, currentRun, "device-00000000000000000007.zst"), []byte("listing does not decode"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(outside, "runs.json"), []byte(fmt.Sprintf(`{"current":%q}`, currentRun)), 0600))
				link(filepath.Join(outside, "runs.json"), filepath.Join(normal, "runs.json"))
				code, out, message := runBundleCommand(t, root, "list")
				require.Zero(t, code, message)
				var listing diagnosticListing
				require.NoError(t, json.Unmarshal([]byte(out), &listing))
				assert.Empty(t, listing.Normal)
				assert.NotEmpty(t, listing.Errors["normal"])
			}
		})
	}
}
