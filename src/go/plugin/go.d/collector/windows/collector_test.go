// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer0200Metrics, _ = os.ReadFile("testdata/v0.20.0/metrics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataVer0200Metrics": dataVer0200Metrics,
	} {
		assert.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Collector)(nil), New())
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success if 'url' is set": {
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9182/metrics"}}},
		},
		"fails on default config": {
			wantFail: true,
			config:   New().Config,
		},
		"fails if 'url' is unset": {
			wantFail: true,
			config:   Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: ""}}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (collr *Collector, cleanup func())
		wantFail bool
	}{
		"success on valid response v0.20.0": {
			prepare: prepareWindowsV0200,
		},
		"fails if endpoint returns invalid data": {
			wantFail: true,
			prepare:  prepareWindowsReturnsInvalidData,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareWindowsConnectionRefused,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  prepareWindowsResponse404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
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

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (collr *Collector, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid response v0.20.0": {
			prepare: prepareWindowsV0200,
			wantCollected: map[string]int64{
				"ad_atq_average_request_latency":                                                                0,
				"ad_atq_outstanding_requests":                                                                   0,
				"ad_binds_total":                                                                                184,
				"ad_database_operations_total_add":                                                              1,
				"ad_database_operations_total_delete":                                                           0,
				"ad_database_operations_total_modify":                                                           30,
				"ad_database_operations_total_recycle":                                                          0,
				"ad_directory_operations_total_read":                                                            726,
				"ad_directory_operations_total_search":                                                          831,
				"ad_directory_operations_total_write":                                                           31,
				"ad_directory_service_threads":                                                                  0,
				"ad_ldap_last_bind_time_seconds":                                                                0,
				"ad_ldap_searches_total":                                                                        1382,
				"ad_name_cache_hits_total":                                                                      41161,
				"ad_name_cache_lookups_total":                                                                   53046,
				"ad_replication_data_intersite_bytes_total_inbound":                                             0,
				"ad_replication_data_intersite_bytes_total_outbound":                                            0,
				"ad_replication_data_intrasite_bytes_total_inbound":                                             0,
				"ad_replication_data_intrasite_bytes_total_outbound":                                            0,
				"ad_replication_inbound_objects_filtered_total":                                                 0,
				"ad_replication_inbound_properties_filtered_total":                                              0,
				"ad_replication_inbound_properties_updated_total":                                               0,
				"ad_replication_inbound_sync_objects_remaining":                                                 0,
				"ad_replication_pending_synchronizations":                                                       0,
				"ad_replication_sync_requests_total":                                                            0,
				"adcs_cert_template_Administrator_challenge_response_processing_time_seconds":                   0,
				"adcs_cert_template_Administrator_challenge_responses_total":                                    0,
				"adcs_cert_template_Administrator_failed_requests_total":                                        0,
				"adcs_cert_template_Administrator_issued_requests_total":                                        0,
				"adcs_cert_template_Administrator_pending_requests_total":                                       0,
				"adcs_cert_template_Administrator_request_cryptographic_signing_time_seconds":                   0,
				"adcs_cert_template_Administrator_request_policy_module_processing_time_seconds":                0,
				"adcs_cert_template_Administrator_request_processing_time_seconds":                              0,
				"adcs_cert_template_Administrator_requests_total":                                               0,
				"adcs_cert_template_Administrator_retrievals_processing_time_seconds":                           0,
				"adcs_cert_template_Administrator_retrievals_total":                                             0,
				"adcs_cert_template_Administrator_signed_certificate_timestamp_list_processing_time_seconds":    0,
				"adcs_cert_template_Administrator_signed_certificate_timestamp_lists_total":                     0,
				"adcs_cert_template_DomainController_challenge_response_processing_time_seconds":                0,
				"adcs_cert_template_DomainController_challenge_responses_total":                                 0,
				"adcs_cert_template_DomainController_failed_requests_total":                                     0,
				"adcs_cert_template_DomainController_issued_requests_total":                                     1,
				"adcs_cert_template_DomainController_pending_requests_total":                                    0,
				"adcs_cert_template_DomainController_request_cryptographic_signing_time_seconds":                0,
				"adcs_cert_template_DomainController_request_policy_module_processing_time_seconds":             16,
				"adcs_cert_template_DomainController_request_processing_time_seconds":                           63,
				"adcs_cert_template_DomainController_requests_total":                                            1,
				"adcs_cert_template_DomainController_retrievals_processing_time_seconds":                        0,
				"adcs_cert_template_DomainController_retrievals_total":                                          0,
				"adcs_cert_template_DomainController_signed_certificate_timestamp_list_processing_time_seconds": 0,
				"adcs_cert_template_DomainController_signed_certificate_timestamp_lists_total":                  0,
				"adfs_ad_login_connection_failures_total":                                                       0,
				"adfs_certificate_authentications_total":                                                        0,
				"adfs_db_artifact_failure_total":                                                                0,
				"adfs_db_artifact_query_time_seconds_total":                                                     0,
				"adfs_db_config_failure_total":                                                                  0,
				"adfs_db_config_query_time_seconds_total":                                                       101,
				"adfs_device_authentications_total":                                                             0,
				"adfs_external_authentications_failure_total":                                                   0,
				"adfs_external_authentications_success_total":                                                   0,
				"adfs_extranet_account_lockouts_total":                                                          0,
				"adfs_federated_authentications_total":                                                          0,
				"adfs_federation_metadata_requests_total":                                                       1,
				"adfs_oauth_authorization_requests_total":                                                       0,
				"adfs_oauth_client_authentication_failure_total":                                                0,
				"adfs_oauth_client_authentication_success_total":                                                0,
				"adfs_oauth_client_credentials_failure_total":                                                   0,
				"adfs_oauth_client_credentials_success_total":                                                   0,
				"adfs_oauth_client_privkey_jtw_authentication_failure_total":                                    0,
				"adfs_oauth_client_privkey_jwt_authentications_success_total":                                   0,
				"adfs_oauth_client_secret_basic_authentications_failure_total":                                  0,
				"adfs_oauth_client_secret_basic_authentications_success_total":                                  0,
				"adfs_oauth_client_secret_post_authentications_failure_total":                                   0,
				"adfs_oauth_client_secret_post_authentications_success_total":                                   0,
				"adfs_oauth_client_windows_authentications_failure_total":                                       0,
				"adfs_oauth_client_windows_authentications_success_total":                                       0,
				"adfs_oauth_logon_certificate_requests_failure_total":                                           0,
				"adfs_oauth_logon_certificate_token_requests_success_total":                                     0,
				"adfs_oauth_password_grant_requests_failure_total":                                              0,
				"adfs_oauth_password_grant_requests_success_total":                                              0,
				"adfs_oauth_token_requests_success_total":                                                       0,
				"adfs_passive_requests_total":                                                                   0,
				"adfs_passport_authentications_total":                                                           0,
				"adfs_password_change_failed_total":                                                             0,
				"adfs_password_change_succeeded_total":                                                          0,
				"adfs_samlp_token_requests_success_total":                                                       0,
				"adfs_sso_authentications_failure_total":                                                        0,
				"adfs_sso_authentications_success_total":                                                        0,
				"adfs_token_requests_total":                                                                     0,
				"adfs_userpassword_authentications_failure_total":                                               0,
				"adfs_userpassword_authentications_success_total":                                               0,
				"adfs_windows_integrated_authentications_total":                                                 0,
				"adfs_wsfed_token_requests_success_total":                                                       0,
				"adfs_wstrust_token_requests_success_total":                                                     0,
				"collector_ad_duration":                                                                         769,
				"collector_ad_status_fail":                                                                      0,
				"collector_ad_status_success":                                                                   1,
				"collector_adcs_duration":                                                                       0,
				"collector_adcs_status_fail":                                                                    0,
				"collector_adcs_status_success":                                                                 1,
				"collector_adfs_duration":                                                                       3,
				"collector_adfs_status_fail":                                                                    0,
				"collector_adfs_status_success":                                                                 1,
				"collector_cpu_duration":                                                                        0,
				"collector_cpu_status_fail":                                                                     0,
				"collector_cpu_status_success":                                                                  1,
				"collector_exchange_duration":                                                                   33,
				"collector_exchange_status_fail":                                                                0,
				"collector_exchange_status_success":                                                             1,
				"collector_hyperv_duration":                                                                     900,
				"collector_hyperv_status_fail":                                                                  0,
				"collector_hyperv_status_success":                                                               1,
				"collector_iis_duration":                                                                        0,
				"collector_iis_status_fail":                                                                     0,
				"collector_iis_status_success":                                                                  1,
				"collector_logical_disk_duration":                                                               0,
				"collector_logical_disk_status_fail":                                                            0,
				"collector_logical_disk_status_success":                                                         1,
				"collector_logon_duration":                                                                      113,
				"collector_logon_status_fail":                                                                   0,
				"collector_logon_status_success":                                                                1,
				"collector_memory_duration":                                                                     0,
				"collector_memory_status_fail":                                                                  0,
				"collector_memory_status_success":                                                               1,
				"collector_mssql_duration":                                                                      3,
				"collector_mssql_status_fail":                                                                   0,
				"collector_mssql_status_success":                                                                1,
				"collector_net_duration":                                                                        0,
				"collector_net_status_fail":                                                                     0,
				"collector_net_status_success":                                                                  1,
				"collector_netframework_clrexceptions_duration":                                                 1437,
				"collector_netframework_clrexceptions_status_fail":                                              0,
				"collector_netframework_clrexceptions_status_success":                                           1,
				"collector_netframework_clrinterop_duration":                                                    1491,
				"collector_netframework_clrinterop_status_fail":                                                 0,
				"collector_netframework_clrinterop_status_success":                                              1,
				"collector_netframework_clrjit_duration":                                                        1278,
				"collector_netframework_clrjit_status_fail":                                                     0,
				"collector_netframework_clrjit_status_success":                                                  1,
				"collector_netframework_clrloading_duration":                                                    1323,
				"collector_netframework_clrloading_status_fail":                                                 0,
				"collector_netframework_clrloading_status_success":                                              1,
				"collector_netframework_clrlocksandthreads_duration":                                            1357,
				"collector_netframework_clrlocksandthreads_status_fail":                                         0,
				"collector_netframework_clrlocksandthreads_status_success":                                      1,
				"collector_netframework_clrmemory_duration":                                                     1406,
				"collector_netframework_clrmemory_status_fail":                                                  0,
				"collector_netframework_clrmemory_status_success":                                               1,
				"collector_netframework_clrremoting_duration":                                                   1519,
				"collector_netframework_clrremoting_status_fail":                                                0,
				"collector_netframework_clrremoting_status_success":                                             1,
				"collector_netframework_clrsecurity_duration":                                                   1467,
				"collector_netframework_clrsecurity_status_fail":                                                0,
				"collector_netframework_clrsecurity_status_success":                                             1,
				"collector_os_duration":                                                                         2,
				"collector_os_status_fail":                                                                      0,
				"collector_os_status_success":                                                                   1,
				"collector_process_duration":                                                                    115,
				"collector_process_status_fail":                                                                 0,
				"collector_process_status_success":                                                              1,
				"collector_service_duration":                                                                    101,
				"collector_service_status_fail":                                                                 0,
				"collector_service_status_success":                                                              1,
				"collector_system_duration":                                                                     0,
				"collector_system_status_fail":                                                                  0,
				"collector_system_status_success":                                                               1,
				"collector_tcp_duration":                                                                        0,
				"collector_tcp_status_fail":                                                                     0,
				"collector_tcp_status_success":                                                                  1,
				"cpu_core_0,0_cstate_c1":                                                                        160233427,
				"cpu_core_0,0_cstate_c2":                                                                        0,
				"cpu_core_0,0_cstate_c3":                                                                        0,
				"cpu_core_0,0_dpc_time":                                                                         67109,
				"cpu_core_0,0_dpcs":                                                                             4871900,
				"cpu_core_0,0_idle_time":                                                                        162455593,
				"cpu_core_0,0_interrupt_time":                                                                   77281,
				"cpu_core_0,0_interrupts":                                                                       155194331,
				"cpu_core_0,0_privileged_time":                                                                  1182109,
				"cpu_core_0,0_user_time":                                                                        1073671,
				"cpu_core_0,1_cstate_c1":                                                                        159528054,
				"cpu_core_0,1_cstate_c2":                                                                        0,
				"cpu_core_0,1_cstate_c3":                                                                        0,
				"cpu_core_0,1_dpc_time":                                                                         11093,
				"cpu_core_0,1_dpcs":                                                                             1650552,
				"cpu_core_0,1_idle_time":                                                                        159478125,
				"cpu_core_0,1_interrupt_time":                                                                   58093,
				"cpu_core_0,1_interrupts":                                                                       79325847,
				"cpu_core_0,1_privileged_time":                                                                  1801234,
				"cpu_core_0,1_user_time":                                                                        3432000,
				"cpu_core_0,2_cstate_c1":                                                                        159891723,
				"cpu_core_0,2_cstate_c2":                                                                        0,
				"cpu_core_0,2_cstate_c3":                                                                        0,
				"cpu_core_0,2_dpc_time":                                                                         16062,
				"cpu_core_0,2_dpcs":                                                                             2236469,
				"cpu_core_0,2_idle_time":                                                                        159848437,
				"cpu_core_0,2_interrupt_time":                                                                   53515,
				"cpu_core_0,2_interrupts":                                                                       67305419,
				"cpu_core_0,2_privileged_time":                                                                  1812546,
				"cpu_core_0,2_user_time":                                                                        3050250,
				"cpu_core_0,3_cstate_c1":                                                                        159544117,
				"cpu_core_0,3_cstate_c2":                                                                        0,
				"cpu_core_0,3_cstate_c3":                                                                        0,
				"cpu_core_0,3_dpc_time":                                                                         8140,
				"cpu_core_0,3_dpcs":                                                                             1185046,
				"cpu_core_0,3_idle_time":                                                                        159527546,
				"cpu_core_0,3_interrupt_time":                                                                   44484,
				"cpu_core_0,3_interrupts":                                                                       60766938,
				"cpu_core_0,3_privileged_time":                                                                  1760828,
				"cpu_core_0,3_user_time":                                                                        3422875,
				"cpu_dpc_time":                                                                                  102404,
				"cpu_idle_time":                                                                                 641309701,
				"cpu_interrupt_time":                                                                            233373,
				"cpu_privileged_time":                                                                           6556717,
				"cpu_user_time":                                                                                 10978796,
				"exchange_activesync_ping_cmds_pending":                                                         0,
				"exchange_activesync_requests_total":                                                            14,
				"exchange_activesync_sync_cmds_total":                                                           0,
				"exchange_autodiscover_requests_total":                                                          1,
				"exchange_avail_service_requests_per_sec":                                                       0,
				"exchange_http_proxy_autodiscover_avg_auth_latency":                                             1,
				"exchange_http_proxy_autodiscover_avg_cas_proccessing_latency_sec":                              3,
				"exchange_http_proxy_autodiscover_mailbox_proxy_failure_rate":                                   0,
				"exchange_http_proxy_autodiscover_mailbox_server_locator_avg_latency_sec":                       8,
				"exchange_http_proxy_autodiscover_outstanding_proxy_requests":                                   0,
				"exchange_http_proxy_autodiscover_requests_total":                                               27122,
				"exchange_http_proxy_eas_avg_auth_latency":                                                      0,
				"exchange_http_proxy_eas_avg_cas_proccessing_latency_sec":                                       3,
				"exchange_http_proxy_eas_mailbox_proxy_failure_rate":                                            0,
				"exchange_http_proxy_eas_mailbox_server_locator_avg_latency_sec":                                8,
				"exchange_http_proxy_eas_outstanding_proxy_requests":                                            0,
				"exchange_http_proxy_eas_requests_total":                                                        32519,
				"exchange_ldap_complianceauditservice_10_long_running_ops_per_sec":                              0,
				"exchange_ldap_complianceauditservice_10_read_time_sec":                                         18,
				"exchange_ldap_complianceauditservice_10_search_time_sec":                                       58,
				"exchange_ldap_complianceauditservice_10_timeout_errors_total":                                  0,
				"exchange_ldap_complianceauditservice_10_write_time_sec":                                        0,
				"exchange_ldap_complianceauditservice_long_running_ops_per_sec":                                 0,
				"exchange_ldap_complianceauditservice_read_time_sec":                                            8,
				"exchange_ldap_complianceauditservice_search_time_sec":                                          46,
				"exchange_ldap_complianceauditservice_timeout_errors_total":                                     0,
				"exchange_ldap_complianceauditservice_write_time_sec":                                           0,
				"exchange_owa_current_unique_users":                                                             0,
				"exchange_owa_requests_total":                                                                   0,
				"exchange_rpc_active_user_count":                                                                0,
				"exchange_rpc_avg_latency_sec":                                                                  1,
				"exchange_rpc_connection_count":                                                                 0,
				"exchange_rpc_operations_total":                                                                 9,
				"exchange_rpc_requests":                                                                         0,
				"exchange_rpc_user_count":                                                                       0,
				"exchange_transport_queues_active_mailbox_delivery_high_priority":                               0,
				"exchange_transport_queues_active_mailbox_delivery_low_priority":                                0,
				"exchange_transport_queues_active_mailbox_delivery_none_priority":                               0,
				"exchange_transport_queues_active_mailbox_delivery_normal_priority":                             0,
				"exchange_transport_queues_external_active_remote_delivery_high_priority":                       0,
				"exchange_transport_queues_external_active_remote_delivery_low_priority":                        0,
				"exchange_transport_queues_external_active_remote_delivery_none_priority":                       0,
				"exchange_transport_queues_external_active_remote_delivery_normal_priority":                     0,
				"exchange_transport_queues_external_largest_delivery_high_priority":                             0,
				"exchange_transport_queues_external_largest_delivery_low_priority":                              0,
				"exchange_transport_queues_external_largest_delivery_none_priority":                             0,
				"exchange_transport_queues_external_largest_delivery_normal_priority":                           0,
				"exchange_transport_queues_internal_active_remote_delivery_high_priority":                       0,
				"exchange_transport_queues_internal_active_remote_delivery_low_priority":                        0,
				"exchange_transport_queues_internal_active_remote_delivery_none_priority":                       0,
				"exchange_transport_queues_internal_active_remote_delivery_normal_priority":                     0,
				"exchange_transport_queues_internal_largest_delivery_high_priority":                             0,
				"exchange_transport_queues_internal_largest_delivery_low_priority":                              0,
				"exchange_transport_queues_internal_largest_delivery_none_priority":                             0,
				"exchange_transport_queues_internal_largest_delivery_normal_priority":                           0,
				"exchange_transport_queues_poison_high_priority":                                                0,
				"exchange_transport_queues_poison_low_priority":                                                 0,
				"exchange_transport_queues_poison_none_priority":                                                0,
				"exchange_transport_queues_poison_normal_priority":                                              0,
				"exchange_transport_queues_retry_mailbox_delivery_high_priority":                                0,
				"exchange_transport_queues_retry_mailbox_delivery_low_priority":                                 0,
				"exchange_transport_queues_retry_mailbox_delivery_none_priority":                                0,
				"exchange_transport_queues_retry_mailbox_delivery_normal_priority":                              0,
				"exchange_transport_queues_unreachable_high_priority":                                           0,
				"exchange_transport_queues_unreachable_low_priority":                                            0,
				"exchange_transport_queues_unreachable_none_priority":                                           0,
				"exchange_transport_queues_unreachable_normal_priority":                                         0,
				"exchange_workload_complianceauditservice_auditcomplianceserviceprioritized_audit_task_execution_manager_active_tasks":    0,
				"exchange_workload_complianceauditservice_auditcomplianceserviceprioritized_audit_task_execution_manager_completed_tasks": 0,
				"exchange_workload_complianceauditservice_auditcomplianceserviceprioritized_audit_task_execution_manager_is_active":       1,
				"exchange_workload_complianceauditservice_auditcomplianceserviceprioritized_audit_task_execution_manager_is_paused":       0,
				"exchange_workload_complianceauditservice_auditcomplianceserviceprioritized_audit_task_execution_manager_queued_tasks":    0,
				"exchange_workload_complianceauditservice_auditcomplianceserviceprioritized_audit_task_execution_manager_yielded_tasks":   0,
				"exchange_workload_microsoft_exchange_servicehost_darruntime_active_tasks":                                                0,
				"exchange_workload_microsoft_exchange_servicehost_darruntime_completed_tasks":                                             0,
				"exchange_workload_microsoft_exchange_servicehost_darruntime_is_active":                                                   1,
				"exchange_workload_microsoft_exchange_servicehost_darruntime_is_paused":                                                   0,
				"exchange_workload_microsoft_exchange_servicehost_darruntime_queued_tasks":                                                0,
				"exchange_workload_microsoft_exchange_servicehost_darruntime_yielded_tasks":                                               0,
				"hyperv_health_critical":                                 0,
				"hyperv_health_ok":                                       1,
				"hyperv_root_partition_1G_device_pages":                  0,
				"hyperv_root_partition_1G_gpa_pages":                     6,
				"hyperv_root_partition_2M_device_pages":                  0,
				"hyperv_root_partition_2M_gpa_pages":                     5255,
				"hyperv_root_partition_4K_device_pages":                  0,
				"hyperv_root_partition_4K_gpa_pages":                     58880,
				"hyperv_root_partition_address_spaces":                   0,
				"hyperv_root_partition_attached_devices":                 1,
				"hyperv_root_partition_deposited_pages":                  31732,
				"hyperv_root_partition_device_dma_errors":                0,
				"hyperv_root_partition_device_interrupt_errors":          0,
				"hyperv_root_partition_device_interrupt_throttle_events": 0,
				"hyperv_root_partition_gpa_space_modifications":          0,
				"hyperv_root_partition_io_tlb_flush":                     23901,
				"hyperv_root_partition_physical_pages_allocated":         0,
				"hyperv_root_partition_virtual_tlb_flush_entires":        15234,
				"hyperv_root_partition_virtual_tlb_pages":                64,
				"hyperv_vid_ubuntu_22_04_lts_physical_pages_allocated":   745472,
				"hyperv_vid_ubuntu_22_04_lts_remote_physical_pages":      0,
				"hyperv_vm_device_--_-d_-ana-vm-hyperv-virtual_machines-3aa8d474-2365-4041-a7cb-2a78287d6fe0_vmgs_bytes_read":                                                83456,
				"hyperv_vm_device_--_-d_-ana-vm-hyperv-virtual_machines-3aa8d474-2365-4041-a7cb-2a78287d6fe0_vmgs_bytes_written":                                             1148928,
				"hyperv_vm_device_--_-d_-ana-vm-hyperv-virtual_machines-3aa8d474-2365-4041-a7cb-2a78287d6fe0_vmgs_error_count":                                               0,
				"hyperv_vm_device_--_-d_-ana-vm-hyperv-virtual_machines-3aa8d474-2365-4041-a7cb-2a78287d6fe0_vmgs_operations_read":                                           6,
				"hyperv_vm_device_--_-d_-ana-vm-hyperv-virtual_machines-3aa8d474-2365-4041-a7cb-2a78287d6fe0_vmgs_operations_written":                                        34,
				"hyperv_vm_device_d_-ana-vm-hyperv-vhd-ubuntu_22_04_lts_838d93a1-7d30-43cd-9f69-f336829c0934_avhdx_bytes_read":                                               531184640,
				"hyperv_vm_device_d_-ana-vm-hyperv-vhd-ubuntu_22_04_lts_838d93a1-7d30-43cd-9f69-f336829c0934_avhdx_bytes_written":                                            425905152,
				"hyperv_vm_device_d_-ana-vm-hyperv-vhd-ubuntu_22_04_lts_838d93a1-7d30-43cd-9f69-f336829c0934_avhdx_error_count":                                              3,
				"hyperv_vm_device_d_-ana-vm-hyperv-vhd-ubuntu_22_04_lts_838d93a1-7d30-43cd-9f69-f336829c0934_avhdx_operations_read":                                          13196,
				"hyperv_vm_device_d_-ana-vm-hyperv-vhd-ubuntu_22_04_lts_838d93a1-7d30-43cd-9f69-f336829c0934_avhdx_operations_written":                                       3866,
				"hyperv_vm_interface_default_switch_312ff9c7-1f07-4eba-81fe-f5b4f445b810_bytes_received":                                                                     473654,
				"hyperv_vm_interface_default_switch_312ff9c7-1f07-4eba-81fe-f5b4f445b810_bytes_sent":                                                                         43550457,
				"hyperv_vm_interface_default_switch_312ff9c7-1f07-4eba-81fe-f5b4f445b810_packets_incoming_dropped":                                                           0,
				"hyperv_vm_interface_default_switch_312ff9c7-1f07-4eba-81fe-f5b4f445b810_packets_outgoing_dropped":                                                           284,
				"hyperv_vm_interface_default_switch_312ff9c7-1f07-4eba-81fe-f5b4f445b810_packets_received":                                                                   6137,
				"hyperv_vm_interface_default_switch_312ff9c7-1f07-4eba-81fe-f5b4f445b810_packets_sent":                                                                       8905,
				"hyperv_vm_interface_ubuntu_22_04_lts_adaptador_de_rede_3aa8d474-2365-4041-a7cb-2a78287d6fe0--98f1dbee-505c-4086-b80e-87a27faecbd4_bytes_received":           43509444,
				"hyperv_vm_interface_ubuntu_22_04_lts_adaptador_de_rede_3aa8d474-2365-4041-a7cb-2a78287d6fe0--98f1dbee-505c-4086-b80e-87a27faecbd4_bytes_sent":               473654,
				"hyperv_vm_interface_ubuntu_22_04_lts_adaptador_de_rede_3aa8d474-2365-4041-a7cb-2a78287d6fe0--98f1dbee-505c-4086-b80e-87a27faecbd4_packets_incoming_dropped": 0,
				"hyperv_vm_interface_ubuntu_22_04_lts_adaptador_de_rede_3aa8d474-2365-4041-a7cb-2a78287d6fe0--98f1dbee-505c-4086-b80e-87a27faecbd4_packets_outgoing_dropped": 0,
				"hyperv_vm_interface_ubuntu_22_04_lts_adaptador_de_rede_3aa8d474-2365-4041-a7cb-2a78287d6fe0--98f1dbee-505c-4086-b80e-87a27faecbd4_packets_received":         8621,
				"hyperv_vm_interface_ubuntu_22_04_lts_adaptador_de_rede_3aa8d474-2365-4041-a7cb-2a78287d6fe0--98f1dbee-505c-4086-b80e-87a27faecbd4_packets_sent":             6137,
				"hyperv_vm_ubuntu_22_04_lts_cpu_guest_run_time":                                                                                                              62534217,
				"hyperv_vm_ubuntu_22_04_lts_cpu_hypervisor_run_time":                                                                                                         4457712,
				"hyperv_vm_ubuntu_22_04_lts_cpu_remote_run_time":                                                                                                             0,
				"hyperv_vm_ubuntu_22_04_lts_memory_physical":                                                                                                                 2628,
				"hyperv_vm_ubuntu_22_04_lts_memory_physical_guest_visible":                                                                                                   2904,
				"hyperv_vm_ubuntu_22_04_lts_memory_pressure_current":                                                                                                         83,
				"hyperv_vswitch_default_switch_broadcast_packets_received_total":                                                                                             51,
				"hyperv_vswitch_default_switch_broadcast_packets_sent_total":                                                                                                 18,
				"hyperv_vswitch_default_switch_bytes_received_total":                                                                                                         44024111,
				"hyperv_vswitch_default_switch_bytes_sent_total":                                                                                                             43983098,
				"hyperv_vswitch_default_switch_directed_packets_received_total":                                                                                              14603,
				"hyperv_vswitch_default_switch_directed_packets_send_total":                                                                                                  14603,
				"hyperv_vswitch_default_switch_dropped_packets_incoming_total":                                                                                               284,
				"hyperv_vswitch_default_switch_dropped_packets_outcoming_total":                                                                                              0,
				"hyperv_vswitch_default_switch_extensions_dropped_packets_incoming_total":                                                                                    0,
				"hyperv_vswitch_default_switch_extensions_dropped_packets_outcoming_total":                                                                                   0,
				"hyperv_vswitch_default_switch_learned_mac_addresses_total":                                                                                                  2,
				"hyperv_vswitch_default_switch_multicast_packets_received_total":                                                                                             388,
				"hyperv_vswitch_default_switch_multicast_packets_sent_total":                                                                                                 137,
				"hyperv_vswitch_default_switch_number_of_send_channel_moves_total":                                                                                           0,
				"hyperv_vswitch_default_switch_number_of_vmq_moves_total":                                                                                                    0,
				"hyperv_vswitch_default_switch_packets_flooded_total":                                                                                                        0,
				"hyperv_vswitch_default_switch_packets_received_total":                                                                                                       15042,
				"hyperv_vswitch_default_switch_purged_mac_addresses_total":                                                                                                   0,
				"iis_website_Default_Web_Site_connection_attempts_all_instances_total":                                                                                       1,
				"iis_website_Default_Web_Site_current_anonymous_users":                                                                                                       0,
				"iis_website_Default_Web_Site_current_connections":                                                                                                           0,
				"iis_website_Default_Web_Site_current_isapi_extension_requests":                                                                                              0,
				"iis_website_Default_Web_Site_current_non_anonymous_users":                                                                                                   0,
				"iis_website_Default_Web_Site_files_received_total":                                                                                                          0,
				"iis_website_Default_Web_Site_files_sent_total":                                                                                                              2,
				"iis_website_Default_Web_Site_isapi_extension_requests_total":                                                                                                0,
				"iis_website_Default_Web_Site_locked_errors_total":                                                                                                           0,
				"iis_website_Default_Web_Site_logon_attempts_total":                                                                                                          4,
				"iis_website_Default_Web_Site_not_found_errors_total":                                                                                                        1,
				"iis_website_Default_Web_Site_received_bytes_total":                                                                                                          10289,
				"iis_website_Default_Web_Site_requests_total":                                                                                                                3,
				"iis_website_Default_Web_Site_sent_bytes_total":                                                                                                              105882,
				"iis_website_Default_Web_Site_service_uptime":                                                                                                                258633,
				"logical_disk_C:_free_space":                                                 43636490240,
				"logical_disk_C:_read_bytes_total":                                           17676328448,
				"logical_disk_C:_read_latency":                                               97420,
				"logical_disk_C:_reads_total":                                                350593,
				"logical_disk_C:_total_space":                                                67938287616,
				"logical_disk_C:_used_space":                                                 24301797376,
				"logical_disk_C:_write_bytes_total":                                          9135282688,
				"logical_disk_C:_write_latency":                                              123912,
				"logical_disk_C:_writes_total":                                               450705,
				"logon_type_batch_sessions":                                                  0,
				"logon_type_cached_interactive_sessions":                                     0,
				"logon_type_cached_remote_interactive_sessions":                              0,
				"logon_type_cached_unlock_sessions":                                          0,
				"logon_type_interactive_sessions":                                            2,
				"logon_type_network_clear_text_sessions":                                     0,
				"logon_type_network_sessions":                                                0,
				"logon_type_new_credentials_sessions":                                        0,
				"logon_type_proxy_sessions":                                                  0,
				"logon_type_remote_interactive_sessions":                                     0,
				"logon_type_service_sessions":                                                0,
				"logon_type_system_sessions":                                                 0,
				"logon_type_unlock_sessions":                                                 0,
				"memory_available_bytes":                                                     1379942400,
				"memory_cache_faults_total":                                                  8009603,
				"memory_cache_total":                                                         1392185344,
				"memory_commit_limit":                                                        5733113856,
				"memory_committed_bytes":                                                     3447439360,
				"memory_modified_page_list_bytes":                                            32653312,
				"memory_not_committed_bytes":                                                 2285674496,
				"memory_page_faults_total":                                                   119093924,
				"memory_pool_nonpaged_bytes_total":                                           126865408,
				"memory_pool_paged_bytes":                                                    303906816,
				"memory_standby_cache_core_bytes":                                            107376640,
				"memory_standby_cache_normal_priority_bytes":                                 1019121664,
				"memory_standby_cache_reserve_bytes":                                         233033728,
				"memory_standby_cache_total":                                                 1359532032,
				"memory_swap_page_reads_total":                                               402087,
				"memory_swap_page_writes_total":                                              7012,
				"memory_swap_pages_read_total":                                               4643279,
				"memory_swap_pages_written_total":                                            312896,
				"memory_used_bytes":                                                          2876776448,
				"mssql_db_master_instance_SQLEXPRESS_active_transactions":                    0,
				"mssql_db_master_instance_SQLEXPRESS_backup_restore_operations":              0,
				"mssql_db_master_instance_SQLEXPRESS_data_files_size_bytes":                  4653056,
				"mssql_db_master_instance_SQLEXPRESS_log_flushed_bytes":                      3702784,
				"mssql_db_master_instance_SQLEXPRESS_log_flushes":                            252,
				"mssql_db_master_instance_SQLEXPRESS_transactions":                           2183,
				"mssql_db_master_instance_SQLEXPRESS_write_transactions":                     236,
				"mssql_db_model_instance_SQLEXPRESS_active_transactions":                     0,
				"mssql_db_model_instance_SQLEXPRESS_backup_restore_operations":               0,
				"mssql_db_model_instance_SQLEXPRESS_data_files_size_bytes":                   8388608,
				"mssql_db_model_instance_SQLEXPRESS_log_flushed_bytes":                       12288,
				"mssql_db_model_instance_SQLEXPRESS_log_flushes":                             3,
				"mssql_db_model_instance_SQLEXPRESS_transactions":                            4467,
				"mssql_db_model_instance_SQLEXPRESS_write_transactions":                      0,
				"mssql_db_msdb_instance_SQLEXPRESS_active_transactions":                      0,
				"mssql_db_msdb_instance_SQLEXPRESS_backup_restore_operations":                0,
				"mssql_db_msdb_instance_SQLEXPRESS_data_files_size_bytes":                    15466496,
				"mssql_db_msdb_instance_SQLEXPRESS_log_flushed_bytes":                        0,
				"mssql_db_msdb_instance_SQLEXPRESS_log_flushes":                              0,
				"mssql_db_msdb_instance_SQLEXPRESS_transactions":                             4582,
				"mssql_db_msdb_instance_SQLEXPRESS_write_transactions":                       0,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_active_transactions":       0,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_backup_restore_operations": 0,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_data_files_size_bytes":     41943040,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_log_flushed_bytes":         0,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_log_flushes":               0,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_transactions":              2,
				"mssql_db_mssqlsystemresource_instance_SQLEXPRESS_write_transactions":        0,
				"mssql_db_tempdb_instance_SQLEXPRESS_active_transactions":                    0,
				"mssql_db_tempdb_instance_SQLEXPRESS_backup_restore_operations":              0,
				"mssql_db_tempdb_instance_SQLEXPRESS_data_files_size_bytes":                  8388608,
				"mssql_db_tempdb_instance_SQLEXPRESS_log_flushed_bytes":                      118784,
				"mssql_db_tempdb_instance_SQLEXPRESS_log_flushes":                            2,
				"mssql_db_tempdb_instance_SQLEXPRESS_transactions":                           1558,
				"mssql_db_tempdb_instance_SQLEXPRESS_write_transactions":                     29,
				"mssql_instance_SQLEXPRESS_accessmethods_page_splits":                        429,
				"mssql_instance_SQLEXPRESS_bufman_buffer_cache_hits":                         86,
				"mssql_instance_SQLEXPRESS_bufman_checkpoint_pages":                          82,
				"mssql_instance_SQLEXPRESS_bufman_page_life_expectancy_seconds":              191350,
				"mssql_instance_SQLEXPRESS_bufman_page_reads":                                797,
				"mssql_instance_SQLEXPRESS_bufman_page_writes":                               92,
				"mssql_instance_SQLEXPRESS_cache_hit_ratio":                                  100,
				"mssql_instance_SQLEXPRESS_genstats_blocked_processes":                       0,
				"mssql_instance_SQLEXPRESS_genstats_user_connections":                        1,
				"mssql_instance_SQLEXPRESS_memmgr_connection_memory_bytes":                   1015808,
				"mssql_instance_SQLEXPRESS_memmgr_external_benefit_of_memory":                0,
				"mssql_instance_SQLEXPRESS_memmgr_pending_memory_grants":                     0,
				"mssql_instance_SQLEXPRESS_memmgr_total_server_memory_bytes":                 198836224,
				"mssql_instance_SQLEXPRESS_resource_AllocUnit_locks_deadlocks":               0,
				"mssql_instance_SQLEXPRESS_resource_AllocUnit_locks_lock_wait_seconds":       0,
				"mssql_instance_SQLEXPRESS_resource_Application_locks_deadlocks":             0,
				"mssql_instance_SQLEXPRESS_resource_Application_locks_lock_wait_seconds":     0,
				"mssql_instance_SQLEXPRESS_resource_Database_locks_deadlocks":                0,
				"mssql_instance_SQLEXPRESS_resource_Database_locks_lock_wait_seconds":        0,
				"mssql_instance_SQLEXPRESS_resource_Extent_locks_deadlocks":                  0,
				"mssql_instance_SQLEXPRESS_resource_Extent_locks_lock_wait_seconds":          0,
				"mssql_instance_SQLEXPRESS_resource_File_locks_deadlocks":                    0,
				"mssql_instance_SQLEXPRESS_resource_File_locks_lock_wait_seconds":            0,
				"mssql_instance_SQLEXPRESS_resource_HoBT_locks_deadlocks":                    0,
				"mssql_instance_SQLEXPRESS_resource_HoBT_locks_lock_wait_seconds":            0,
				"mssql_instance_SQLEXPRESS_resource_Key_locks_deadlocks":                     0,
				"mssql_instance_SQLEXPRESS_resource_Key_locks_lock_wait_seconds":             0,
				"mssql_instance_SQLEXPRESS_resource_Metadata_locks_deadlocks":                0,
				"mssql_instance_SQLEXPRESS_resource_Metadata_locks_lock_wait_seconds":        0,
				"mssql_instance_SQLEXPRESS_resource_OIB_locks_deadlocks":                     0,
				"mssql_instance_SQLEXPRESS_resource_OIB_locks_lock_wait_seconds":             0,
				"mssql_instance_SQLEXPRESS_resource_Object_locks_deadlocks":                  0,
				"mssql_instance_SQLEXPRESS_resource_Object_locks_lock_wait_seconds":          0,
				"mssql_instance_SQLEXPRESS_resource_Page_locks_deadlocks":                    0,
				"mssql_instance_SQLEXPRESS_resource_Page_locks_lock_wait_seconds":            0,
				"mssql_instance_SQLEXPRESS_resource_RID_locks_deadlocks":                     0,
				"mssql_instance_SQLEXPRESS_resource_RID_locks_lock_wait_seconds":             0,
				"mssql_instance_SQLEXPRESS_resource_RowGroup_locks_deadlocks":                0,
				"mssql_instance_SQLEXPRESS_resource_RowGroup_locks_lock_wait_seconds":        0,
				"mssql_instance_SQLEXPRESS_resource_Xact_locks_deadlocks":                    0,
				"mssql_instance_SQLEXPRESS_resource_Xact_locks_lock_wait_seconds":            0,
				"mssql_instance_SQLEXPRESS_sql_errors_total_db_offline_errors":               0,
				"mssql_instance_SQLEXPRESS_sql_errors_total_info_errors":                     766,
				"mssql_instance_SQLEXPRESS_sql_errors_total_kill_connection_errors":          0,
				"mssql_instance_SQLEXPRESS_sql_errors_total_user_errors":                     29,
				"mssql_instance_SQLEXPRESS_sqlstats_auto_parameterization_attempts":          37,
				"mssql_instance_SQLEXPRESS_sqlstats_batch_requests":                          2972,
				"mssql_instance_SQLEXPRESS_sqlstats_safe_auto_parameterization_attempts":     2,
				"mssql_instance_SQLEXPRESS_sqlstats_sql_compilations":                        376,
				"mssql_instance_SQLEXPRESS_sqlstats_sql_recompilations":                      8,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_bytes_received":              38290755856,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_bytes_sent":                  8211165504,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_packets_outbound_discarded":  0,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_packets_outbound_errors":     0,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_packets_received_discarded":  0,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_packets_received_errors":     0,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_packets_received_total":      4120869,
				"net_nic_Intel_R_PRO_1000_MT_Network_Connection_packets_sent_total":          1332466,
				"netframework_WMSvc_clrexception_filters_total":                              0,
				"netframework_WMSvc_clrexception_finallys_total":                             0,
				"netframework_WMSvc_clrexception_throw_to_catch_depth_total":                 0,
				"netframework_WMSvc_clrexception_thrown_total":                               0,
				"netframework_WMSvc_clrinterop_com_callable_wrappers_total":                  2,
				"netframework_WMSvc_clrinterop_interop_marshalling_total":                    0,
				"netframework_WMSvc_clrinterop_interop_stubs_created_total":                  29,
				"netframework_WMSvc_clrjit_il_bytes_total":                                   4007,
				"netframework_WMSvc_clrjit_methods_total":                                    27,
				"netframework_WMSvc_clrjit_standard_failures_total":                          0,
				"netframework_WMSvc_clrjit_time_percent":                                     0,
				"netframework_WMSvc_clrloading_appdomains_loaded_total":                      1,
				"netframework_WMSvc_clrloading_appdomains_unloaded_total":                    0,
				"netframework_WMSvc_clrloading_assemblies_loaded_total":                      5,
				"netframework_WMSvc_clrloading_class_load_failures_total":                    0,
				"netframework_WMSvc_clrloading_classes_loaded_total":                         18,
				"netframework_WMSvc_clrloading_loader_heap_size_bytes":                       270336,
				"netframework_WMSvc_clrlocksandthreads_contentions_total":                    0,
				"netframework_WMSvc_clrlocksandthreads_current_logical_threads":              2,
				"netframework_WMSvc_clrlocksandthreads_physical_threads_current":             1,
				"netframework_WMSvc_clrlocksandthreads_queue_length_total":                   0,
				"netframework_WMSvc_clrlocksandthreads_recognized_threads_total":             1,
				"netframework_WMSvc_clrmemory_allocated_bytes_total":                         227792,
				"netframework_WMSvc_clrmemory_collections_total":                             2,
				"netframework_WMSvc_clrmemory_committed_bytes":                               270336,
				"netframework_WMSvc_clrmemory_finalization_survivors":                        7,
				"netframework_WMSvc_clrmemory_gc_time_percent":                               0,
				"netframework_WMSvc_clrmemory_heap_size_bytes":                               4312696,
				"netframework_WMSvc_clrmemory_induced_gc_total":                              0,
				"netframework_WMSvc_clrmemory_number_gc_handles":                             24,
				"netframework_WMSvc_clrmemory_number_pinned_objects":                         1,
				"netframework_WMSvc_clrmemory_number_sink_blocksinuse":                       1,
				"netframework_WMSvc_clrmemory_promoted_bytes":                                49720,
				"netframework_WMSvc_clrmemory_reserved_bytes":                                402644992,
				"netframework_WMSvc_clrremoting_channels_total":                              0,
				"netframework_WMSvc_clrremoting_context_bound_classes_loaded":                0,
				"netframework_WMSvc_clrremoting_context_bound_objects_total":                 0,
				"netframework_WMSvc_clrremoting_context_proxies_total":                       0,
				"netframework_WMSvc_clrremoting_contexts":                                    1,
				"netframework_WMSvc_clrremoting_remote_calls_total":                          0,
				"netframework_WMSvc_clrsecurity_checks_time_percent":                         0,
				"netframework_WMSvc_clrsecurity_link_time_checks_total":                      0,
				"netframework_WMSvc_clrsecurity_runtime_checks_total":                        3,
				"netframework_WMSvc_clrsecurity_stack_walk_depth":                            1,
				"netframework_powershell_clrexception_filters_total":                         0,
				"netframework_powershell_clrexception_finallys_total":                        56,
				"netframework_powershell_clrexception_throw_to_catch_depth_total":            140,
				"netframework_powershell_clrexception_thrown_total":                          37,
				"netframework_powershell_clrinterop_com_callable_wrappers_total":             5,
				"netframework_powershell_clrinterop_interop_marshalling_total":               0,
				"netframework_powershell_clrinterop_interop_stubs_created_total":             345,
				"netframework_powershell_clrjit_il_bytes_total":                              47021,
				"netframework_powershell_clrjit_methods_total":                               344,
				"netframework_powershell_clrjit_standard_failures_total":                     0,
				"netframework_powershell_clrjit_time_percent":                                0,
				"netframework_powershell_clrloading_appdomains_loaded_total":                 1,
				"netframework_powershell_clrloading_appdomains_unloaded_total":               0,
				"netframework_powershell_clrloading_assemblies_loaded_total":                 20,
				"netframework_powershell_clrloading_class_load_failures_total":               1,
				"netframework_powershell_clrloading_classes_loaded_total":                    477,
				"netframework_powershell_clrloading_loader_heap_size_bytes":                  2285568,
				"netframework_powershell_clrlocksandthreads_contentions_total":               10,
				"netframework_powershell_clrlocksandthreads_current_logical_threads":         16,
				"netframework_powershell_clrlocksandthreads_physical_threads_current":        13,
				"netframework_powershell_clrlocksandthreads_queue_length_total":              3,
				"netframework_powershell_clrlocksandthreads_recognized_threads_total":        6,
				"netframework_powershell_clrmemory_allocated_bytes_total":                    46333800,
				"netframework_powershell_clrmemory_collections_total":                        11,
				"netframework_powershell_clrmemory_committed_bytes":                          20475904,
				"netframework_powershell_clrmemory_finalization_survivors":                   244,
				"netframework_powershell_clrmemory_gc_time_percent":                          0,
				"netframework_powershell_clrmemory_heap_size_bytes":                          34711872,
				"netframework_powershell_clrmemory_induced_gc_total":                         0,
				"netframework_powershell_clrmemory_number_gc_handles":                        834,
				"netframework_powershell_clrmemory_number_pinned_objects":                    0,
				"netframework_powershell_clrmemory_number_sink_blocksinuse":                  42,
				"netframework_powershell_clrmemory_promoted_bytes":                           107352,
				"netframework_powershell_clrmemory_reserved_bytes":                           402644992,
				"netframework_powershell_clrremoting_channels_total":                         0,
				"netframework_powershell_clrremoting_context_bound_classes_loaded":           0,
				"netframework_powershell_clrremoting_context_bound_objects_total":            0,
				"netframework_powershell_clrremoting_context_proxies_total":                  0,
				"netframework_powershell_clrremoting_contexts":                               1,
				"netframework_powershell_clrremoting_remote_calls_total":                     0,
				"netframework_powershell_clrsecurity_checks_time_percent":                    0,
				"netframework_powershell_clrsecurity_link_time_checks_total":                 0,
				"netframework_powershell_clrsecurity_runtime_checks_total":                   4386,
				"netframework_powershell_clrsecurity_stack_walk_depth":                       1,
				"os_paging_free_bytes":                                                       1414107136,
				"os_paging_limit_bytes":                                                      1476395008,
				"os_paging_used_bytes":                                                       62287872,
				"os_physical_memory_free_bytes":                                              1379946496,
				"os_processes":                                                               152,
				"os_processes_limit":                                                         4294967295,
				"os_users":                                                                   2,
				"os_visible_memory_bytes":                                                    4256718848,
				"os_visible_memory_used_bytes":                                               2876772352,
				"process_msedge_cpu_time":                                                    1919893,
				"process_msedge_handles":                                                     5779,
				"process_msedge_io_bytes":                                                    3978227378,
				"process_msedge_io_operations":                                               16738642,
				"process_msedge_page_faults":                                                 5355941,
				"process_msedge_page_file_bytes":                                             681603072,
				"process_msedge_threads":                                                     213,
				"process_msedge_working_set_private_bytes":                                   461344768,
				"service_dhcp_state_continue_pending":                                        0,
				"service_dhcp_state_pause_pending":                                           0,
				"service_dhcp_state_paused":                                                  0,
				"service_dhcp_state_running":                                                 1,
				"service_dhcp_state_start_pending":                                           0,
				"service_dhcp_state_stop_pending":                                            0,
				"service_dhcp_state_stopped":                                                 0,
				"service_dhcp_state_unknown":                                                 0,
				"service_dhcp_status_degraded":                                               0,
				"service_dhcp_status_error":                                                  0,
				"service_dhcp_status_lost_comm":                                              0,
				"service_dhcp_status_no_contact":                                             0,
				"service_dhcp_status_nonrecover":                                             0,
				"service_dhcp_status_ok":                                                     1,
				"service_dhcp_status_pred_fail":                                              0,
				"service_dhcp_status_service":                                                0,
				"service_dhcp_status_starting":                                               0,
				"service_dhcp_status_stopping":                                               0,
				"service_dhcp_status_stressed":                                               0,
				"service_dhcp_status_unknown":                                                0,
				"system_threads":                                                             1559,
				"system_up_time":                                                             16208210,
				"tcp_ipv4_conns_active":                                                      4301,
				"tcp_ipv4_conns_established":                                                 7,
				"tcp_ipv4_conns_failures":                                                    137,
				"tcp_ipv4_conns_passive":                                                     501,
				"tcp_ipv4_conns_resets":                                                      1282,
				"tcp_ipv4_segments_received":                                                 676388,
				"tcp_ipv4_segments_retransmitted":                                            2120,
				"tcp_ipv4_segments_sent":                                                     871379,
				"tcp_ipv6_conns_active":                                                      214,
				"tcp_ipv6_conns_established":                                                 0,
				"tcp_ipv6_conns_failures":                                                    214,
				"tcp_ipv6_conns_passive":                                                     0,
				"tcp_ipv6_conns_resets":                                                      0,
				"tcp_ipv6_segments_received":                                                 1284,
				"tcp_ipv6_segments_retransmitted":                                            428,
				"tcp_ipv6_segments_sent":                                                     856,
			},
		},
		"fails if endpoint returns invalid data": {
			prepare: prepareWindowsReturnsInvalidData,
		},
		"fails on connection refused": {
			prepare: prepareWindowsConnectionRefused,
		},
		"fails on 404 response": {
			prepare: prepareWindowsResponse404,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			if mx != nil && test.wantCollected != nil {
				mx["system_up_time"] = test.wantCollected["system_up_time"]
			}

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				testCharts(t, collr, mx)
			}
		})
	}
}

