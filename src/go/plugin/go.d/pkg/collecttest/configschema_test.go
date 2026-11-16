// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// flatArtifacts writes a tab-less form and a metadata document that documents `documented`.
func flatArtifacts(t *testing.T, documented string) (schemaPath, metadataPath string) {
	t.Helper()
	dir := t.TempDir()
	schemaPath = filepath.Join(dir, "config_schema.json")
	metadataPath = filepath.Join(dir, "metadata.yaml")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "jsonSchema": {
    "type": "object",
    "properties": {
      "update_every": {"type": "integer", "description": "Data collection interval in seconds.", "default": 60},
      "verbose": {"type": "boolean", "description": "Log every request.", "default": false}
    }
  },
  "uiSchema": {}
}`), 0o600))
	require.NoError(t, os.WriteFile(metadataPath, []byte("modules:\n  - setup:\n      configuration:\n        options:\n          list:\n"+documented), 0o600))
	return schemaPath, metadataPath
}

const flatAligned = `            - name: update_every
              description: Data collection interval in seconds.
              default_value: 60
            - name: verbose
              description: Log every request.
              default_value: no
`

func TestAssertConfigSchemaMatchesMetadata_FlatForm(t *testing.T) {
	tests := map[string]struct {
		documented string
		defaults   bool
		wantFail   bool
	}{
		"aligned":           {documented: flatAligned},
		"aligned, defaults": {documented: flatAligned, defaults: true},
		"undocumented property": {
			documented: `            - name: update_every
              description: Data collection interval in seconds.
              default_value: 60
`,
			wantFail: true,
		},
		"unknown option": {
			documented: flatAligned + `            - name: timeout
              description: Request timeout.
              default_value: 1
`,
			wantFail: true,
		},
		"description drift": {
			documented: `            - name: update_every
              description: Interval in seconds.
              default_value: 60
            - name: verbose
              description: Log every request.
              default_value: no
`,
			wantFail: true,
		},
		"default drift ignored without the option": {
			documented: `            - name: update_every
              description: Data collection interval in seconds.
              default_value: 30
            - name: verbose
              description: Log every request.
              default_value: no
`,
		},
		"default drift": {
			documented: `            - name: update_every
              description: Data collection interval in seconds.
              default_value: 30
            - name: verbose
              description: Log every request.
              default_value: no
`,
			defaults: true,
			wantFail: true,
		},
		"group on a flat form": {
			documented: `            - name: update_every
              group: General
              description: Data collection interval in seconds.
              default_value: 60
            - name: verbose
              description: Log every request.
              default_value: no
`,
			wantFail: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			schemaPath, metadataPath := flatArtifacts(t, test.documented)
			probe := &probeT{T: t}
			func() {
				defer func() { _ = recover() }() // require.* in the helper panics through runtime.Goexit substitutes
				AssertConfigSchemaMatchesMetadataWith(probe, schemaPath, metadataPath, ConfigSchemaCheck{Defaults: test.defaults})
			}()
			require.Equal(t, test.wantFail, probe.failed, "failures: %v", probe.messages)
		})
	}
}

// probeT records failures instead of failing the enclosing test.
type probeT struct {
	*testing.T
	failed   bool
	messages []string
}

func (p *probeT) Errorf(format string, args ...any) {
	p.failed = true
	p.messages = append(p.messages, format)
}

func (p *probeT) FailNow() {
	p.failed = true
	panic("FailNow")
}

func (p *probeT) Helper() {}
