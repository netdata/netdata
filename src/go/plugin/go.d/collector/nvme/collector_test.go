// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package nvme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	dataNVMeListEmptyJson, _ = os.ReadFile("testdata/nvme-list-empty.json")

	dataVer23NVMeListJson, _           = os.ReadFile("testdata/v2.3/nvme-list.json")
	dataVer23NVMeSmartLogJson, _       = os.ReadFile("testdata/v2.3/nvme-smart-log.json")
	dataVer23NVMeSmartLogStringJson, _ = os.ReadFile("testdata/v2.3/nvme-smart-log-string.json")
	dataVer23NVMeSmartLogFloatJson, _  = os.ReadFile("testdata/v2.3/nvme-smart-log-float.json")

	dataVer211NVMeListJson, _     = os.ReadFile("testdata/v2.11/nvme-list.json")
	dataVer211NVMeSmartLogJson, _ = os.ReadFile("testdata/v2.11/nvme-smart-log.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":             dataConfigJSON,
		"dataConfigYAML":             dataConfigYAML,
		"dataNVMeListEmptyJSON":      dataNVMeListEmptyJson,
		"dataNVMeListJSON":           dataVer23NVMeListJson,
		"dataNVMeSmartLogStringJSON": dataVer23NVMeSmartLogStringJson,
		"dataNVMeSmartLogFloatJSON":  dataVer23NVMeSmartLogFloatJson,
		"dataVer211NVMeListJson":     dataVer211NVMeListJson,
		"dataVer211NVMeSmartLogJson": dataVer211NVMeSmartLogJson,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(*Collector)
	}{
		"success if all calls successful": {
			wantFail: false,
			prepare:  prepareCaseVer23OK,
		},
		"fails if 'nvme list' returns an empty list": {
			wantFail: true,
			prepare:  prepareCaseEmptyList,
		},
		"fails if 'nvme list' returns an error": {
			wantFail: true,
			prepare:  prepareCaseErrOnList,
		},
		"fails if 'nvme smart-log' returns an error": {
			wantFail: true,
			prepare:  prepareCaseErrOnSmartLog,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			test.prepare(collr)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	type testCaseStep struct {
		prepare func(*Collector)
		check   func(*testing.T, *Collector)
	}

	tests := map[string][]testCaseStep{
		"v2.11: success if all calls successful": {
			{
				prepare: prepareCaseVer211OK,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"device_nvme0_available_spare":                              100,
						"device_nvme0_controller_busy_time":                         4172940,
						"device_nvme0_critical_comp_time":                           0,
						"device_nvme0_critical_warning_available_spare":             0,
						"device_nvme0_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0_critical_warning_read_only":                   0,
						"device_nvme0_critical_warning_temp_threshold":              0,
						"device_nvme0_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0_data_units_read":                              91155062272000,
						"device_nvme0_data_units_written":                           987485941760000,
						"device_nvme0_host_read_commands":                           5808178366,
						"device_nvme0_host_write_commands":                          24273507789,
						"device_nvme0_media_errors":                                 0,
						"device_nvme0_num_err_log_entries":                          212,
						"device_nvme0_percentage_used":                              30,
						"device_nvme0_power_cycles":                                 99,
						"device_nvme0_power_on_time":                                87926400,
						"device_nvme0_temperature":                                  50,
						"device_nvme0_thm_temp1_total_time":                         1474,
						"device_nvme0_thm_temp1_trans_count":                        3,
						"device_nvme0_thm_temp2_total_time":                         0,
						"device_nvme0_thm_temp2_trans_count":                        0,
						"device_nvme0_unsafe_shutdowns":                             49,
						"device_nvme0_warning_temp_time":                            0,
						"device_nvme1_available_spare":                              100,
						"device_nvme1_controller_busy_time":                         4172940,
						"device_nvme1_critical_comp_time":                           0,
						"device_nvme1_critical_warning_available_spare":             0,
						"device_nvme1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1_critical_warning_read_only":                   0,
						"device_nvme1_critical_warning_temp_threshold":              0,
						"device_nvme1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1_data_units_read":                              91155062272000,
						"device_nvme1_data_units_written":                           987485941760000,
						"device_nvme1_host_read_commands":                           5808178366,
						"device_nvme1_host_write_commands":                          24273507789,
						"device_nvme1_media_errors":                                 0,
						"device_nvme1_num_err_log_entries":                          212,
						"device_nvme1_percentage_used":                              30,
						"device_nvme1_power_cycles":                                 99,
						"device_nvme1_power_on_time":                                87926400,
						"device_nvme1_temperature":                                  50,
						"device_nvme1_thm_temp1_total_time":                         1474,
						"device_nvme1_thm_temp1_trans_count":                        3,
						"device_nvme1_thm_temp2_total_time":                         0,
						"device_nvme1_thm_temp2_trans_count":                        0,
						"device_nvme1_unsafe_shutdowns":                             49,
						"device_nvme1_warning_temp_time":                            0,
						"device_nvme2_available_spare":                              100,
						"device_nvme2_controller_busy_time":                         4172940,
						"device_nvme2_critical_comp_time":                           0,
						"device_nvme2_critical_warning_available_spare":             0,
						"device_nvme2_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme2_critical_warning_persistent_memory_read_only": 0,
						"device_nvme2_critical_warning_read_only":                   0,
						"device_nvme2_critical_warning_temp_threshold":              0,
						"device_nvme2_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme2_data_units_read":                              91155062272000,
						"device_nvme2_data_units_written":                           987485941760000,
						"device_nvme2_host_read_commands":                           5808178366,
						"device_nvme2_host_write_commands":                          24273507789,
						"device_nvme2_media_errors":                                 0,
						"device_nvme2_num_err_log_entries":                          212,
						"device_nvme2_percentage_used":                              30,
						"device_nvme2_power_cycles":                                 99,
						"device_nvme2_power_on_time":                                87926400,
						"device_nvme2_temperature":                                  50,
						"device_nvme2_thm_temp1_total_time":                         1474,
						"device_nvme2_thm_temp1_trans_count":                        3,
						"device_nvme2_thm_temp2_total_time":                         0,
						"device_nvme2_thm_temp2_trans_count":                        0,
						"device_nvme2_unsafe_shutdowns":                             49,
						"device_nvme2_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"v2.3: success if all calls successful": {
			{
				prepare: prepareCaseVer23OK,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"device_nvme0_available_spare":                              100,
						"device_nvme0_controller_busy_time":                         497040,
						"device_nvme0_critical_comp_time":                           0,
						"device_nvme0_critical_warning_available_spare":             0,
						"device_nvme0_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0_critical_warning_read_only":                   0,
						"device_nvme0_critical_warning_temp_threshold":              0,
						"device_nvme0_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0_data_units_read":                              5068041216000,
						"device_nvme0_data_units_written":                           69712734208000,
						"device_nvme0_host_read_commands":                           313528805,
						"device_nvme0_host_write_commands":                          1928062610,
						"device_nvme0_media_errors":                                 0,
						"device_nvme0_num_err_log_entries":                          110,
						"device_nvme0_percentage_used":                              2,
						"device_nvme0_power_cycles":                                 64,
						"device_nvme0_power_on_time":                                17906400,
						"device_nvme0_temperature":                                  36,
						"device_nvme0_thm_temp1_total_time":                         0,
						"device_nvme0_thm_temp1_trans_count":                        0,
						"device_nvme0_thm_temp2_total_time":                         0,
						"device_nvme0_thm_temp2_trans_count":                        0,
						"device_nvme0_unsafe_shutdowns":                             39,
						"device_nvme0_warning_temp_time":                            0,
						"device_nvme1_available_spare":                              100,
						"device_nvme1_controller_busy_time":                         497040,
						"device_nvme1_critical_comp_time":                           0,
						"device_nvme1_critical_warning_available_spare":             0,
						"device_nvme1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1_critical_warning_read_only":                   0,
						"device_nvme1_critical_warning_temp_threshold":              0,
						"device_nvme1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1_data_units_read":                              5068041216000,
						"device_nvme1_data_units_written":                           69712734208000,
						"device_nvme1_host_read_commands":                           313528805,
						"device_nvme1_host_write_commands":                          1928062610,
						"device_nvme1_media_errors":                                 0,
						"device_nvme1_num_err_log_entries":                          110,
						"device_nvme1_percentage_used":                              2,
						"device_nvme1_power_cycles":                                 64,
						"device_nvme1_power_on_time":                                17906400,
						"device_nvme1_temperature":                                  36,
						"device_nvme1_thm_temp1_total_time":                         0,
						"device_nvme1_thm_temp1_trans_count":                        0,
						"device_nvme1_thm_temp2_total_time":                         0,
						"device_nvme1_thm_temp2_trans_count":                        0,
						"device_nvme1_unsafe_shutdowns":                             39,
						"device_nvme1_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"v2.3: success if all calls successful with string values": {
			{
				prepare: prepareCaseVer23StringValuesOK,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"device_nvme0_available_spare":                              100,
						"device_nvme0_controller_busy_time":                         497040,
						"device_nvme0_critical_comp_time":                           0,
						"device_nvme0_critical_warning_available_spare":             0,
						"device_nvme0_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0_critical_warning_read_only":                   0,
						"device_nvme0_critical_warning_temp_threshold":              0,
						"device_nvme0_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0_data_units_read":                              5068041216000,
						"device_nvme0_data_units_written":                           69712734208000,
						"device_nvme0_host_read_commands":                           313528805,
						"device_nvme0_host_write_commands":                          1928062610,
						"device_nvme0_media_errors":                                 0,
						"device_nvme0_num_err_log_entries":                          110,
						"device_nvme0_percentage_used":                              2,
						"device_nvme0_power_cycles":                                 64,
						"device_nvme0_power_on_time":                                17906400,
						"device_nvme0_temperature":                                  36,
						"device_nvme0_thm_temp1_total_time":                         0,
						"device_nvme0_thm_temp1_trans_count":                        0,
						"device_nvme0_thm_temp2_total_time":                         0,
						"device_nvme0_thm_temp2_trans_count":                        0,
						"device_nvme0_unsafe_shutdowns":                             39,
						"device_nvme0_warning_temp_time":                            0,
						"device_nvme1_available_spare":                              100,
						"device_nvme1_controller_busy_time":                         497040,
						"device_nvme1_critical_comp_time":                           0,
						"device_nvme1_critical_warning_available_spare":             0,
						"device_nvme1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1_critical_warning_read_only":                   0,
						"device_nvme1_critical_warning_temp_threshold":              0,
						"device_nvme1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1_data_units_read":                              5068041216000,
						"device_nvme1_data_units_written":                           69712734208000,
						"device_nvme1_host_read_commands":                           313528805,
						"device_nvme1_host_write_commands":                          1928062610,
						"device_nvme1_media_errors":                                 0,
						"device_nvme1_num_err_log_entries":                          110,
						"device_nvme1_percentage_used":                              2,
						"device_nvme1_power_cycles":                                 64,
						"device_nvme1_power_on_time":                                17906400,
						"device_nvme1_temperature":                                  36,
						"device_nvme1_thm_temp1_total_time":                         0,
						"device_nvme1_thm_temp1_trans_count":                        0,
						"device_nvme1_thm_temp2_total_time":                         0,
						"device_nvme1_thm_temp2_trans_count":                        0,
						"device_nvme1_unsafe_shutdowns":                             39,
						"device_nvme1_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"v2.3: success if all calls successful with float values": {
			{
				prepare: prepareCaseVer23FloatValuesOK,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					expected := map[string]int64{
						"device_nvme0_available_spare":                              100,
						"device_nvme0_controller_busy_time":                         497040,
						"device_nvme0_critical_comp_time":                           0,
						"device_nvme0_critical_warning_available_spare":             0,
						"device_nvme0_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0_critical_warning_read_only":                   0,
						"device_nvme0_critical_warning_temp_threshold":              0,
						"device_nvme0_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0_data_units_read":                              5068041216000,
						"device_nvme0_data_units_written":                           69712734208000,
						"device_nvme0_host_read_commands":                           313528805,
						"device_nvme0_host_write_commands":                          1928062610,
						"device_nvme0_media_errors":                                 0,
						"device_nvme0_num_err_log_entries":                          110,
						"device_nvme0_percentage_used":                              2,
						"device_nvme0_power_cycles":                                 64,
						"device_nvme0_power_on_time":                                17906400,
						"device_nvme0_temperature":                                  36,
						"device_nvme0_thm_temp1_total_time":                         0,
						"device_nvme0_thm_temp1_trans_count":                        0,
						"device_nvme0_thm_temp2_total_time":                         0,
						"device_nvme0_thm_temp2_trans_count":                        0,
						"device_nvme0_unsafe_shutdowns":                             39,
						"device_nvme0_warning_temp_time":                            0,
						"device_nvme1_available_spare":                              100,
						"device_nvme1_controller_busy_time":                         497040,
						"device_nvme1_critical_comp_time":                           0,
						"device_nvme1_critical_warning_available_spare":             0,
						"device_nvme1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1_critical_warning_read_only":                   0,
						"device_nvme1_critical_warning_temp_threshold":              0,
						"device_nvme1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1_data_units_read":                              5068041216000,
						"device_nvme1_data_units_written":                           69712734208000,
						"device_nvme1_host_read_commands":                           313528805,
						"device_nvme1_host_write_commands":                          1928062610,
						"device_nvme1_media_errors":                                 0,
						"device_nvme1_num_err_log_entries":                          110,
						"device_nvme1_percentage_used":                              2,
						"device_nvme1_power_cycles":                                 64,
						"device_nvme1_power_on_time":                                17906400,
						"device_nvme1_temperature":                                  36,
						"device_nvme1_thm_temp1_total_time":                         0,
						"device_nvme1_thm_temp1_trans_count":                        0,
						"device_nvme1_thm_temp2_total_time":                         0,
						"device_nvme1_thm_temp2_trans_count":                        0,
						"device_nvme1_unsafe_shutdowns":                             39,
						"device_nvme1_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"fail if 'nvme list' returns an empty list": {
			{
				prepare: prepareCaseEmptyList,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					assert.Equal(t, (map[string]int64)(nil), mx)
				},
			},
		},
		"fail if 'nvme list' returns an error": {
			{
				prepare: prepareCaseErrOnList,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					assert.Equal(t, (map[string]int64)(nil), mx)
				},
			},
		},
		"fail if 'nvme smart-log' returns an error": {
			{
				prepare: prepareCaseErrOnSmartLog,
				check: func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())

					assert.Equal(t, (map[string]int64)(nil), mx)
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepare(collr)
					step.check(t, collr)
				})
			}
		})
	}
}

