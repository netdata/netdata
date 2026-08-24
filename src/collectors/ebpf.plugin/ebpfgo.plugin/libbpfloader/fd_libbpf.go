//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct netdata_ebpf_fd_runtime;
struct netdata_ebpf_fd_snapshot {
    unsigned long long open_call;
    unsigned long long open_err;
    unsigned long long close_call;
    unsigned long long close_err;
};

struct netdata_ebpf_fd_pid_snapshot {
    unsigned int pid;
    unsigned int ppid;
    unsigned long long ct;
    char comm[96];
    unsigned int open_call;
    unsigned int close_call;
    unsigned int open_err;
    unsigned int close_err;
};

struct netdata_ebpf_fd_pid_snapshot_list {
    struct netdata_ebpf_fd_pid_snapshot *items;
    size_t count;
};

struct netdata_ebpf_fd_runtime *netdata_fd_runtime_open_mode(const char *path, int use_core);
int netdata_fd_runtime_prepare(
    struct netdata_ebpf_fd_runtime *rt,
    unsigned int pid_table_size,
    int maps_per_core,
    const char *close_target);
int netdata_fd_runtime_load(struct netdata_ebpf_fd_runtime *rt);
int netdata_fd_runtime_attach(
    struct netdata_ebpf_fd_runtime *rt,
    const char *open_target,
    const char *close_target);
int netdata_fd_runtime_update_controller(
    struct netdata_ebpf_fd_runtime *rt,
    int apps_enabled,
    int apps_level);
int netdata_fd_runtime_supports_core(void);
int netdata_fd_runtime_snapshot(
    struct netdata_ebpf_fd_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_fd_snapshot *out);
int netdata_fd_runtime_snapshot_apps(
    struct netdata_ebpf_fd_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_fd_pid_snapshot_list *out);
void netdata_fd_runtime_free_apps_snapshot(struct netdata_ebpf_fd_pid_snapshot_list *out);
int netdata_fd_runtime_delete_pid(struct netdata_ebpf_fd_runtime *rt, unsigned int pid);
int netdata_fd_runtime_delete_pids(
    struct netdata_ebpf_fd_runtime *rt,
    unsigned int *pids,
    size_t count);
void netdata_fd_runtime_close(struct netdata_ebpf_fd_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type FDRuntime struct {
	ptr *C.struct_netdata_ebpf_fd_runtime

	// appsBuf is a persistent output buffer reused across SnapshotApps calls so
	// the per-cycle allocation pressure is zero in the steady state.  Owned by
	// the runtime; cleared in Close().
	//
	// LIFETIME: the slice SnapshotApps returns aliases this buffer.  Consume it
	// before the next SnapshotApps call; it must not be retained past that.
	appsBuf []FDAppSnapshot
}

// FDSupportsCore reports whether this build embeds the fd CO-RE skeletons.  It
// is separate from the cachestat and dcstat equivalents because the object
// families are compiled independently and one may be present without the others.
func FDSupportsCore() bool {
	return C.netdata_fd_runtime_supports_core() != 0
}

func NewFDRuntime(path string, useCore bool) (*FDRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	rt := C.netdata_fd_runtime_open_mode(cpath, cUseCore)
	if rt == nil {
		return nil, fmt.Errorf("open fd object %q failed", path)
	}

	return &FDRuntime{ptr: rt}, nil
}

// Prepare needs the resolved close symbol: the base object ships one close
// program per kernel symbol name and the one naming a symbol this host does not
// export would fail to load, taking the whole object with it.
func (r *FDRuntime) Prepare(pidTableSize uint32, mapsPerCore bool, closeTarget string) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	cClose := C.CString(closeTarget)
	defer C.free(unsafe.Pointer(cClose))

	if ret := C.netdata_fd_runtime_prepare(r.ptr, C.uint(pidTableSize), cMapsPerCore, cClose); ret != 0 {
		return fmt.Errorf("prepare fd runtime failed: %d", int(ret))
	}

	return nil
}

func (r *FDRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_fd_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load fd runtime failed: %d", int(ret))
	}

	return nil
}

