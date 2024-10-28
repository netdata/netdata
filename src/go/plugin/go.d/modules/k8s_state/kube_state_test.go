// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
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

func TestKubeState_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &KubeState{}, dataConfigJSON, dataConfigYAML)
}

func TestKubeState_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func() *KubeState
	}{
		"success when no error on initializing K8s client": {
			wantFail: false,
			prepare: func() *KubeState {
				ks := New()
				ks.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return ks
			},
		},
		"fail when get an error on initializing K8s client": {
			wantFail: true,
			prepare: func() *KubeState {
				ks := New()
				ks.newKubeClient = func() (kubernetes.Interface, error) { return nil, errors.New("newKubeClient() error") }
				return ks
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ks := test.prepare()

			if test.wantFail {
				assert.Error(t, ks.Init())
			} else {
				assert.NoError(t, ks.Init())
			}
		})
	}
}

func TestKubeState_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func() *KubeState
	}{
		"success when connected to the K8s API": {
			wantFail: false,
			prepare: func() *KubeState {
				ks := New()
				ks.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return ks
			},
		},
		"fail when not connected to the K8s API": {
			wantFail: true,
			prepare: func() *KubeState {
				ks := New()
				client := &brokenInfoKubeClient{fake.NewClientset()}
				ks.newKubeClient = func() (kubernetes.Interface, error) { return client, nil }
				return ks
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ks := test.prepare()
			require.NoError(t, ks.Init())

			if test.wantFail {
				assert.Error(t, ks.Check())
			} else {
				assert.NoError(t, ks.Check())
			}
		})
	}
}

func TestKubeState_Charts(t *testing.T) {
	ks := New()

	assert.NotEmpty(t, *ks.Charts())
}

func TestKubeState_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func() *KubeState
		doInit    bool
		doCollect bool
	}{
		"before init": {
			doInit:    false,
			doCollect: false,
			prepare: func() *KubeState {
				ks := New()
				ks.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return ks
			},
		},
		"after init": {
			doInit:    true,
			doCollect: false,
			prepare: func() *KubeState {
				ks := New()
				ks.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return ks
			},
		},
		"after collect": {
			doInit:    true,
			doCollect: true,
			prepare: func() *KubeState {
				ks := New()
				ks.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return ks
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ks := test.prepare()

			if test.doInit {
				_ = ks.Init()
			}
			if test.doCollect {
				_ = ks.Collect()
				time.Sleep(ks.initDelay)
			}

			assert.NotPanics(t, ks.Cleanup)
			time.Sleep(time.Second)
			if test.doCollect {
				assert.True(t, ks.discoverer.stopped())
			}
		})
	}
}

