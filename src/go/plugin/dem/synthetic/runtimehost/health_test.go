// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

type healthLog struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (l *healthLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.data.Write(p)
}
func (l *healthLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.data.String() }

func TestSetupLoggingRequiresDemandAndChangedError(t *testing.T) {
	unavailable := errors.New("installed manifest missing")
	h := New(Config{Unavailable: unavailable})
	var logs healthLog
	h.log = logger.NewWithWriter(&logs)
	h.setState(StateUnavailable, unavailable)
	require.Empty(t, logs.String())
	intent, err := h.Prepare(collectorapi.JobConfigIdentity{}, collectorapi.JobConfigIdentity{1}, nil)
	require.NoError(t, err)
	intent.Commit()
	for range 3 {
		h.setState(StateUnavailable, unavailable)
	}
	require.Equal(t, 1, strings.Count(logs.String(), "DEM runtime setup unavailable"))
	h.setState(StateUnavailable, errors.New("runtime downloads disabled"))
	require.Equal(t, 2, strings.Count(logs.String(), "DEM runtime setup unavailable"))
	h.setState(StateReady, nil)
	require.Equal(t, 2, strings.Count(logs.String(), "DEM runtime"))
	require.NoError(t, h.Close())
}

func TestMaintenanceLoggingAndRecoveryAreOperationSpecific(t *testing.T) {
	h := New(Config{})
	var logs healthLog
	h.log = logger.NewWithWriter(&logs)
	h.setMaintenance("cleanup", errors.New("trash blocked"))
	h.setMaintenance("cleanup", errors.New("trash blocked"))
	require.Equal(t, 1, strings.Count(logs.String(), "cleanup failed"))
	h.setMaintenance("demand commit", errors.New("claim metadata blocked"))
	h.setMaintenance("cleanup", nil)
	require.Contains(t, h.Status().MaintenanceError, "claim metadata blocked")
	require.NotContains(t, h.Status().MaintenanceError, "trash blocked")
	require.Equal(t, 1, strings.Count(logs.String(), "cleanup recovered"))
	h.setMaintenance("demand commit", nil)
	require.Empty(t, h.Status().MaintenanceError)
	require.Equal(t, 1, strings.Count(logs.String(), "demand commit recovered"))
}
