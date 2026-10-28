// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func() *Collector
	}{
		"success when no error on initializing K8s client": {
			wantFail: false,
			prepare: func() *Collector {
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return collr
			},
		},
		"fail when get an error on initializing K8s client": {
			wantFail: true,
			prepare: func() *Collector {
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return nil, errors.New("newKubeClient() error") }
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

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
		wantFail bool
		prepare  func() *Collector
	}{
		"success when connected to the K8s API": {
			wantFail: false,
			prepare: func() *Collector {
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return collr
			},
		},
		"fail when not connected to the K8s API": {
			wantFail: true,
			prepare: func() *Collector {
				collr := New()
				client := fake.NewClientset()
				client.PrependReactor("get", "version", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("ServerVersion() error")
				})
				collr.newKubeClient = func() (kubernetes.Interface, error) { return client, nil }
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()
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
	collr := New()

	assert.NotEmpty(t, *collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func() *Collector
		doInit    bool
		doCollect bool
		wantPVC   bool
	}{
		"before init": {
			doInit:    false,
			doCollect: false,
			prepare: func() *Collector {
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return collr
			},
		},
		"after init": {
			doInit:    true,
			doCollect: false,
			prepare: func() *Collector {
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return collr
			},
		},
		"after collect": {
			doInit:    true,
			doCollect: true,
			prepare: func() *Collector {
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				return collr
			},
		},
		"after collect with the PVC discoverer": {
			doInit:    true,
			doCollect: true,
			wantPVC:   true,
			prepare: func() *Collector {
				client := fake.NewClientset()
				allowPVCAccess(client, "list", "watch")
				collr := New()
				collr.newKubeClient = func() (kubernetes.Interface, error) { return client, nil }
				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			if test.doInit {
				_ = collr.Init(context.Background())
			}
			if test.doCollect {
				_ = collr.Collect(context.Background())
				time.Sleep(collr.initDelay)
			}

			// The PVC discoverer is optional: without it stopped() would pass without checking it.
			var pvcd *pvcDiscoverer
			if test.doCollect {
				kd, ok := collr.discoverer.(*kubeDiscovery)
				require.True(t, ok)
				require.True(t, kd.ready()) // also makes kd.discoverers safe to read
				for _, d := range kd.discoverers {
					if v, ok := d.(*pvcDiscoverer); ok {
						pvcd = v
					}
				}
				require.Equal(t, test.wantPVC, pvcd != nil)
			}

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			time.Sleep(time.Second)
			if test.doCollect {
				assert.True(t, collr.discoverer.stopped())
			}
			if pvcd != nil {
				assert.True(t, pvcd.stopped())
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	type (
		testCaseStep func(t *testing.T, ks *Collector)
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

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
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
						"node_node01_cond_DiskPressure":                0,
						"node_node01_cond_MemoryPressure":              0,
						"node_node01_cond_NetworkUnavailable":          0,
						"node_node01_cond_PIDPressure":                 0,
						"node_node01_cond_Ready":                       1,
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
						len(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
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
						"pod_default_pod01_status_reason_Evicted":                                                0,
						"pod_default_pod01_status_reason_NodeAffinity":                                           0,
						"pod_default_pod01_status_reason_NodeLost":                                               0,
						"pod_default_pod01_status_reason_Other":                                                  0,
						"pod_default_pod01_status_reason_Shutdown":                                               0,
						"pod_default_pod01_status_reason_UnexpectedAdmissionError":                               0,
					}

					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(podChartsTmpl)+len(containerChartsTmpl)*len(pod.Spec.Containers)+len(baseCharts),
						len(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"Nodes and Pods and Deployment": {
			create: func(t *testing.T) testCase {
				node := newNode("node01")
				pod := newPod(node.Name, "pod01")
				deploy := newDeployment("replicaset01")
				client := fake.NewClientset(
					node,
					pod,
					deploy,
				)

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"deploy_default_replicaset01_age":                                                        3,
						"deploy_default_replicaset01_condition_available":                                        1,
						"deploy_default_replicaset01_condition_progressing":                                      1,
						"deploy_default_replicaset01_condition_replica_failure":                                  1,
						"deploy_default_replicaset01_current_replicas":                                           1,
						"deploy_default_replicaset01_desired_replicas":                                           2,
						"deploy_default_replicaset01_ready_replicas":                                             3,
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
						"node_node01_cond_DiskPressure":                                                          0,
						"node_node01_cond_MemoryPressure":                                                        0,
						"node_node01_cond_NetworkUnavailable":                                                    0,
						"node_node01_cond_PIDPressure":                                                           0,
						"node_node01_cond_Ready":                                                                 1,
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
						"pod_default_pod01_status_reason_Evicted":                                                0,
						"pod_default_pod01_status_reason_NodeAffinity":                                           0,
						"pod_default_pod01_status_reason_NodeLost":                                               0,
						"pod_default_pod01_status_reason_Other":                                                  0,
						"pod_default_pod01_status_reason_Shutdown":                                               0,
						"pod_default_pod01_status_reason_UnexpectedAdmissionError":                               0,
					}

					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(nodeChartsTmpl)+
							len(podChartsTmpl)+
							len(containerChartsTmpl)*len(pod.Spec.Containers)+
							len(deploymentChartsTmpl)+
							len(baseCharts),
						len(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"CronJobs": {
			create: func(t *testing.T) testCase {
				cj := prepareCronJob("cronjob01")
				cjSuspended := prepareCronJob("cronjob02")
				cjSuspended.Spec.Suspend = new(true)
				jobNotStarted := prepareCronJobNotStartedJob("job-not-started", cj)
				jobComplete := prepareCronJobCompleteJob("job-complete", cj)
				jobFailed := prepareCronJobFailedJob("job-failed", cj)
				jobRunning := prepareCronJobRunningJob("job-running", cj)
				jobSuspended := prepareCronJobSuspendedJob("job-suspended", cj)

				client := fake.NewClientset(
					cjSuspended,
					cj,
					jobNotStarted,
					jobComplete,
					jobFailed,
					jobRunning,
					jobSuspended,
				)

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"cronjob_default_cronjob01_age":                                       10,
						"cronjob_default_cronjob01_complete_jobs":                             1,
						"cronjob_default_cronjob01_failed_jobs":                               1,
						"cronjob_default_cronjob01_failed_jobs_reason_backoff_limit_exceeded": 0,
						"cronjob_default_cronjob01_failed_jobs_reason_deadline_exceeded":      1,
						"cronjob_default_cronjob01_failed_jobs_reason_pod_failure_policy":     0,
						"cronjob_default_cronjob01_last_completion_duration":                  60,
						"cronjob_default_cronjob01_last_execution_status_failed":              0,
						"cronjob_default_cronjob01_last_execution_status_succeeded":           1,
						"cronjob_default_cronjob01_last_schedule_seconds_ago":                 130,
						"cronjob_default_cronjob01_last_successful_seconds_ago":               70,
						"cronjob_default_cronjob01_running_jobs":                              1,
						"cronjob_default_cronjob01_suspend_status_enabled":                    1,
						"cronjob_default_cronjob01_suspend_status_suspended":                  0,
						"cronjob_default_cronjob01_suspended_jobs":                            1,
						"cronjob_default_cronjob02_age":                                       10,
						"cronjob_default_cronjob02_complete_jobs":                             0,
						"cronjob_default_cronjob02_failed_jobs":                               0,
						"cronjob_default_cronjob02_failed_jobs_reason_backoff_limit_exceeded": 0,
						"cronjob_default_cronjob02_failed_jobs_reason_deadline_exceeded":      0,
						"cronjob_default_cronjob02_failed_jobs_reason_pod_failure_policy":     0,
						"cronjob_default_cronjob02_last_execution_status_failed":              0,
						"cronjob_default_cronjob02_last_execution_status_succeeded":           0,
						"cronjob_default_cronjob02_last_schedule_seconds_ago":                 130,
						"cronjob_default_cronjob02_last_successful_seconds_ago":               70,
						"cronjob_default_cronjob02_running_jobs":                              0,
						"cronjob_default_cronjob02_suspend_status_enabled":                    0,
						"cronjob_default_cronjob02_suspend_status_suspended":                  1,
						"cronjob_default_cronjob02_suspended_jobs":                            0,
						"discovery_node_discoverer_state":                                     1,
						"discovery_pod_discoverer_state":                                      1,
					}

					copyIfSuffix(expected, mx, "age", "ago")

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(cronJobChartsTmpl)*2+
							len(baseCharts),
						len(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, func(chart *collectorapi.Chart, dim *collectorapi.Dim) bool {
						return strings.Contains(chart.ID, cjSuspended.Name) && strings.HasSuffix(chart.ID, "last_completion_duration")
					})
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
				step1 := func(t *testing.T, collr *Collector) {
					_ = collr.Collect(context.Background())
					_ = client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
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
						"node_node01_cond_DiskPressure":                0,
						"node_node01_cond_MemoryPressure":              0,
						"node_node01_cond_NetworkUnavailable":          0,
						"node_node01_cond_PIDPressure":                 0,
						"node_node01_cond_Ready":                       1,
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
						len(*collr.Charts()),
					)
					assert.Equal(t,
						len(podChartsTmpl)+len(containerChartsTmpl)*len(pod.Spec.Containers),
						calcObsoleteCharts(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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

				step1 := func(t *testing.T, collr *Collector) {
					_ = collr.Collect(context.Background())
					for _, c := range *collr.Charts() {
						if strings.HasPrefix(c.ID, "pod_") {
							ok := isLabelValueSet(c, labelKeyNodeName)
							assert.Falsef(t, ok, "chart '%s' has no empty %s label", c.ID, labelKeyNodeName)
						}
					}
				}
				step2 := func(t *testing.T, collr *Collector) {
					_, _ = client.CoreV1().Pods(podOrig.Namespace).Update(ctx, podUpdated, metav1.UpdateOptions{})
					time.Sleep(time.Millisecond * 50)
					_ = collr.Collect(context.Background())

					for _, c := range *collr.Charts() {
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
				step1 := func(t *testing.T, collr *Collector) {
					_ = collr.Collect(context.Background())
					_, _ = client.CoreV1().Pods(pod1.Namespace).Create(ctx, pod2, metav1.CreateOptions{})
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
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
						"node_node01_cond_DiskPressure":                                                          0,
						"node_node01_cond_MemoryPressure":                                                        0,
						"node_node01_cond_NetworkUnavailable":                                                    0,
						"node_node01_cond_PIDPressure":                                                           0,
						"node_node01_cond_Ready":                                                                 1,
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
						"pod_default_pod01_status_reason_Evicted":                                                0,
						"pod_default_pod01_status_reason_NodeAffinity":                                           0,
						"pod_default_pod01_status_reason_NodeLost":                                               0,
						"pod_default_pod01_status_reason_Other":                                                  0,
						"pod_default_pod01_status_reason_Shutdown":                                               0,
						"pod_default_pod01_status_reason_UnexpectedAdmissionError":                               0,
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
						"pod_default_pod02_status_reason_Evicted":                                                0,
						"pod_default_pod02_status_reason_NodeAffinity":                                           0,
						"pod_default_pod02_status_reason_NodeLost":                                               0,
						"pod_default_pod02_status_reason_Other":                                                  0,
						"pod_default_pod02_status_reason_Shutdown":                                               0,
						"pod_default_pod02_status_reason_UnexpectedAdmissionError":                               0,
					}

					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(nodeChartsTmpl)+
							len(podChartsTmpl)*2+
							len(containerChartsTmpl)*len(pod1.Spec.Containers)+
							len(containerChartsTmpl)*len(pod2.Spec.Containers)+
							len(baseCharts),
						len(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"PVCs": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				bound := newPVC("bound", "10Gi", "10Gi")
				expanding := newPVC("expanding", "20Gi", "10Gi")
				pending := newPVC("pending", "5Gi", "")
				noRequest := newPVC("no-request", "", "1Gi")
				client := fake.NewClientset(bound, expanding, pending, noRequest)
				allowPVCAccess(client, "list", "watch")

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"discovery_node_discoverer_state":               1,
						"discovery_pod_discoverer_state":                1,
						"pvc_default_bound_capacity":                    10737418240,
						"pvc_default_bound_requested":                   10737418240,
						"pvc_default_bound_requested_capacity_diff":     0,
						"pvc_default_expanding_capacity":                10737418240,
						"pvc_default_expanding_requested":               21474836480,
						"pvc_default_expanding_requested_capacity_diff": 10737418240,
						"pvc_default_no-request_capacity":               1073741824,
						"pvc_default_pending_requested":                 5368709120,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*4+
							len(baseCharts),
						len(*collr.Charts()),
					)
					// A value that is not set is a gap: a Pending PVC has no capacity, this one has no request.
					missing := []string{
						"pvc_default_pending_capacity",
						"pvc_default_pending_requested_capacity_diff",
						"pvc_default_no-request_requested",
						"pvc_default_no-request_requested_capacity_diff",
					}
					skip := func(_ *collectorapi.Chart, dim *collectorapi.Dim) bool {
						return slices.Contains(missing, dim.ID)
					}
					collecttest.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, skip)

					chart := collr.Charts().Get("pvc_default_bound.capacity")
					require.NotNil(t, chart)
					assert.Equal(t, []collectorapi.Label{
						{Key: "k8s_cluster_id", Value: collr.kubeClusterID, Source: collectorapi.LabelSourceK8s},
						{Key: "k8s_cluster_name", Value: collr.kubeClusterName, Source: collectorapi.LabelSourceK8s},
						{Key: "k8s_namespace", Value: "default", Source: collectorapi.LabelSourceK8s},
						{Key: "k8s_pvc_name", Value: "bound", Source: collectorapi.LabelSourceK8s},
					}, chart.Labels)
					assert.Equal(t, "PVC Capacity", chart.Title)
					assert.Equal(t, "k8s_state.pvc_capacity", chart.Ctx)
					var dimIDs, dimNames []string
					for _, dim := range chart.Dims {
						dimIDs = append(dimIDs, dim.ID)
						dimNames = append(dimNames, dim.Name)
					}
					assert.Equal(t, []string{
						"pvc_default_bound_requested",
						"pvc_default_bound_capacity",
						"pvc_default_bound_requested_capacity_diff",
					}, dimIDs)
					assert.Equal(t, []string{"requested", "capacity", "requested_capacity_diff"}, dimNames)

					// The pending PVC gets bound, the expansion of the other one completes.
					pvcs := client.CoreV1().PersistentVolumeClaims(corev1.NamespaceDefault)
					_, _ = pvcs.Update(ctx, newPVC("pending", "5Gi", "5Gi"), metav1.UpdateOptions{})
					_, _ = pvcs.Update(ctx, newPVC("expanding", "20Gi", "20Gi"), metav1.UpdateOptions{})
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"discovery_node_discoverer_state":               1,
						"discovery_pod_discoverer_state":                1,
						"pvc_default_bound_capacity":                    10737418240,
						"pvc_default_bound_requested":                   10737418240,
						"pvc_default_bound_requested_capacity_diff":     0,
						"pvc_default_expanding_capacity":                21474836480,
						"pvc_default_expanding_requested":               21474836480,
						"pvc_default_expanding_requested_capacity_diff": 0,
						"pvc_default_no-request_capacity":               1073741824,
						"pvc_default_pending_capacity":                  5368709120,
						"pvc_default_pending_requested":                 5368709120,
						"pvc_default_pending_requested_capacity_diff":   0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*4+
							len(baseCharts),
						len(*collr.Charts()),
					)
					assert.Equal(t, 0, calcObsoleteCharts(*collr.Charts()))
					missing := []string{
						"pvc_default_no-request_requested",
						"pvc_default_no-request_requested_capacity_diff",
					}
					skip := func(_ *collectorapi.Chart, dim *collectorapi.Dim) bool {
						return slices.Contains(missing, dim.ID)
					}
					collecttest.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, skip)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"PVC requested capacity diff": {
			create: func(t *testing.T) testCase {
				client := fake.NewClientset(
					newPVC("equal-2gi", "2Gi", "2Gi"),
					newPVC("equal-8gi", "8Gi", "8Gi"),
					newPVC("equal-20gi-plus-1b", "21474836481", "21474836481"), // 20Gi+1 byte
					newPVC("short-by-1b", "21474836481", "20Gi"),
					newPVC("larger-capacity", "1Gi", "2Gi"),
					newPVC("pending", "5Gi", ""),
					newPVC("no-request", "", "1Gi"),
					newPVC("max-requested", "9223372036854775807", "1"), // math.MaxInt64 bytes
					newPVC("max-capacity", "1", "9223372036854775807"),
				)
				allowPVCAccess(client, "list", "watch")

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"discovery_node_discoverer_state":                        1,
						"discovery_pod_discoverer_state":                         1,
						"pvc_default_equal-20gi-plus-1b_capacity":                21474836481,
						"pvc_default_equal-20gi-plus-1b_requested":               21474836481,
						"pvc_default_equal-20gi-plus-1b_requested_capacity_diff": 0,
						"pvc_default_equal-2gi_capacity":                         2147483648,
						"pvc_default_equal-2gi_requested":                        2147483648,
						"pvc_default_equal-2gi_requested_capacity_diff":          0,
						"pvc_default_equal-8gi_capacity":                         8589934592,
						"pvc_default_equal-8gi_requested":                        8589934592,
						"pvc_default_equal-8gi_requested_capacity_diff":          0,
						"pvc_default_larger-capacity_capacity":                   2147483648,
						"pvc_default_larger-capacity_requested":                  1073741824,
						"pvc_default_larger-capacity_requested_capacity_diff":    -1073741824,
						"pvc_default_max-capacity_capacity":                      9223372036854775807,
						"pvc_default_max-capacity_requested":                     1,
						"pvc_default_max-capacity_requested_capacity_diff":       -9223372036854775806,
						"pvc_default_max-requested_capacity":                     1,
						"pvc_default_max-requested_requested":                    9223372036854775807,
						"pvc_default_max-requested_requested_capacity_diff":      9223372036854775806,
						"pvc_default_no-request_capacity":                        1073741824,
						"pvc_default_pending_requested":                          5368709120,
						"pvc_default_short-by-1b_capacity":                       21474836480,
						"pvc_default_short-by-1b_requested":                      21474836481,
						"pvc_default_short-by-1b_requested_capacity_diff":        1,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*9+
							len(baseCharts),
						len(*collr.Charts()),
					)
					// A value that is not set is a gap, so is the difference that needs it.
					missing := []string{
						"pvc_default_pending_capacity",
						"pvc_default_pending_requested_capacity_diff",
						"pvc_default_no-request_requested",
						"pvc_default_no-request_requested_capacity_diff",
					}
					skip := func(_ *collectorapi.Chart, dim *collectorapi.Dim) bool {
						return slices.Contains(missing, dim.ID)
					}
					collecttest.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, skip)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"PVC requested_capacity_diff stays positive until the capacity catches up": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				client := fake.NewClientset(newPVC("healthy", "8Gi", "8Gi"), newPVC("stuck", "8Gi", "8Gi"))
				allowPVCAccess(client, "list", "watch")
				pvcs := client.CoreV1().PersistentVolumeClaims(corev1.NamespaceDefault)

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":             1,
						"discovery_pod_discoverer_state":              1,
						"pvc_default_healthy_capacity":                8589934592,
						"pvc_default_healthy_requested":               8589934592,
						"pvc_default_healthy_requested_capacity_diff": 0,
						"pvc_default_stuck_capacity":                  8589934592,
						"pvc_default_stuck_requested":                 8589934592,
						"pvc_default_stuck_requested_capacity_diff":   0,
					}

					assert.Equal(t, expected, mx)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

					// The request grows, the volume does not follow.
					_, _ = pvcs.Update(ctx, newPVC("stuck", "10Gi", "8Gi"), metav1.UpdateOptions{})
				}

				step2 := func(t *testing.T, collr *Collector) {
					expected := map[string]int64{
						"discovery_node_discoverer_state":             1,
						"discovery_pod_discoverer_state":              1,
						"pvc_default_healthy_capacity":                8589934592,
						"pvc_default_healthy_requested":               8589934592,
						"pvc_default_healthy_requested_capacity_diff": 0,
						"pvc_default_stuck_capacity":                  8589934592,
						"pvc_default_stuck_requested":                 10737418240,
						"pvc_default_stuck_requested_capacity_diff":   2147483648,
					}

					// The positive requested_capacity_diff is reported on every collection while the volume is not
					// expanded.
					for range 2 {
						mx := collr.Collect(ctx)
						assert.Equal(t, expected, mx)
						collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
					}

					// The volume is expanded.
					_, _ = pvcs.Update(ctx, newPVC("stuck", "10Gi", "10Gi"), metav1.UpdateOptions{})
				}

				step3 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":             1,
						"discovery_pod_discoverer_state":              1,
						"pvc_default_healthy_capacity":                8589934592,
						"pvc_default_healthy_requested":               8589934592,
						"pvc_default_healthy_requested_capacity_diff": 0,
						"pvc_default_stuck_capacity":                  10737418240,
						"pvc_default_stuck_requested":                 10737418240,
						"pvc_default_stuck_requested_capacity_diff":   0,
					}

					assert.Equal(t, expected, mx)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2, step3},
				}
			},
		},
		"delete a PVC in runtime": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				pvc := newPVC("data", "10Gi", "10Gi")
				pvcSibling := newPVC("data-old", "20Gi", "20Gi") // "default_data" is a prefix of "default_data-old"
				client := fake.NewClientset(pvc, pvcSibling)
				allowPVCAccess(client, "list", "watch")

				step1 := func(t *testing.T, collr *Collector) {
					_ = collr.Collect(context.Background())
					pvcs := client.CoreV1().PersistentVolumeClaims(pvc.Namespace)
					_ = pvcs.Delete(ctx, pvc.Name, metav1.DeleteOptions{})
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"discovery_node_discoverer_state":              1,
						"discovery_pod_discoverer_state":               1,
						"pvc_default_data-old_capacity":                21474836480,
						"pvc_default_data-old_requested":               21474836480,
						"pvc_default_data-old_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*2+
							len(baseCharts),
						len(*collr.Charts()),
					)
					assert.Equal(t,
						len(pvcChartsTmpl),
						calcObsoleteCharts(*collr.Charts()),
					)
					obsolete := make(map[string]bool)
					for _, chart := range *collr.Charts() {
						obsolete[chart.ID] = chart.Obsolete
					}
					assert.Equal(t, map[string]bool{
						discoveryStatusChart.ID:         false,
						"pvc_default_data.capacity":     true,
						"pvc_default_data-old.capacity": false,
					}, obsolete)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"PVCs with dots in the name": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				dotted := newPVC("data.0", "10Gi", "10Gi")
				dashed := newPVC("data-0", "20Gi", "20Gi") // "data.0" with the dot replaced by a dash
				client := fake.NewClientset(dotted, dashed)
				allowPVCAccess(client, "list", "watch")

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":            1,
						"discovery_pod_discoverer_state":             1,
						"pvc_default_data-0_capacity":                21474836480,
						"pvc_default_data-0_requested":               21474836480,
						"pvc_default_data-0_requested_capacity_diff": 0,
						"pvc_default_data.0_capacity":                10737418240,
						"pvc_default_data.0_requested":               10737418240,
						"pvc_default_data.0_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*2+
							len(baseCharts),
						len(*collr.Charts()),
					)
					assert.Equal(t, map[string]string{
						"pvc_default_data_0.capacity": "default/data.0",
						"pvc_default_data-0.capacity": "default/data-0",
					}, pvcChartLabels(collr.Charts()))
					assert.Equal(t, 0, calcObsoleteCharts(*collr.Charts()))
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

					pvcs := client.CoreV1().PersistentVolumeClaims(corev1.NamespaceDefault)
					_ = pvcs.Delete(ctx, dotted.Name, metav1.DeleteOptions{})
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":            1,
						"discovery_pod_discoverer_state":             1,
						"pvc_default_data-0_capacity":                21474836480,
						"pvc_default_data-0_requested":               21474836480,
						"pvc_default_data-0_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*2+
							len(baseCharts),
						len(*collr.Charts()),
					)
					// Only the chart of the deleted PVC is obsoleted, the other one keeps its values.
					obsolete := make(map[string]bool)
					for _, chart := range *collr.Charts() {
						obsolete[chart.ID] = chart.Obsolete
					}
					assert.Equal(t, map[string]bool{
						discoveryStatusChart.ID:       false,
						"pvc_default_data_0.capacity": true,
						"pvc_default_data-0.capacity": false,
					}, obsolete)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"PVC in a non-default namespace": {
			create: func(t *testing.T) testCase {
				client := fake.NewClientset(
					newPVCInNamespace("team-a", "data.0", "30Gi", "30Gi"),
					newPVC("data.0", "10Gi", "10Gi"), // the same name in the default namespace
				)
				allowPVCAccess(client, "list", "watch")

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"discovery_node_discoverer_state":            1,
						"discovery_pod_discoverer_state":             1,
						"pvc_default_data.0_capacity":                10737418240,
						"pvc_default_data.0_requested":               10737418240,
						"pvc_default_data.0_requested_capacity_diff": 0,
						"pvc_team-a_data.0_capacity":                 32212254720,
						"pvc_team-a_data.0_requested":                32212254720,
						"pvc_team-a_data.0_requested_capacity_diff":  0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*2+
							len(baseCharts),
						len(*collr.Charts()),
					)
					assert.Equal(t, map[string]string{
						"pvc_default_data_0.capacity": "default/data.0",
						"pvc_team-a_data_0.capacity":  "team-a/data.0",
					}, pvcChartLabels(collr.Charts()))

					chart := collr.Charts().Get("pvc_team-a_data_0.capacity")
					require.NotNil(t, chart)
					assert.Equal(t, []collectorapi.Label{
						{Key: "k8s_cluster_id", Value: collr.kubeClusterID, Source: collectorapi.LabelSourceK8s},
						{Key: "k8s_cluster_name", Value: collr.kubeClusterName, Source: collectorapi.LabelSourceK8s},
						{Key: "k8s_namespace", Value: "team-a", Source: collectorapi.LabelSourceK8s},
						{Key: "k8s_pvc_name", Value: "data.0", Source: collectorapi.LabelSourceK8s},
					}, chart.Labels)
					var dimIDs []string
					for _, dim := range chart.Dims {
						dimIDs = append(dimIDs, dim.ID)
					}
					assert.Equal(t, []string{
						"pvc_team-a_data.0_requested",
						"pvc_team-a_data.0_capacity",
						"pvc_team-a_data.0_requested_capacity_diff",
					}, dimIDs)
					assert.Equal(t, 0, calcObsoleteCharts(*collr.Charts()))
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
		"delete and re-create a PVC between two collections": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				client := fake.NewClientset(newPVC("data", "10Gi", "10Gi"))
				allowPVCAccess(client, "list", "watch")
				pvcs := client.CoreV1().PersistentVolumeClaims(corev1.NamespaceDefault)
				source := pvcSource(corev1.NamespaceDefault, "data")

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":          1,
						"discovery_pod_discoverer_state":           1,
						"pvc_default_data_capacity":                10737418240,
						"pvc_default_data_requested":               10737418240,
						"pvc_default_data_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

					// Wait for the delete to reach the state first, else both events collapse into one update.
					require.NoError(t, pvcs.Delete(ctx, "data", metav1.DeleteOptions{}))
					requirePVCState(t, collr, source, func(ps *pvcState) bool { return ps.deleted })

					_, err := pvcs.Create(ctx, newPVC("data", "20Gi", "20Gi"), metav1.CreateOptions{})
					require.NoError(t, err)
					requirePVCState(t, collr, source, func(ps *pvcState) bool { return ps.requested == 21474836480 })
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":          1,
						"discovery_pod_discoverer_state":           1,
						"pvc_default_data_capacity":                21474836480,
						"pvc_default_data_requested":               21474836480,
						"pvc_default_data_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)+
							len(baseCharts),
						len(*collr.Charts()),
					)
					assert.Equal(t, 0, calcObsoleteCharts(*collr.Charts()))
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2},
				}
			},
		},
		"delete a PVC, collect, then re-create it": {
			create: func(t *testing.T) testCase {
				ctx := context.Background()
				client := fake.NewClientset(newPVC("data", "10Gi", "10Gi"))
				allowPVCAccess(client, "list", "watch")
				pvcs := client.CoreV1().PersistentVolumeClaims(corev1.NamespaceDefault)
				source := pvcSource(corev1.NamespaceDefault, "data")
				chartID := "pvc_default_data.capacity"

				// obsoleteFlags returns Obsolete of every chart with the ID, in order. The job runtime drops a removed
				// chart after emitting it, the test has no runtime, so the old chart stays before the re-created one.
				obsoleteFlags := func(collr *Collector) []bool {
					var flags []bool
					for _, chart := range *collr.Charts() {
						if chart.ID == chartID {
							flags = append(flags, chart.Obsolete)
						}
					}
					return flags
				}

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":          1,
						"discovery_pod_discoverer_state":           1,
						"pvc_default_data_capacity":                10737418240,
						"pvc_default_data_requested":               10737418240,
						"pvc_default_data_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t, []bool{false}, obsoleteFlags(collr))
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

					require.NoError(t, pvcs.Delete(ctx, "data", metav1.DeleteOptions{}))
					requirePVCState(t, collr, source, func(ps *pvcState) bool { return ps.deleted })
				}

				step2 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state": 1,
						"discovery_pod_discoverer_state":  1,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)+
							len(baseCharts),
						len(*collr.Charts()),
					)
					assert.Equal(t, []bool{true}, obsoleteFlags(collr))
					assert.Equal(t, len(pvcChartsTmpl), calcObsoleteCharts(*collr.Charts()))

					// The collection removed the state entry, so the next event creates a new one.
					_, err := pvcs.Create(ctx, newPVC("data", "20Gi", "20Gi"), metav1.CreateOptions{})
					require.NoError(t, err)
					requirePVCState(t, collr, source, func(ps *pvcState) bool { return ps.requested == 21474836480 })
				}

				step3 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(ctx)
					expected := map[string]int64{
						"discovery_node_discoverer_state":          1,
						"discovery_pod_discoverer_state":           1,
						"pvc_default_data_capacity":                21474836480,
						"pvc_default_data_requested":               21474836480,
						"pvc_default_data_requested_capacity_diff": 0,
					}

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(pvcChartsTmpl)*2+
							len(baseCharts),
						len(*collr.Charts()),
					)
					// The old chart stays obsolete, the re-created one is active.
					assert.Equal(t, []bool{true, false}, obsoleteFlags(collr))
					assert.Equal(t, len(pvcChartsTmpl), calcObsoleteCharts(*collr.Charts()))
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1, step2, step3},
				}
			},
		},
		"PVC access denied": {
			create: func(t *testing.T) testCase {
				deploy := newDeployment("replicaset01")
				client := fake.NewClientset(deploy, newPVC("data", "10Gi", "10Gi"))
				allowPVCAccess(client)
				forbidPVCs(client)

				step1 := func(t *testing.T, collr *Collector) {
					mx := collr.Collect(context.Background())
					expected := map[string]int64{
						"deploy_default_replicaset01_age":                       3,
						"deploy_default_replicaset01_condition_available":       1,
						"deploy_default_replicaset01_condition_progressing":     1,
						"deploy_default_replicaset01_condition_replica_failure": 1,
						"deploy_default_replicaset01_current_replicas":          1,
						"deploy_default_replicaset01_desired_replicas":          2,
						"deploy_default_replicaset01_ready_replicas":            3,
						"discovery_node_discoverer_state":                       1,
						"discovery_pod_discoverer_state":                        1,
					}

					copyAge(expected, mx)

					assert.Equal(t, expected, mx)
					assert.Equal(t,
						len(deploymentChartsTmpl)+
							len(baseCharts),
						len(*collr.Charts()),
					)
					collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)

					kd, ok := collr.discoverer.(*kubeDiscovery)
					require.True(t, ok)
					assert.Len(t, kd.discoverers, 5)
				}

				return testCase{
					client: client,
					steps:  []testCaseStep{step1},
				}
			},
		},
	}

	for name, creator := range tests {
		t.Run(name, func(t *testing.T) {
			test := creator.create(t)

			collr := New()
			collr.newKubeClient = func() (kubernetes.Interface, error) { return test.client, nil }

			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))
			defer collr.Cleanup(context.Background())

			for i, executeStep := range test.steps {
				if i == 0 {
					_ = collr.Collect(context.Background())
					time.Sleep(collr.initDelay)
				} else {
					time.Sleep(time.Second)
				}
				executeStep(t, collr)
			}
		})
	}
}

