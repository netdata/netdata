// SPDX-License-Identifier: GPL-3.0-or-later

package nvme

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataNVMeListJSON, _           = os.ReadFile("testdata/nvme-list.json")
	dataNVMeListEmptyJSON, _      = os.ReadFile("testdata/nvme-list-empty.json")
	dataNVMeSmartLogJSON, _       = os.ReadFile("testdata/nvme-smart-log.json")
	dataNVMeSmartLogStringJSON, _ = os.ReadFile("testdata/nvme-smart-log-string.json")
	dataNVMeSmartLogFloatJSON, _  = os.ReadFile("testdata/nvme-smart-log-float.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":             dataConfigJSON,
		"dataConfigYAML":             dataConfigYAML,
		"dataNVMeListJSON":           dataNVMeListJSON,
		"dataNVMeListEmptyJSON":      dataNVMeListEmptyJSON,
		"dataNVMeSmartLogStringJSON": dataNVMeSmartLogStringJSON,
		"dataNVMeSmartLogFloatJSON":  dataNVMeSmartLogFloatJSON,
	} {
		require.NotNil(t, data, name)
	}
}

func TestNVMe_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &NVMe{}, dataConfigJSON, dataConfigYAML)
}

func TestNVMe_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"fails if 'ndsudo' not found": {
			wantFail: true,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			nv := New()

			if test.wantFail {
				assert.Error(t, nv.Init())
			} else {
				assert.NoError(t, nv.Init())
			}
		})
	}
}

func TestNVMe_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestNVMe_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestNVMe_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(n *NVMe)
	}{
		"success if all calls successful": {
			wantFail: false,
			prepare:  prepareCaseOK,
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
			n := New()

			test.prepare(n)

			if test.wantFail {
				assert.Error(t, n.Check())
			} else {
				assert.NoError(t, n.Check())
			}
		})
	}
}

