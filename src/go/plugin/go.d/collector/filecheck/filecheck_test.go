// SPDX-License-Identifier: GPL-3.0-or-later

package filecheck

import (
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

func TestFilecheck_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Filecheck{}, dataConfigJSON, dataConfigYAML)
}

func TestFilecheck_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestFilecheck_Init(t *testing.T) {
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
			fc := New()
			fc.Config = test.config

			if test.wantFail {
				assert.Error(t, fc.Init())
			} else {
				require.NoError(t, fc.Init())
			}
		})
	}
}

func TestFilecheck_Check(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Filecheck
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
			fc := test.prepare()
			require.NoError(t, fc.Init())

			assert.NoError(t, fc.Check())
		})
	}
}

func TestFilecheck_Collect(t *testing.T) {
	// TODO: should use TEMP dir and create files/dirs dynamically during a test case
	tests := map[string]struct {
		prepare       func() *Filecheck
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
			fc := test.prepare()
			require.NoError(t, fc.Init())

			mx := fc.Collect()

			copyModTime(test.wantCollected, mx)

			assert.Equal(t, test.wantCollected, mx)

			module.TestMetricsHasAllChartsDims(t, fc.Charts(), mx)
		})
	}
}

func prepareFilecheckFiles() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/empty_file.log",
		"testdata/file.log",
		"testdata/non_existent_file.log",
	}
	return fc
}

func prepareFilecheckGlobFiles() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/*.log",
	}
	return fc
}

func prepareFilecheckNonExistentFiles() *Filecheck {
	fc := New()
	fc.Config.Files.Include = []string{
		"testdata/non_existent_file.log",
	}
	return fc
}

func prepareFilecheckDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckGlobDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/*ir",
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckDirsWithoutSize() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckNonExistentDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.Include = []string{
		"testdata/non_existent_dir",
	}
	return fc
}

func prepareFilecheckFilesDirs() *Filecheck {
	fc := New()
	fc.Config.Dirs.CollectDirSize = true
	fc.Config.Files.Include = []string{
		"testdata/empty_file.log",
		"testdata/file.log",
		"testdata/non_existent_file.log",
	}
	fc.Config.Dirs.Include = []string{
		"testdata/dir",
		"testdata/non_existent_dir",
	}
	return fc
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
