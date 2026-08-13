//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct netdata_ebpf_dcstat_runtime;
struct netdata_ebpf_dcstat_snapshot {
    unsigned long long reference;
    unsigned long long slow;
    unsigned long long miss;
};

struct netdata_ebpf_dcstat_pid_snapshot {
    unsigned int pid;
    unsigned int ppid;
    unsigned long long ct;
    char comm[96];
    unsigned long long cache_access;
    unsigned long long file_system;
    unsigned long long not_found;
};

struct netdata_ebpf_dcstat_pid_snapshot_list {
    struct netdata_ebpf_dcstat_pid_snapshot *items;
    size_t count;
};

struct netdata_ebpf_dcstat_runtime *netdata_dcstat_runtime_open_mode(const char *path, int use_core);
int netdata_dcstat_runtime_prepare(
    struct netdata_ebpf_dcstat_runtime *rt,
    unsigned int pid_table_size,
    int maps_per_core);
int netdata_dcstat_runtime_load(struct netdata_ebpf_dcstat_runtime *rt);
int netdata_dcstat_runtime_attach(
    struct netdata_ebpf_dcstat_runtime *rt,
    const char *lookup_fast_target,
    const char *d_lookup_target);
int netdata_dcstat_runtime_update_controller(
    struct netdata_ebpf_dcstat_runtime *rt,
    int apps_enabled,
    int apps_level);
int netdata_dcstat_runtime_supports_core(void);
int netdata_dcstat_runtime_snapshot(
    struct netdata_ebpf_dcstat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_dcstat_snapshot *out);
int netdata_dcstat_runtime_snapshot_apps(
    struct netdata_ebpf_dcstat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_dcstat_pid_snapshot_list *out);
void netdata_dcstat_runtime_free_apps_snapshot(struct netdata_ebpf_dcstat_pid_snapshot_list *out);
int netdata_dcstat_runtime_delete_pid(struct netdata_ebpf_dcstat_runtime *rt, unsigned int pid);
int netdata_dcstat_runtime_delete_pids(
    struct netdata_ebpf_dcstat_runtime *rt,
    unsigned int *pids,
    size_t count);
void netdata_dcstat_runtime_close(struct netdata_ebpf_dcstat_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type DCStatRuntime struct {
	ptr *C.struct_netdata_ebpf_dcstat_runtime

	// appsBuf is a persistent output buffer reused across SnapshotApps calls so
	// the per-cycle allocation pressure is zero in the steady state.  Owned by
	// the runtime; cleared in Close().
	//
	// LIFETIME: the slice SnapshotApps returns aliases this buffer.  Consume it
	// before the next SnapshotApps call; it must not be retained past that.
	appsBuf []DCStatAppSnapshot
}

// DCStatSupportsCore reports whether this build embeds the dcstat CO-RE
// skeletons.  It is separate from SupportsCore() (cachestat) because the two
// object families are compiled independently and one may be present without
// the other.
func DCStatSupportsCore() bool {
	return C.netdata_dcstat_runtime_supports_core() != 0
}

func NewDCStatRuntime(path string, useCore bool) (*DCStatRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	rt := C.netdata_dcstat_runtime_open_mode(cpath, cUseCore)
	if rt == nil {
		return nil, fmt.Errorf("open dcstat object %q failed", path)
	}

	return &DCStatRuntime{ptr: rt}, nil
}

func (r *DCStatRuntime) Prepare(pidTableSize uint32, mapsPerCore bool) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_dcstat_runtime_prepare(r.ptr, C.uint(pidTableSize), cMapsPerCore); ret != 0 {
		return fmt.Errorf("prepare dcstat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *DCStatRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_dcstat_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load dcstat runtime failed: %d", int(ret))
	}

	return nil
}