func TestNVMe_Collect(t *testing.T) {
	type testCaseStep struct {
		prepare func(n *NVMe)
		check   func(t *testing.T, n *NVMe)
	}

	tests := map[string][]testCaseStep{
		"success if all calls successful": {
			{
				prepare: prepareCaseOK,
				check: func(t *testing.T, n *NVMe) {
					mx := n.Collect()

					expected := map[string]int64{
						"device_nvme0n1_available_spare":                              100,
						"device_nvme0n1_controller_busy_time":                         497040,
						"device_nvme0n1_critical_comp_time":                           0,
						"device_nvme0n1_critical_warning_available_spare":             0,
						"device_nvme0n1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0n1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0n1_critical_warning_read_only":                   0,
						"device_nvme0n1_critical_warning_temp_threshold":              0,
						"device_nvme0n1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0n1_data_units_read":                              5068041216000,
						"device_nvme0n1_data_units_written":                           69712734208000,
						"device_nvme0n1_host_read_commands":                           313528805,
						"device_nvme0n1_host_write_commands":                          1928062610,
						"device_nvme0n1_media_errors":                                 0,
						"device_nvme0n1_num_err_log_entries":                          110,
						"device_nvme0n1_percentage_used":                              2,
						"device_nvme0n1_power_cycles":                                 64,
						"device_nvme0n1_power_on_time":                                17906400,
						"device_nvme0n1_temperature":                                  36,
						"device_nvme0n1_thm_temp1_total_time":                         0,
						"device_nvme0n1_thm_temp1_trans_count":                        0,
						"device_nvme0n1_thm_temp2_total_time":                         0,
						"device_nvme0n1_thm_temp2_trans_count":                        0,
						"device_nvme0n1_unsafe_shutdowns":                             39,
						"device_nvme0n1_warning_temp_time":                            0,
						"device_nvme1n1_available_spare":                              100,
						"device_nvme1n1_controller_busy_time":                         497040,
						"device_nvme1n1_critical_comp_time":                           0,
						"device_nvme1n1_critical_warning_available_spare":             0,
						"device_nvme1n1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1n1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1n1_critical_warning_read_only":                   0,
						"device_nvme1n1_critical_warning_temp_threshold":              0,
						"device_nvme1n1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1n1_data_units_read":                              5068041216000,
						"device_nvme1n1_data_units_written":                           69712734208000,
						"device_nvme1n1_host_read_commands":                           313528805,
						"device_nvme1n1_host_write_commands":                          1928062610,
						"device_nvme1n1_media_errors":                                 0,
						"device_nvme1n1_num_err_log_entries":                          110,
						"device_nvme1n1_percentage_used":                              2,
						"device_nvme1n1_power_cycles":                                 64,
						"device_nvme1n1_power_on_time":                                17906400,
						"device_nvme1n1_temperature":                                  36,
						"device_nvme1n1_thm_temp1_total_time":                         0,
						"device_nvme1n1_thm_temp1_trans_count":                        0,
						"device_nvme1n1_thm_temp2_total_time":                         0,
						"device_nvme1n1_thm_temp2_trans_count":                        0,
						"device_nvme1n1_unsafe_shutdowns":                             39,
						"device_nvme1n1_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success if all calls successful with string values": {
			{
				prepare: prepareCaseStringValuesOK,
				check: func(t *testing.T, n *NVMe) {
					mx := n.Collect()

					expected := map[string]int64{
						"device_nvme0n1_available_spare":                              100,
						"device_nvme0n1_controller_busy_time":                         497040,
						"device_nvme0n1_critical_comp_time":                           0,
						"device_nvme0n1_critical_warning_available_spare":             0,
						"device_nvme0n1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0n1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0n1_critical_warning_read_only":                   0,
						"device_nvme0n1_critical_warning_temp_threshold":              0,
						"device_nvme0n1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0n1_data_units_read":                              5068041216000,
						"device_nvme0n1_data_units_written":                           69712734208000,
						"device_nvme0n1_host_read_commands":                           313528805,
						"device_nvme0n1_host_write_commands":                          1928062610,
						"device_nvme0n1_media_errors":                                 0,
						"device_nvme0n1_num_err_log_entries":                          110,
						"device_nvme0n1_percentage_used":                              2,
						"device_nvme0n1_power_cycles":                                 64,
						"device_nvme0n1_power_on_time":                                17906400,
						"device_nvme0n1_temperature":                                  36,
						"device_nvme0n1_thm_temp1_total_time":                         0,
						"device_nvme0n1_thm_temp1_trans_count":                        0,
						"device_nvme0n1_thm_temp2_total_time":                         0,
						"device_nvme0n1_thm_temp2_trans_count":                        0,
						"device_nvme0n1_unsafe_shutdowns":                             39,
						"device_nvme0n1_warning_temp_time":                            0,
						"device_nvme1n1_available_spare":                              100,
						"device_nvme1n1_controller_busy_time":                         497040,
						"device_nvme1n1_critical_comp_time":                           0,
						"device_nvme1n1_critical_warning_available_spare":             0,
						"device_nvme1n1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1n1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1n1_critical_warning_read_only":                   0,
						"device_nvme1n1_critical_warning_temp_threshold":              0,
						"device_nvme1n1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1n1_data_units_read":                              5068041216000,
						"device_nvme1n1_data_units_written":                           69712734208000,
						"device_nvme1n1_host_read_commands":                           313528805,
						"device_nvme1n1_host_write_commands":                          1928062610,
						"device_nvme1n1_media_errors":                                 0,
						"device_nvme1n1_num_err_log_entries":                          110,
						"device_nvme1n1_percentage_used":                              2,
						"device_nvme1n1_power_cycles":                                 64,
						"device_nvme1n1_power_on_time":                                17906400,
						"device_nvme1n1_temperature":                                  36,
						"device_nvme1n1_thm_temp1_total_time":                         0,
						"device_nvme1n1_thm_temp1_trans_count":                        0,
						"device_nvme1n1_thm_temp2_total_time":                         0,
						"device_nvme1n1_thm_temp2_trans_count":                        0,
						"device_nvme1n1_unsafe_shutdowns":                             39,
						"device_nvme1n1_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"success if all calls successful with float values": {
			{
				prepare: prepareCaseFloatValuesOK,
				check: func(t *testing.T, n *NVMe) {
					mx := n.Collect()

					expected := map[string]int64{
						"device_nvme0n1_available_spare":                              100,
						"device_nvme0n1_controller_busy_time":                         497040,
						"device_nvme0n1_critical_comp_time":                           0,
						"device_nvme0n1_critical_warning_available_spare":             0,
						"device_nvme0n1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme0n1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme0n1_critical_warning_read_only":                   0,
						"device_nvme0n1_critical_warning_temp_threshold":              0,
						"device_nvme0n1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme0n1_data_units_read":                              5068041216000,
						"device_nvme0n1_data_units_written":                           69712734208000,
						"device_nvme0n1_host_read_commands":                           313528805,
						"device_nvme0n1_host_write_commands":                          1928062610,
						"device_nvme0n1_media_errors":                                 0,
						"device_nvme0n1_num_err_log_entries":                          110,
						"device_nvme0n1_percentage_used":                              2,
						"device_nvme0n1_power_cycles":                                 64,
						"device_nvme0n1_power_on_time":                                17906400,
						"device_nvme0n1_temperature":                                  36,
						"device_nvme0n1_thm_temp1_total_time":                         0,
						"device_nvme0n1_thm_temp1_trans_count":                        0,
						"device_nvme0n1_thm_temp2_total_time":                         0,
						"device_nvme0n1_thm_temp2_trans_count":                        0,
						"device_nvme0n1_unsafe_shutdowns":                             39,
						"device_nvme0n1_warning_temp_time":                            0,
						"device_nvme1n1_available_spare":                              100,
						"device_nvme1n1_controller_busy_time":                         497040,
						"device_nvme1n1_critical_comp_time":                           0,
						"device_nvme1n1_critical_warning_available_spare":             0,
						"device_nvme1n1_critical_warning_nvm_subsystem_reliability":   0,
						"device_nvme1n1_critical_warning_persistent_memory_read_only": 0,
						"device_nvme1n1_critical_warning_read_only":                   0,
						"device_nvme1n1_critical_warning_temp_threshold":              0,
						"device_nvme1n1_critical_warning_volatile_mem_backup_failed":  0,
						"device_nvme1n1_data_units_read":                              5068041216000,
						"device_nvme1n1_data_units_written":                           69712734208000,
						"device_nvme1n1_host_read_commands":                           313528805,
						"device_nvme1n1_host_write_commands":                          1928062610,
						"device_nvme1n1_media_errors":                                 0,
						"device_nvme1n1_num_err_log_entries":                          110,
						"device_nvme1n1_percentage_used":                              2,
						"device_nvme1n1_power_cycles":                                 64,
						"device_nvme1n1_power_on_time":                                17906400,
						"device_nvme1n1_temperature":                                  36,
						"device_nvme1n1_thm_temp1_total_time":                         0,
						"device_nvme1n1_thm_temp1_trans_count":                        0,
						"device_nvme1n1_thm_temp2_total_time":                         0,
						"device_nvme1n1_thm_temp2_trans_count":                        0,
						"device_nvme1n1_unsafe_shutdowns":                             39,
						"device_nvme1n1_warning_temp_time":                            0,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"fail if 'nvme list' returns an empty list": {
			{
				prepare: prepareCaseEmptyList,
				check: func(t *testing.T, n *NVMe) {
					mx := n.Collect()

					assert.Equal(t, (map[string]int64)(nil), mx)
				},
			},
		},
		"fail if 'nvme list' returns an error": {
			{
				prepare: prepareCaseErrOnList,
				check: func(t *testing.T, n *NVMe) {
					mx := n.Collect()

					assert.Equal(t, (map[string]int64)(nil), mx)
				},
			},
		},
		"fail if 'nvme smart-log' returns an error": {
			{
				prepare: prepareCaseErrOnSmartLog,
				check: func(t *testing.T, n *NVMe) {
					mx := n.Collect()

					assert.Equal(t, (map[string]int64)(nil), mx)
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			n := New()

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepare(n)
					step.check(t, n)
				})
			}
		})
	}
}

func prepareCaseOK(n *NVMe) {
	n.exec = &mockNVMeCLIExec{}
}

func prepareCaseStringValuesOK(n *NVMe) {
	n.exec = &mockNVMeCLIExec{smartLogString: true}
}

func prepareCaseFloatValuesOK(n *NVMe) {
	n.exec = &mockNVMeCLIExec{smartLogFloat: true}
}

func prepareCaseEmptyList(n *NVMe) {
	n.exec = &mockNVMeCLIExec{emptyList: true}
}

func prepareCaseErrOnList(n *NVMe) {
	n.exec = &mockNVMeCLIExec{errOnList: true}
}

func prepareCaseErrOnSmartLog(n *NVMe) {
	n.exec = &mockNVMeCLIExec{errOnSmartLog: true}
}

type mockNVMeCLIExec struct {
	errOnList      bool
	errOnSmartLog  bool
	emptyList      bool
	smartLogString bool
	smartLogFloat  bool
}

func (m *mockNVMeCLIExec) list() (*nvmeDeviceList, error) {
	if m.errOnList {
		return nil, errors.New("mock.list() error")
	}

	data := dataNVMeListJSON
	if m.emptyList {
		data = dataNVMeListEmptyJSON
	}

	var v nvmeDeviceList
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

func (m *mockNVMeCLIExec) smartLog(_ string) (*nvmeDeviceSmartLog, error) {
	if m.errOnSmartLog {
		return nil, errors.New("mock.smartLog() error")
	}
	if m.emptyList {
		return nil, errors.New("mock.smartLog() no devices error")
	}

	data := dataNVMeSmartLogJSON
	if m.smartLogString {
		data = dataNVMeSmartLogStringJSON
	}
	if m.smartLogFloat {
		data = dataNVMeSmartLogFloatJSON
	}

	var v nvmeDeviceSmartLog
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	return &v, nil
}
