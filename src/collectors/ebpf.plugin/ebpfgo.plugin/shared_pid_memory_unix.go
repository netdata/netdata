//go:build linux && cgo

package main

/*
#include <stddef.h>
#include <stdlib.h>

#include "shared_pid_memory.h"

// Inline offset getters for per-field ABI verification in assertSharedPidMemoryLayout.
// Declared as static inline to avoid external-linkage requirements across CGo
// translation units; CGo inlines the call and needs no exported symbol.
static inline size_t ebpf_pid_stat_off_pid_fn(void) { return offsetof(struct ebpf_pid_stat, pid); }
static inline size_t ebpf_pid_stat_off_comm_fn(void) { return offsetof(struct ebpf_pid_stat, comm); }
static inline size_t ebpf_pid_stat_off_ppid_fn(void) { return offsetof(struct ebpf_pid_stat, ppid); }
static inline size_t ebpf_pid_stat_off_cachestat_fn(void) { return offsetof(struct ebpf_pid_stat, cachestat); }
static inline size_t ebpf_pid_stat_off_dc_fn(void) { return offsetof(struct ebpf_pid_stat, dc); }
static inline size_t ebpf_pid_stat_off_fd_fn(void) { return offsetof(struct ebpf_pid_stat, fd); }
static inline size_t ebpf_pid_stat_off_process_fn(void) { return offsetof(struct ebpf_pid_stat, process); }
static inline size_t ebpf_pid_stat_off_shm_fn(void) { return offsetof(struct ebpf_pid_stat, shm); }
static inline size_t ebpf_pid_stat_off_swap_fn(void) { return offsetof(struct ebpf_pid_stat, swap); }
static inline size_t ebpf_pid_stat_off_socket_fn(void) { return offsetof(struct ebpf_pid_stat, socket); }
static inline size_t ebpf_pid_stat_off_vfs_fn(void) { return offsetof(struct ebpf_pid_stat, vfs); }
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const sharedPidMemoryRowSize = C.sizeof_struct_ebpf_pid_stat

// assertSharedPidMemoryLayout panics if the Go ebpfPidStat layout drifts from
// the C struct.
//
// EVERY field is checked, not a sample: the total size catches a field being
// added or resized, but a reorder that preserves size (two same-width modules
// swapped) moves offsets while sizeof stays put, so a partial check lets it
// through silently and every consumer then reads the wrong module's counters.
func assertSharedPidMemoryLayout() {
	if got := unsafe.Sizeof(ebpfPidStat{}); got != uintptr(sharedPidMemoryRowSize) {
		panic(fmt.Sprintf("ebpf_pid_stat ABI mismatch: Go=%d C=%d", got, sharedPidMemoryRowSize))
	}

	var row ebpfPidStat
	for _, f := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"pid", unsafe.Offsetof(row.pid), uintptr(C.ebpf_pid_stat_off_pid_fn())},
		{"comm", unsafe.Offsetof(row.comm), uintptr(C.ebpf_pid_stat_off_comm_fn())},
		{"ppid", unsafe.Offsetof(row.ppid), uintptr(C.ebpf_pid_stat_off_ppid_fn())},
		{"cachestat", unsafe.Offsetof(row.cachestat), uintptr(C.ebpf_pid_stat_off_cachestat_fn())},
		{"dc", unsafe.Offsetof(row.dc), uintptr(C.ebpf_pid_stat_off_dc_fn())},
		{"fd", unsafe.Offsetof(row.fd), uintptr(C.ebpf_pid_stat_off_fd_fn())},
		{"process", unsafe.Offsetof(row.process), uintptr(C.ebpf_pid_stat_off_process_fn())},
		{"shm", unsafe.Offsetof(row.shm), uintptr(C.ebpf_pid_stat_off_shm_fn())},
		{"swap", unsafe.Offsetof(row.swap), uintptr(C.ebpf_pid_stat_off_swap_fn())},
		{"socket", unsafe.Offsetof(row.socket), uintptr(C.ebpf_pid_stat_off_socket_fn())},
		{"vfs", unsafe.Offsetof(row.vfs), uintptr(C.ebpf_pid_stat_off_vfs_fn())},
	} {
		if f.got != f.want {
			panic(fmt.Sprintf("ebpf_pid_stat.%s offset mismatch: Go=%d C=%d", f.name, f.got, f.want))
		}
	}
}

type SharedPidMemoryPublisher struct {
	ptr *C.struct_shared_pid_memory
}

func NewSharedPidMemoryPublisher(shmName, semName string, total uint32, updateEverySec uint32) (*SharedPidMemoryPublisher, error) {
	assertSharedPidMemoryLayout()

	cShm := C.CString(shmName)
	defer C.free(unsafe.Pointer(cShm))
	cSem := C.CString(semName)
	defer C.free(unsafe.Pointer(cSem))

	ctx := C.shared_pid_memory_open(cShm, cSem, C.size_t(total), C.uint32_t(updateEverySec))
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
