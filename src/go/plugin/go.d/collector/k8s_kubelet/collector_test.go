// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataMetrics, _             = os.ReadFile("testdata/metrics.txt")
	dataServiceAccountToken, _ = os.ReadFile("testdata/token.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":          dataConfigJSON,
		"dataConfigYAML":          dataConfigYAML,
		"dataMetrics":             dataMetrics,
		"dataServiceAccountToken": dataServiceAccountToken,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Init(t *testing.T) {
	assert.NoError(t, New().Init(context.Background()))
}

func TestCollector_InitErrorOnCreatingClientWrongTLSCA(t *testing.T) {
	collr := New()
	collr.ClientConfig.TLSConfig.TLSCA = "testdata/tls"

	assert.Error(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.BearerTokenFile = ""
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_Check_ConnectionRefused(t *testing.T) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Collect(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(dataMetrics)
			}))
	defer ts.Close()

	collr := New()
	collr.BearerTokenFile = ""
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

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
		"pvc_app_cache_space_available":                                              1500000,
		"pvc_app_cache_space_reserved":                                               0,
		"pvc_app_cache_space_used":                                                   500000,
		"pvc_app_cache_space_utilization":                                            2500,
		"pvc_app_data-db-0_space_available":                                          450000,
		"pvc_app_data-db-0_space_reserved":                                           100000,
		"pvc_app_data-db-0_space_used":                                               450000,
		"pvc_app_data-db-0_space_utilization":                                        5000,
		"rest_client_requests_200":                                                   368,
		"rest_client_requests_201":                                                   43,
		"rest_client_requests_403":                                                   2,
		"rest_client_requests_409":                                                   1,
		"rest_client_requests_<error>":                                               45,
		"rest_client_requests_GET":                                                   230,
		"rest_client_requests_PATCH":                                                 177,
		"rest_client_requests_POST":                                                  52,
		"token_count":                                                                0,
		"token_fail_count":                                                           0,
		"volume_manager_plugin_kubernetes.io/configmap_state_actual":                 3,
		"volume_manager_plugin_kubernetes.io/configmap_state_desired":                3,
		"volume_manager_plugin_kubernetes.io/host-path_state_actual":                 15,
		"volume_manager_plugin_kubernetes.io/host-path_state_desired":                15,
		"volume_manager_plugin_kubernetes.io/secret_state_actual":                    4,
		"volume_manager_plugin_kubernetes.io/secret_state_desired":                   4,
	}

	assert.Equal(t, expected, collr.Collect(context.Background()))
}

