// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	tests := map[string]struct {
		path    string
		wantCfg UnboundConfig
		wantErr bool
	}{
		"valid include": {
			path: "testdata/valid_include.conf",
			wantCfg: UnboundConfig{
				cumulative: "yes",
				enable:     "yes",
				iface:      "10.0.0.1",
				port:       "8955",
				useCert:    "yes",
				keyFile:    "/etc/unbound/unbound_control_2.key",
				certFile:   "/etc/unbound/unbound_control_2.pem",
			},
		},
		"valid include-toplevel": {
			path: "testdata/valid_include_toplevel.conf",
			wantCfg: UnboundConfig{
				cumulative: "yes",
				enable:     "yes",
				iface:      "10.0.0.1",
				port:       "8955",
				useCert:    "yes",
				keyFile:    "/etc/unbound/unbound_control_2.key",
				certFile:   "/etc/unbound/unbound_control_2.pem",
			},
		},
		"valid glob include": {
			path: "testdata/valid_glob.conf",
			wantCfg: UnboundConfig{
				cumulative: "yes",
				enable:     "yes",
				iface:      "10.0.0.1",
				port:       "8955",
				useCert:    "yes",
				keyFile:    "/etc/unbound/unbound_control_2.key",
				certFile:   "/etc/unbound/unbound_control_2.pem",
			},
		},
		"non existent glob include": {
			path: "testdata/non_existent_glob_include.conf",
			wantCfg: UnboundConfig{
				cumulative: "yes",
				enable:     "yes",
				iface:      "10.0.0.1",
				port:       "8953",
				useCert:    "yes",
				keyFile:    "/etc/unbound/unbound_control.key",
				certFile:   "/etc/unbound/unbound_control.pem",
			},
		},
		"infinite recursion include": {
			path:    "testdata/infinite_rec.conf",
			wantErr: true,
		},
		"non existent include": {
			path:    "testdata/non_existent_include.conf",
			wantErr: true,
		},
		"non existent path": {
			path:    "testdata/non_existent_path.conf",
			wantErr: true,
		},
	}

	for name, test := range tests {
		name = fmt.Sprintf("%s (%s)", name, test.path)
		t.Run(name, func(t *testing.T) {
			cfg, err := Parse(test.path)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.wantCfg, *cfg)
			}
		})
	}
}
