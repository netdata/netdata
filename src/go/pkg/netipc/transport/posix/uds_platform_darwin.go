//go:build darwin

package posix

import "syscall"

// darwin's syscall package has no MSG_NOSIGNAL; setNoSigpipe covers it.
const sendFlags = 0

// setNoSigpipe keeps a write to a closed peer from raising SIGPIPE in an
// embedding process (SO_NOSIGPIPE is macOS's documented way).
func setNoSigpipe(fd int) {
	_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_NOSIGPIPE, 1)
}
