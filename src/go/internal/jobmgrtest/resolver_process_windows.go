//go:build windows

package jobmgrtest

func ResolverDriverSupported() bool {
	return false
}

func resolverProcessGone(int) bool {
	return true
}
