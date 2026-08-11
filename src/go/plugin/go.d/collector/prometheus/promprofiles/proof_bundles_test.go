// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/proof"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/testutil"
)

func TestDefaultCatalog_StockProfileProofBundles(t *testing.T) {
	repoRoot, err := filepath.Abs("../../../../../../..")
	require.NoError(t, err)
	bundles, err := promproof.Discover(repoRoot)
	require.NoError(t, err)
	require.NotEmpty(t, bundles)
	proofProfiles := make(map[string]struct{}, len(bundles))
	for _, bundle := range bundles {
		proofProfiles[bundle.Descriptor.Profile] = struct{}{}
	}
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)
	require.Len(t, proofProfiles, len(catalog.OrderedProfiles()), "every stock profile must have exactly one proof")
	for _, profile := range catalog.OrderedProfiles() {
		_, ok := proofProfiles[profile.Name]
		require.Truef(t, ok, "stock profile %q has no proof", profile.Name)
	}

	testdataRoot := proofTestdataRoot(t, bundles[0])
	_, err = promproof.LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, bundles)
	require.NoError(t, err)

	metadataPath := filepath.Join(repoRoot, "src/go/plugin/go.d/collector/prometheus/metadata.yaml")
	for _, bundle := range bundles {
		t.Run(bundle.Descriptor.Profile, func(t *testing.T) {
			assertProofMetadataExample(t, metadataPath, bundle)
		})
	}
}

func proofTestdataRoot(t *testing.T, bundle promproof.Bundle) string {
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
	require.NotEmpty(t, fixture, "proof bundle %q has no fixture", bundle.Descriptor.Profile)
	relative := bundle.FixturePath(fixture)
	resolved := promtestutil.Require(t, relative)
	root := resolved
	for path := filepath.FromSlash(relative); path != "."; path = filepath.Dir(path) {
		root = filepath.Dir(root)
	}
	return root
}

func assertProofMetadataExample(t *testing.T, metadataPath string, bundle promproof.Bundle) {
	t.Helper()
	identity := bundle.Descriptor.MetadataExample
	if identity == nil {
		return
	}

	type example struct {
		Name   string `yaml:"name"`
		Config string `yaml:"config"`
	}
	type module struct {
		Meta struct {
			ID string `yaml:"id"`
		} `yaml:"meta"`
		Setup struct {
			Configuration struct {
				Examples struct {
					List []example `yaml:"list"`
				} `yaml:"examples"`
			} `yaml:"configuration"`
		} `yaml:"setup"`
	}
	var metadata struct {
		Modules []module `yaml:"modules"`
	}
	content, err := os.ReadFile(metadataPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &metadata))

	var config string
	for _, mod := range metadata.Modules {
		if mod.Meta.ID != identity.IntegrationID {
			continue
		}
		for _, example := range mod.Setup.Configuration.Examples.List {
			if example.Name == identity.ExampleName {
				config = example.Config
			}
		}
	}
	require.NotEmptyf(t, config, "metadata example %q for %q", identity.ExampleName, identity.IntegrationID)

	var exampleConfig struct {
		Jobs []map[string]any `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(config), &exampleConfig))
	var metadataJob map[string]any
	for _, job := range exampleConfig.Jobs {
		if job["name"] == identity.JobName {
			metadataJob = job
		}
	}
	require.NotNilf(t, metadataJob, "metadata job %q in example %q", identity.JobName, identity.ExampleName)
	require.NotContains(t, metadataJob, "profiles", "stock metadata examples must exercise automatic profile selection")
	require.NotContains(t, metadataJob, "app", "stock metadata examples must use profile-derived application identity")
}
