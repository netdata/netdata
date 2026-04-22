//go:build linux && amd64

package posix

// spinPause emits a PAUSE instruction (defined in shm_pause_amd64.s).
func spinPause()
