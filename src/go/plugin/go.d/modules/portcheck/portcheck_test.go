// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

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

func TestPortCheck_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &PortCheck{}, dataConfigJSON, dataConfigYAML)
}

func TestPortCheck_Init(t *testing.T) {
	job := New()

	job.Host = "127.0.0.1"
	job.Ports = []int{39001, 39002}
	assert.NoError(t, job.Init())
	assert.Len(t, job.tcpPorts, 2)
}
func TestPortCheck_InitNG(t *testing.T) {
	job := New()

	assert.Error(t, job.Init())
	job.Host = "127.0.0.1"
	assert.Error(t, job.Init())
	job.Ports = []int{39001, 39002}
	assert.NoError(t, job.Init())
}

func TestPortCheck_Check(t *testing.T) {
	assert.Error(t, New().Check())
}

func TestPortCheck_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestPortCheck_Charts(t *testing.T) {
	job := New()
	job.Ports = []int{1, 2}
	job.Host = "localhost"
	require.NoError(t, job.Init())
}

func TestPortCheck_Collect(t *testing.T) {
	job := New()

	job.Host = "127.0.0.1"
	job.Ports = []int{39001, 39002}
	job.UpdateEvery = 5
	job.dialTCP = testDial(nil)
	require.NoError(t, job.Init())
	require.NoError(t, job.Check())

	copyLatency := func(dst, src map[string]int64) {
		for k := range dst {
			if strings.HasSuffix(k, "latency") {
				dst[k] = src[k]
			}
		}
	}

	expected := map[string]int64{
		"tcp_port_39001_current_state_duration": int64(job.UpdateEvery * 2),
		"tcp_port_39001_failed":                 0,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                1,
		"tcp_port_39001_timeout":                0,
		"tcp_port_39002_current_state_duration": int64(job.UpdateEvery * 2),
		"tcp_port_39002_failed":                 0,
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                1,
		"tcp_port_39002_timeout":                0,
	}
	collected := job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)

	expected = map[string]int64{
		"tcp_port_39001_current_state_duration": int64(job.UpdateEvery) * 3,
		"tcp_port_39001_failed":                 0,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                1,
		"tcp_port_39001_timeout":                0,
		"tcp_port_39002_current_state_duration": int64(job.UpdateEvery) * 3,
		"tcp_port_39002_failed":                 0,
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                1,
		"tcp_port_39002_timeout":                0,
	}
	collected = job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)

	job.dialTCP = testDial(errors.New("checkStateFailed"))

	expected = map[string]int64{
		"tcp_port_39001_current_state_duration": int64(job.UpdateEvery),
		"tcp_port_39001_failed":                 1,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                0,
		"tcp_port_39001_timeout":                0,
		"tcp_port_39002_current_state_duration": int64(job.UpdateEvery),
		"tcp_port_39002_failed":                 1,
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                0,
		"tcp_port_39002_timeout":                0,
	}
	collected = job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)

	job.dialTCP = testDial(timeoutError{})

	expected = map[string]int64{
		"tcp_port_39001_current_state_duration": int64(job.UpdateEvery),
		"tcp_port_39001_failed":                 0,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                0,
		"tcp_port_39001_timeout":                1,
		"tcp_port_39002_current_state_duration": int64(job.UpdateEvery),
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                0,
		"tcp_port_39002_timeout":                1,
		"tcp_port_39002_failed":                 0,
	}
	collected = job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)
}

func testDial(err error) dialTCPFunc {
	return func(_, _ string, _ time.Duration) (net.Conn, error) { return &net.TCPConn{}, err }
}

type timeoutError struct{}

func (timeoutError) Error() string { return "timeout" }
func (timeoutError) Timeout() bool { return true }
