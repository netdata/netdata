// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	typesImage "github.com/docker/docker/api/types/image"
	typesSystem "github.com/docker/docker/api/types/system"
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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default config": {
			wantFail: false,
			config:   New().Config,
		},
		"unset 'address'": {
			wantFail: false,
			config: Config{
				Address: "",
			},
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

func TestCollector_Charts(t *testing.T) {
	assert.Equal(t, len(summaryCharts), len(*New().Charts()))
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(*Collector)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(*Collector) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()) },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()); _ = c.Check(context.Background()) },
		},
		"after Collect": {
			wantClose: true,
			prepare:   func(c *Collector) { _ = c.Init(context.Background()); c.Collect(context.Background()) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := &mockClient{}
			collr := New()
			collr.newClient = prepareNewClientFunc(m)

			test.prepare(collr)

			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })

			if test.wantClose {
				assert.True(t, m.closeCalled)
			} else {
				assert.False(t, m.closeCalled)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		wantFail bool
	}{
		"case success": {
			wantFail: false,
			prepare: func() *Collector {
				return prepareCaseSuccess()
			},
		},
		"case success without container size": {
			wantFail: false,
			prepare: func() *Collector {
				return prepareCaseSuccessWithoutContainerSize()
			},
		},
		"fail on case err on Info()": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCaseErrOnInfo()
			},
		},
		"fail on case err on ImageList()": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCaseErrOnImageList()
			},
		},
		"fail on case err on ContainerList()": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCaseErrOnContainerList()
			},
		},
		"fail on case err on creating Docker client": {
			wantFail: true,
			prepare: func() *Collector {
				return prepareCaseErrCreatingClient()
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

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Collector
		expected map[string]int64
	}{
		"case success": {
			prepare: prepareCaseSuccess,
			expected: map[string]int64{
				"container_container10_health_status_healthy":               0,
				"container_container10_health_status_none":                  0,
				"container_container10_health_status_not_running_unhealthy": 1,
				"container_container10_health_status_starting":              0,
				"container_container10_health_status_unhealthy":             0,
				"container_container10_size_root_fs":                        0,
				"container_container10_size_rw":                             0,
				"container_container10_state_created":                       0,
				"container_container10_state_dead":                          1,
				"container_container10_state_exited":                        0,
				"container_container10_state_paused":                        0,
				"container_container10_state_removing":                      0,
				"container_container10_state_restarting":                    0,
				"container_container10_state_running":                       0,
				"container_container11_health_status_healthy":               0,
				"container_container11_health_status_none":                  0,
				"container_container11_health_status_not_running_unhealthy": 0,
				"container_container11_health_status_starting":              1,
				"container_container11_health_status_unhealthy":             0,
				"container_container11_size_root_fs":                        0,
				"container_container11_size_rw":                             0,
				"container_container11_state_created":                       0,
				"container_container11_state_dead":                          0,
				"container_container11_state_exited":                        0,
				"container_container11_state_paused":                        0,
				"container_container11_state_removing":                      1,
				"container_container11_state_restarting":                    0,
				"container_container11_state_running":                       0,
				"container_container12_health_status_healthy":               0,
				"container_container12_health_status_none":                  0,
				"container_container12_health_status_not_running_unhealthy": 0,
				"container_container12_health_status_starting":              1,
				"container_container12_health_status_unhealthy":             0,
				"container_container12_size_root_fs":                        0,
				"container_container12_size_rw":                             0,
				"container_container12_state_created":                       0,
				"container_container12_state_dead":                          0,
				"container_container12_state_exited":                        1,
				"container_container12_state_paused":                        0,
				"container_container12_state_removing":                      0,
				"container_container12_state_restarting":                    0,
				"container_container12_state_running":                       0,
				"container_container13_health_status_healthy":               0,
				"container_container13_health_status_none":                  0,
				"container_container13_health_status_not_running_unhealthy": 0,
				"container_container13_health_status_starting":              1,
				"container_container13_health_status_unhealthy":             0,
				"container_container13_size_root_fs":                        0,
				"container_container13_size_rw":                             0,
				"container_container13_state_created":                       0,
				"container_container13_state_dead":                          0,
				"container_container13_state_exited":                        1,
				"container_container13_state_paused":                        0,
				"container_container13_state_removing":                      0,
				"container_container13_state_restarting":                    0,
				"container_container13_state_running":                       0,
				"container_container14_health_status_healthy":               0,
				"container_container14_health_status_none":                  1,
				"container_container14_health_status_not_running_unhealthy": 0,
				"container_container14_health_status_starting":              0,
				"container_container14_health_status_unhealthy":             0,
				"container_container14_size_root_fs":                        0,
				"container_container14_size_rw":                             0,
				"container_container14_state_created":                       0,
				"container_container14_state_dead":                          1,
				"container_container14_state_exited":                        0,
				"container_container14_state_paused":                        0,
				"container_container14_state_removing":                      0,
				"container_container14_state_restarting":                    0,
				"container_container14_state_running":                       0,
				"container_container15_health_status_healthy":               0,
				"container_container15_health_status_none":                  1,
				"container_container15_health_status_not_running_unhealthy": 0,
				"container_container15_health_status_starting":              0,
				"container_container15_health_status_unhealthy":             0,
				"container_container15_size_root_fs":                        0,
				"container_container15_size_rw":                             0,
				"container_container15_state_created":                       0,
				"container_container15_state_dead":                          1,
				"container_container15_state_exited":                        0,
				"container_container15_state_paused":                        0,
				"container_container15_state_removing":                      0,
				"container_container15_state_restarting":                    0,
				"container_container15_state_running":                       0,
				"container_container16_health_status_healthy":               0,
				"container_container16_health_status_none":                  1,
				"container_container16_health_status_not_running_unhealthy": 0,
				"container_container16_health_status_starting":              0,
				"container_container16_health_status_unhealthy":             0,
				"container_container16_size_root_fs":                        0,
				"container_container16_size_rw":                             0,
				"container_container16_state_created":                       0,
				"container_container16_state_dead":                          1,
				"container_container16_state_exited":                        0,
				"container_container16_state_paused":                        0,
				"container_container16_state_removing":                      0,
				"container_container16_state_restarting":                    0,
				"container_container16_state_running":                       0,
				"container_container1_health_status_healthy":                1,
				"container_container1_health_status_none":                   0,
				"container_container1_health_status_not_running_unhealthy":  0,
				"container_container1_health_status_starting":               0,
				"container_container1_health_status_unhealthy":              0,
				"container_container1_size_root_fs":                         0,
				"container_container1_size_rw":                              0,
				"container_container1_state_created":                        1,
				"container_container1_state_dead":                           0,
				"container_container1_state_exited":                         0,
				"container_container1_state_paused":                         0,
				"container_container1_state_removing":                       0,
				"container_container1_state_restarting":                     0,
				"container_container1_state_running":                        0,
				"container_container2_health_status_healthy":                1,
				"container_container2_health_status_none":                   0,
				"container_container2_health_status_not_running_unhealthy":  0,
				"container_container2_health_status_starting":               0,
				"container_container2_health_status_unhealthy":              0,
				"container_container2_size_root_fs":                         0,
				"container_container2_size_rw":                              0,
				"container_container2_state_created":                        0,
				"container_container2_state_dead":                           0,
				"container_container2_state_exited":                         0,
				"container_container2_state_paused":                         0,
				"container_container2_state_removing":                       0,
				"container_container2_state_restarting":                     0,
				"container_container2_state_running":                        1,
				"container_container3_health_status_healthy":                1,
				"container_container3_health_status_none":                   0,
				"container_container3_health_status_not_running_unhealthy":  0,
				"container_container3_health_status_starting":               0,
				"container_container3_health_status_unhealthy":              0,
				"container_container3_size_root_fs":                         0,
				"container_container3_size_rw":                              0,
				"container_container3_state_created":                        0,
				"container_container3_state_dead":                           0,
				"container_container3_state_exited":                         0,
				"container_container3_state_paused":                         0,
				"container_container3_state_removing":                       0,
				"container_container3_state_restarting":                     0,
				"container_container3_state_running":                        1,
				"container_container4_health_status_healthy":                0,
				"container_container4_health_status_none":                   0,
				"container_container4_health_status_not_running_unhealthy":  1,
				"container_container4_health_status_starting":               0,
				"container_container4_health_status_unhealthy":              0,
				"container_container4_size_root_fs":                         0,
				"container_container4_size_rw":                              0,
				"container_container4_state_created":                        1,
				"container_container4_state_dead":                           0,
				"container_container4_state_exited":                         0,
				"container_container4_state_paused":                         0,
				"container_container4_state_removing":                       0,
				"container_container4_state_restarting":                     0,
				"container_container4_state_running":                        0,
				"container_container5_health_status_healthy":                0,
				"container_container5_health_status_none":                   0,
				"container_container5_health_status_not_running_unhealthy":  0,
				"container_container5_health_status_starting":               0,
				"container_container5_health_status_unhealthy":              1,
				"container_container5_size_root_fs":                         0,
				"container_container5_size_rw":                              0,
				"container_container5_state_created":                        0,
				"container_container5_state_dead":                           0,
				"container_container5_state_exited":                         0,
				"container_container5_state_paused":                         0,
				"container_container5_state_removing":                       0,
				"container_container5_state_restarting":                     0,
				"container_container5_state_running":                        1,
				"container_container6_health_status_healthy":                0,
				"container_container6_health_status_none":                   0,
				"container_container6_health_status_not_running_unhealthy":  1,
				"container_container6_health_status_starting":               0,
				"container_container6_health_status_unhealthy":              0,
				"container_container6_size_root_fs":                         0,
				"container_container6_size_rw":                              0,
				"container_container6_state_created":                        0,
				"container_container6_state_dead":                           0,
				"container_container6_state_exited":                         0,
				"container_container6_state_paused":                         1,
				"container_container6_state_removing":                       0,
				"container_container6_state_restarting":                     0,
				"container_container6_state_running":                        0,
				"container_container7_health_status_healthy":                0,
				"container_container7_health_status_none":                   0,
				"container_container7_health_status_not_running_unhealthy":  1,
				"container_container7_health_status_starting":               0,
				"container_container7_health_status_unhealthy":              0,
				"container_container7_size_root_fs":                         0,
				"container_container7_size_rw":                              0,
				"container_container7_state_created":                        0,
				"container_container7_state_dead":                           0,
				"container_container7_state_exited":                         0,
				"container_container7_state_paused":                         0,
				"container_container7_state_removing":                       0,
				"container_container7_state_restarting":                     1,
				"container_container7_state_running":                        0,
				"container_container8_health_status_healthy":                0,
				"container_container8_health_status_none":                   0,
				"container_container8_health_status_not_running_unhealthy":  1,
				"container_container8_health_status_starting":               0,
				"container_container8_health_status_unhealthy":              0,
				"container_container8_size_root_fs":                         0,
				"container_container8_size_rw":                              0,
				"container_container8_state_created":                        0,
				"container_container8_state_dead":                           0,
				"container_container8_state_exited":                         0,
				"container_container8_state_paused":                         0,
				"container_container8_state_removing":                       1,
				"container_container8_state_restarting":                     0,
				"container_container8_state_running":                        0,
				"container_container9_health_status_healthy":                0,
				"container_container9_health_status_none":                   0,
				"container_container9_health_status_not_running_unhealthy":  1,
				"container_container9_health_status_starting":               0,
				"container_container9_health_status_unhealthy":              0,
				"container_container9_size_root_fs":                         0,
				"container_container9_size_rw":                              0,
				"container_container9_state_created":                        0,
				"container_container9_state_dead":                           0,
				"container_container9_state_exited":                         1,
				"container_container9_state_paused":                         0,
				"container_container9_state_removing":                       0,
				"container_container9_state_restarting":                     0,
				"container_container9_state_running":                        0,
				"containers_health_status_healthy":                          3,
				"containers_health_status_none":                             4,
				"containers_health_status_not_running_unhealthy":            6,
				"containers_health_status_starting":                         3,
				"containers_health_status_unhealthy":                        1,
				"containers_state_exited":                                   6,
				"containers_state_paused":                                   5,
				"containers_state_running":                                  4,
				"images_active":                                             1,
				"images_dangling":                                           1,
				"images_size":                                               300,
			},
		},
		"case success without container size": {
			prepare: prepareCaseSuccessWithoutContainerSize,
			expected: map[string]int64{
				"container_container10_health_status_healthy":               0,
				"container_container10_health_status_none":                  0,
				"container_container10_health_status_not_running_unhealthy": 1,
				"container_container10_health_status_starting":              0,
				"container_container10_health_status_unhealthy":             0,
				"container_container10_size_root_fs":                        0,
				"container_container10_size_rw":                             0,
				"container_container10_state_created":                       0,
				"container_container10_state_dead":                          1,
				"container_container10_state_exited":                        0,
				"container_container10_state_paused":                        0,
				"container_container10_state_removing":                      0,
				"container_container10_state_restarting":                    0,
				"container_container10_state_running":                       0,
				"container_container11_health_status_healthy":               0,
				"container_container11_health_status_none":                  0,
				"container_container11_health_status_not_running_unhealthy": 0,
				"container_container11_health_status_starting":              1,
				"container_container11_health_status_unhealthy":             0,
				"container_container11_size_root_fs":                        0,
				"container_container11_size_rw":                             0,
				"container_container11_state_created":                       0,
				"container_container11_state_dead":                          0,
				"container_container11_state_exited":                        0,
				"container_container11_state_paused":                        0,
				"container_container11_state_removing":                      1,
				"container_container11_state_restarting":                    0,
				"container_container11_state_running":                       0,
				"container_container12_health_status_healthy":               0,
				"container_container12_health_status_none":                  0,
				"container_container12_health_status_not_running_unhealthy": 0,
				"container_container12_health_status_starting":              1,
				"container_container12_health_status_unhealthy":             0,
				"container_container12_size_root_fs":                        0,
				"container_container12_size_rw":                             0,
				"container_container12_state_created":                       0,
				"container_container12_state_dead":                          0,
				"container_container12_state_exited":                        1,
				"container_container12_state_paused":                        0,
				"container_container12_state_removing":                      0,
				"container_container12_state_restarting":                    0,
				"container_container12_state_running":                       0,
				"container_container13_health_status_healthy":               0,
				"container_container13_health_status_none":                  0,
				"container_container13_health_status_not_running_unhealthy": 0,
				"container_container13_health_status_starting":              1,
				"container_container13_health_status_unhealthy":             0,
				"container_container13_size_root_fs":                        0,
				"container_container13_size_rw":                             0,
				"container_container13_state_created":                       0,
				"container_container13_state_dead":                          0,
				"container_container13_state_exited":                        1,
				"container_container13_state_paused":                        0,
				"container_container13_state_removing":                      0,
				"container_container13_state_restarting":                    0,
				"container_container13_state_running":                       0,
				"container_container14_health_status_healthy":               0,
				"container_container14_health_status_none":                  1,
				"container_container14_health_status_not_running_unhealthy": 0,
				"container_container14_health_status_starting":              0,
				"container_container14_health_status_unhealthy":             0,
				"container_container14_size_root_fs":                        0,
				"container_container14_size_rw":                             0,
				"container_container14_state_created":                       0,
				"container_container14_state_dead":                          1,
				"container_container14_state_exited":                        0,
				"container_container14_state_paused":                        0,
				"container_container14_state_removing":                      0,
				"container_container14_state_restarting":                    0,
				"container_container14_state_running":                       0,
				"container_container15_health_status_healthy":               0,
				"container_container15_health_status_none":                  1,
				"container_container15_health_status_not_running_unhealthy": 0,
				"container_container15_health_status_starting":              0,
				"container_container15_health_status_unhealthy":             0,
				"container_container15_size_root_fs":                        0,
				"container_container15_size_rw":                             0,
				"container_container15_state_created":                       0,
				"container_container15_state_dead":                          1,
				"container_container15_state_exited":                        0,
				"container_container15_state_paused":                        0,
				"container_container15_state_removing":                      0,
				"container_container15_state_restarting":                    0,
				"container_container15_state_running":                       0,
				"container_container16_health_status_healthy":               0,
				"container_container16_health_status_none":                  1,
				"container_container16_health_status_not_running_unhealthy": 0,
				"container_container16_health_status_starting":              0,
				"container_container16_health_status_unhealthy":             0,
				"container_container16_size_root_fs":                        0,
				"container_container16_size_rw":                             0,
				"container_container16_state_created":                       0,
				"container_container16_state_dead":                          1,
				"container_container16_state_exited":                        0,
				"container_container16_state_paused":                        0,
				"container_container16_state_removing":                      0,
				"container_container16_state_restarting":                    0,
				"container_container16_state_running":                       0,
				"container_container1_health_status_healthy":                1,
				"container_container1_health_status_none":                   0,
				"container_container1_health_status_not_running_unhealthy":  0,
				"container_container1_health_status_starting":               0,
				"container_container1_health_status_unhealthy":              0,
				"container_container1_size_root_fs":                         0,
				"container_container1_size_rw":                              0,
				"container_container1_state_created":                        1,
				"container_container1_state_dead":                           0,
				"container_container1_state_exited":                         0,
				"container_container1_state_paused":                         0,
				"container_container1_state_removing":                       0,
				"container_container1_state_restarting":                     0,
				"container_container1_state_running":                        0,
				"container_container2_health_status_healthy":                1,
				"container_container2_health_status_none":                   0,
				"container_container2_health_status_not_running_unhealthy":  0,
				"container_container2_health_status_starting":               0,
				"container_container2_health_status_unhealthy":              0,
				"container_container2_size_root_fs":                         0,
				"container_container2_size_rw":                              0,
				"container_container2_state_created":                        0,
				"container_container2_state_dead":                           0,
				"container_container2_state_exited":                         0,
				"container_container2_state_paused":                         0,
				"container_container2_state_removing":                       0,
				"container_container2_state_restarting":                     0,
				"container_container2_state_running":                        1,
				"container_container3_health_status_healthy":                1,
				"container_container3_health_status_none":                   0,
				"container_container3_health_status_not_running_unhealthy":  0,
				"container_container3_health_status_starting":               0,
				"container_container3_health_status_unhealthy":              0,
				"container_container3_size_root_fs":                         0,
				"container_container3_size_rw":                              0,
				"container_container3_state_created":                        0,
				"container_container3_state_dead":                           0,
				"container_container3_state_exited":                         0,
				"container_container3_state_paused":                         0,
				"container_container3_state_removing":                       0,
				"container_container3_state_restarting":                     0,
				"container_container3_state_running":                        1,
				"container_container4_health_status_healthy":                0,
				"container_container4_health_status_none":                   0,
				"container_container4_health_status_not_running_unhealthy":  1,
				"container_container4_health_status_starting":               0,
				"container_container4_health_status_unhealthy":              0,
				"container_container4_size_root_fs":                         0,
				"container_container4_size_rw":                              0,
				"container_container4_state_created":                        1,
				"container_container4_state_dead":                           0,
				"container_container4_state_exited":                         0,
				"container_container4_state_paused":                         0,
				"container_container4_state_removing":                       0,
				"container_container4_state_restarting":                     0,
				"container_container4_state_running":                        0,
				"container_container5_health_status_healthy":                0,
				"container_container5_health_status_none":                   0,
				"container_container5_health_status_not_running_unhealthy":  0,
				"container_container5_health_status_starting":               0,
				"container_container5_health_status_unhealthy":              1,
				"container_container5_size_root_fs":                         0,
				"container_container5_size_rw":                              0,
				"container_container5_state_created":                        0,
				"container_container5_state_dead":                           0,
				"container_container5_state_exited":                         0,
				"container_container5_state_paused":                         0,
				"container_container5_state_removing":                       0,
				"container_container5_state_restarting":                     0,
				"container_container5_state_running":                        1,
				"container_container6_health_status_healthy":                0,
				"container_container6_health_status_none":                   0,
				"container_container6_health_status_not_running_unhealthy":  1,
				"container_container6_health_status_starting":               0,
				"container_container6_health_status_unhealthy":              0,
				"container_container6_size_root_fs":                         0,
				"container_container6_size_rw":                              0,
				"container_container6_state_created":                        0,
				"container_container6_state_dead":                           0,
				"container_container6_state_exited":                         0,
				"container_container6_state_paused":                         1,
				"container_container6_state_removing":                       0,
				"container_container6_state_restarting":                     0,
				"container_container6_state_running":                        0,
				"container_container7_health_status_healthy":                0,
				"container_container7_health_status_none":                   0,
				"container_container7_health_status_not_running_unhealthy":  1,
				"container_container7_health_status_starting":               0,
				"container_container7_health_status_unhealthy":              0,
				"container_container7_size_root_fs":                         0,
				"container_container7_size_rw":                              0,
				"container_container7_state_created":                        0,
				"container_container7_state_dead":                           0,
				"container_container7_state_exited":                         0,
				"container_container7_state_paused":                         0,
				"container_container7_state_removing":                       0,
				"container_container7_state_restarting":                     1,
				"container_container7_state_running":                        0,
				"container_container8_health_status_healthy":                0,
				"container_container8_health_status_none":                   0,
				"container_container8_health_status_not_running_unhealthy":  1,
				"container_container8_health_status_starting":               0,
				"container_container8_health_status_unhealthy":              0,
				"container_container8_size_root_fs":                         0,
				"container_container8_size_rw":                              0,
				"container_container8_state_created":                        0,
				"container_container8_state_dead":                           0,
				"container_container8_state_exited":                         0,
				"container_container8_state_paused":                         0,
				"container_container8_state_removing":                       1,
				"container_container8_state_restarting":                     0,
				"container_container8_state_running":                        0,
				"container_container9_health_status_healthy":                0,
				"container_container9_health_status_none":                   0,
				"container_container9_health_status_not_running_unhealthy":  1,
				"container_container9_health_status_starting":               0,
				"container_container9_health_status_unhealthy":              0,
				"container_container9_size_root_fs":                         0,
				"container_container9_size_rw":                              0,
				"container_container9_state_created":                        0,
				"container_container9_state_dead":                           0,
				"container_container9_state_exited":                         1,
				"container_container9_state_paused":                         0,
				"container_container9_state_removing":                       0,
				"container_container9_state_restarting":                     0,
				"container_container9_state_running":                        0,
				"containers_health_status_healthy":                          3,
				"containers_health_status_none":                             4,
				"containers_health_status_not_running_unhealthy":            6,
				"containers_health_status_starting":                         3,
				"containers_health_status_unhealthy":                        1,
				"containers_state_exited":                                   6,
				"containers_state_paused":                                   5,
				"containers_state_running":                                  4,
				"images_active":                                             1,
				"images_dangling":                                           1,
				"images_size":                                               300,
			},
		},
		"fail on case err on Info()": {
			prepare:  prepareCaseErrOnInfo,
			expected: nil,
		},
		"fail on case err on ImageList()": {
			prepare:  prepareCaseErrOnImageList,
			expected: nil,
		},
		"fail on case err on ContainerList()": {
			prepare:  prepareCaseErrOnContainerList,
			expected: nil,
		},
		"fail on case err on creating Docker client": {
			prepare:  prepareCaseErrCreatingClient,
			expected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			require.Equal(t, test.expected, mx)

			if collr.client != nil {
				m, ok := collr.client.(*mockClient)
				require.True(t, ok)
				require.True(t, m.negotiateAPIVersionCalled)
			}

		})
	}
}

