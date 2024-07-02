// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
)

func TestWatcher_String(t *testing.T) {
	assert.NotEmpty(t, NewWatcher(confgroup.Registry{}, nil))
}

func TestNewWatcher(t *testing.T) {
	tests := map[string]struct {
		reg   confgroup.Registry
		paths []string
	}{
		"empty inputs": {
			reg:   confgroup.Registry{},
			paths: []string{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NotNil(t, NewWatcher(test.reg, test.paths))
		})
	}
}

func TestWatcher_Run(t *testing.T) {
	tests := map[string]struct {
		createSim func(tmp *tmpDir) discoverySim
	}{
		"file exists before start": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "module",
					},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
				}

				sim := discoverySim{
					discovery: discovery,
					beforeRun: func() {
						tmp.writeYAML(filename, cfg)
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
		"empty file": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
					},
				}

				sim := discoverySim{
					discovery: discovery,
					beforeRun: func() {
						tmp.writeString(filename, "")
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
		"only comments, no data": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
					},
				}

				sim := discoverySim{
					discovery: discovery,
					beforeRun: func() {
						tmp.writeString(filename, "# a comment")
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
		"add file": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "module",
					},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
				}

				sim := discoverySim{
					discovery: discovery,
					afterRun: func() {
						tmp.writeYAML(filename, cfg)
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
		"remove file": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "module",
					},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
					{
						Source:  filename,
						Configs: nil,
					},
				}

				sim := discoverySim{
					discovery: discovery,
					beforeRun: func() {
						tmp.writeYAML(filename, cfg)
					},
					afterRun: func() {
						tmp.removeFile(filename)
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
		"change file": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				cfgOrig := sdConfig{
					{
						"name":   "name",
						"module": "module",
					},
				}
				cfgChanged := sdConfig{
					{
						"name":   "name_changed",
						"module": "module",
					},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name_changed",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
				}

				sim := discoverySim{
					discovery: discovery,
					beforeRun: func() {
						tmp.writeYAML(filename, cfgOrig)
					},
					afterRun: func() {
						tmp.writeYAML(filename, cfgChanged)
						time.Sleep(time.Millisecond * 500)
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
		"vim 'backupcopy=no' (writing to a file and backup)": {
			createSim: func(tmp *tmpDir) discoverySim {
				reg := confgroup.Registry{
					"module": {},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "module",
					},
				}
				filename := tmp.join("module.conf")
				discovery := prepareDiscovery(t, Config{
					Registry: reg,
					Watch:    []string{tmp.join("*.conf")},
				})
				expected := []*confgroup.Group{
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     confgroup.TypeStock,
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
					{
						Source: filename,
						Configs: []confgroup.Config{
							{
								"name":                "name",
								"module":              "module",
								"update_every":        module.UpdateEvery,
								"autodetection_retry": module.AutoDetectionRetry,
								"priority":            module.Priority,
								"__provider__":        "file watcher",
								"__source_type__":     "stock",
								"__source__":          fmt.Sprintf("discoverer=file_watcher,file=%s", filename),
							},
						},
					},
				}

				sim := discoverySim{
					discovery: discovery,
					beforeRun: func() {
						tmp.writeYAML(filename, cfg)
					},
					afterRun: func() {
						newFilename := filename + ".swp"
						tmp.renameFile(filename, newFilename)
						tmp.writeYAML(filename, cfg)
						tmp.removeFile(newFilename)
						time.Sleep(time.Millisecond * 500)
					},
					expectedGroups: expected,
				}
				return sim
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tmp := newTmpDir(t, "watch-run-*")
			defer tmp.cleanup()

			test.createSim(tmp).run(t)
		})
	}
}
