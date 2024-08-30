// SPDX-License-Identifier: GPL-3.0-or-later

package file

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	const (
		jobDef = 11
		cfgDef = 22
		modDef = 33
	)
	tests := map[string]struct {
		test func(t *testing.T, tmp *tmpDir)
	}{
		"static, default: +job +conf +module": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"module": {
						UpdateEvery:        modDef,
						AutoDetectionRetry: modDef,
						Priority:           modDef,
					},
				}
				cfg := staticConfig{
					Default: confgroup.Default{
						UpdateEvery:        cfgDef,
						AutoDetectionRetry: cfgDef,
						Priority:           cfgDef,
					},
					Jobs: []confgroup.Config{
						{
							"name":                "name",
							"update_every":        jobDef,
							"autodetection_retry": jobDef,
							"priority":            jobDef,
						},
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "module",
							"update_every":        jobDef,
							"autodetection_retry": jobDef,
							"priority":            jobDef,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"static, default: +job +conf +module (merge all)": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"module": {
						Priority: modDef,
					},
				}
				cfg := staticConfig{
					Default: confgroup.Default{
						AutoDetectionRetry: cfgDef,
					},
					Jobs: []confgroup.Config{
						{
							"name":         "name",
							"update_every": jobDef,
						},
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "module",
							"update_every":        jobDef,
							"autodetection_retry": cfgDef,
							"priority":            modDef,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"static, default: -job +conf +module": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"module": {
						UpdateEvery:        modDef,
						AutoDetectionRetry: modDef,
						Priority:           modDef,
					},
				}
				cfg := staticConfig{
					Default: confgroup.Default{
						UpdateEvery:        cfgDef,
						AutoDetectionRetry: cfgDef,
						Priority:           cfgDef,
					},
					Jobs: []confgroup.Config{
						{
							"name": "name",
						},
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "module",
							"update_every":        cfgDef,
							"autodetection_retry": cfgDef,
							"priority":            cfgDef,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"static, default: -job -conf +module": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"module": {
						UpdateEvery:        modDef,
						AutoDetectionRetry: modDef,
						Priority:           modDef,
					},
				}
				cfg := staticConfig{
					Jobs: []confgroup.Config{
						{
							"name": "name",
						},
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "module",
							"autodetection_retry": modDef,
							"priority":            modDef,
							"update_every":        modDef,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"static, default: -job -conf -module (+global)": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"module": {},
				}
				cfg := staticConfig{
					Jobs: []confgroup.Config{
						{
							"name": "name",
						},
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "module",
							"autodetection_retry": module.AutoDetectionRetry,
							"priority":            module.Priority,
							"update_every":        module.UpdateEvery,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"sd, default: +job +module": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"sd_module": {
						UpdateEvery:        modDef,
						AutoDetectionRetry: modDef,
						Priority:           modDef,
					},
				}
				cfg := sdConfig{
					{
						"name":                "name",
						"module":              "sd_module",
						"update_every":        jobDef,
						"autodetection_retry": jobDef,
						"priority":            jobDef,
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"module":              "sd_module",
							"name":                "name",
							"update_every":        jobDef,
							"autodetection_retry": jobDef,
							"priority":            jobDef,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"sd, default: -job +module": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"sd_module": {
						UpdateEvery:        modDef,
						AutoDetectionRetry: modDef,
						Priority:           modDef,
					},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "sd_module",
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "sd_module",
							"update_every":        modDef,
							"autodetection_retry": modDef,
							"priority":            modDef,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"sd, default: -job -module (+global)": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"sd_module": {},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "sd_module",
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source: filename,
					Configs: []confgroup.Config{
						{
							"name":                "name",
							"module":              "sd_module",
							"update_every":        module.UpdateEvery,
							"autodetection_retry": module.AutoDetectionRetry,
							"priority":            module.Priority,
						},
					},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"sd, job has no 'module' or 'module' is empty": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"sd_module": {},
				}
				cfg := sdConfig{
					{
						"name": "name",
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source:  filename,
					Configs: []confgroup.Config{},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"conf registry has no module": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"sd_module": {},
				}
				cfg := sdConfig{
					{
						"name":   "name",
						"module": "module",
					},
				}
				filename := tmp.join("module.conf")
				tmp.writeYAML(filename, cfg)

				expected := &confgroup.Group{
					Source:  filename,
					Configs: []confgroup.Config{},
				}

				group, err := parse(reg, filename)

				require.NoError(t, err)
				assert.Equal(t, expected, group)
			},
		},
		"empty file": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{
					"module": {},
				}

				filename := tmp.createFile("empty-*")
				group, err := parse(reg, filename)

				assert.Nil(t, group)
				require.NoError(t, err)
			},
		},
		"only comments, unknown empty format": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{}

				filename := tmp.createFile("unknown-empty-format-*")
				tmp.writeString(filename, "# a comment")
				group, err := parse(reg, filename)

				assert.Nil(t, group)
				assert.NoError(t, err)
			},
		},
		"unknown format": {
			test: func(t *testing.T, tmp *tmpDir) {
				reg := confgroup.Registry{}

				filename := tmp.createFile("unknown-format-*")
				tmp.writeYAML(filename, "unknown")
				group, err := parse(reg, filename)

				assert.Nil(t, group)
				assert.Error(t, err)
			},
		},
	}

	for name, scenario := range tests {
		t.Run(name, func(t *testing.T) {
			tmp := newTmpDir(t, "parse-file-*")
			defer tmp.cleanup()

			scenario.test(t, tmp)
		})
	}
}
