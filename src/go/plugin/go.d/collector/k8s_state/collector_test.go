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

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
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
				client := &brokenInfoKubeClient{fake.NewClientset()}
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

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			time.Sleep(time.Second)
			if test.doCollect {
				assert.True(t, collr.discoverer.stopped())
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
					module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
					module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
					module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
				cjSuspended.Spec.Suspend = ptr(true)
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
					module.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, func(chart *module.Chart, dim *module.Dim) bool {
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
					module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
					module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
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
					Controller: ptr(true),
					Kind:       "CronJob",
					UID:        cj.UID,
					Name:       cj.Name,
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

func isLabelValueSet(c *module.Chart, name string) bool {
	for _, l := range c.Labels {
		if l.Key == name {
			return l.Value != ""
		}
	}
	return false
}
