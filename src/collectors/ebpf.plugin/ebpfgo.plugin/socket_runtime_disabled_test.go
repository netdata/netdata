//go:build !netdata_ebpf_libbpf

package main

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestSocketRuntimeDisabledBuild verifies that all SocketRuntime methods
// return ErrDisabled in the non-libbpf build.  This guards against silent
// no-ops silently masking load failures when libbpf is unavailable.
func TestSocketRuntimeDisabledBuild(t *testing.T) {
	_, err := libbpfloader.NewSocketRuntime("/dev/null", false)
	if !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("NewSocketRuntime: got %v, want ErrDisabled", err)
	}

	var rt *libbpfloader.SocketRuntime

	if err := rt.Prepare(false); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Prepare: got %v, want ErrDisabled", err)
	}
	if err := rt.Load(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Load: got %v, want ErrDisabled", err)
	}
	if err := rt.Attach(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Attach: got %v, want ErrDisabled", err)
	}
	rt.Close() // must not panic
}
