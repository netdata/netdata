// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactAssociationSurvivesRestartAndSupportsOlderRecords(t *testing.T) {
	for _, trace := range []bool{false, true} {
		kind, mime, payload := "screenshot", "image/png", png
		if trace {
			kind, mime, payload = "trace", "application/zip", []byte("PK\x03\x04fixture")
		}
		t.Run(kind, func(t *testing.T) {
			s, root := store(t)
			work, c := captured(t, s, run1, payload)
			c.Kind, c.MIME = kind, mime
			c.Test = synthetic.ArtifactTest{ID: "test1", Title: "Checkout", TextTruncated: true}
			_, err := s.Finalize(t.Context(), run1, []synthetic.Capture{c})
			require.NoError(t, err)
			_, err = os.Stat(work)
			assert.ErrorIs(t, err, os.ErrNotExist)
			require.NoError(t, s.Close())
			s, err = artifacts.Open(root)
			require.NoError(t, err)
			defer s.Close()
			meta, body, err := s.Fetch(t.Context(), "lighthouse:test", run1, c.ID)
			require.NoError(t, err)
			assert.Equal(t, c.Test, meta.Test)
			assert.Equal(t, payload, body)
			filename := filepath.Join(root, "runs", run1, "manifest.json")
			raw, err := os.ReadFile(filename)
			require.NoError(t, err)
			var manifest map[string]any
			require.NoError(t, json.Unmarshal(raw, &manifest))
			assert.Equal(t, float64(2), manifest["version"])
			// JSON decoding ignores unknown association fields; kind admission is a separate check.
			var old struct {
				Version   int
				Artifacts []struct {
					ID, Kind, MIME, SHA256 string
					Bytes                  int64
				}
			}
			require.NoError(t, json.Unmarshal(raw, &old))
			require.Len(t, old.Artifacts, 1)
			assert.Equal(t, c.ID, old.Artifacts[0].ID)
			entry := manifest["artifacts"].([]any)[0].(map[string]any)
			delete(entry, "test")
			raw, err = json.Marshal(manifest)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filename, raw, 0600))
			metadata, _, err := s.Fetch(t.Context(), "lighthouse:test", run1, c.ID)
			require.NoError(t, err)
			assert.Empty(t, metadata.Test.ID)
			entry["bytes"] = "corrupt authoritative metadata"
			raw, err = json.Marshal(manifest)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filename, raw, 0600))
			_, _, err = s.Fetch(t.Context(), "lighthouse:test", run1, c.ID)
			require.Error(t, err)
		})
	}
}
