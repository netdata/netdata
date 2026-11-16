//go:build darwin || freebsd

package raw

import "syscall"

// macOS and FreeBSD use the stream UDS baseline.
const testSockType = syscall.SOCK_STREAM
