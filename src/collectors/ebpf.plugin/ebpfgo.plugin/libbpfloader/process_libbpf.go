//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct netdata_ebpf_process_runtime;
struct netdata_ebpf_process_snapshot {
    unsigned long long exits;
    unsigned long long task_close;
    unsigned long long forks;
    unsigned long long clones;
    unsigned long long errors;
};

struct netdata_ebpf_process_pid_snapshot {
    unsigned int pid;
    unsigned int ppid;
    unsigned long long ct;
    char comm[96];
    unsigned int exits;
    unsigned int task_close;
    unsigned int forks;
    unsigned int clones;
    unsigned int errors;
};

struct netdata_ebpf_process_pid_snapshot_list {
    struct netdata_ebpf_process_pid_snapshot *items;
    size_t count;
};

struct netdata_ebpf_process_runtime *netdata_process_runtime_open_mode(const char *path, int use_core);
int netdata_process_runtime_prepare(
    struct netdata_ebpf_process_runtime *rt,
    unsigned int pid_table_size,
    int maps_per_core);
int netdata_process_runtime_load(struct netdata_ebpf_process_runtime *rt);
int netdata_process_runtime_attach(struct netdata_ebpf_process_runtime *rt);
int netdata_process_runtime_update_controller(
    struct netdata_ebpf_process_runtime *rt,
    int apps_enabled,
    int apps_level);
int netdata_process_runtime_supports_core(void);
int netdata_process_runtime_snapshot(
    struct netdata_ebpf_process_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_process_snapshot *out);
int netdata_process_runtime_snapshot_apps(
    struct netdata_ebpf_process_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_process_pid_snapshot_list *out);
int netdata_process_runtime_delete_pids(
    struct netdata_ebpf_process_runtime *rt,
    unsigned int *pids,
    size_t count);
void netdata_ebpf_process_pid_snapshot_list_free(struct netdata_ebpf_process_pid_snapshot_list *psl);
void netdata_process_runtime_close(struct netdata_ebpf_process_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type ProcessRuntime struct {
	ptr *C.struct_netdata_ebpf_process_runtime
}

func ProcessRuntimeSupportsCore() bool {
	return C.netdata_process_runtime_supports_core() != 0
}

func ProcessRuntimeOpenMode(path string, useCore bool) (*ProcessRuntime, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	rt := C.netdata_process_runtime_open_mode(cPath, cUseCore)
	if rt == nil {
		return nil, fmt.Errorf("open process object %q failed", path)
	}

	return &ProcessRuntime{ptr: rt}, nil
}

func (r *ProcessRuntime) Prepare(pidTableSize uint32, mapsPerCore bool) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_process_runtime_prepare(r.ptr, C.uint(pidTableSize), cMapsPerCore); ret != 0 {
		return fmt.Errorf("prepare process runtime failed: %d", int(ret))
	}

	return nil
}

func (r *ProcessRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_process_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load process runtime failed: %d", int(ret))
	}

	return nil
}

func (r *ProcessRuntime) Attach() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_process_runtime_attach(r.ptr); ret != 0 {
		return fmt.Errorf("attach process runtime failed: %d", int(ret))
	}

	return nil
}

func (r *ProcessRuntime) UpdateController(appsEnabled bool, appsLevel int) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cAppsEnabled := C.int(0)
	if appsEnabled {
		cAppsEnabled = 1
	}

	if ret := C.netdata_process_runtime_update_controller(r.ptr, cAppsEnabled, C.int(appsLevel)); ret != 0 {
		return fmt.Errorf("update process controller failed: %d", int(ret))
	}

	return nil
}

func (r *ProcessRuntime) SupportsCore() bool {
	return C.netdata_process_runtime_supports_core() != 0
}

func (r *ProcessRuntime) Snapshot(mapsPerCore bool) (ProcessSnapshot, error) {
	if r == nil || r.ptr == nil {
		return ProcessSnapshot{}, ErrDisabled
	}

	var cSnapshot C.struct_netdata_ebpf_process_snapshot
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_process_runtime_snapshot(r.ptr, cMapsPerCore, &cSnapshot); ret != 0 {
		return ProcessSnapshot{}, fmt.Errorf("snapshot process runtime failed: %d", int(ret))
	}

	return ProcessSnapshot{
		Exits:     uint64(cSnapshot.exits),
		TaskClose: uint64(cSnapshot.task_close),
		Forks:     uint64(cSnapshot.forks),
		Clones:    uint64(cSnapshot.clones),
		Errors:    uint64(cSnapshot.errors),
	}, nil
}

func (r *ProcessRuntime) SnapshotApps(mapsPerCore bool) ([]ProcessAppSnapshot, error) {
	if r == nil || r.ptr == nil {
		return nil, ErrDisabled
	}

	var cList C.struct_netdata_ebpf_process_pid_snapshot_list
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_process_runtime_snapshot_apps(r.ptr, cMapsPerCore, &cList); ret != 0 {
		return nil, fmt.Errorf("snapshot process apps failed: %d", int(ret))
	}
	defer C.netdata_ebpf_process_pid_snapshot_list_free(&cList)

	if cList.count == 0 || cList.items == nil {
		return nil, nil
	}

	items := unsafe.Slice((*C.struct_netdata_ebpf_process_pid_snapshot)(unsafe.Pointer(cList.items)), int(cList.count))
	result := make([]ProcessAppSnapshot, len(items))

	for i, item := range items {
		var comm [ProcessAppCommLen]byte
		copy(comm[:], unsafe.Slice((*byte)(unsafe.Pointer(&item.comm[0])), ProcessAppCommLen))
		result[i] = ProcessAppSnapshot{
			Pid:       uint32(item.pid),
			Ppid:      uint32(item.ppid),
			Comm:      comm,
			Ct:        uint64(item.ct),
			Exits:     uint32(item.exits),
			TaskClose: uint32(item.task_close),
			Forks:     uint32(item.forks),
			Clones:    uint32(item.clones),
			Errors:    uint32(item.errors),
		}
	}

	return result, nil
}

func (r *ProcessRuntime) DeletePids(pids []uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}
	if len(pids) == 0 {
		return nil
	}
	if ret := C.netdata_process_runtime_delete_pids(r.ptr, (*C.uint)(unsafe.Pointer(&pids[0])), C.size_t(len(pids))); ret != 0 {
		return fmt.Errorf("delete process PIDs failed: %d", int(ret))
	}
	return nil
}

func (r *ProcessRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	if r.ptr != nil {
		C.netdata_process_runtime_close(r.ptr)
		r.ptr = nil
	}
}
