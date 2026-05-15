//go:build unix && cgo

package main

import (
	"testing"
	"unsafe"
)

func TestSharedPidMemoryLayoutMatchesGo(t *testing.T) {
	if got, want := unsafe.Sizeof(ebpfPidStat{}), uintptr(sharedPidMemoryRowSize); got != want {
		t.Fatalf("ebpfPidStat size = %d, want %d", got, want)
	}
}