func TestKubeDiscovery_canListWatchPVCs(t *testing.T) {
	reviewList := authorizationv1.ResourceAttributes{
		Verb:     "list",
		Resource: "persistentvolumeclaims",
	}
	reviewWatch := authorizationv1.ResourceAttributes{
		Verb:     "watch",
		Resource: "persistentvolumeclaims",
	}

	tests := map[string]struct {
		prepare      func(client *fake.Clientset)
		wantAllowed  bool
		wantReviewed []authorizationv1.ResourceAttributes
		wantWarning  string
	}{
		"list and watch allowed": {
			prepare:      func(client *fake.Clientset) { allowPVCAccess(client, "list", "watch") },
			wantAllowed:  true,
			wantReviewed: []authorizationv1.ResourceAttributes{reviewList, reviewWatch},
		},
		"list and watch denied": {
			prepare:      func(client *fake.Clientset) { allowPVCAccess(client) },
			wantReviewed: []authorizationv1.ResourceAttributes{reviewList, reviewWatch},
			wantWarning:  "the service account cannot list/watch persistentvolumeclaims",
		},
		"list denied": {
			prepare:      func(client *fake.Clientset) { allowPVCAccess(client, "watch") },
			wantReviewed: []authorizationv1.ResourceAttributes{reviewList, reviewWatch},
			wantWarning:  "the service account cannot list persistentvolumeclaims",
		},
		"watch denied": {
			prepare:      func(client *fake.Clientset) { allowPVCAccess(client, "list") },
			wantReviewed: []authorizationv1.ResourceAttributes{reviewList, reviewWatch},
			wantWarning:  "the service account cannot watch persistentvolumeclaims",
		},
		"access review fails": {
			prepare:      func(client *fake.Clientset) { failAccessReviews(client, errors.New("access review error")) },
			wantReviewed: []authorizationv1.ResourceAttributes{reviewList},
			wantWarning:  "cannot check if the service account can list persistentvolumeclaims: access review error",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			client := fake.NewClientset()
			test.prepare(client)
			var logs bytes.Buffer
			kd := newKubeDiscovery(client, logger.NewWithWriter(&logs))

			assert.Equal(t, test.wantAllowed, kd.canListWatchPVCs(context.Background()))
			assert.Equal(t, test.wantReviewed, reviewedAttributes(client))

			if test.wantWarning == "" {
				assert.Empty(t, logs.String())
				return
			}
			assert.Equal(t, 1, strings.Count(logs.String(), "PersistentVolumeClaim metrics disabled"))
			assert.Contains(t, logs.String(), test.wantWarning)
			assert.Contains(t, logs.String(), "restart the collector")
			assert.Contains(t, logs.String(), "the check runs only at startup")
		})
	}
}

