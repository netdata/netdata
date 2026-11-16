//go:build unix && !darwin

package posix

import "syscall"

const sendFlags = syscall.MSG_NOSIGNAL