func testCharts(t *testing.T, collr *Collector, mx map[string]int64) {
	ensureChartsDimsCreated(t, collr)
	module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func ensureChartsDimsCreated(t *testing.T, collr *Collector) {
	for _, chart := range cpuCharts {
		if collr.cache.collection[collectorCPU] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range memCharts {
		if collr.cache.collection[collectorMemory] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range tcpCharts {
		if collr.cache.collection[collectorTCP] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range osCharts {
		if collr.cache.collection[collectorOS] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range systemCharts {
		if collr.cache.collection[collectorSystem] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range logonCharts {
		if collr.cache.collection[collectorLogon] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range processesCharts {
		if collr.cache.collection[collectorProcess] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRExceptionsChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRExceptions] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRInteropChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRInterop] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRJITChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRJIT] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRLoadingChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRLoading] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRLocksAndThreadsChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRLocksAndThreads] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRMemoryChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRMemory] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRRemotingChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRRemoting] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for _, chart := range netFrameworkCLRSecurityChartsTmpl {
		if collr.cache.collection[collectorNetFrameworkCLRSecurity] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}

	for core := range collr.cache.cores {
		for _, chart := range cpuCoreChartsTmpl {
			id := fmt.Sprintf(chart.ID, core)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' core", id, core)
		}
	}
	for disk := range collr.cache.volumes {
		for _, chart := range diskChartsTmpl {
			id := fmt.Sprintf(chart.ID, disk)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' disk", id, disk)
		}
	}
	for nic := range collr.cache.nics {
		for _, chart := range nicChartsTmpl {
			id := fmt.Sprintf(chart.ID, nic)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' nic", id, nic)
		}
	}
	for zone := range collr.cache.thermalZones {
		for _, chart := range thermalzoneChartsTmpl {
			id := fmt.Sprintf(chart.ID, zone)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' thermalzone", id, zone)
		}
	}
	for svc := range collr.cache.services {
		for _, chart := range serviceChartsTmpl {
			id := fmt.Sprintf(chart.ID, svc)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' service", id, svc)
		}
	}
	for website := range collr.cache.iis {
		for _, chart := range iisWebsiteChartsTmpl {
			id := fmt.Sprintf(chart.ID, website)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' website", id, website)
		}
	}
	for instance := range collr.cache.mssqlInstances {
		for _, chart := range mssqlInstanceChartsTmpl {
			id := fmt.Sprintf(chart.ID, instance)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' instance", id, instance)
		}
	}
	for instanceDB := range collr.cache.mssqlDBs {
		s := strings.Split(instanceDB, ":")
		if assert.Lenf(t, s, 2, "can not extract intance/database from cache.mssqlDBs") {
			instance, db := s[0], s[1]
			for _, chart := range mssqlDatabaseChartsTmpl {
				id := fmt.Sprintf(chart.ID, db, instance)
				assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' instance", id, instance)
			}
		}
	}
	for _, chart := range adCharts {
		if collr.cache.collection[collectorAD] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for template := range collr.cache.adcs {
		for _, chart := range adcsCertTemplateChartsTmpl {
			id := fmt.Sprintf(chart.ID, template)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' template certificate", id, template)
		}
	}
	for name := range collr.cache.collectors {
		for _, chart := range collectorChartsTmpl {
			id := fmt.Sprintf(chart.ID, name)
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' collector", id, name)
		}
	}

	for _, chart := range processesCharts {
		if chart = collr.Charts().Get(chart.ID); chart == nil {
			continue
		}
		for proc := range collr.cache.processes {
			var found bool
			for _, dim := range chart.Dims {
				if found = strings.HasPrefix(dim.ID, "process_"+proc); found {
					break
				}
			}
			assert.Truef(t, found, "chart '%s' has not dim for '%s' process", chart.ID, proc)
		}
	}

	for _, chart := range hypervChartsTmpl {
		if collr.cache.collection[collectorHyperv] {
			assert.Truef(t, collr.Charts().Has(chart.ID), "chart '%s' not created", chart.ID)
		} else {
			assert.Falsef(t, collr.Charts().Has(chart.ID), "chart '%s' created", chart.ID)
		}
	}
	for vm := range collr.cache.hypervVMMem {
		for _, chart := range hypervVMChartsTemplate {
			id := fmt.Sprintf(chart.ID, hypervCleanName(vm))
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' virtual machine", id, vm)
		}
	}
	for device := range collr.cache.hypervVMDevices {
		for _, chart := range hypervVMDeviceChartsTemplate {
			id := fmt.Sprintf(chart.ID, hypervCleanName(device))
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' vm storage device", id, device)
		}
	}
	for iface := range collr.cache.hypervVMInterfaces {
		for _, chart := range hypervVMInterfaceChartsTemplate {
			id := fmt.Sprintf(chart.ID, hypervCleanName(iface))
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' vm network interface", id, iface)
		}
	}
	for vswitch := range collr.cache.hypervVswitch {
		for _, chart := range hypervVswitchChartsTemplate {
			id := fmt.Sprintf(chart.ID, hypervCleanName(vswitch))
			assert.Truef(t, collr.Charts().Has(id), "charts has no '%s' chart for '%s' virtual switch", id, vswitch)
		}
	}
}

func prepareWindowsV0200() (collr *Collector, cleanup func()) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer0200Metrics)
		}))

	collr = New()
	collr.URL = ts.URL
	return collr, ts.Close
}

func prepareWindowsReturnsInvalidData() (collr *Collector, cleanup func()) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	collr = New()
	collr.URL = ts.URL
	return collr, ts.Close
}

func prepareWindowsConnectionRefused() (collr *Collector, cleanup func()) {
	collr = New()
	collr.URL = "http://127.0.0.1:38001"
	return collr, func() {}
}

func prepareWindowsResponse404() (collr *Collector, cleanup func()) {
	ts := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

	collr = New()
	collr.URL = ts.URL
	return collr, ts.Close
}
