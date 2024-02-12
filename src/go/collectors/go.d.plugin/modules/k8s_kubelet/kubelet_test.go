// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testMetricsData, _ = os.ReadFile("testdata/metrics.txt")
	testTokenData, _   = os.ReadFile("testdata/token.txt")
)

func Test_readTestData(t *testing.T) {
	assert.NotNil(t, testMetricsData)
	assert.NotNil(t, testTokenData)
}

func TestNew(t *testing.T) {
	job := New()

	assert.IsType(t, (*Kubelet)(nil), job)
}

func TestKubelet_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestKubelet_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestKubelet_Init(t *testing.T) {
	assert.True(t, New().Init())
}

func TestKubelet_Init_ReadServiceAccountToken(t *testing.T) {
	job := New()
	job.TokenPath = "testdata/token.txt"

	assert.True(t, job.Init())
	assert.Equal(t, "Bearer "+string(testTokenData), job.Request.Headers["Authorization"])
}

func TestKubelet_InitErrorOnCreatingClientWrongTLSCA(t *testing.T) {
	job := New()
	job.Client.TLSConfig.TLSCA = "testdata/tls"

	assert.False(t, job.Init())
}

func TestKubelet_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testMetricsData)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.True(t, job.Init())
	assert.True(t, job.Check())
}

func TestKubelet_Check_ConnectionRefused(t *testing.T) {
	job := New()
	job.URL = "http://127.0.0.1:38001/metrics"
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestKubelet_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(testMetricsData)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.True(t, job.Init())
	require.True(t, job.Check())

	expected := map[string]int64{
		"apiserver_audit_requests_rejected_total":                                    0,
		"apiserver_storage_data_key_generation_bucket_+Inf":                          1,
		"apiserver_storage_data_key_generation_bucket_10":                            1,
		"apiserver_storage_data_key_generation_bucket_10240":                         1,
		"apiserver_storage_data_key_generation_bucket_1280":                          1,
		"apiserver_storage_data_key_generation_bucket_160":                           1,
		"apiserver_storage_data_key_generation_bucket_20":                            1,
		"apiserver_storage_data_key_generation_bucket_20480":                         1,
		"apiserver_storage_data_key_generation_bucket_2560":                          1,
		"apiserver_storage_data_key_generation_bucket_320":                           1,
		"apiserver_storage_data_key_generation_bucket_40":                            1,
		"apiserver_storage_data_key_generation_bucket_40960":                         1,
		"apiserver_storage_data_key_generation_bucket_5":                             6,
		"apiserver_storage_data_key_generation_bucket_5120":                          1,
		"apiserver_storage_data_key_generation_bucket_640":                           1,
		"apiserver_storage_data_key_generation_bucket_80":                            1,
		"apiserver_storage_data_key_generation_failures_total":                       0,
		"apiserver_storage_envelope_transformation_cache_misses_total":               0,
		"kubelet_docker_operations_create_container":                                 19,
		"kubelet_docker_operations_errors_inspect_container":                         14,
		"kubelet_docker_operations_errors_remove_container":                          4,
		"kubelet_docker_operations_info":                                             2,
		"kubelet_docker_operations_inspect_container":                                223,
		"kubelet_docker_operations_inspect_image":                                    110,
		"kubelet_docker_operations_list_containers":                                  5157,
		"kubelet_docker_operations_list_images":                                      195,
		"kubelet_docker_operations_remove_container":                                 23,
		"kubelet_docker_operations_start_container":                                  19,
		"kubelet_docker_operations_stop_container":                                   23,
		"kubelet_docker_operations_version":                                          472,
		"kubelet_log_file_system_usage_kube-system_coredns-86c58d9df4-d22hv":         28672,
		"kubelet_log_file_system_usage_kube-system_coredns-86c58d9df4-ks5dj":         28672,
		"kubelet_log_file_system_usage_kube-system_etcd-minikube":                    36864,
		"kubelet_log_file_system_usage_kube-system_kube-addon-manager-minikube":      45056,
		"kubelet_log_file_system_usage_kube-system_kube-apiserver-minikube":          36864,
		"kubelet_log_file_system_usage_kube-system_kube-controller-manager-minikube": 57344,
		"kubelet_log_file_system_usage_kube-system_kube-proxy-q2fvs":                 28672,
		"kubelet_log_file_system_usage_kube-system_kube-scheduler-minikube":          40960,
		"kubelet_log_file_system_usage_kube-system_storage-provisioner":              24576,
		"kubelet_node_config_error":                                                  1,
		"kubelet_pleg_relist_interval_05":                                            1013125,
		"kubelet_pleg_relist_interval_09":                                            1016820,
		"kubelet_pleg_relist_interval_099":                                           1032022,
		"kubelet_pleg_relist_latency_05":                                             12741,
		"kubelet_pleg_relist_latency_09":                                             16211,
		"kubelet_pleg_relist_latency_099":                                            31234,
		"kubelet_running_container":                                                  9,
		"kubelet_running_pod":                                                        9,
		"kubelet_runtime_operations_container_status":                                90,
		"kubelet_runtime_operations_create_container":                                10,
		"kubelet_runtime_operations_errors_container_status":                         14,
		"kubelet_runtime_operations_errors_remove_container":                         4,
		"kubelet_runtime_operations_exec_sync":                                       138,
		"kubelet_runtime_operations_image_status":                                    25,
		"kubelet_runtime_operations_list_containers":                                 2586,
		"kubelet_runtime_operations_list_images":                                     195,
		"kubelet_runtime_operations_list_podsandbox":                                 2562,
		"kubelet_runtime_operations_podsandbox_status":                               77,
		"kubelet_runtime_operations_remove_container":                                14,
		"kubelet_runtime_operations_run_podsandbox":                                  9,
		"kubelet_runtime_operations_start_container":                                 10,
		"kubelet_runtime_operations_status":                                          279,
		"kubelet_runtime_operations_stop_podsandbox":                                 14,
		"kubelet_runtime_operations_version":                                         190,
		"rest_client_requests_200":                                                   177,
		"rest_client_requests_201":                                                   43,
		"rest_client_requests_403":                                                   2,
		"rest_client_requests_409":                                                   1,
		"rest_client_requests_<error>":                                               8,
		"rest_client_requests_GET":                                                   37,
		"rest_client_requests_PATCH":                                                 177,
		"rest_client_requests_POST":                                                  8,
		"token_count":                                                                0,
		"token_fail_count":                                                           0,
		"volume_manager_plugin_kubernetes.io/configmap_state_actual":                 3,
		"volume_manager_plugin_kubernetes.io/configmap_state_desired":                3,
		"volume_manager_plugin_kubernetes.io/host-path_state_actual":                 15,
		"volume_manager_plugin_kubernetes.io/host-path_state_desired":                15,
		"volume_manager_plugin_kubernetes.io/secret_state_actual":                    4,
		"volume_manager_plugin_kubernetes.io/secret_state_desired":                   4,
	}

	assert.Equal(t, expected, job.Collect())
}

func TestKubelet_Collect_ReceiveInvalidResponse(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.True(t, job.Init())
	assert.False(t, job.Check())
}

func TestKubelet_Collect_Receive404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	job := New()
	job.URL = ts.URL + "/metrics"
	require.True(t, job.Init())
	assert.False(t, job.Check())
}
