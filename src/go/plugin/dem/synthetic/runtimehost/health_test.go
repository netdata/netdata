// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
)

type healthLog struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func TestMissingNativePackageHasPackageGuidanceAndSafeClose(t *testing.T) {
	h := New(Config{Mode: "native", Unavailable: runtime.ErrUnavailable})
	require.ErrorContains(t, h.Ready(), "netdata-dem-runtime package")
	require.Equal(t, StateMissing, h.Status().State)
	require.Equal(t, "native", h.Status().Mode)
	require.Nil(t, h.Status().Storage)
	require.NoError(t, h.Close())
}

func (l *healthLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.data.Write(p)
}
func (l *healthLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.data.String() }

func TestUnavailableSetupKeepsDependenciesUsable(t *testing.T) {
	h := New(Config{Unavailable: errors.New("runtime unavailable")})
	defer h.Close()
	dependencies, err := h.Dependencies(nil)
	require.NoError(t, err)
	require.Len(t, dependencies, 1)
	require.ErrorContains(t, dependencies[0].Ready(), "runtime unavailable")
	_, err = h.BeginUse()
	require.Error(t, err)
}

func TestMaintenanceLoggingAndRecoveryAreOperationSpecific(t *testing.T) {
	h := New(Config{})
	var logs healthLog
	h.log = logger.NewWithWriter(&logs)
	h.setMaintenance("cleanup", errors.New("trash blocked"))
	h.setMaintenance("cleanup", errors.New("trash blocked"))
	require.Equal(t, 1, strings.Count(logs.String(), "cleanup failed"))
	h.setMaintenance("execution completion", errors.New("execution metadata blocked"))
	h.setMaintenance("cleanup", nil)
	require.Contains(t, h.Status().MaintenanceError, "execution metadata blocked")
	require.NotContains(t, h.Status().MaintenanceError, "trash blocked")
	require.Equal(t, 1, strings.Count(logs.String(), "cleanup recovered"))
	h.setMaintenance("execution completion", nil)
	require.Empty(t, h.Status().MaintenanceError)
	require.Equal(t, 1, strings.Count(logs.String(), "execution completion recovered"))
}