// Attach wires the kprobe on lookupFastTarget and the kretprobe on
// dLookupTarget.  Both names come from the caller because lookup_fast is a
// static kernel function whose symbol is often suffixed by the compiler.
func (r *DCStatRuntime) Attach(lookupFastTarget, dLookupTarget string) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cLookupFast := C.CString(lookupFastTarget)
	defer C.free(unsafe.Pointer(cLookupFast))
	cDLookup := C.CString(dLookupTarget)
	defer C.free(unsafe.Pointer(cDLookup))

	if ret := C.netdata_dcstat_runtime_attach(r.ptr, cLookupFast, cDLookup); ret != 0 {
		return fmt.Errorf("attach dcstat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *DCStatRuntime) UpdateController(appsEnabled bool, appsLevel int) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cAppsEnabled := C.int(0)
	if appsEnabled {
		cAppsEnabled = 1
	}

	if ret := C.netdata_dcstat_runtime_update_controller(r.ptr, cAppsEnabled, C.int(appsLevel)); ret != 0 {
		return fmt.Errorf("update dcstat controller failed: %d", int(ret))
	}

	return nil
}

func (r *DCStatRuntime) Snapshot(mapsPerCore bool) (DCStatSnapshot, error) {
	if r == nil || r.ptr == nil {
		return DCStatSnapshot{}, ErrDisabled
	}

	var cSnapshot C.struct_netdata_ebpf_dcstat_snapshot
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_dcstat_runtime_snapshot(r.ptr, cMapsPerCore, &cSnapshot); ret != 0 {
		return DCStatSnapshot{}, fmt.Errorf("snapshot dcstat runtime failed: %d", int(ret))
	}

	return DCStatSnapshot{
		Reference: uint64(cSnapshot.reference),
		Slow:      uint64(cSnapshot.slow),
		Miss:      uint64(cSnapshot.miss),
	}, nil
}

func (r *DCStatRuntime) SnapshotApps(mapsPerCore bool) ([]DCStatAppSnapshot, error) {
	if r == nil || r.ptr == nil {
		return nil, ErrDisabled
	}

	var cList C.struct_netdata_ebpf_dcstat_pid_snapshot_list
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_dcstat_runtime_snapshot_apps(r.ptr, cMapsPerCore, &cList); ret != 0 {
		return nil, fmt.Errorf("snapshot dcstat apps failed: %d", int(ret))
	}
	defer C.netdata_dcstat_runtime_free_apps_snapshot(&cList)

	out := r.appsBuf[:0]
	if cList.count == 0 || cList.items == nil {
		r.appsBuf = out
		return nil, nil
	}

	items := unsafe.Slice((*C.struct_netdata_ebpf_dcstat_pid_snapshot)(unsafe.Pointer(cList.items)), int(cList.count))
	if cap(out) < len(items) {
		out = make([]DCStatAppSnapshot, 0, len(items))
	}
	for _, item := range items {
		var comm [DCStatAppCommLen]byte
		copy(comm[:], unsafe.Slice((*byte)(unsafe.Pointer(&item.comm[0])), DCStatAppCommLen))
		out = append(out, DCStatAppSnapshot{
			Pid:         uint32(item.pid),
			Ppid:        uint32(item.ppid),
			Comm:        comm,
			Ct:          uint64(item.ct),
			CacheAccess: uint64(item.cache_access),
			FileSystem:  uint64(item.file_system),
			NotFound:    uint64(item.not_found),
		})
	}
	r.appsBuf = out
	return out, nil
}

func (r *DCStatRuntime) DeletePid(pid uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_dcstat_runtime_delete_pid(r.ptr, C.uint(pid)); ret != 0 {
		return fmt.Errorf("delete pid %d from dcstat_pid failed: %d", pid, int(ret))
	}

	return nil
}

// DeletePids removes a batch of stale PIDs in a single CGO call.  See
// CachestatRuntime.DeletePids for the batch/fallback rationale; the C side
// shares the same structure.  An empty input is a no-op and never errors.
func (r *DCStatRuntime) DeletePids(pids []uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}
	if len(pids) == 0 {
		return nil
	}

	if ret := C.netdata_dcstat_runtime_delete_pids(
		r.ptr,
		(*C.uint)(unsafe.Pointer(&pids[0])),
		C.size_t(len(pids)),
	); ret != 0 {
		return fmt.Errorf("delete %d pids from dcstat_pid failed: %d", len(pids), int(ret))
	}

	return nil
}

func (r *DCStatRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_dcstat_runtime_close(r.ptr)
	r.ptr = nil
	r.appsBuf = nil
}