func TestCollector_Collect_VolumeStats(t *testing.T) {
	type pvcChart struct {
		ctx    string
		labels map[string]string
	}
	labels := func(pvc string) map[string]string {
		return map[string]string{"k8s_namespace": "app", "k8s_pvc_name": pvc}
	}
	spaceCharts := func(id, pvc string) map[string]pvcChart {
		return map[string]pvcChart{
			"pvc_" + id + "_space_usage":       {ctx: "k8s_kubelet.pvc_space_usage", labels: labels(pvc)},
			"pvc_" + id + "_space_utilization": {ctx: "k8s_kubelet.pvc_space_utilization", labels: labels(pvc)},
		}
	}
	merge := func(ms ...map[string]pvcChart) map[string]pvcChart {
		out := make(map[string]pvcChart)
		for _, m := range ms {
			maps.Copy(out, m)
		}
		return out
	}

	tests := map[string]struct {
		series      []volumeStatsSeries
		wantMetrics map[string]int64
		wantCharts  map[string]pvcChart
	}{
		"no volume stats": {
			wantMetrics: map[string]int64{},
			wantCharts:  map[string]pvcChart{},
		},
		"all series reported": {
			series: volumeStats("data", 1000, 600, 300, 100, 40, 60),
			wantMetrics: map[string]int64{
				"pvc_app_data_space_available":   300,
				"pvc_app_data_space_used":        600,
				"pvc_app_data_space_reserved":    100,
				"pvc_app_data_space_utilization": 6666,
			},
			wantCharts: spaceCharts("app_data", "data"),
		},
		"empty volume is real data": {
			series: volumeStats("data", 1000, 0, 1000, 100, 0, 100),
			wantMetrics: map[string]int64{
				"pvc_app_data_space_available":   1000,
				"pvc_app_data_space_used":        0,
				"pvc_app_data_space_reserved":    0,
				"pvc_app_data_space_utilization": 0,
			},
			wantCharts: spaceCharts("app_data", "data"),
		},
		"inode series are ignored (SMB share reports 0 inodes)": {
			series: volumeStats("share", 1000, 600, 400, 0, 0, 0),
			wantMetrics: map[string]int64{
				"pvc_app_share_space_available":   400,
				"pvc_app_share_space_used":        600,
				"pvc_app_share_space_reserved":    0,
				"pvc_app_share_space_utilization": 6000,
			},
			wantCharts: spaceCharts("app_share", "share"),
		},
		"capacity only (block volume): no charts": {
			series:      volumeStats("disk", 1000, 0, 0, 0, 0, 0),
			wantMetrics: map[string]int64{},
			wantCharts:  map[string]pvcChart{},
		},
		"missing space series: no charts": {
			series: slices.DeleteFunc(volumeStats("data", 1000, 600, 300, 100, 40, 60), func(s volumeStatsSeries) bool {
				return s.name == "kubelet_volume_stats_used_bytes"
			}),
			wantMetrics: map[string]int64{},
			wantCharts:  map[string]pvcChart{},
		},
		"used + available above capacity: reserved clamped at 0": {
			series: volumeStats("data", 100, 80, 30, 100, 40, 60),
			wantMetrics: map[string]int64{
				"pvc_app_data_space_available":   30,
				"pvc_app_data_space_used":        80,
				"pvc_app_data_space_reserved":    0,
				"pvc_app_data_space_utilization": 7272,
			},
			wantCharts: spaceCharts("app_data", "data"),
		},
		"names differing only in '.' and '-' get distinct charts": {
			series: slices.Concat(
				volumeStats("data.0", 1000, 600, 400, 0, 0, 0),
				volumeStats("data-0", 2000, 500, 1500, 0, 0, 0),
			),
			wantMetrics: map[string]int64{
				"pvc_app_data_0_space_available":   400,
				"pvc_app_data_0_space_used":        600,
				"pvc_app_data_0_space_reserved":    0,
				"pvc_app_data_0_space_utilization": 6000,
				"pvc_app_data-0_space_available":   1500,
				"pvc_app_data-0_space_used":        500,
				"pvc_app_data-0_space_reserved":    0,
				"pvc_app_data-0_space_utilization": 2500,
			},
			wantCharts: merge(spaceCharts("app_data_0", "data.0"), spaceCharts("app_data-0", "data-0")),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, setScrape := newVolumeStatsCollector(t)
			setScrape(test.series)

			mx := collr.Collect(context.Background())
			require.NotNil(t, mx)

			assert.Equal(t, test.wantMetrics, pvcMetricsOf(mx))
			charts := make(map[string]pvcChart)
			for _, chart := range *collr.Charts() {
				if strings.HasPrefix(chart.ID, "pvc_") {
					charts[chart.ID] = pvcChart{ctx: chart.Ctx, labels: chartLabels(chart)}
				}
			}
			assert.Equal(t, test.wantCharts, charts)
			collecttest.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, func(chart *Chart, _ *Dim) bool {
				return !strings.HasPrefix(chart.ID, "pvc_")
			})
		})
	}
}

