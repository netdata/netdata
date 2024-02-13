// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	job := New()

	assert.Implements(t, (*module.Module)(nil), job)
}

func TestPortCheck_Init(t *testing.T) {
	job := New()

	job.Host = "127.0.0.1"
	job.Ports = []int{39001, 39002}
	assert.True(t, job.Init())
	assert.Len(t, job.ports, 2)
}
func TestPortCheck_InitNG(t *testing.T) {
	job := New()

	assert.False(t, job.Init())
	job.Host = "127.0.0.1"
	assert.False(t, job.Init())
	job.Ports = []int{39001, 39002}
	assert.True(t, job.Init())
}

func TestPortCheck_Check(t *testing.T) {
	assert.True(t, New().Check())
}

func TestPortCheck_Cleanup(t *testing.T) {
	New().Cleanup()
}

func TestPortCheck_Charts(t *testing.T) {
	job := New()
	job.Ports = []int{1, 2}
	job.Host = "localhost"
	require.True(t, job.Init())
	assert.Len(t, *job.Charts(), len(chartsTmpl)*len(job.Ports))
}

func TestPortCheck_Collect(t *testing.T) {
	job := New()

	job.Host = "127.0.0.1"
	job.Ports = []int{39001, 39002}
	job.UpdateEvery = 5
	job.dial = testDial(nil)
	require.True(t, job.Init())
	require.True(t, job.Check())

	copyLatency := func(dst, src map[string]int64) {
		for k := range dst {
			if strings.HasSuffix(k, "latency") {
				dst[k] = src[k]
			}
		}
	}

	expected := map[string]int64{
		"port_39001_current_state_duration": int64(job.UpdateEvery),
		"port_39001_failed":                 0,
		"port_39001_latency":                0,
		"port_39001_success":                1,
		"port_39001_timeout":                0,
		"port_39002_current_state_duration": int64(job.UpdateEvery),
		"port_39002_failed":                 0,
		"port_39002_latency":                0,
		"port_39002_success":                1,
		"port_39002_timeout":                0,
	}
	collected := job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)

	expected = map[string]int64{
		"port_39001_current_state_duration": int64(job.UpdateEvery) * 2,
		"port_39001_failed":                 0,
		"port_39001_latency":                0,
		"port_39001_success":                1,
		"port_39001_timeout":                0,
		"port_39002_current_state_duration": int64(job.UpdateEvery) * 2,
		"port_39002_failed":                 0,
		"port_39002_latency":                0,
		"port_39002_success":                1,
		"port_39002_timeout":                0,
	}
	collected = job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)

	job.dial = testDial(errors.New("checkStateFailed"))

	expected = map[string]int64{
		"port_39001_current_state_duration": int64(job.UpdateEvery),
		"port_39001_failed":                 1,
		"port_39001_latency":                0,
		"port_39001_success":                0,
		"port_39001_timeout":                0,
		"port_39002_current_state_duration": int64(job.UpdateEvery),
		"port_39002_failed":                 1,
		"port_39002_latency":                0,
		"port_39002_success":                0,
		"port_39002_timeout":                0,
	}
	collected = job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)

	job.dial = testDial(timeoutError{})

	expected = map[string]int64{
		"port_39001_current_state_duration": int64(job.UpdateEvery),
		"port_39001_failed":                 0,
		"port_39001_latency":                0,
		"port_39001_success":                0,
		"port_39001_timeout":                1,
		"port_39002_current_state_duration": int64(job.UpdateEvery),
		"port_39002_failed":                 0,
		"port_39002_latency":                0,
		"port_39002_success":                0,
		"port_39002_timeout":                1,
	}
	collected = job.Collect()
	copyLatency(expected, collected)

	assert.Equal(t, expected, collected)
}

func testDial(err error) dialFunc {
	return func(_, _ string, _ time.Duration) (net.Conn, error) { return &net.TCPConn{}, err }
}

type timeoutError struct{}

func (timeoutError) Error() string { return "timeout" }
func (timeoutError) Timeout() bool { return true }
