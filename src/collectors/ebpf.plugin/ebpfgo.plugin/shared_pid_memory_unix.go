//go:build linux && cgo

package main

/*
#include <stdlib.h>

#include "shared_pid_memory.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const sharedPidMemoryRowSize = C.sizeof_struct_ebpf_pid_stat

// assertSharedPidMemoryLayout panics if the Go ebpfPidStat layout drifts from
// the C struct that the SHM consumer reads.  This is a startup-only check; the
// cost is one uintptr compare and we only fail fast on a true ABI break.
func assertSharedPidMemoryLayout() {
	if got := unsafe.Sizeof(ebpfPidStat{}); got != uintptr(sharedPidMemoryRowSize) {
		panic(fmt.Sprintf("ebpf_pid_stat ABI mismatch: Go=%d C=%d", got, sharedPidMemoryRowSize))
	}
}

type SharedPidMemoryPublisher struct {
	ptr *C.struct_shared_pid_memory
}

func NewSharedPidMemoryPublisher(total uint32) (*SharedPidMemoryPublisher, error) {
	assertSharedPidMemoryLayout()

	ctx := C.shared_pid_memory_open(C.size_t(total))
	if ctx == nil {
		return nil, fmt.Errorf("open shared pid memory failed")
	}

	return &SharedPidMemoryPublisher{ptr: ctx}, nil
}

func (p *SharedPidMemoryPublisher) Publish(entries []ebpfPidStat, flags uint32) error {
	if p == nil || p.ptr == nil {
		return nil
	}

	var ptr unsafe.Pointer
	if len(entries) > 0 {
		ptr = unsafe.Pointer(&entries[0])
	}

	if ret := C.shared_pid_memory_publish(p.ptr, (*C.struct_ebpf_pid_stat)(ptr), C.size_t(len(entries)), C.uint32_t(flags)); ret != 0 {
		return fmt.Errorf("publish shared pid memory failed: %d", int(ret))
	}

	return nil
}

func (p *SharedPidMemoryPublisher) Close() {
	if p == nil || p.ptr == nil {
		return
	}

	C.shared_pid_memory_close(p.ptr)
	p.ptr = nil
}