func TestCollector_Collect_DeleteObjectKeepsLongerNamedSibling(t *testing.T) {
	// Deleting "x-1" must obsolete only the charts of "x-1", not those of the live "x-10".
	// The expected chart sets come from collectors that only ever saw one of the two objects.
	const deleted, live = "x-1", "x-10"
	ctx := context.Background()

	tests := map[string]struct {
		objects func(names ...string) []runtime.Object
		delete  func(client kubernetes.Interface, name string) error
	}{
		"Node": {
			objects: func(names ...string) []runtime.Object {
				var objs []runtime.Object
				for _, name := range names {
					objs = append(objs, newNode(name))
				}
				return objs
			},
			delete: func(client kubernetes.Interface, name string) error {
				return client.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{})
			},
		},
		"Pod": {
			objects: func(names ...string) []runtime.Object {
				objs := []runtime.Object{newNode("node01")}
				for _, name := range names {
					objs = append(objs, newPod("node01", name))
				}
				return objs
			},
			delete: func(client kubernetes.Interface, name string) error {
				return client.CoreV1().Pods(corev1.NamespaceDefault).Delete(ctx, name, metav1.DeleteOptions{})
			},
		},
		"Deployment": {
			objects: func(names ...string) []runtime.Object {
				var objs []runtime.Object
				for _, name := range names {
					objs = append(objs, newDeployment(name))
				}
				return objs
			},
			delete: func(client kubernetes.Interface, name string) error {
				return client.AppsV1().Deployments(corev1.NamespaceDefault).Delete(ctx, name, metav1.DeleteOptions{})
			},
		},
		"CronJob": {
			objects: func(names ...string) []runtime.Object {
				var objs []runtime.Object
				for _, name := range names {
					objs = append(objs, prepareCronJob(name))
				}
				return objs
			},
			delete: func(client kubernetes.Interface, name string) error {
				return client.BatchV1().CronJobs(corev1.NamespaceDefault).Delete(ctx, name, metav1.DeleteOptions{})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := fake.NewClientset(test.objects(deleted, live)...)
			collrs := startCollectors(t, client,
				fake.NewClientset(test.objects(deleted)...),
				fake.NewClientset(test.objects(live)...),
			)
			collr, deletedOnly, liveOnly := collrs[0], collrs[1], collrs[2]

			expected := make(map[string]bool) // chart ID -> obsolete
			for _, c := range *deletedOnly.Charts() {
				expected[c.ID] = true
			}
			// Runs second: charts both collectors have (discovery, node01) must stay active.
			for _, c := range *liveOnly.Charts() {
				expected[c.ID] = false
			}
			var own int
			for _, obsolete := range expected {
				if obsolete {
					own++
				}
			}
			require.NotZero(t, own, "the deleted object must have charts")

			require.NoError(t, test.delete(client, deleted))
			require.Eventually(t, func() bool {
				_ = collr.Collect(ctx)
				for id, obsolete := range expected {
					if c := collr.Charts().Get(id); obsolete && (c == nil || !c.Obsolete) {
						return false
					}
				}
				return true
			}, 5*time.Second, 50*time.Millisecond, "the deleted object's charts were not removed")

			actual := make(map[string]bool)
			for _, c := range *collr.Charts() {
				actual[c.ID] = c.Obsolete
			}

			assert.Equal(t, expected, actual)
		})
	}
}

