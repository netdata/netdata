// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeError_Unwrap(t *testing.T) {
	err := &ProbeError{Host: "host", Stage: "run", Err: syscall.EPERM}

	var errno syscall.Errno
	require.True(t, errors.As(err, &errno))
	assert.Equal(t, syscall.EPERM, errno)
}

func TestClient_ProbePreservesErrno(t *testing.T) {
	runner := &fakeRunner{
		byHost: map[string][]fakeResult{
			"host": {{err: syscall.EPERM}},
		},
	}

	c, err := newClient(testConfig(), logger.NewWithWriter(nil), runner)
	require.NoError(t, err)

	_, err = c.Probe(context.Background(), "host")
	require.Error(t, err)

	var probeErr *ProbeError
	require.True(t, errors.As(err, &probeErr))
	assert.Equal(t, "host", probeErr.Host)

	var errno syscall.Errno
	require.True(t, errors.As(err, &errno))
	assert.Equal(t, syscall.EPERM, errno)
}
