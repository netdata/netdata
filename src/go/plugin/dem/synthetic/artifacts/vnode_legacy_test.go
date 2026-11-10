// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacySiteAssociationDoesNotBecomeVnodeIdentity(t *testing.T) {
	ctx := context.Background()
	root, id := t.TempDir(), fixtureRunID()
	s, err := artifacts.Open(root)
	require.NoError(t, err)
	work, err := s.Begin(id, "lighthouse:home", "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output/report"), []byte("<html>report</html>"), 0600))
	_, err = s.Finalize(ctx, id, []synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report"}})
	require.NoError(t, err)
	require.NoError(t, s.Close())
	path := filepath.Join(root, "runs", id, "manifest.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var legacy map[string]any
	require.NoError(t, json.Unmarshal(raw, &legacy))
	legacy["site_id"] = "11111111-1111-4111-8111-111111111111"
	raw, err = json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	s, err = artifacts.Open(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	manifest, err := s.Manifest(ctx, "lighthouse:home", id)
	require.NoError(t, err)
	assert.Empty(t, manifest.VnodeGUID)
	_, body, err := s.Fetch(ctx, "lighthouse:home", id, "report")
	require.NoError(t, err)
	assert.Equal(t, "<html>report</html>", string(body))
}
