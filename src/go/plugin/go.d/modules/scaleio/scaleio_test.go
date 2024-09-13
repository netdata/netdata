// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/scaleio/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataSelectedStatistics, _ = os.ReadFile("testdata/selected_statistics.json")
	dataInstances, _          = os.ReadFile("testdata/instances.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":         dataConfigJSON,
		"dataConfigYAML":         dataConfigYAML,
		"dataSelectedStatistics": dataSelectedStatistics,
		"dataInstances":          dataInstances,
	} {
		require.NotNil(t, data, name)
	}
}

func TestScaleIO_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &ScaleIO{}, dataConfigJSON, dataConfigYAML)
}

func TestScaleIO_Init(t *testing.T) {
	scaleIO := New()
	scaleIO.Username = "username"
	scaleIO.Password = "password"

	assert.NoError(t, scaleIO.Init())
}
func TestScaleIO_Init_UsernameAndPasswordNotSet(t *testing.T) {
	assert.Error(t, New().Init())
}

func TestScaleIO_Init_ErrorOnCreatingClientWrongTLSCA(t *testing.T) {
	job := New()
	job.Username = "username"
	job.Password = "password"
	job.ClientConfig.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, job.Init())
}

func TestScaleIO_Check(t *testing.T) {
	srv, _, scaleIO := prepareSrvMockScaleIO(t)
	defer srv.Close()
	require.NoError(t, scaleIO.Init())

	assert.NoError(t, scaleIO.Check())
}

func TestScaleIO_Check_ErrorOnLogin(t *testing.T) {
	srv, mock, scaleIO := prepareSrvMockScaleIO(t)
	defer srv.Close()
	require.NoError(t, scaleIO.Init())
	mock.Password = "new password"

	assert.Error(t, scaleIO.Check())
}

func TestScaleIO_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestScaleIO_Cleanup(t *testing.T) {
	srv, _, scaleIO := prepareSrvMockScaleIO(t)
	defer srv.Close()
	require.NoError(t, scaleIO.Init())
	require.NoError(t, scaleIO.Check())

	scaleIO.Cleanup()
	assert.False(t, scaleIO.client.LoggedIn())
}

