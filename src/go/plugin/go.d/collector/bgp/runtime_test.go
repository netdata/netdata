// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"errors"
	"io/fs"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockTimeoutError struct{}

func (mockTimeoutError) Error() string   { return "timeout" }
func (mockTimeoutError) Timeout() bool   { return true }
func (mockTimeoutError) Temporary() bool { return false }

func TestClassifyCollectorError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: nil, want: collectorStatusOK},
		{err: fs.ErrPermission, want: collectorStatusPermissionError},
		{err: syscall.EACCES, want: collectorStatusPermissionError},
		{err: syscall.EPERM, want: collectorStatusPermissionError},
		{err: &net.OpError{Err: mockTimeoutError{}}, want: collectorStatusTimeout},
		{err: errors.New("broken"), want: collectorStatusQueryError},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.want, classifyCollectorError(tc.err))
	}
}