func TestCollector_Collect_ScheduledPodUpdatesOnlyItsNodeLabel(t *testing.T) {
	// Scheduling the pending "x-1" must not rewrite the node label of the live "x-10".
	ctx := context.Background()
	pending := newPod("node01", "x-1")
	pending.Spec.NodeName = ""
	client := fake.NewClientset(newNode("node01"), newNode("node02"), pending, newPod("node02", "x-10"))
	collr := startCollectors(t, client)[0]

	_, err := client.CoreV1().Pods(pending.Namespace).Update(ctx, newPod("node01", "x-1"), metav1.UpdateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_ = collr.Collect(ctx)
		for _, c := range *collr.Charts() {
			if chartLabelValue(c, labelKeyPodName) == "x-1" && chartLabelValue(c, labelKeyNodeName) != "node01" {
				return false
			}
		}
		return true
	}, 5*time.Second, 50*time.Millisecond, "the scheduled pod's charts did not get its node label")

	podNode := map[string]string{"x-1": "node01", "x-10": "node02"}
	seen := make(map[string]bool)
	expected := make(map[string]string) // chart ID -> node label
	actual := make(map[string]string)
	for _, c := range *collr.Charts() {
		if pod := chartLabelValue(c, labelKeyPodName); pod != "" {
			seen[pod] = true
			expected[c.ID] = podNode[pod]
			actual[c.ID] = chartLabelValue(c, labelKeyNodeName)
		}
	}

	require.Equal(t, map[string]bool{"x-1": true, "x-10": true}, seen, "both pods must have charts")
	assert.Equal(t, expected, actual)
}