func TestScaleIO_Collect(t *testing.T) {
	srv, _, scaleIO := prepareSrvMockScaleIO(t)
	defer srv.Close()
	require.NoError(t, scaleIO.Init())
	require.NoError(t, scaleIO.Check())

	expected := map[string]int64{
		"sdc_6076fd0f00000000_bandwidth_read":                                    0,
		"sdc_6076fd0f00000000_bandwidth_read_write":                              0,
		"sdc_6076fd0f00000000_bandwidth_write":                                   0,
		"sdc_6076fd0f00000000_io_size_read":                                      0,
		"sdc_6076fd0f00000000_io_size_read_write":                                0,
		"sdc_6076fd0f00000000_io_size_write":                                     0,
		"sdc_6076fd0f00000000_iops_read":                                         0,
		"sdc_6076fd0f00000000_iops_read_write":                                   0,
		"sdc_6076fd0f00000000_iops_write":                                        0,
		"sdc_6076fd0f00000000_mdm_connection_state":                              1,
		"sdc_6076fd0f00000000_num_of_mapped_volumes":                             1,
		"sdc_6076fd1000000001_bandwidth_read":                                    1000,
		"sdc_6076fd1000000001_bandwidth_read_write":                              117400000,
		"sdc_6076fd1000000001_bandwidth_write":                                   117399000,
		"sdc_6076fd1000000001_io_size_read":                                      1000,
		"sdc_6076fd1000000001_io_size_read_write":                                695668,
		"sdc_6076fd1000000001_io_size_write":                                     694668,
		"sdc_6076fd1000000001_iops_read":                                         1000,
		"sdc_6076fd1000000001_iops_read_write":                                   170000,
		"sdc_6076fd1000000001_iops_write":                                        169000,
		"sdc_6076fd1000000001_mdm_connection_state":                              0,
		"sdc_6076fd1000000001_num_of_mapped_volumes":                             1,
		"sdc_6076fd1100000002_bandwidth_read":                                    0,
		"sdc_6076fd1100000002_bandwidth_read_write":                              118972000,
		"sdc_6076fd1100000002_bandwidth_write":                                   118972000,
		"sdc_6076fd1100000002_io_size_read":                                      0,
		"sdc_6076fd1100000002_io_size_read_write":                                820496,
		"sdc_6076fd1100000002_io_size_write":                                     820496,
		"sdc_6076fd1100000002_iops_read":                                         0,
		"sdc_6076fd1100000002_iops_read_write":                                   145000,
		"sdc_6076fd1100000002_iops_write":                                        145000,
		"sdc_6076fd1100000002_mdm_connection_state":                              0,
		"sdc_6076fd1100000002_num_of_mapped_volumes":                             1,
		"storage_pool_40395b7b00000000_capacity_alert_critical_threshold":        90,
		"storage_pool_40395b7b00000000_capacity_alert_high_threshold":            80,
		"storage_pool_40395b7b00000000_capacity_available_for_volume_allocation": 100663296,
		"storage_pool_40395b7b00000000_capacity_decreased":                       0,
		"storage_pool_40395b7b00000000_capacity_degraded":                        0,
		"storage_pool_40395b7b00000000_capacity_failed":                          0,
		"storage_pool_40395b7b00000000_capacity_in_maintenance":                  0,
		"storage_pool_40395b7b00000000_capacity_in_use":                          50110464,
		"storage_pool_40395b7b00000000_capacity_max_capacity":                    311424000,
		"storage_pool_40395b7b00000000_capacity_protected":                       50110464,
		"storage_pool_40395b7b00000000_capacity_snapshot":                        749568,
		"storage_pool_40395b7b00000000_capacity_spare":                           31141888,
		"storage_pool_40395b7b00000000_capacity_thick_in_use":                    0,
		"storage_pool_40395b7b00000000_capacity_thin_in_use":                     49360896,
		"storage_pool_40395b7b00000000_capacity_unreachable_unused":              0,
		"storage_pool_40395b7b00000000_capacity_unused":                          229422080,
		"storage_pool_40395b7b00000000_capacity_utilization":                     1787,
		"storage_pool_40395b7b00000000_num_of_devices":                           3,
		"storage_pool_40395b7b00000000_num_of_snapshots":                         1,
		"storage_pool_40395b7b00000000_num_of_volumes":                           3,
		"storage_pool_40395b7b00000000_num_of_vtrees":                            2,
		"storage_pool_4039828b00000001_capacity_alert_critical_threshold":        90,
		"storage_pool_4039828b00000001_capacity_alert_high_threshold":            80,
		"storage_pool_4039828b00000001_capacity_available_for_volume_allocation": 142606336,
		"storage_pool_4039828b00000001_capacity_decreased":                       0,
		"storage_pool_4039828b00000001_capacity_degraded":                        0,
		"storage_pool_4039828b00000001_capacity_failed":                          0,
		"storage_pool_4039828b00000001_capacity_in_maintenance":                  0,
		"storage_pool_4039828b00000001_capacity_in_use":                          0,
		"storage_pool_4039828b00000001_capacity_max_capacity":                    332395520,
		"storage_pool_4039828b00000001_capacity_protected":                       0,
		"storage_pool_4039828b00000001_capacity_snapshot":                        0,
		"storage_pool_4039828b00000001_capacity_spare":                           33239040,
		"storage_pool_4039828b00000001_capacity_thick_in_use":                    0,
		"storage_pool_4039828b00000001_capacity_thin_in_use":                     0,
		"storage_pool_4039828b00000001_capacity_unreachable_unused":              0,
		"storage_pool_4039828b00000001_capacity_unused":                          299156480,
		"storage_pool_4039828b00000001_capacity_utilization":                     0,
		"storage_pool_4039828b00000001_num_of_devices":                           3,
		"storage_pool_4039828b00000001_num_of_snapshots":                         0,
		"storage_pool_4039828b00000001_num_of_volumes":                           0,
		"storage_pool_4039828b00000001_num_of_vtrees":                            0,
		"system_backend_primary_bandwidth_read":                                  800,
		"system_backend_primary_bandwidth_read_write":                            238682400,
		"system_backend_primary_bandwidth_write":                                 238681600,
		"system_backend_primary_io_size_read":                                    4000,
		"system_backend_primary_io_size_read_write":                              770971,
		"system_backend_primary_io_size_write":                                   766971,
		"system_backend_primary_iops_read":                                       200,
		"system_backend_primary_iops_read_write":                                 311400,
		"system_backend_primary_iops_write":                                      311200,
		"system_backend_secondary_bandwidth_read":                                0,
		"system_backend_secondary_bandwidth_read_write":                          233926400,
		"system_backend_secondary_bandwidth_write":                               233926400,
		"system_backend_secondary_io_size_read":                                  0,
		"system_backend_secondary_io_size_read_write":                            764465,
		"system_backend_secondary_io_size_write":                                 764465,
		"system_backend_secondary_iops_read":                                     0,
		"system_backend_secondary_iops_read_write":                               306000,
		"system_backend_secondary_iops_write":                                    306000,
		"system_backend_total_bandwidth_read":                                    800,
		"system_backend_total_bandwidth_read_write":                              472608800,
		"system_backend_total_bandwidth_write":                                   472608000,
		"system_backend_total_io_size_read":                                      4000,
		"system_backend_total_io_size_read_write":                                1535437,
		"system_backend_total_io_size_write":                                     1531437,
		"system_backend_total_iops_read":                                         200,
		"system_backend_total_iops_read_write":                                   617400,
		"system_backend_total_iops_write":                                        617200,
		"system_capacity_available_for_volume_allocation":                        243269632,
		"system_capacity_decreased":                                              0,
		"system_capacity_degraded":                                               0,
		"system_capacity_failed":                                                 0,
		"system_capacity_in_maintenance":                                         0,
		"system_capacity_in_use":                                                 50110464,
		"system_capacity_max_capacity":                                           643819520,
		"system_capacity_protected":                                              50110464,
		"system_capacity_snapshot":                                               749568,
		"system_capacity_spare":                                                  64380928,
		"system_capacity_thick_in_use":                                           0,
		"system_capacity_thin_in_use":                                            49360896,
		"system_capacity_unreachable_unused":                                     0,
		"system_capacity_unused":                                                 528578560,
		"system_frontend_user_data_bandwidth_read":                               0,
		"system_frontend_user_data_bandwidth_read_write":                         227170000,
		"system_frontend_user_data_bandwidth_write":                              227170000,
		"system_frontend_user_data_io_size_read":                                 0,
		"system_frontend_user_data_io_size_read_write":                           797087,
		"system_frontend_user_data_io_size_write":                                797087,
		"system_frontend_user_data_iops_read":                                    0,
		"system_frontend_user_data_iops_read_write":                              285000,
		"system_frontend_user_data_iops_write":                                   285000,
		"system_num_of_devices":                                                  6,
		"system_num_of_fault_sets":                                               0,
		"system_num_of_mapped_to_all_volumes":                                    0,
		"system_num_of_mapped_volumes":                                           3,
		"system_num_of_protection_domains":                                       1,
		"system_num_of_rfcache_devices":                                          0,
		"system_num_of_sdc":                                                      3,
		"system_num_of_sds":                                                      3,
		"system_num_of_snapshots":                                                1,
		"system_num_of_storage_pools":                                            2,
		"system_num_of_thick_base_volumes":                                       0,
		"system_num_of_thin_base_volumes":                                        2,
		"system_num_of_unmapped_volumes":                                         0,
		"system_num_of_volumes":                                                  3,
		"system_num_of_vtrees":                                                   2,
		"system_rebalance_bandwidth_read":                                        0,
		"system_rebalance_bandwidth_read_write":                                  0,
		"system_rebalance_bandwidth_write":                                       0,
		"system_rebalance_io_size_read":                                          0,
		"system_rebalance_io_size_read_write":                                    0,
		"system_rebalance_io_size_write":                                         0,
		"system_rebalance_iops_read":                                             0,
		"system_rebalance_iops_read_write":                                       0,
		"system_rebalance_iops_write":                                            0,
		"system_rebalance_pending_capacity_in_Kb":                                0,
		"system_rebalance_time_until_finish":                                     0,
		"system_rebuild_backward_bandwidth_read":                                 0,
		"system_rebuild_backward_bandwidth_read_write":                           0,
		"system_rebuild_backward_bandwidth_write":                                0,
		"system_rebuild_backward_io_size_read":                                   0,
		"system_rebuild_backward_io_size_read_write":                             0,
		"system_rebuild_backward_io_size_write":                                  0,
		"system_rebuild_backward_iops_read":                                      0,
		"system_rebuild_backward_iops_read_write":                                0,
		"system_rebuild_backward_iops_write":                                     0,
		"system_rebuild_backward_pending_capacity_in_Kb":                         0,
		"system_rebuild_forward_bandwidth_read":                                  0,
		"system_rebuild_forward_bandwidth_read_write":                            0,
		"system_rebuild_forward_bandwidth_write":                                 0,
		"system_rebuild_forward_io_size_read":                                    0,
		"system_rebuild_forward_io_size_read_write":                              0,
		"system_rebuild_forward_io_size_write":                                   0,
		"system_rebuild_forward_iops_read":                                       0,
		"system_rebuild_forward_iops_read_write":                                 0,
		"system_rebuild_forward_iops_write":                                      0,
		"system_rebuild_forward_pending_capacity_in_Kb":                          0,
		"system_rebuild_normal_bandwidth_read":                                   0,
		"system_rebuild_normal_bandwidth_read_write":                             0,
		"system_rebuild_normal_bandwidth_write":                                  0,
		"system_rebuild_normal_io_size_read":                                     0,
		"system_rebuild_normal_io_size_read_write":                               0,
		"system_rebuild_normal_io_size_write":                                    0,
		"system_rebuild_normal_iops_read":                                        0,
		"system_rebuild_normal_iops_read_write":                                  0,
		"system_rebuild_normal_iops_write":                                       0,
		"system_rebuild_normal_pending_capacity_in_Kb":                           0,
		"system_rebuild_total_bandwidth_read":                                    0,
		"system_rebuild_total_bandwidth_read_write":                              0,
		"system_rebuild_total_bandwidth_write":                                   0,
		"system_rebuild_total_io_size_read":                                      0,
		"system_rebuild_total_io_size_read_write":                                0,
		"system_rebuild_total_io_size_write":                                     0,
		"system_rebuild_total_iops_read":                                         0,
		"system_rebuild_total_iops_read_write":                                   0,
		"system_rebuild_total_iops_write":                                        0,
		"system_rebuild_total_pending_capacity_in_Kb":                            0,
		"system_total_bandwidth_read":                                            800,
		"system_total_bandwidth_read_write":                                      472608800,
		"system_total_bandwidth_write":                                           472608000,
		"system_total_io_size_read":                                              4000,
		"system_total_io_size_read_write":                                        769729,
		"system_total_io_size_write":                                             765729,
		"system_total_iops_read":                                                 200,
		"system_total_iops_read_write":                                           617400,
		"system_total_iops_write":                                                617200,
	}

	mx := scaleIO.Collect()

	assert.Equal(t, expected, mx)

	testCharts(t, scaleIO, mx)
}

