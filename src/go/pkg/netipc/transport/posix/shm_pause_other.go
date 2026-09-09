//go:build linux && !amd64

package posix

import "runtime"

// spinPause is a no-op fallback on non-amd64 architectures.
// Gosched yields to the scheduler briefly, which is the closest
// portable equivalent to a CPU pause hint.
func spinPause() {
	runtime.Gosched()
}
