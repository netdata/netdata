//go:build windows && amd64

package windows

// spinPause emits a PAUSE instruction (defined in shm_pause_amd64.s).
func spinPause()
