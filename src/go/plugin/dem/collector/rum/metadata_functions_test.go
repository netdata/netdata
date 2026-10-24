// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestMetadataDocumentsOptionalDisplayLabel(t *testing.T) {
	data, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	var document struct {
		Modules []struct {
			Metrics struct {
				Scopes []struct {
					Name   string `yaml:"name"`
					Labels []struct {
						Name        string `yaml:"name"`
						Description string `yaml:"description"`
					} `yaml:"labels"`
				} `yaml:"scopes"`
			} `yaml:"metrics"`
		} `yaml:"modules"`
	}
	require.NoError(t, yaml.Unmarshal(data, &document))
	require.Len(t, document.Modules, 1)
	for _, scope := range document.Modules[0].Metrics.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			found := false
			for _, label := range scope.Labels {
				if label.Name == "display_name" {
					found = true
					require.Contains(t, label.Description, "Optional")
				}
			}
			require.True(t, found, "display_name applies to every site chart scope")
		})
	}
}
