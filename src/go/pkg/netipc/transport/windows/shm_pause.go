//go:build windows && !amd64

package windows

// spinPause yields the current Windows thread on architectures where we
// do not provide a CPU pause instruction helper.
func spinPause() {
	procSwitchToThread.Call()
}
