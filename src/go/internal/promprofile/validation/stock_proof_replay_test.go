// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/proof"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/testutil"
)

func TestStockProfileProofsReplay(t *testing.T) {
	repoRoot := stockProofRepoRoot(t)
	bundles, err := promproof.Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	testdataRoot := stockProofTestdataRoot(t, bundles[0])
	catalog, err := promproof.LoadCompiledCatalog(
		context.Background(), repoRoot, testdataRoot, bundles,
	)
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(
		repoRoot,
		"src/go/plugin/go.d/collector/prometheus/metadata.yaml",
	)
	if err := promproof.VerifyCompiledCatalog(
		context.Background(),
		repoRoot,
		testdataRoot,
		catalog,
		func(ctx context.Context, input prominput.ReplayCase) ([]promreplay.Result, error) {
			input.MetadataPath = metadataPath
			return ReplayProofCase(ctx, input)
		},
	); err != nil {
		t.Fatal(err)
	}
}

func stockProofRepoRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		proofRoot := filepath.Join(directory, filepath.FromSlash(promproof.ProofRoot))
		if info, err := os.Stat(proofRoot); err == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatalf("repository root containing %s was not found", promproof.ProofRoot)
		}
		directory = parent
	}
}

func stockProofTestdataRoot(t *testing.T, bundle promproof.Bundle) string {
	t.Helper()
	var fixture string
	for _, proofCase := range bundle.Descriptor.Cases {
		if proofCase.Fixture != "" {
			fixture = proofCase.Fixture
			break
		}
		if len(proofCase.Steps) != 0 {
			fixture = proofCase.Steps[0].Fixture
			break
		}
	}
	if fixture == "" {
		t.Fatalf("proof bundle %q has no fixture", bundle.Descriptor.Profile)
	}
	relative := bundle.FixturePath(fixture)
	resolved := promtestutil.Require(t, relative)
	root := resolved
	for path := filepath.FromSlash(relative); path != "."; path = filepath.Dir(path) {
		root = filepath.Dir(root)
	}
	return root
}
