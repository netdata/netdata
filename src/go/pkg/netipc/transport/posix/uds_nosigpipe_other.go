//go:build unix && !darwin && !linux && !freebsd

package posix

// MSG_NOSIGNAL on every send covers SIGPIPE here; only macOS needs a
// per-socket option.
func setNoSigpipe(int) {}