// startCollectors starts one collector per client and waits once for all of them to pass the initial delay.
func startCollectors(t *testing.T, clients ...kubernetes.Interface) []*Collector {
	t.Helper()
	ctx := context.Background()

	var collrs []*Collector
	for _, client := range clients {
		collr := New()
		collr.newKubeClient = func() (kubernetes.Interface, error) { return client, nil }
		require.NoError(t, collr.Init(ctx))
		require.NoError(t, collr.Check(ctx))
		t.Cleanup(func() { collr.Cleanup(ctx) })
		_ = collr.Collect(ctx)
		collrs = append(collrs, collr)
	}

	time.Sleep(collrs[0].initDelay)
	for _, collr := range collrs {
		_ = collr.Collect(ctx)
	}
	return collrs
}

// requirePVCState waits until the state entry of the PVC source satisfies cond, i.e. the discoverer applied the event.
func requirePVCState(t *testing.T, collr *Collector, source string, cond func(*pvcState) bool) {
	t.Helper()

	require.Eventually(t, func() bool {
		collr.state.Lock()
		defer collr.state.Unlock()

		ps, ok := collr.state.pvcs[source]
		return ok && cond(ps)
	}, 5*time.Second, 10*time.Millisecond, "the PVC state did not reach the expected value")
}

