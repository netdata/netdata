//go:build unix && cgo

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

type SharedPidMemoryPublisher struct {
	ptr *C.struct_shared_pid_memory
}

func NewSharedPidMemoryPublisher(total uint32) (*SharedPidMemoryPublisher, error) {
	ctx := C.shared_pid_memory_open(C.size_t(total))
	if ctx == nil {
		return nil, fmt.Errorf("open shared pid memory failed")
	}

	return &SharedPidMemoryPublisher{ptr: ctx}, nil
}

func (p *SharedPidMemoryPublisher) Publish(entries []ebpfPidStat) error {
	if p == nil || p.ptr == nil {
		return nil
	}

	var ptr unsafe.Pointer
	if len(entries) > 0 {
		ptr = unsafe.Pointer(&entries[0])
	}

	if ret := C.shared_pid_memory_publish(p.ptr, (*C.struct_ebpf_pid_stat)(ptr), C.size_t(len(entries))); ret != 0 {
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