// Attach wires both probes as return probes on openTarget and closeTarget.  The
// names come from the caller because the kernel symbol differs by version
// (do_sys_openat2 vs do_sys_open, close_fd vs __close_fd).
func (r *FDRuntime) Attach(openTarget, closeTarget string) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cOpen := C.CString(openTarget)
	defer C.free(unsafe.Pointer(cOpen))
	cClose := C.CString(closeTarget)
	defer C.free(unsafe.Pointer(cClose))

	if ret := C.netdata_fd_runtime_attach(r.ptr, cOpen, cClose); ret != 0 {
		return fmt.Errorf("attach fd runtime failed: %d", int(ret))
	}

	return nil
}

func (r *FDRuntime) UpdateController(appsEnabled bool, appsLevel int) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cAppsEnabled := C.int(0)
	if appsEnabled {
		cAppsEnabled = 1
	}

	if ret := C.netdata_fd_runtime_update_controller(r.ptr, cAppsEnabled, C.int(appsLevel)); ret != 0 {
		return fmt.Errorf("update fd controller failed: %d", int(ret))
	}

	return nil
}

func (r *FDRuntime) Snapshot(mapsPerCore bool) (FDSnapshot, error) {
	if r == nil || r.ptr == nil {
		return FDSnapshot{}, ErrDisabled
	}

	var cSnapshot C.struct_netdata_ebpf_fd_snapshot
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_fd_runtime_snapshot(r.ptr, cMapsPerCore, &cSnapshot); ret != 0 {
		return FDSnapshot{}, fmt.Errorf("snapshot fd runtime failed: %d", int(ret))
	}

	return FDSnapshot{
		OpenCall:  uint64(cSnapshot.open_call),
		OpenErr:   uint64(cSnapshot.open_err),
		CloseCall: uint64(cSnapshot.close_call),
		CloseErr:  uint64(cSnapshot.close_err),
	}, nil
}

func (r *FDRuntime) SnapshotApps(mapsPerCore bool) ([]FDAppSnapshot, error) {
	if r == nil || r.ptr == nil {
		return nil, ErrDisabled
	}

	var cList C.struct_netdata_ebpf_fd_pid_snapshot_list
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_fd_runtime_snapshot_apps(r.ptr, cMapsPerCore, &cList); ret != 0 {
		return nil, fmt.Errorf("snapshot fd apps failed: %d", int(ret))
	}
	defer C.netdata_fd_runtime_free_apps_snapshot(&cList)

	out := r.appsBuf[:0]
	if cList.count == 0 || cList.items == nil {
		r.appsBuf = out
		return nil, nil
	}

	items := unsafe.Slice((*C.struct_netdata_ebpf_fd_pid_snapshot)(unsafe.Pointer(cList.items)), int(cList.count))
	if cap(out) < len(items) {
		out = make([]FDAppSnapshot, 0, len(items))
	}
	for _, item := range items {
		var comm [FDAppCommLen]byte
		copy(comm[:], unsafe.Slice((*byte)(unsafe.Pointer(&item.comm[0])), FDAppCommLen))
		out = append(out, FDAppSnapshot{
			Pid:       uint32(item.pid),
			Ppid:      uint32(item.ppid),
			Comm:      comm,
			Ct:        uint64(item.ct),
			OpenCall:  uint32(item.open_call),
			CloseCall: uint32(item.close_call),
			OpenErr:   uint32(item.open_err),
			CloseErr:  uint32(item.close_err),
		})
	}
	r.appsBuf = out
	return out, nil
}

func (r *FDRuntime) DeletePid(pid uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_fd_runtime_delete_pid(r.ptr, C.uint(pid)); ret != 0 {
		return fmt.Errorf("delete pid %d from tbl_fd_pid failed: %d", pid, int(ret))
	}

	return nil
}

// DeletePids removes a batch of stale PIDs in a single CGO call.  See
// CachestatRuntime.DeletePids for the batch/fallback rationale; the C side
// shares the same structure.  An empty input is a no-op and never errors.
func (r *FDRuntime) DeletePids(pids []uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}
	if len(pids) == 0 {
		return nil
	}

	if ret := C.netdata_fd_runtime_delete_pids(
		r.ptr,
		(*C.uint)(unsafe.Pointer(&pids[0])),
		C.size_t(len(pids)),
	); ret != 0 {
		return fmt.Errorf("delete %d pids from tbl_fd_pid failed: %d", len(pids), int(ret))
	}

	return nil
}

func (r *FDRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_fd_runtime_close(r.ptr)
	r.ptr = nil
	r.appsBuf = nil
}