// pvcChartLabels returns "<namespace>/<name>" of the PVC by the ID of every PVC chart.
func pvcChartLabels(charts *collectorapi.Charts) map[string]string {
	labels := make(map[string]string)
	for _, c := range *charts {
		if name := chartLabelValue(c, labelKeyPVCName); name != "" {
			labels[c.ID] = chartLabelValue(c, labelKeyNamespace) + "/" + name
		}
	}
	return labels
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

func newDeployment(name string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         corev1.NamespaceDefault,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentAvailable,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   appsv1.DeploymentReplicaFailure,
					Status: corev1.ConditionTrue,
				},
			},
			AvailableReplicas: 1,
			Replicas:          2,
			ReadyReplicas:     3,
		},
	}
}

// newPVC creates a PVC in the default namespace. An empty requested has no storage request, an empty capacity
// leaves the PVC Pending (no status.capacity).
func newPVC(name, requested, capacity string) *corev1.PersistentVolumeClaim {
	return newPVCInNamespace(corev1.NamespaceDefault, name, requested, capacity)
}

func newPVCInNamespace(namespace, name, requested, capacity string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimPending,
		},
	}
	if requested != "" {
		pvc.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: mustQuantity(requested)}
	}
	if capacity != "" {
		pvc.Status.Phase = corev1.ClaimBound
		pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: mustQuantity(capacity)}
	}
	return pvc
}

