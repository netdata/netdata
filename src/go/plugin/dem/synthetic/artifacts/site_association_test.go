// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogueSnapshotsSiteWithoutChangingOwnership(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := artifacts.Open(root)
	require.NoError(t, err)
	ids := []string{fixtureRunID(), fixtureRunID(), fixtureRunID()}
	for i, site := range []string{"storefront", "other-site", ""} {
		work, err := s.Begin(ids[i], "lighthouse:home", site)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(work, "output/report"), []byte("<html>report</html>"), 0600))
		_, err = s.Finalize(ctx, ids[i], []synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report"}})
		require.NoError(t, err)
	}
	require.NoError(t, s.Close())
	s, err = artifacts.Open(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	filtered, err := s.Catalogue(ctx, artifacts.Filter{SiteID: "storefront", JobID: "lighthouse:home", Limit: 1})
	require.NoError(t, err)
	require.Len(t, filtered.Sets, 1)
	assert.Equal(t, ids[0], filtered.Sets[0].RunID)
	assert.Equal(t, "storefront", filtered.Sets[0].SiteID)
	mismatch, err := s.Catalogue(ctx, artifacts.Filter{SiteID: "storefront", JobID: "journey:home", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, mismatch.Sets)
	all, err := s.Catalogue(ctx, artifacts.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, all.Sets, 3)
	assert.Equal(t, 3, all.Status.Inventory.KnownSets)
	assert.Equal(t, 1, all.Status.Inventory.KnownJobs)
	manifest, err := s.Manifest(ctx, "lighthouse:home", ids[2])
	require.NoError(t, err)
	assert.Empty(t, manifest.SiteID)
	_, _, err = s.Fetch(ctx, "journey:home", ids[0], "report")
	assert.ErrorIs(t, err, artifacts.ErrOwnerMismatch)
	_, body, err := s.Fetch(ctx, "lighthouse:home", ids[0], "report")
	require.NoError(t, err)
	assert.Equal(t, "<html>report</html>", string(body))
}
