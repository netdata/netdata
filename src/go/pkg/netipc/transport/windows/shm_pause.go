//go:build windows

package windows

// spinPause yields the current Windows thread during spin loops without
// involving the Go scheduler's heavier goroutine handoff path.
func spinPause() {
	procSwitchToThread.Call()
}