func TestScaleIO_Collect_ConnectionRefused(t *testing.T) {
	srv, _, scaleIO := prepareSrvMockScaleIO(t)
	defer srv.Close()
	require.NoError(t, scaleIO.Init())
	require.NoError(t, scaleIO.Check())
	scaleIO.client.Request.URL = "http://127.0.0.1:38001"

	assert.Nil(t, scaleIO.Collect())
}

func testCharts(t *testing.T, scaleIO *ScaleIO, collected map[string]int64) {
	t.Helper()
	ensureStoragePoolChartsAreCreated(t, scaleIO)
	ensureSdcChartsAreCreated(t, scaleIO)
	module.TestMetricsHasAllChartsDims(t, scaleIO.Charts(), collected)
}

func ensureStoragePoolChartsAreCreated(t *testing.T, scaleIO *ScaleIO) {
	for _, pool := range scaleIO.discovered.pool {
		for _, chart := range *newStoragePoolCharts(pool) {
			assert.Truef(t, scaleIO.Charts().Has(chart.ID), "chart '%s' is not created", chart.ID)
		}
	}
}

func ensureSdcChartsAreCreated(t *testing.T, scaleIO *ScaleIO) {
	for _, sdc := range scaleIO.discovered.sdc {
		for _, chart := range *newSdcCharts(sdc) {
			assert.Truef(t, scaleIO.Charts().Has(chart.ID), "chart '%s' is not created", chart.ID)
		}
	}
}

func prepareSrvMockScaleIO(t *testing.T) (*httptest.Server, *client.MockScaleIOAPIServer, *ScaleIO) {
	t.Helper()
	const (
		user     = "user"
		password = "password"
		version  = "2.5"
		token    = "token"
	)
	var stats client.SelectedStatistics
	err := json.Unmarshal(dataSelectedStatistics, &stats)
	require.NoError(t, err)

	var ins client.Instances
	err = json.Unmarshal(dataInstances, &ins)
	require.NoError(t, err)

	mock := client.MockScaleIOAPIServer{
		User:       user,
		Password:   password,
		Version:    version,
		Token:      token,
		Instances:  ins,
		Statistics: stats,
	}
	srv := httptest.NewServer(&mock)
	require.NoError(t, err)

	scaleIO := New()
	scaleIO.URL = srv.URL
	scaleIO.Username = user
	scaleIO.Password = password
	return srv, &mock, scaleIO
}