func prepareCronJob(name string) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         corev1.NamespaceDefault,
			UID:               types.UID(name),
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Status: batchv1.CronJobStatus{
			LastScheduleTime:   &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
			LastSuccessfulTime: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
		},
	}
}

func prepareCronJobNotStartedJob(name string, cj *batchv1.CronJob) *batchv1.Job {
	return prepareCronJobJob(name, cj)
}

func prepareCronJobRunningJob(name string, cj *batchv1.CronJob) *batchv1.Job {
	job := prepareCronJobJob(name, cj)
	job.Status.StartTime = &metav1.Time{Time: time.Now()}
	job.Status.Active = 1
	return job
}

func prepareCronJobCompleteJob(name string, cj *batchv1.CronJob) *batchv1.Job {
	job := prepareCronJobJob(name, cj)
	job.Status.StartTime = &metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
	job.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	job.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
	}
	return job
}

func prepareCronJobFailedJob(name string, cj *batchv1.CronJob) *batchv1.Job {
	job := prepareCronJobJob(name, cj)
	job.Status.StartTime = &metav1.Time{Time: time.Now()}
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:               batchv1.JobFailed,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			Reason:             batchv1.JobReasonDeadlineExceeded,
		},
	}
	return job
}

func prepareCronJobSuspendedJob(name string, cj *batchv1.CronJob) *batchv1.Job {
	job := prepareCronJobJob(name, cj)
	job.Status.StartTime = &metav1.Time{Time: time.Now()}
	job.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobSuspended, Status: corev1.ConditionTrue},
	}
	return job
}

