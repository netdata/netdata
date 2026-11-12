// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"gopkg.in/yaml.v2"
)

// Decode legacy inline vnode objects into the canonical local_vnode field.
// Configuration output then gives the form one stable type for each field.
type plainConfig Config

func (c *Config) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	vnode := bytes.TrimSpace(fields["vnode"])
	if len(vnode) > 0 && vnode[0] == '{' {
		if local := bytes.TrimSpace(fields["local_vnode"]); len(local) > 0 && !bytes.Equal(local, []byte("null")) {
			return errors.New("legacy vnode object cannot be combined with local_vnode")
		}
		fields["local_vnode"] = vnode
		fields["vnode"] = json.RawMessage(`""`)
		normalized, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		data = normalized
	}
	return json.Unmarshal(data, (*plainConfig)(c))
}

func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	var fields map[string]any
	if err := unmarshal(&fields); err != nil {
		return err
	}
	switch fields["vnode"].(type) {
	case map[any]any, map[string]any:
		if fields["local_vnode"] != nil {
			return errors.New("legacy vnode object cannot be combined with local_vnode")
		}
		fields["local_vnode"] = fields["vnode"]
		fields["vnode"] = ""
		normalized, err := yaml.Marshal(fields)
		if err != nil {
			return err
		}
		return yaml.Unmarshal(normalized, (*plainConfig)(c))
	case nil, string:
		return unmarshal((*plainConfig)(c))
	default:
		return errors.New("vnode must be a name or a legacy inline object")
	}
}

func (c *Collector) SetConfiguredVnode(v vnodes.VirtualNode) {
	if c.Vnode == "" {
		return
	}
	// Runtime transfers ownership and calls this only at lifecycle boundaries.
	c.configuredVnode = &v
	// A state that is not published yet (new, suspended or just rebuilt) takes
	// this snapshot with its first publication, so no partial identity is
	// registered here.
	if c.devicePublished {
		c.registerDeviceState(c.sysInfo, c.preparedProfileMetadata())
	}
}

func (c *Collector) deviceVnode() *vnodes.VirtualNode {
	if c.Vnode != "" {
		return c.configuredVnode
	}
	return c.vnode
}
