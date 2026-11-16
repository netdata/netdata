//go:build unix && !darwin && !freebsd

package raw

import "syscall"

const testSockType = syscall.SOCK_SEQPACKET
