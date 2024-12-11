// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default": {
			wantFail: true,
			config:   New().Config,
		},
		"empty files->include and dirs->include": {
			wantFail: true,
			config: Config{
				Files: filesConfig{},
				Dirs:  dirsConfig{},
			},
		},
		"files->include and dirs->include": {
			wantFail: false,
			config: Config{
				Files: filesConfig{
					Include: []string{
						"/path/to/file1",
						"/path/to/file2",
					},
				},
				Dirs: dirsConfig{
					Include: []string{
						"/path/to/dir1",
						"/path/to/dir2",
					},
					CollectDirSize: true,
				},
			},
		},
		"only files->include": {
			wantFail: false,
			config: Config{
				Files: filesConfig{
					Include: []string{
						"/path/to/file1",
						"/path/to/file2",
					},
				},
			},
		},
		"only dirs->include": {
			wantFail: false,
			config: Config{
				Dirs: dirsConfig{
					Include: []string{
						"/path/to/dir1",
						"/path/to/dir2",
					},
					CollectDirSize: true,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				require.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
	}{
		"collect files":                   {prepare: prepareFilecheckFiles},
		"collect files filepath pattern":  {prepare: prepareFilecheckGlobFiles},
		"collect only non existent files": {prepare: prepareFilecheckNonExistentFiles},
		"collect dirs":                    {prepare: prepareFilecheckDirs},
		"collect dirs filepath pattern":   {prepare: prepareFilecheckGlobDirs},
		"collect only non existent dirs":  {prepare: prepareFilecheckNonExistentDirs},
		"collect files and dirs":          {prepare: prepareFilecheckFilesDirs},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			assert.NoError(t, collr.Check(context.Background()))
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	// TODO: should use TEMP dir and create files/dirs dynamically during a test case
	tests := map[string]struct {
		prepare       func() *Collector
		wantCollected map[string]int64
	}{
		"collect files": {
			prepare: prepareFilecheckFiles,
			wantCollected: map[string]int64{
				"file_testdata/empty_file.log_existence_status_exist":            1,
				"file_testdata/empty_file.log_existence_status_not_exist":        0,
				"file_testdata/empty_file.log_mtime_ago":                         517996,
				"file_testdata/empty_file.log_size_bytes":                        0,
				"file_testdata/file.log_existence_status_exist":                  1,
				"file_testdata/file.log_existence_status_not_exist":              0,
				"file_testdata/file.log_mtime_ago":                               517996,
				"file_testdata/file.log_size_bytes":                              5707,
				"file_testdata/non_existent_file.log_existence_status_exist":     0,
				"file_testdata/non_existent_file.log_existence_status_not_exist": 1,
			},
		},
		"collect files filepath pattern": {
			prepare: prepareFilecheckGlobFiles,
			wantCollected: map[string]int64{
				"file_testdata/empty_file.log_existence_status_exist":     1,
				"file_testdata/empty_file.log_existence_status_not_exist": 0,
				"file_testdata/empty_file.log_mtime_ago":                  517985,
				"file_testdata/empty_file.log_size_bytes":                 0,
				"file_testdata/file.log_existence_status_exist":           1,
				"file_testdata/file.log_existence_status_not_exist":       0,
				"file_testdata/file.log_mtime_ago":                        517985,
				"file_testdata/file.log_size_bytes":                       5707,
			},
		},
		"collect only non existent files": {
			prepare: prepareFilecheckNonExistentFiles,
			wantCollected: map[string]int64{
				"file_testdata/non_existent_file.log_existence_status_exist":     0,
				"file_testdata/non_existent_file.log_existence_status_not_exist": 1,
			},
		},
		"collect dirs": {
			prepare: prepareFilecheckDirs,
			wantCollected: map[string]int64{
				"dir_testdata/dir_existence_status_exist":                  1,
				"dir_testdata/dir_existence_status_not_exist":              0,
				"dir_testdata/dir_files_count":                             3,
				"dir_testdata/dir_mtime_ago":                               517914,
				"dir_testdata/non_existent_dir_existence_status_exist":     0,
				"dir_testdata/non_existent_dir_existence_status_not_exist": 1,
			},
		},
		"collect dirs filepath pattern": {
			prepare: prepareFilecheckGlobDirs,
			wantCollected: map[string]int64{
				"dir_testdata/dir_existence_status_exist":                  1,
				"dir_testdata/dir_existence_status_not_exist":              0,
				"dir_testdata/dir_files_count":                             3,
				"dir_testdata/dir_mtime_ago":                               517902,
				"dir_testdata/non_existent_dir_existence_status_exist":     0,
				"dir_testdata/non_existent_dir_existence_status_not_exist": 1,
			},
		},
		"collect dirs w/o size": {
			prepare: prepareFilecheckDirsWithoutSize,
			wantCollected: map[string]int64{
				"dir_testdata/dir_existence_status_exist":                  1,
				"dir_testdata/dir_existence_status_not_exist":              0,
				"dir_testdata/dir_files_count":                             3,
				"dir_testdata/dir_mtime_ago":                               517892,
				"dir_testdata/non_existent_dir_existence_status_exist":     0,
				"dir_testdata/non_existent_dir_existence_status_not_exist": 1,
			},
		},
		"collect only non existent dirs": {
			prepare: prepareFilecheckNonExistentDirs,
			wantCollected: map[string]int64{
				"dir_testdata/non_existent_dir_existence_status_exist":     0,
				"dir_testdata/non_existent_dir_existence_status_not_exist": 1,
			},
		},
		"collect files and dirs": {
			prepare: prepareFilecheckFilesDirs,
			wantCollected: map[string]int64{
				"dir_testdata/dir_existence_status_exist":                        1,
				"dir_testdata/dir_existence_status_not_exist":                    0,
				"dir_testdata/dir_files_count":                                   3,
				"dir_testdata/dir_mtime_ago":                                     517858,
				"dir_testdata/dir_size_bytes":                                    8160,
				"dir_testdata/non_existent_dir_existence_status_exist":           0,
				"dir_testdata/non_existent_dir_existence_status_not_exist":       1,
				"file_testdata/empty_file.log_existence_status_exist":            1,
				"file_testdata/empty_file.log_existence_status_not_exist":        0,
				"file_testdata/empty_file.log_mtime_ago":                         517858,
				"file_testdata/empty_file.log_size_bytes":                        0,
				"file_testdata/file.log_existence_status_exist":                  1,
				"file_testdata/file.log_existence_status_not_exist":              0,
				"file_testdata/file.log_mtime_ago":                               517858,
				"file_testdata/file.log_size_bytes":                              5707,
				"file_testdata/non_existent_file.log_existence_status_exist":     0,
				"file_testdata/non_existent_file.log_existence_status_not_exist": 1,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			copyModTime(test.wantCollected, mx)

			assert.Equal(t, test.wantCollected, mx)

			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareFilecheckFiles() *Collector {
	collr := New()
	collr.Config.Files.Include = []string{
		"testdata/empty_file.log",
		"testdata/file.log",
		"testdata/non_existent_file.log",
	}
	return collr
}

func prepareFilecheckGlobFiles() *Collector {
	collr := New()
	collr.Config.Files.Include = []string{
		"testdata/*.log",
	}
	return collr
}

func prepareFilecheckNonExistentFiles() *Collector {
	collr := New()
	collr.Config.Files.Include = []string{
		"testdata/non_existent_file.log",
	}
	return collr
}

func prepareFilecheckDirs() *Collector {
	collr := New()
	collr.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return collr
}

func prepareFilecheckGlobDirs() *Collector {
	collr := New()
	collr.Config.Dirs.Include = []string{
		"testdata/*ir",
		"testdata/non_existent_dir",
	}
	return collr
}

func prepareFilecheckDirsWithoutSize() *Collector {
	collr := New()
	collr.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return collr
}

func prepareFilecheckNonExistentDirs() *Collector {
	collr := New()
	collr.Config.Dirs.Include = []string{
		"testdata/non_existent_dir",
	}
	return collr
}

func prepareFilecheckFilesDirs() *Collector {
	collr := New()
	collr.Config.Dirs.CollectDirSize = true
	collr.Config.Files.Include = []string{
		"testdata/empty_file.log",
		"testdata/file.log",
		"testdata/non_existent_file.log",
	}
	collr.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return collr
}

func copyModTime(dst, src map[string]int64) {
	if src == nil || dst == nil {
		return
	}
	for key := range src {
		if strings.Contains(key, "mtime") {
			dst[key] = src[key]
		}
	}
}
