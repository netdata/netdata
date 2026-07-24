//go:build linux && cgo

package main

/*
#include <stddef.h>
#include <stdlib.h>

#include "shared_pid_memory.h"

// Inline offset getters for per-field ABI verification in assertSharedPidMemoryLayout.
// Declared as static inline to avoid external-linkage requirements across CGo
// translation units; CGo inlines the call and needs no exported symbol.
static inline size_t ebpf_pid_stat_off_pid_fn(void)    { return offsetof(struct ebpf_pid_stat, pid); }
static inline size_t ebpf_pid_stat_off_socket_fn(void) { return offsetof(struct ebpf_pid_stat, socket); }
static inline size_t ebpf_pid_stat_off_vfs_fn(void)    { return offsetof(struct ebpf_pid_stat, vfs); }
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const sharedPidMemoryRowSize = C.sizeof_struct_ebpf_pid_stat

// assertSharedPidMemoryLayout panics if the Go ebpfPidStat layout drifts from
// the C struct.  Checks both total size and key field offsets so a field
// reorder that preserves struct size is also caught.
func assertSharedPidMemoryLayout() {
	if got := unsafe.Sizeof(ebpfPidStat{}); got != uintptr(sharedPidMemoryRowSize) {
		panic(fmt.Sprintf("ebpf_pid_stat ABI mismatch: Go=%d C=%d", got, sharedPidMemoryRowSize))
	}
	if got, want := unsafe.Offsetof(ebpfPidStat{}.pid), uintptr(C.ebpf_pid_stat_off_pid_fn()); got != want {
		panic(fmt.Sprintf("ebpf_pid_stat.pid offset mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Offsetof(ebpfPidStat{}.socket), uintptr(C.ebpf_pid_stat_off_socket_fn()); got != want {
		panic(fmt.Sprintf("ebpf_pid_stat.socket offset mismatch: Go=%d C=%d", got, want))
	}
	if got, want := unsafe.Offsetof(ebpfPidStat{}.vfs), uintptr(C.ebpf_pid_stat_off_vfs_fn()); got != want {
		panic(fmt.Sprintf("ebpf_pid_stat.vfs offset mismatch: Go=%d C=%d", got, want))
	}
}

type SharedPidMemoryPublisher struct {
	ptr *C.struct_shared_pid_memory
}

func NewSharedPidMemoryPublisher(total uint32, updateEverySec uint32) (*SharedPidMemoryPublisher, error) {
	assertSharedPidMemoryLayout()

	ctx := C.shared_pid_memory_open(C.size_t(total), C.uint32_t(updateEverySec))
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