func TestKubeState_Collect(t *testing.T) {
	type (
		testCaseStep func(t *testing.T, ks *KubeState)
		testCase     struct {
			client kubernetes.Interface
			steps  []testCaseStep
		}
	)

	tests := map[string]struct {
		create func(t *testing.T) testCase
	}{
		"Node only": {
			create: func(t *testing.T) testCase {
				client := fake.NewClientset(
					newNode("node01"),
				)

				step1 := func(t *testing.T, ks *KubeState) {
					mx := ks.Collect()
					expected := map[string]int64{
						"discovery_node_discoverer_state":              1,
						"discovery_pod_discoverer_state":               1,
						"node_node01_age":                              3,
						"node_node01_alloc_cpu_limits_used":            0,
						"node_node01_alloc_cpu_limits_util":            0,
						"node_node01_alloc_cpu_requests_used":          0,
						"node_node01_alloc_cpu_requests_util":          0,
						"node_node01_alloc_mem_limits_used":            0,
						"node_node01_alloc_mem_limits_util":            0,
						"node_node01_alloc_mem_requests_used":          0,
						"node_node01_alloc_mem_requests_util":          0,
						"node_node01_alloc_pods_allocated":             0,
						"node_node01_alloc_pods_available":             110,
						"node_node01_alloc_pods_util":                  0,
						"node_node01_cond_diskpressure":                0,
						"node_node01_cond_memorypressure":              0,
						"node_node01_cond_networkunavailable":          0,
						"node_node01_cond_pidpressure":                 0,
						"node_node01_cond_ready":                       1,
						"node_node01_schedulability_schedulable":       1,
						"node_node01_schedulability_unschedulable":     0,
						"node_node01_containers":                       0,
						"node_node01_containers_state_running":         0,
						"node_node01_containers_state_terminated":      0,
						"node_node01_containers_state_waiting":         0,
						"node_node01_init_containers":                  0,
						"node_node01_init_containers_state_running":    0,
						"node_node01_init_containers_state_terminated": 0,
						"node_node01_init_containers_state_waiting":    0,
						"node_node01_pods_cond_containersready":        0,
						"node_node01_pods_cond_podinitialized":         0,
						"node_node01_pods_cond_podready":               0,
						"node_node01_pods_cond_podscheduled":           0,
						"node_node01_pods_phase_failed":                0,
						"node_node01_pods_phase_pending":               0,
						"node_node01_pods_phase_running":               0,
						"node_node01_pods_phase_succeeded":             0,
						"node_node01_pods_readiness":                   0,
						"node_node01_pods_readiness_ready":             0,
						"node_node01_pods_readiness_unready":           0,
					}
					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(nodeChartsTmpl)+len(baseCharts),
						len(*ks.Charts()),
					)
					module.TestMetricsHasAllChartsDims(t, ks.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"Pod only": {
			create: func(t *testing.T) testCase {
				pod := newPod("node01", "pod01")
				client := fake.NewClientset(
					pod,
				)

				step1 := func(t *testing.T, ks *KubeState) {
					mx := ks.Collect()
					expected := map[string]int64{
						"discovery_node_discoverer_state":                                                        1,
						"discovery_pod_discoverer_state":                                                         1,
						"pod_default_pod01_age":                                                                  3,
						"pod_default_pod01_cond_containersready":                                                 1,
						"pod_default_pod01_cond_podinitialized":                                                  1,
						"pod_default_pod01_cond_podready":                                                        1,
						"pod_default_pod01_cond_podscheduled":                                                    1,
						"pod_default_pod01_container_container1_readiness":                                       1,
						"pod_default_pod01_container_container1_restarts":                                        0,
						"pod_default_pod01_container_container1_state_running":                                   1,
						"pod_default_pod01_container_container1_state_terminated":                                0,
						"pod_default_pod01_container_container1_state_terminated_reason_Completed":               0,
						"pod_default_pod01_container_container1_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod01_container_container1_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod01_container_container1_state_terminated_reason_Error":                   0,
						"pod_default_pod01_container_container1_state_terminated_reason_Evicted":                 0,
						"pod_default_pod01_container_container1_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod01_container_container1_state_terminated_reason_Other":                   0,
						"pod_default_pod01_container_container1_state_waiting":                                   0,
						"pod_default_pod01_container_container1_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod01_container_container1_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod01_container_container1_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod01_container_container1_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod01_container_container1_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_Other":                      0,
						"pod_default_pod01_container_container1_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod01_container_container2_readiness":                                       1,
						"pod_default_pod01_container_container2_restarts":                                        0,
						"pod_default_pod01_container_container2_state_running":                                   1,
						"pod_default_pod01_container_container2_state_terminated":                                0,
						"pod_default_pod01_container_container2_state_terminated_reason_Completed":               0,
						"pod_default_pod01_container_container2_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod01_container_container2_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod01_container_container2_state_terminated_reason_Error":                   0,
						"pod_default_pod01_container_container2_state_terminated_reason_Evicted":                 0,
						"pod_default_pod01_container_container2_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod01_container_container2_state_terminated_reason_Other":                   0,
						"pod_default_pod01_container_container2_state_waiting":                                   0,
						"pod_default_pod01_container_container2_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod01_container_container2_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod01_container_container2_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod01_container_container2_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod01_container_container2_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_Other":                      0,
						"pod_default_pod01_container_container2_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod01_containers":                                                           2,
						"pod_default_pod01_containers_state_running":                                             2,
						"pod_default_pod01_containers_state_terminated":                                          0,
						"pod_default_pod01_containers_state_waiting":                                             0,
						"pod_default_pod01_cpu_limits_used":                                                      400,
						"pod_default_pod01_cpu_requests_used":                                                    200,
						"pod_default_pod01_init_containers":                                                      1,
						"pod_default_pod01_init_containers_state_running":                                        0,
						"pod_default_pod01_init_containers_state_terminated":                                     1,
						"pod_default_pod01_init_containers_state_waiting":                                        0,
						"pod_default_pod01_mem_limits_used":                                                      419430400,
						"pod_default_pod01_mem_requests_used":                                                    209715200,
						"pod_default_pod01_phase_failed":                                                         0,
						"pod_default_pod01_phase_pending":                                                        0,
						"pod_default_pod01_phase_running":                                                        1,
						"pod_default_pod01_phase_succeeded":                                                      0,
					}

					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(podChartsTmpl)+len(containerChartsTmpl)*len(pod.Spec.Containers)+len(baseCharts),
						len(*ks.Charts()),
					)
					module.TestMetricsHasAllChartsDims(t, ks.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"Nodes and Pods": {
			create: func(t *testing.T) testCase {
				node := newNode("node01")
				pod := newPod(node.Name, "pod01")
				client := fake.NewClientset(
					node,
					pod,
				)

				step1 := func(t *testing.T, ks *KubeState) {
					mx := ks.Collect()
					expected := map[string]int64{
						"discovery_node_discoverer_state":                                                        1,
						"discovery_pod_discoverer_state":                                                         1,
						"node_node01_age":                                                                        3,
						"node_node01_alloc_cpu_limits_used":                                                      400,
						"node_node01_alloc_cpu_limits_util":                                                      11428,
						"node_node01_alloc_cpu_requests_used":                                                    200,
						"node_node01_alloc_cpu_requests_util":                                                    5714,
						"node_node01_alloc_mem_limits_used":                                                      419430400,
						"node_node01_alloc_mem_limits_util":                                                      11428,
						"node_node01_alloc_mem_requests_used":                                                    209715200,
						"node_node01_alloc_mem_requests_util":                                                    5714,
						"node_node01_alloc_pods_allocated":                                                       1,
						"node_node01_alloc_pods_available":                                                       109,
						"node_node01_alloc_pods_util":                                                            909,
						"node_node01_cond_diskpressure":                                                          0,
						"node_node01_cond_memorypressure":                                                        0,
						"node_node01_cond_networkunavailable":                                                    0,
						"node_node01_cond_pidpressure":                                                           0,
						"node_node01_cond_ready":                                                                 1,
						"node_node01_containers":                                                                 2,
						"node_node01_containers_state_running":                                                   2,
						"node_node01_containers_state_terminated":                                                0,
						"node_node01_containers_state_waiting":                                                   0,
						"node_node01_init_containers":                                                            1,
						"node_node01_init_containers_state_running":                                              0,
						"node_node01_init_containers_state_terminated":                                           1,
						"node_node01_init_containers_state_waiting":                                              0,
						"node_node01_pods_cond_containersready":                                                  1,
						"node_node01_pods_cond_podinitialized":                                                   1,
						"node_node01_pods_cond_podready":                                                         1,
						"node_node01_pods_cond_podscheduled":                                                     1,
						"node_node01_pods_phase_failed":                                                          0,
						"node_node01_pods_phase_pending":                                                         0,
						"node_node01_pods_phase_running":                                                         1,
						"node_node01_pods_phase_succeeded":                                                       0,
						"node_node01_pods_readiness":                                                             100000,
						"node_node01_pods_readiness_ready":                                                       1,
						"node_node01_pods_readiness_unready":                                                     0,
						"node_node01_schedulability_schedulable":                                                 1,
						"node_node01_schedulability_unschedulable":                                               0,
						"pod_default_pod01_age":                                                                  3,
						"pod_default_pod01_cond_containersready":                                                 1,
						"pod_default_pod01_cond_podinitialized":                                                  1,
						"pod_default_pod01_cond_podready":                                                        1,
						"pod_default_pod01_cond_podscheduled":                                                    1,
						"pod_default_pod01_container_container1_readiness":                                       1,
						"pod_default_pod01_container_container1_restarts":                                        0,
						"pod_default_pod01_container_container1_state_running":                                   1,
						"pod_default_pod01_container_container1_state_terminated":                                0,
						"pod_default_pod01_container_container1_state_terminated_reason_Completed":               0,
						"pod_default_pod01_container_container1_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod01_container_container1_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod01_container_container1_state_terminated_reason_Error":                   0,
						"pod_default_pod01_container_container1_state_terminated_reason_Evicted":                 0,
						"pod_default_pod01_container_container1_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod01_container_container1_state_terminated_reason_Other":                   0,
						"pod_default_pod01_container_container1_state_waiting":                                   0,
						"pod_default_pod01_container_container1_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod01_container_container1_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod01_container_container1_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod01_container_container1_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod01_container_container1_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_Other":                      0,
						"pod_default_pod01_container_container1_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod01_container_container2_readiness":                                       1,
						"pod_default_pod01_container_container2_restarts":                                        0,
						"pod_default_pod01_container_container2_state_running":                                   1,
						"pod_default_pod01_container_container2_state_terminated":                                0,
						"pod_default_pod01_container_container2_state_terminated_reason_Completed":               0,
						"pod_default_pod01_container_container2_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod01_container_container2_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod01_container_container2_state_terminated_reason_Error":                   0,
						"pod_default_pod01_container_container2_state_terminated_reason_Evicted":                 0,
						"pod_default_pod01_container_container2_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod01_container_container2_state_terminated_reason_Other":                   0,
						"pod_default_pod01_container_container2_state_waiting":                                   0,
						"pod_default_pod01_container_container2_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod01_container_container2_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod01_container_container2_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod01_container_container2_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod01_container_container2_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_Other":                      0,
						"pod_default_pod01_container_container2_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod01_containers":                                                           2,
						"pod_default_pod01_containers_state_running":                                             2,
						"pod_default_pod01_containers_state_terminated":                                          0,
						"pod_default_pod01_containers_state_waiting":                                             0,
						"pod_default_pod01_cpu_limits_used":                                                      400,
						"pod_default_pod01_cpu_requests_used":                                                    200,
						"pod_default_pod01_init_containers":                                                      1,
						"pod_default_pod01_init_containers_state_running":                                        0,
						"pod_default_pod01_init_containers_state_terminated":                                     1,
						"pod_default_pod01_init_containers_state_waiting":                                        0,
						"pod_default_pod01_mem_limits_used":                                                      419430400,
						"pod_default_pod01_mem_requests_used":                                                    209715200,
						"pod_default_pod01_phase_failed":                                                         0,
						"pod_default_pod01_phase_pending":                                                        0,
						"pod_default_pod01_phase_running":                                                        1,
						"pod_default_pod01_phase_succeeded":                                                      0,
					}

					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(nodeChartsTmpl)+len(podChartsTmpl)+len(containerChartsTmpl)*len(pod.Spec.Containers)+len(baseCharts),
						len(*ks.Charts()),
					)
					module.TestMetricsHasAllChartsDims(t, ks.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"delete a Pod in runtime": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				node := newNode("node01")
				pod := newPod(node.Name, "pod01")
				client := fake.NewClientset(
					node,
					pod,
				)
				step1 := func(t *testing.T, ks *KubeState) {
					_ = ks.Collect()
					_ = client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
				}

				step2 := func(t *testing.T, ks *KubeState) {
					mx := ks.Collect()
					expected := map[string]int64{
						"discovery_node_discoverer_state":              1,
						"discovery_pod_discoverer_state":               1,
						"node_node01_age":                              4,
						"node_node01_alloc_cpu_limits_used":            0,
						"node_node01_alloc_cpu_limits_util":            0,
						"node_node01_alloc_cpu_requests_used":          0,
						"node_node01_alloc_cpu_requests_util":          0,
						"node_node01_alloc_mem_limits_used":            0,
						"node_node01_alloc_mem_limits_util":            0,
						"node_node01_alloc_mem_requests_used":          0,
						"node_node01_alloc_mem_requests_util":          0,
						"node_node01_alloc_pods_allocated":             0,
						"node_node01_alloc_pods_available":             110,
						"node_node01_alloc_pods_util":                  0,
						"node_node01_cond_diskpressure":                0,
						"node_node01_cond_memorypressure":              0,
						"node_node01_cond_networkunavailable":          0,
						"node_node01_cond_pidpressure":                 0,
						"node_node01_cond_ready":                       1,
						"node_node01_schedulability_schedulable":       1,
						"node_node01_schedulability_unschedulable":     0,
						"node_node01_containers":                       0,
						"node_node01_containers_state_running":         0,
						"node_node01_containers_state_terminated":      0,
						"node_node01_containers_state_waiting":         0,
						"node_node01_init_containers":                  0,
						"node_node01_init_containers_state_running":    0,
						"node_node01_init_containers_state_terminated": 0,
						"node_node01_init_containers_state_waiting":    0,
						"node_node01_pods_cond_containersready":        0,
						"node_node01_pods_cond_podinitialized":         0,
						"node_node01_pods_cond_podready":               0,
						"node_node01_pods_cond_podscheduled":           0,
						"node_node01_pods_phase_failed":                0,
						"node_node01_pods_phase_pending":               0,
						"node_node01_pods_phase_running":               0,
						"node_node01_pods_phase_succeeded":             0,
						"node_node01_pods_readiness":                   0,
						"node_node01_pods_readiness_ready":             0,
						"node_node01_pods_readiness_unready":           0,
					}
					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(nodeChartsTmpl)+len(podChartsTmpl)+len(containerChartsTmpl)*len(pod.Spec.Containers)+len(baseCharts),
						len(*ks.Charts()),
					)
					assert.Equal(t,
						len(podChartsTmpl)+len(containerChartsTmpl)*len(pod.Spec.Containers),
						calcObsoleteCharts(*ks.Charts()),
					)
					module.TestMetricsHasAllChartsDims(t, ks.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"slow spec.NodeName set": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				node := newNode("node01")
				podOrig := newPod(node.Name, "pod01")
				podOrig.Spec.NodeName = ""
				client := fake.NewClientset(
					node,
					podOrig,
				)
				podUpdated := newPod(node.Name, "pod01") // with set Spec.NodeName

				step1 := func(t *testing.T, ks *KubeState) {
					_ = ks.Collect()
					for _, c := range *ks.Charts() {
						if strings.HasPrefix(c.ID, "pod_") {
							ok := isLabelValueSet(c, labelKeyNodeName)
							assert.Falsef(t, ok, "chart '%s' has no empty %s label", c.ID, labelKeyNodeName)
						}
					}
				}
				step2 := func(t *testing.T, ks *KubeState) {
					_, _ = client.CoreV1().Pods(podOrig.Namespace).Update(ctx, podUpdated, metav1.UpdateOptions{})
					time.Sleep(time.Millisecond * 50)
					_ = ks.Collect()

					for _, c := range *ks.Charts() {
						if strings.HasPrefix(c.ID, "pod_") {
							ok := isLabelValueSet(c, labelKeyNodeName)
							assert.Truef(t, ok, "chart '%s' has empty %s label", c.ID, labelKeyNodeName)
						}
					}
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"add a Pod in runtime": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				node := newNode("node01")
				pod1 := newPod(node.Name, "pod01")
				pod2 := newPod(node.Name, "pod02")
				client := fake.NewClientset(
					node,
					pod1,
				)
				step1 := func(t *testing.T, ks *KubeState) {
					_ = ks.Collect()
					_, _ = client.CoreV1().Pods(pod1.Namespace).Create(ctx, pod2, metav1.CreateOptions{})
				}

				step2 := func(t *testing.T, ks *KubeState) {
					mx := ks.Collect()
					expected := map[string]int64{
						"discovery_node_discoverer_state":                                                        1,
						"discovery_pod_discoverer_state":                                                         1,
						"node_node01_age":                                                                        4,
						"node_node01_alloc_cpu_limits_used":                                                      800,
						"node_node01_alloc_cpu_limits_util":                                                      22857,
						"node_node01_alloc_cpu_requests_used":                                                    400,
						"node_node01_alloc_cpu_requests_util":                                                    11428,
						"node_node01_alloc_mem_limits_used":                                                      838860800,
						"node_node01_alloc_mem_limits_util":                                                      22857,
						"node_node01_alloc_mem_requests_used":                                                    419430400,
						"node_node01_alloc_mem_requests_util":                                                    11428,
						"node_node01_alloc_pods_allocated":                                                       2,
						"node_node01_alloc_pods_available":                                                       108,
						"node_node01_alloc_pods_util":                                                            1818,
						"node_node01_cond_diskpressure":                                                          0,
						"node_node01_cond_memorypressure":                                                        0,
						"node_node01_cond_networkunavailable":                                                    0,
						"node_node01_cond_pidpressure":                                                           0,
						"node_node01_cond_ready":                                                                 1,
						"node_node01_containers":                                                                 4,
						"node_node01_containers_state_running":                                                   4,
						"node_node01_containers_state_terminated":                                                0,
						"node_node01_containers_state_waiting":                                                   0,
						"node_node01_init_containers":                                                            2,
						"node_node01_init_containers_state_running":                                              0,
						"node_node01_init_containers_state_terminated":                                           2,
						"node_node01_init_containers_state_waiting":                                              0,
						"node_node01_pods_cond_containersready":                                                  2,
						"node_node01_pods_cond_podinitialized":                                                   2,
						"node_node01_pods_cond_podready":                                                         2,
						"node_node01_pods_cond_podscheduled":                                                     2,
						"node_node01_pods_phase_failed":                                                          0,
						"node_node01_pods_phase_pending":                                                         0,
						"node_node01_pods_phase_running":                                                         2,
						"node_node01_pods_phase_succeeded":                                                       0,
						"node_node01_pods_readiness":                                                             100000,
						"node_node01_pods_readiness_ready":                                                       2,
						"node_node01_pods_readiness_unready":                                                     0,
						"node_node01_schedulability_schedulable":                                                 1,
						"node_node01_schedulability_unschedulable":                                               0,
						"pod_default_pod01_age":                                                                  4,
						"pod_default_pod01_cond_containersready":                                                 1,
						"pod_default_pod01_cond_podinitialized":                                                  1,
						"pod_default_pod01_cond_podready":                                                        1,
						"pod_default_pod01_cond_podscheduled":                                                    1,
						"pod_default_pod01_container_container1_readiness":                                       1,
						"pod_default_pod01_container_container1_restarts":                                        0,
						"pod_default_pod01_container_container1_state_running":                                   1,
						"pod_default_pod01_container_container1_state_terminated":                                0,
						"pod_default_pod01_container_container1_state_terminated_reason_Completed":               0,
						"pod_default_pod01_container_container1_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod01_container_container1_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod01_container_container1_state_terminated_reason_Error":                   0,
						"pod_default_pod01_container_container1_state_terminated_reason_Evicted":                 0,
						"pod_default_pod01_container_container1_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod01_container_container1_state_terminated_reason_Other":                   0,
						"pod_default_pod01_container_container1_state_waiting":                                   0,
						"pod_default_pod01_container_container1_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod01_container_container1_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod01_container_container1_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod01_container_container1_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod01_container_container1_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod01_container_container1_state_waiting_reason_Other":                      0,
						"pod_default_pod01_container_container1_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod01_container_container2_readiness":                                       1,
						"pod_default_pod01_container_container2_restarts":                                        0,
						"pod_default_pod01_container_container2_state_running":                                   1,
						"pod_default_pod01_container_container2_state_terminated":                                0,
						"pod_default_pod01_container_container2_state_terminated_reason_Completed":               0,
						"pod_default_pod01_container_container2_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod01_container_container2_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod01_container_container2_state_terminated_reason_Error":                   0,
						"pod_default_pod01_container_container2_state_terminated_reason_Evicted":                 0,
						"pod_default_pod01_container_container2_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod01_container_container2_state_terminated_reason_Other":                   0,
						"pod_default_pod01_container_container2_state_waiting":                                   0,
						"pod_default_pod01_container_container2_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod01_container_container2_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod01_container_container2_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod01_container_container2_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod01_container_container2_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod01_container_container2_state_waiting_reason_Other":                      0,
						"pod_default_pod01_container_container2_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod01_containers":                                                           2,
						"pod_default_pod01_containers_state_running":                                             2,
						"pod_default_pod01_containers_state_terminated":                                          0,
						"pod_default_pod01_containers_state_waiting":                                             0,
						"pod_default_pod01_cpu_limits_used":                                                      400,
						"pod_default_pod01_cpu_requests_used":                                                    200,
						"pod_default_pod01_init_containers":                                                      1,
						"pod_default_pod01_init_containers_state_running":                                        0,
						"pod_default_pod01_init_containers_state_terminated":                                     1,
						"pod_default_pod01_init_containers_state_waiting":                                        0,
						"pod_default_pod01_mem_limits_used":                                                      419430400,
						"pod_default_pod01_mem_requests_used":                                                    209715200,
						"pod_default_pod01_phase_failed":                                                         0,
						"pod_default_pod01_phase_pending":                                                        0,
						"pod_default_pod01_phase_running":                                                        1,
						"pod_default_pod01_phase_succeeded":                                                      0,
						"pod_default_pod02_age":                                                                  4,
						"pod_default_pod02_cond_containersready":                                                 1,
						"pod_default_pod02_cond_podinitialized":                                                  1,
						"pod_default_pod02_cond_podready":                                                        1,
						"pod_default_pod02_cond_podscheduled":                                                    1,
						"pod_default_pod02_container_container1_readiness":                                       1,
						"pod_default_pod02_container_container1_restarts":                                        0,
						"pod_default_pod02_container_container1_state_running":                                   1,
						"pod_default_pod02_container_container1_state_terminated":                                0,
						"pod_default_pod02_container_container1_state_terminated_reason_Completed":               0,
						"pod_default_pod02_container_container1_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod02_container_container1_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod02_container_container1_state_terminated_reason_Error":                   0,
						"pod_default_pod02_container_container1_state_terminated_reason_Evicted":                 0,
						"pod_default_pod02_container_container1_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod02_container_container1_state_terminated_reason_Other":                   0,
						"pod_default_pod02_container_container1_state_waiting":                                   0,
						"pod_default_pod02_container_container1_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod02_container_container1_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod02_container_container1_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod02_container_container1_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod02_container_container1_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod02_container_container1_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod02_container_container1_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod02_container_container1_state_waiting_reason_Other":                      0,
						"pod_default_pod02_container_container1_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod02_container_container2_readiness":                                       1,
						"pod_default_pod02_container_container2_restarts":                                        0,
						"pod_default_pod02_container_container2_state_running":                                   1,
						"pod_default_pod02_container_container2_state_terminated":                                0,
						"pod_default_pod02_container_container2_state_terminated_reason_Completed":               0,
						"pod_default_pod02_container_container2_state_terminated_reason_ContainerCannotRun":      0,
						"pod_default_pod02_container_container2_state_terminated_reason_DeadlineExceeded":        0,
						"pod_default_pod02_container_container2_state_terminated_reason_Error":                   0,
						"pod_default_pod02_container_container2_state_terminated_reason_Evicted":                 0,
						"pod_default_pod02_container_container2_state_terminated_reason_OOMKilled":               0,
						"pod_default_pod02_container_container2_state_terminated_reason_Other":                   0,
						"pod_default_pod02_container_container2_state_waiting":                                   0,
						"pod_default_pod02_container_container2_state_waiting_reason_ContainerCreating":          0,
						"pod_default_pod02_container_container2_state_waiting_reason_CrashLoopBackOff":           0,
						"pod_default_pod02_container_container2_state_waiting_reason_CreateContainerConfigError": 0,
						"pod_default_pod02_container_container2_state_waiting_reason_CreateContainerError":       0,
						"pod_default_pod02_container_container2_state_waiting_reason_ErrImagePull":               0,
						"pod_default_pod02_container_container2_state_waiting_reason_ImagePullBackOff":           0,
						"pod_default_pod02_container_container2_state_waiting_reason_InvalidImageName":           0,
						"pod_default_pod02_container_container2_state_waiting_reason_Other":                      0,
						"pod_default_pod02_container_container2_state_waiting_reason_PodInitializing":            0,
						"pod_default_pod02_containers":                                                           2,
						"pod_default_pod02_containers_state_running":                                             2,
						"pod_default_pod02_containers_state_terminated":                                          0,
						"pod_default_pod02_containers_state_waiting":                                             0,
						"pod_default_pod02_cpu_limits_used":                                                      400,
						"pod_default_pod02_cpu_requests_used":                                                    200,
						"pod_default_pod02_init_containers":                                                      1,
						"pod_default_pod02_init_containers_state_running":                                        0,
						"pod_default_pod02_init_containers_state_terminated":                                     1,
						"pod_default_pod02_init_containers_state_waiting":                                        0,
						"pod_default_pod02_mem_limits_used":                                                      419430400,
						"pod_default_pod02_mem_requests_used":                                                    209715200,
						"pod_default_pod02_phase_failed":                                                         0,
						"pod_default_pod02_phase_pending":                                                        0,
						"pod_default_pod02_phase_running":                                                        1,
						"pod_default_pod02_phase_succeeded":                                                      0,
					}
					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(nodeChartsTmpl)+
							len(podChartsTmpl)*2+
							len(containerChartsTmpl)*len(pod1.Spec.Containers)+
							len(containerChartsTmpl)*len(pod2.Spec.Containers)+
							len(baseCharts),
						len(*ks.Charts()),
					)
					module.TestMetricsHasAllChartsDims(t, ks.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
	}

	for name, creator := range tests {
		t.Run(name, func(t *testing.T) {
			test := creator.create(t)

			ks := New()
			ks.newKubeClient = func() (kubernetes.Interface, error) { return test.client, nil }

			require.NoError(t, ks.Init())
			require.NoError(t, ks.Check())
			defer ks.Cleanup()

			for i, executeStep := range test.steps {
				if i == 0 {
					_ = ks.Collect()
					time.Sleep(ks.initDelay)
				} else {
					time.Sleep(time.Second)
				}
				executeStep(t, ks)
			}
		})
	}
}

func newNode(name string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    mustQuantity("4000m"),
				corev1.ResourceMemory: mustQuantity("4000Mi"),
				"pods":                mustQuantity("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    mustQuantity("3500m"),
				corev1.ResourceMemory: mustQuantity("3500Mi"),
				"pods":                mustQuantity("110"),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodePIDPressure, Status: corev1.ConditionFalse},
				{Type: corev1.NodeNetworkUnavailable, Status: corev1.ConditionFalse},
			},
		},
	}
}

func newPod(nodeName, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         corev1.NamespaceDefault,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			InitContainers: []corev1.Container{
				{
					Name: "init-container1",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    mustQuantity("50m"),
							corev1.ResourceMemory: mustQuantity("50Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    mustQuantity("10m"),
							corev1.ResourceMemory: mustQuantity("10Mi"),
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name: "container1",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    mustQuantity("200m"),
							corev1.ResourceMemory: mustQuantity("200Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    mustQuantity("100m"),
							corev1.ResourceMemory: mustQuantity("100Mi"),
						},
					},
				},
				{
					Name: "container2",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    mustQuantity("200m"),
							corev1.ResourceMemory: mustQuantity("200Mi")},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    mustQuantity("100m"),
							corev1.ResourceMemory: mustQuantity("100Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
				{Type: corev1.PodInitialized, Status: corev1.ConditionTrue},
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue},
			},
			InitContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "init-container1",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}},
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "container1",
					Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
				{
					Name:  "container2",
					Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				},
			},
		},
	}
}

type brokenInfoKubeClient struct {
	kubernetes.Interface
}

func (kc *brokenInfoKubeClient) Discovery() discovery.DiscoveryInterface {
	return &brokenInfoDiscovery{kc.Interface.Discovery()}
}

type brokenInfoDiscovery struct {
	discovery.DiscoveryInterface
}

func (d *brokenInfoDiscovery) ServerVersion() (*version.Info, error) {
	return nil, errors.New("brokenInfoDiscovery.ServerVersion() error")
}

func calcObsoleteCharts(charts module.Charts) (num int) {
	for _, c := range charts {
		if c.Obsolete {
			num++
		}
	}
	return num
}

func mustQuantity(s string) apiresource.Quantity {
	q, err := apiresource.ParseQuantity(s)
	if err != nil {
		panic(fmt.Sprintf("fail to create resource quantity: %v", err))
	}
	return q
}

func copyAge(dst, src map[string]int64) {
	for k, v := range src {
		if !strings.HasSuffix(k, "_age") {
			continue
		}
		if _, ok := dst[k]; ok {
			dst[k] = v
		}
	}
}

func isLabelValueSet(c *module.Chart, name string) bool {
	for _, l := range c.Labels {
		if l.Key == name {
			return l.Value != ""
		}
	}
	return false
}
