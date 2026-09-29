// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"gopkg.in/yaml.v2"
)

// A description carries assets inline and cannot choose its execution command.
type packageDescription struct {
	packageSpec  `yaml:",inline"`
	Charts       string         `yaml:"charts"        json:"charts"`
	ConfigSchema map[string]any `yaml:"config_schema" json:"config_schema"`
}

func parseDescription(data []byte, command []string) (manifest, *chartengine.TemplateSet, error) {
	var description packageDescription
	var m manifest
	// YAML flow mappings can also start with '{'. Only complete JSON selects this
	// path; encoding/json handles surrogate-pair escapes that yaml.v2 rejects.
	if json.Valid(data) {
		if decodeMessage(data, "description", &description) != nil {
			return m, nil, fmt.Errorf("invalid package description document")
		}
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.SetStrict(true)
		if decoder.Decode(&description) != nil {
			return m, nil, fmt.Errorf("invalid package description document")
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return m, nil, fmt.Errorf("package description must contain one document")
		}
	}
	m.packageSpec = description.packageSpec
	m.Command = append([]string(nil), command...)
	if m.validate() != nil {
		return m, nil, fmt.Errorf("invalid package description declarations")
	}
	if description.ConfigSchema != nil {
		value, err := jsonValue(description.ConfigSchema)
		if err != nil {
			return m, nil, fmt.Errorf("invalid inline configuration form")
		}
		data, err := json.Marshal(value)
		if err != nil {
			return m, nil, fmt.Errorf("invalid inline configuration form")
		}
		m.config, err = parsePackageConfig(data)
		if err != nil {
			return m, nil, fmt.Errorf("invalid inline configuration form")
		}
	}
	var charts []byte
	if description.Charts != "" {
		charts = []byte(description.Charts)
	}
	templates, err := m.chartTemplates(charts)
	if err != nil {
		return m, nil, fmt.Errorf("invalid inline chart template")
	}
	return m, templates, nil
}
