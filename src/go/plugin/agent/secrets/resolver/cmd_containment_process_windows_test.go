//go:build windows

package secretresolver

func resolverContainmentSupported() bool {
	return false
}

func resolverProcessGone(int) bool {
	return true
}
