// SPDX-License-Identifier: GPL-3.0-or-later
package native

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotFormatSelection(t *testing.T) {
	for _, source := range []string{"command", "manifest"} {
		for _, format := range []string{"", modeAuto, formatJSON, formatLines, "invalid"} {
			t.Run(source+"/"+format, func(t *testing.T) {
				c, dir := fixtureCollector(t, "exit 0\n")
				appendFile(t, c.Manifest, "snapshot_format: lines\n")
				if source == "command" {
					c.Manifest = ""
					c.Command = []string{filepath.Join(dir, "collect.sh")}
				}
				c.SnapshotFormat = confopt.Enum[jobSnapshotFormatSpec](format)
				err := c.Init(context.Background())
				if format == "invalid" || (source == "manifest" && format != "" && format != modeAuto) {
					require.ErrorContains(t, err, "snapshot_format")
					return
				}
				require.NoError(t, err)
				expected := formatJSON
				if source == "manifest" || format == formatLines {
					expected = formatLines
				}
				assert.Equal(t, expected, c.definition.SnapshotFormat)
			})
		}
	}
}
func TestPackageSnapshotFormat(t *testing.T) {
	for _, source := range []string{"manifest", "description JSON", "description YAML"} {
		for _, format := range []string{"omitted", "null", "", formatJSON, formatLines, modeAuto, "invalid"} {
			for _, functionOnly := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/functionOnly=%t", source, format, functionOnly), func(t *testing.T) {
					declaration := "version: v1\n"
					if format != "omitted" {
						declaration += fmt.Sprintf("snapshot_format: %q\n", format)
					}
					if format == "null" {
						declaration = "version: v1\nsnapshot_format: null\n"
					}
					if functionOnly {
						declaration += "collect: false\nfunctions: [{id: items, name: Items, help: Show items.}]\n"
					}
					var def packageDefinition
					var err error
					switch source {
					case "manifest":
						_, dir := fixtureCollector(t, "exit 0\n")
						path := filepath.Join(dir, "format.yaml")
						require.NoError(t, os.WriteFile(path, []byte(declaration+"command: [./collect.sh]\n"), 0644))
						def, err = loadManifest(path, statExecutable)
					case "description YAML":
						def, err = parseDescription([]byte(declaration), []string{"/collect"})
					case "description JSON":
						declaration = `{"version":"v1"`
						if format == "null" {
							declaration += `,"snapshot_format":null`
						} else if format != "omitted" {
							declaration += fmt.Sprintf(`,"snapshot_format":%q`, format)
						}
						if functionOnly {
							declaration += `,"collect":false,"functions":[{"id":"items","name":"Items","help":"Show items."}]`
						}
						def, err = parseDescription([]byte(declaration+"}"), []string{"/collect"})
					}
					if format == modeAuto || format == "invalid" || (functionOnly && format == formatLines) {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					expected := formatJSON
					if format == formatLines {
						expected = formatLines
					}
					assert.Equal(t, expected, def.SnapshotFormat)
				})
			}
		}
	}
}
