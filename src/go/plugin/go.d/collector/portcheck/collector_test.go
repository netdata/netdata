// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"context"
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

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	collr := New()

	collr.Host = "127.0.0.1"
	collr.Ports = []int{39001, 39002}
	assert.NoError(t, collr.Init(context.Background()))
	assert.Len(t, collr.tcpPorts, 2)
}
func TestCollector_InitNG(t *testing.T) {
	collr := New()

	assert.Error(t, collr.Init(context.Background()))
	collr.Host = "127.0.0.1"
	assert.Error(t, collr.Init(context.Background()))
	collr.Ports = []int{39001, 39002}
	assert.NoError(t, collr.Init(context.Background()))
}

func TestCollector_Check(t *testing.T) {
	assert.Error(t, New().Check(context.Background()))
}

func TestCollector_Cleanup(t *testing.T) {
	New().Cleanup(context.Background())
}

func TestCollector_Charts(t *testing.T) {
	collr := New()
	collr.Ports = []int{1, 2}
	collr.Host = "localhost"
	require.NoError(t, collr.Init(context.Background()))
}

func TestCollector_Collect(t *testing.T) {
	collr := New()

	collr.Host = "127.0.0.1"
	collr.Ports = []int{39001, 39002}
	collr.UpdateEvery = 5
	collr.dialTCP = testDial(nil)
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	copyLatencyDuration := func(dst, src map[string]int64) {
		for k := range dst {
			if strings.HasSuffix(k, "latency") || strings.HasSuffix(k, "duration") {
				dst[k] = src[k]
			}
		}
	}

	expected := map[string]int64{
		"tcp_port_39001_current_state_duration": int64(collr.UpdateEvery * 2),
		"tcp_port_39001_failed":                 0,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                1,
		"tcp_port_39001_timeout":                0,
		"tcp_port_39002_current_state_duration": int64(collr.UpdateEvery * 2),
		"tcp_port_39002_failed":                 0,
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                1,
		"tcp_port_39002_timeout":                0,
	}
	mx := collr.Collect(context.Background())
	copyLatencyDuration(expected, mx)

	assert.Equal(t, expected, mx)

	expected = map[string]int64{
		"tcp_port_39001_current_state_duration": int64(collr.UpdateEvery) * 3,
		"tcp_port_39001_failed":                 0,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                1,
		"tcp_port_39001_timeout":                0,
		"tcp_port_39002_current_state_duration": int64(collr.UpdateEvery) * 3,
		"tcp_port_39002_failed":                 0,
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                1,
		"tcp_port_39002_timeout":                0,
	}
	mx = collr.Collect(context.Background())
	copyLatencyDuration(expected, mx)

	assert.Equal(t, expected, mx)

	collr.dialTCP = testDial(errors.New("checkStateFailed"))

	expected = map[string]int64{
		"tcp_port_39001_current_state_duration": int64(collr.UpdateEvery),
		"tcp_port_39001_failed":                 1,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                0,
		"tcp_port_39001_timeout":                0,
		"tcp_port_39002_current_state_duration": int64(collr.UpdateEvery),
		"tcp_port_39002_failed":                 1,
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                0,
		"tcp_port_39002_timeout":                0,
	}
	mx = collr.Collect(context.Background())
	copyLatencyDuration(expected, mx)

	assert.Equal(t, expected, mx)

	collr.dialTCP = testDial(timeoutError{})

	expected = map[string]int64{
		"tcp_port_39001_current_state_duration": int64(collr.UpdateEvery),
		"tcp_port_39001_failed":                 0,
		"tcp_port_39001_latency":                0,
		"tcp_port_39001_success":                0,
		"tcp_port_39001_timeout":                1,
		"tcp_port_39002_current_state_duration": int64(collr.UpdateEvery),
		"tcp_port_39002_latency":                0,
		"tcp_port_39002_success":                0,
		"tcp_port_39002_timeout":                1,
		"tcp_port_39002_failed":                 0,
	}
	mx = collr.Collect(context.Background())
	copyLatencyDuration(expected, mx)

	assert.Equal(t, expected, mx)
}

func testDial(err error) dialTCPFunc {
	return func(_, _ string, _ time.Duration) (net.Conn, error) { return &net.TCPConn{}, err }
}

type timeoutError struct{}

func (timeoutError) Error() string { return "timeout" }
func (timeoutError) Timeout() bool { return true }
