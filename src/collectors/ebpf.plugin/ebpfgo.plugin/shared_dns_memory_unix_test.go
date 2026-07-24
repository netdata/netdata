//go:build linux && cgo

package main

import (
	"strings"
	"testing"
)

func TestCopyDNSDomainAlwaysTerminates(t *testing.T) {
	var dst [256]byte

	copyDNSDomain(&dst, strings.Repeat("a", len(dst)))

	if dst[len(dst)-1] != 0 {
		t.Fatalf("last domain byte = %d, want NUL terminator", dst[len(dst)-1])
	}
	for i := 0; i < len(dst)-1; i++ {
		if dst[i] != 'a' {
			t.Fatalf("domain byte %d = %d, want 'a'", i, dst[i])
		}
	}
}

func TestCopyDNSDomainTerminatesShortDomain(t *testing.T) {
	var dst [256]byte

	copyDNSDomain(&dst, "netdata.cloud")

	if got := string(dst[:13]); got != "netdata.cloud" {
		t.Fatalf("domain prefix = %q, want netdata.cloud", got)
	}
	if dst[13] != 0 {
		t.Fatalf("terminator byte = %d, want NUL", dst[13])
	}
}
