//go:build !unix

package main

func runSharedSnapshotService() {
	// No-op on non-Unix platforms because this plugin cannot expose the Unix socket service there.
}
