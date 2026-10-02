// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/configform"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/strictjson"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
)

// packageDescription is the describe output of a self-contained package. It
// carries assets inline and cannot choose its execution command.
type packageDescription struct {
	packageSpec  `               yaml:",inline"`
	Charts       string         `yaml:"charts"        json:"charts"`
	ConfigSchema map[string]any `yaml:"config_schema" json:"config_schema"`
}

// Description metadata may use null for any optional value, as YAML can.
var descriptionShape = strictjson.Optional(strictjson.Fields{
	"functions":     nativefunc.DefinitionShape(),
	"config_schema": strictjson.Any(),
}, "version", "mode", "snapshot_format", "collect", "charts")

// loadDescribedPackage runs a self-contained package's describe operation once
// and validates its output.
func loadDescribedPackage(ctx context.Context, command []string) (packageDefinition, error) {
	data, err := runDescribe(ctx, command)
	if err != nil {
		return packageDefinition{}, err
	}
	return parseDescription(data, command)
}

// parseDescription validates a description. Errors name the failing stage only:
// the document, declarations, form and template are script-produced.
func parseDescription(data []byte, command []string) (packageDefinition, error) {
	var description packageDescription
	// YAML flow mappings can also start with '{'. Only complete JSON selects this
	// path; encoding/json handles surrogate-pair escapes that yaml.v2 rejects.
	if json.Valid(data) {
		if strictjson.Decode(data, descriptionShape, &description) != nil {
			return packageDefinition{}, errors.New("invalid package description document")
		}
	} else if err := decodeYAMLDocument(data, &description); err != nil {
		return packageDefinition{}, fmt.Errorf("package description %w", err)
	}
	def, err := newPackageDefinition(description.packageSpec, slices.Clone(command))
	if err != nil {
		return packageDefinition{}, errors.New("invalid package description declarations")
	}
	if description.ConfigSchema != nil {
		if def.form, err = configform.ParseValue(description.ConfigSchema); err != nil {
			return packageDefinition{}, errors.New("invalid inline configuration form")
		}
	}
	var charts []byte
	if description.Charts != "" {
		charts = []byte(description.Charts)
	}
	if def.templates, err = def.chartTemplates(charts); err != nil {
		return packageDefinition{}, errors.New("invalid inline chart template")
	}
	return def, nil
}