func prepareCaseSuccess() *Collector {
	collr := New()
	collr.CollectContainerSize = true
	collr.newClient = prepareNewClientFunc(&mockClient{})
	return collr
}

func prepareCaseSuccessWithoutContainerSize() *Collector {
	collr := New()
	collr.CollectContainerSize = false
	collr.newClient = prepareNewClientFunc(&mockClient{})
	return collr
}

func prepareCaseErrOnInfo() *Collector {
	collr := New()
	collr.newClient = prepareNewClientFunc(&mockClient{errOnInfo: true})
	return collr
}

func prepareCaseErrOnImageList() *Collector {
	collr := New()
	collr.newClient = prepareNewClientFunc(&mockClient{errOnImageList: true})
	return collr
}

func prepareCaseErrOnContainerList() *Collector {
	collr := New()
	collr.newClient = prepareNewClientFunc(&mockClient{errOnContainerList: true})
	return collr
}

func prepareCaseErrCreatingClient() *Collector {
	collr := New()
	collr.newClient = prepareNewClientFunc(nil)
	return collr
}

func prepareNewClientFunc(m *mockClient) func(_ Config) (dockerClient, error) {
	if m == nil {
		return func(_ Config) (dockerClient, error) { return nil, errors.New("mock.newClient() error") }
	}
	return func(_ Config) (dockerClient, error) { return m, nil }
}

