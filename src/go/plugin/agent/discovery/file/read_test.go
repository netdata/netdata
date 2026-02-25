// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/stretchr/testify/assert"
)

func TestReader_New(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"string is not empty": {
			run: func(t *testing.T) {
				assert.NotEmpty(t, NewReader(confgroup.Registry{}, nil))
			},
		},
		"empty inputs create reader": {
			run: func(t *testing.T) {
				assert.NotNil(t, NewReader(confgroup.Registry{}, []string{}))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func TestReader_Run(t *testing.T) {
	tests := map[string]struct {
		createSim func(tmp *tmpDir) discoverySim
	}{
		"read multiple files": {
			createSim: func(tmp *tmpDir) discoverySim {
				module1 := tmp.join("module1.conf")
				module2 := tmp.join("module2.conf")
				module3 := tmp.join("module3.conf")

				tmp.writeYAML(module1, staticConfig{
					Jobs: []confgroup.Config{{"name": "name"}},
				})
				tmp.writeYAML(module2, staticConfig{
					Jobs: []confgroup.Config{{"name": "name"}},
				})
				tmp.writeString(module3, "# a comment")

				reg := confgroup.Registry{
					"module1": {},
					"module2": {},
					"module3": {},
				}
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Read:     []string{module1, module2, module3},
				})
				expected := []*confgroup.Group{
					{
						Source: module1,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module1",
								"update_every":        collectorapi.UpdateEvery,
								"autodetection_retry": collectorapi.AutoDetectionRetry,
								"priority":            collectorapi.Priority,
								"__provider__":        "file reader",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_reader,file=%s", module1),
							},
						},
					},
					{
						Source: module2,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module2",
								"update_every":        collectorapi.UpdateEvery,
								"autodetection_retry": collectorapi.AutoDetectionRetry,
								"priority":            collectorapi.Priority,
								"__provider__":        "file reader",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_reader,file=%s", module2),
							},
						},
					},
					{
						Source: module3,
					},
				}

				return discoverySim{
					discovery:      discovery,
					expectedGroups: expected,
				}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tmp := newTmpDir(t, "reader-run-*")
			defer tmp.cleanup()

			test.createSim(tmp).run(t)
		})
	}
}