func prepareCaseVer211OK(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{
		dataList:     dataVer211NVMeListJson,
		dataSmartLog: dataVer211NVMeSmartLogJson,
	}
}

func prepareCaseVer23OK(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{
		dataList:     dataVer23NVMeListJson,
		dataSmartLog: dataVer23NVMeSmartLogJson,
	}
}

func prepareCaseVer23StringValuesOK(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{
		dataList:     dataVer23NVMeListJson,
		dataSmartLog: dataVer23NVMeSmartLogStringJson,
	}
}

func prepareCaseVer23FloatValuesOK(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{
		dataList:     dataVer23NVMeListJson,
		dataSmartLog: dataVer23NVMeSmartLogFloatJson,
	}
}

func prepareCaseEmptyList(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{
		dataList: dataNVMeListEmptyJson,
	}
}

func prepareCaseErrOnList(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{errOnList: true}
}

func prepareCaseErrOnSmartLog(collr *Collector) {
	collr.exec = &mockNVMeCLIExec{errOnSmartLog: true}
}

type mockNVMeCLIExec struct {
	errOnList     bool
	errOnSmartLog bool

	dataList     []byte
	dataSmartLog []byte
}

func (m *mockNVMeCLIExec) list() (*nvmeDeviceList, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	var v nvmeDeviceList
	if err := json.Unmarshal(m.dataList, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

func (m *mockNVMeCLIExec) smartLog(device string) (*nvmeDeviceSmartLog, error) {
	if m.errOnSmartLog {
		return nil, errors.New("mock.smartLog() error")
	}
	if !strings.HasPrefix(device, "/dev/") {
		return nil, errors.New("mock.smartLog() expects device path /dev/")
	}

	var v nvmeDeviceSmartLog
	if err := json.Unmarshal(m.dataSmartLog, &v); err != nil {
		return nil, err
	}

	return &v, nil
}