type mockClient struct {
	errOnInfo                 bool
	errOnImageList            bool
	errOnContainerList        bool
	negotiateAPIVersionCalled bool
	closeCalled               bool
}

func (m *mockClient) Info(_ context.Context) (typesSystem.Info, error) {
	if m.errOnInfo {
		return typesSystem.Info{}, errors.New("mockClient.Info() error")
	}

	return typesSystem.Info{
		ContainersRunning: 4,
		ContainersPaused:  5,
		ContainersStopped: 6,
	}, nil
}

func (m *mockClient) ContainerList(_ context.Context, opts typesContainer.ListOptions) ([]types.Container, error) {
	if m.errOnContainerList {
		return nil, errors.New("mockClient.ContainerList() error")
	}

	v := opts.Filters.Get("health")

	if len(v) == 0 {
		return nil, errors.New("mockClient.ContainerList() error (expect 'health' filter)")
	}

	var containers []types.Container

	switch v[0] {
	case types.Healthy:
		containers = []types.Container{
			{Names: []string{"container1"}, State: "created", Image: "example/example:v1"},
			{Names: []string{"container2"}, State: "running", Image: "example/example:v1"},
			{Names: []string{"container3"}, State: "running", Image: "example/example:v1"},
		}
	case types.Unhealthy:
		containers = []types.Container{
			{Names: []string{"container4"}, State: "created", Image: "example/example:v2"},
			{Names: []string{"container5"}, State: "running", Image: "example/example:v2"},
			{Names: []string{"container6"}, State: "paused", Image: "example/example:v2"},
			{Names: []string{"container7"}, State: "restarting", Image: "example/example:v2"},
			{Names: []string{"container8"}, State: "removing", Image: "example/example:v2"},
			{Names: []string{"container9"}, State: "exited", Image: "example/example:v2"},
			{Names: []string{"container10"}, State: "dead", Image: "example/example:v2"},
		}
	case types.Starting:
		containers = []types.Container{
			{Names: []string{"container11"}, State: "removing", Image: "example/example:v3"},
			{Names: []string{"container12"}, State: "exited", Image: "example/example:v3"},
			{Names: []string{"container13"}, State: "exited", Image: "example/example:v3"},
		}
	case types.NoHealthcheck:
		containers = []types.Container{
			{Names: []string{"container14"}, State: "dead", Image: "example/example:v4"},
			{Names: []string{"container15"}, State: "dead", Image: "example/example:v4"},
			{Names: []string{"container16"}, State: "dead", Image: "example/example:v4"},
			{Names: []string{"container17"}, State: "dead", Image: "example/example:v4",
				Labels: map[string]string{"netdata.cloud/ignore": "true"},
			},
		}
	}

	if opts.Size {
		for _, c := range containers {
			c.SizeRw = 123
			c.SizeRootFs = 321
		}
	}

	return containers, nil
}

func (m *mockClient) ImageList(_ context.Context, _ typesImage.ListOptions) ([]typesImage.Summary, error) {
	if m.errOnImageList {
		return nil, errors.New("mockClient.ImageList() error")
	}

	return []typesImage.Summary{
		{
			Containers: 0,
			Size:       100,
		},
		{
			Containers: 1,
			Size:       200,
		},
	}, nil
}

func (m *mockClient) NegotiateAPIVersion(_ context.Context) {
	m.negotiateAPIVersionCalled = true
}

func (m *mockClient) Close() error {
	m.closeCalled = true
	return nil
}