func TestCollector_Collect_VolumeStatsChartLifecycle(t *testing.T) {
	collr, setScrape := newVolumeStatsCollector(t)

	data := volumeStats("data", 1000, 600, 300, 100, 40, 60)
	cache := volumeStats("cache", 2000, 500, 1500, 200, 20, 180)
	// a name containing "space" must not be mistaken for the "_space_" metric suffix
	space := volumeStats("my.space", 3000, 300, 2700, 300, 30, 270)

	// chart ID => state of each chart with that ID, in creation order; removed charts stay in the list until the
	// job runtime prunes them, so a re-added PVC has a removed and an active entry
	chartStates := func() map[string][]string {
		states := make(map[string][]string)
		for _, chart := range *collr.Charts() {
			if !strings.HasPrefix(chart.ID, "pvc_") {
				continue
			}
			state := "active"
			if chart.IsRemoved() {
				require.True(t, chart.Obsolete, chart.ID)
				require.False(t, chart.IsCreated(), chart.ID)
				state = "removed"
			}
			states[chart.ID] = append(states[chart.ID], state)
		}
		return states
	}
	allCharts := func(dataState, cacheState, spaceState []string) map[string][]string {
		return map[string][]string{
			"pvc_app_data_space_usage":           dataState,
			"pvc_app_data_space_utilization":     dataState,
			"pvc_app_cache_space_usage":          cacheState,
			"pvc_app_cache_space_utilization":    cacheState,
			"pvc_app_my_space_space_usage":       spaceState,
			"pvc_app_my_space_space_utilization": spaceState,
		}
	}

	steps := []struct {
		name       string
		series     []volumeStatsSeries
		wantPVCs   []string
		wantCharts map[string][]string
	}{
		{
			name:       "all PVCs",
			series:     slices.Concat(data, cache, space),
			wantPVCs:   []string{"app_cache", "app_data", "app_my_space"},
			wantCharts: allCharts([]string{"active"}, []string{"active"}, []string{"active"}),
		},
		{
			name:       "cache disappears",
			series:     slices.Concat(data, space),
			wantPVCs:   []string{"app_data", "app_my_space"},
			wantCharts: allCharts([]string{"active"}, []string{"removed"}, []string{"active"}),
		},
		{
			name:       "all disappear",
			wantCharts: allCharts([]string{"removed"}, []string{"removed"}, []string{"removed"}),
		},
		{
			name:       "cache reappears",
			series:     cache,
			wantPVCs:   []string{"app_cache"},
			wantCharts: allCharts([]string{"removed"}, []string{"removed", "active"}, []string{"removed"}),
		},
	}

	for _, step := range steps {
		// the job runtime marks emitted charts as created; a removed chart must reset it to send the obsolete CHART
		for _, chart := range *collr.Charts() {
			if !chart.IsRemoved() {
				chart.SetCreated(true)
			}
		}
		setScrape(step.series)
		mx := collr.Collect(context.Background())
		require.NotNil(t, mx, step.name)

		var pvcs []string
		for key := range pvcMetricsOf(mx) {
			// the PVC ID ends at the last "_space_": the ID itself may contain it
			end := strings.LastIndex(key, "_space_")
			require.Positive(t, end, key)
			pvc := key[len("pvc_"):end]
			if !slices.Contains(pvcs, pvc) {
				pvcs = append(pvcs, pvc)
			}
		}
		slices.Sort(pvcs)
		assert.Equal(t, step.wantPVCs, pvcs, step.name)
		assert.Equal(t, step.wantCharts, chartStates(), step.name)
	}
}

type volumeStatsSeries struct {
	name  string
	pvc   string
	value float64
}

func volumeStats(pvc string, capacity, used, available, inodes, inodesUsed, inodesFree float64) []volumeStatsSeries {
	return []volumeStatsSeries{
		{name: "kubelet_volume_stats_capacity_bytes", pvc: pvc, value: capacity},
		{name: "kubelet_volume_stats_used_bytes", pvc: pvc, value: used},
		{name: "kubelet_volume_stats_available_bytes", pvc: pvc, value: available},
		{name: "kubelet_volume_stats_inodes", pvc: pvc, value: inodes},
		{name: "kubelet_volume_stats_inodes_used", pvc: pvc, value: inodesUsed},
		{name: "kubelet_volume_stats_inodes_free", pvc: pvc, value: inodesFree},
	}
}

// newVolumeStatsCollector returns a collector scraping the shared sample without its volume stats, plus the given
// volume stats series.
func newVolumeStatsCollector(t *testing.T) (*Collector, func([]volumeStatsSeries)) {
	var base strings.Builder
	for _, line := range strings.SplitAfter(string(dataMetrics), "\n") {
		if !strings.Contains(line, "kubelet_volume_stats_") {
			base.WriteString(line)
		}
	}

	var mu sync.Mutex
	var scrape []byte
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				_, _ = w.Write(scrape)
			}))
	t.Cleanup(ts.Close)

	collr := New()
	collr.BearerTokenFile = ""
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func(series []volumeStatsSeries) {
		var b strings.Builder
		b.WriteString(base.String())
		for _, s := range series {
			_, _ = fmt.Fprintf(&b, "%s{namespace=\"app\",persistentvolumeclaim=%q} %g\n", s.name, s.pvc, s.value)
		}
		mu.Lock()
		defer mu.Unlock()
		scrape = []byte(b.String())
	}
}

func pvcMetricsOf(mx map[string]int64) map[string]int64 {
	out := make(map[string]int64)
	for k, v := range mx {
		if strings.HasPrefix(k, "pvc_") {
			out[k] = v
		}
	}
	return out
}

func chartLabels(chart *Chart) map[string]string {
	labels := make(map[string]string)
	for _, l := range chart.Labels {
		labels[l.Key] = l.Value
	}
	return labels
}

func TestCollector_Collect_ReceiveInvalidResponse(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello and goodbye"))
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}

func TestCollector_Collect_Receive404(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
	defer ts.Close()

	collr := New()
	collr.URL = ts.URL + "/metrics"
	require.NoError(t, collr.Init(context.Background()))
	assert.Error(t, collr.Check(context.Background()))
}