func prepareCronJobJob(name string, cj *batchv1.CronJob) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         corev1.NamespaceDefault,
			UID:               types.UID(name),
			CreationTimestamp: metav1.Time{Time: time.Now()},
			OwnerReferences: []metav1.OwnerReference{
				{
					Controller: new(true),
					Kind:       "CronJob",
					UID:        cj.UID,
					Name:       cj.Name,
				},
			},
		},
	}
}

func calcObsoleteCharts(charts collectorapi.Charts) (num int) {
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

func copyIfSuffix(dst, src map[string]int64, suffixes ...string) {
	for k, v := range src {
		for _, suffix := range suffixes {
			if strings.HasSuffix(k, suffix) {
				if _, ok := dst[k]; ok {
					dst[k] = v
				}
			}
		}
	}
}

func copyAge(dst, src map[string]int64) {
	copyIfSuffix(dst, src, "_age")
}

func isLabelValueSet(c *collectorapi.Chart, name string) bool {
	for _, l := range c.Labels {
		if l.Key == name {
			return l.Value != ""
		}
	}
	return false
}

// allowPVCAccess makes the fake API server grant the verbs on PersistentVolumeClaims (core group, all namespaces)
// in SelfSubjectAccessReviews, and deny everything else. Without it the fake denies every review.
func allowPVCAccess(client *fake.Clientset, verbs ...string) {
	reactor := func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		attrs := review.Spec.ResourceAttributes
		review.Status.Allowed = attrs != nil &&
			attrs.Group == "" && attrs.Namespace == "" && attrs.Resource == "persistentvolumeclaims" &&
			slices.Contains(verbs, attrs.Verb)
		return true, review, nil
	}
	client.PrependReactor("create", "selfsubjectaccessreviews", reactor)
}

// failAccessReviews makes the fake API server fail every SelfSubjectAccessReview with err.
func failAccessReviews(client *fake.Clientset, err error) {
	client.PrependReactor("create", "selfsubjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
}

// forbidPVCs makes the fake API server refuse to list and watch PersistentVolumeClaims, as a real one does for a
// service account without the permission: the informer of such a resource never syncs.
func forbidPVCs(client *fake.Clientset) {
	err := errors.New("RBAC: access denied")
	forbidden := apierrors.NewForbidden(corev1.Resource("persistentvolumeclaims"), "", err)

	client.PrependReactor("list", "persistentvolumeclaims", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, forbidden
	})
	client.PrependWatchReactor("persistentvolumeclaims", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, forbidden
	})
}

// reviewedAttributes returns the resource attributes of the SelfSubjectAccessReviews the client sent, in order.
func reviewedAttributes(client *fake.Clientset) []authorizationv1.ResourceAttributes {
	var attrs []authorizationv1.ResourceAttributes
	for _, action := range client.Actions() {
		create, ok := action.(k8stesting.CreateAction)
		if !ok || create.GetResource().Resource != "selfsubjectaccessreviews" {
			continue
		}
		review := create.GetObject().(*authorizationv1.SelfSubjectAccessReview)
		attrs = append(attrs, *review.Spec.ResourceAttributes)
	}
	return attrs
}

func chartLabelValue(c *collectorapi.Chart, name string) string {
	for _, l := range c.Labels {
		if l.Key == name {
			return l.Value
		}
	}
	return ""
}
