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

func TestDocker_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Docker{}, dataConfigJSON, dataConfigYAML)
}

func TestDocker_Init(t *testing.T) {
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
			d := New()
			d.Config = test.config

			if test.wantFail {
				assert.Error(t, d.Init())
			} else {
				assert.NoError(t, d.Init())
			}
		})
	}
}

func TestDocker_Charts(t *testing.T) {
	assert.Equal(t, len(summaryCharts), len(*New().Charts()))
}

func TestDocker_Cleanup(t *testing.T) {
	tests := map[string]struct {
		prepare   func(d *Docker)
		wantClose bool
	}{
		"after New": {
			wantClose: false,
			prepare:   func(d *Docker) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(d *Docker) { _ = d.Init() },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(d *Docker) { _ = d.Init(); _ = d.Check() },
		},
		"after Collect": {
			wantClose: true,
			prepare:   func(d *Docker) { _ = d.Init(); d.Collect() },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := &mockClient{}
			d := New()
			d.newClient = prepareNewClientFunc(m)

			test.prepare(d)

			require.NotPanics(t, d.Cleanup)

			if test.wantClose {
				assert.True(t, m.closeCalled)
			} else {
				assert.False(t, m.closeCalled)
			}
		})
	}
}

func TestDocker_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Docker
		wantFail bool
	}{
		"case success": {
			wantFail: false,
			prepare: func() *Docker {
				return prepareCaseSuccess()
			},
		},
		"case success without container size": {
			wantFail: false,
			prepare: func() *Docker {
				return prepareCaseSuccessWithoutContainerSize()
			},
		},
		"fail on case err on Info()": {
			wantFail: true,
			prepare: func() *Docker {
				return prepareCaseErrOnInfo()
			},
		},
		"fail on case err on ImageList()": {
			wantFail: true,
			prepare: func() *Docker {
				return prepareCaseErrOnImageList()
			},
		},
		"fail on case err on ContainerList()": {
			wantFail: true,
			prepare: func() *Docker {
				return prepareCaseErrOnContainerList()
			},
		},
		"fail on case err on creating Docker client": {
			wantFail: true,
			prepare: func() *Docker {
				return prepareCaseErrCreatingClient()
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d := test.prepare()

			require.NoError(t, d.Init())

			if test.wantFail {
				assert.Error(t, d.Check())
			} else {
				assert.NoError(t, d.Check())
			}
		})
	}
}

func TestDocker_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *Docker
		expected map[string]int64
	}{
		"case success": {
			prepare: func() *Docker {
				return prepareCaseSuccess()
			},
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
				"containers_health_status_none":                             3,
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
			prepare: func() *Docker {
				return prepareCaseSuccessWithoutContainerSize()
			},
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
				"containers_health_status_none":                             3,
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
			prepare: func() *Docker {
				return prepareCaseErrOnInfo()
			},
			expected: nil,
		},
		"fail on case err on ImageList()": {
			prepare: func() *Docker {
				return prepareCaseErrOnImageList()
			},
			expected: nil,
		},
		"fail on case err on ContainerList()": {
			prepare: func() *Docker {
				return prepareCaseErrOnContainerList()
			},
			expected: nil,
		},
		"fail on case err on creating Docker client": {
			prepare: func() *Docker {
				return prepareCaseErrCreatingClient()
			},
			expected: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d := test.prepare()

			require.NoError(t, d.Init())

			mx := d.Collect()

			require.Equal(t, test.expected, mx)

			if d.client != nil {
				m, ok := d.client.(*mockClient)
				require.True(t, ok)
				require.True(t, m.negotiateAPIVersionCalled)
			}

		})
	}
}

func prepareCaseSuccess() *Docker {
	d := New()
	d.CollectContainerSize = true
	d.newClient = prepareNewClientFunc(&mockClient{})
	return d
}

func prepareCaseSuccessWithoutContainerSize() *Docker {
	d := New()
	d.CollectContainerSize = false
	d.newClient = prepareNewClientFunc(&mockClient{})
	return d
}

func prepareCaseErrOnInfo() *Docker {
	d := New()
	d.newClient = prepareNewClientFunc(&mockClient{errOnInfo: true})
	return d
}

func prepareCaseErrOnImageList() *Docker {
	d := New()
	d.newClient = prepareNewClientFunc(&mockClient{errOnImageList: true})
	return d
}

func prepareCaseErrOnContainerList() *Docker {
	d := New()
	d.newClient = prepareNewClientFunc(&mockClient{errOnContainerList: true})
	return d
}

func prepareCaseErrCreatingClient() *Docker {
	d := New()
	d.newClient = prepareNewClientFunc(nil)
	return d
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
