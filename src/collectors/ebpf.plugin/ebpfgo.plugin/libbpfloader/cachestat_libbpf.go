//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct netdata_ebpf_cachestat_runtime;
struct netdata_ebpf_cachestat_snapshot {
    unsigned long long mark_page_accessed;
    unsigned long long mark_buffer_dirty;
    unsigned long long add_to_page_cache_lru;
    unsigned long long account_page_dirtied;
};

struct netdata_ebpf_cachestat_pid_snapshot {
    unsigned int pid;
    unsigned int ppid;
    unsigned long long ct;
    char comm[96];
    unsigned int add_to_page_cache_lru;
    unsigned int mark_page_accessed;
    unsigned int account_page_dirtied;
    unsigned int mark_buffer_dirty;
};

struct netdata_ebpf_cachestat_pid_snapshot_list {
    struct netdata_ebpf_cachestat_pid_snapshot *items;
    size_t count;
};

struct netdata_ebpf_cachestat_runtime *netdata_cachestat_runtime_open_mode(const char *path, int use_core);
int netdata_cachestat_runtime_prepare(
    struct netdata_ebpf_cachestat_runtime *rt,
    unsigned int pid_table_size,
    int maps_per_core,
    const char *account_function);
int netdata_cachestat_runtime_load(struct netdata_ebpf_cachestat_runtime *rt);
int netdata_cachestat_runtime_attach(struct netdata_ebpf_cachestat_runtime *rt, const char *account_function);
int netdata_cachestat_runtime_update_controller(
    struct netdata_ebpf_cachestat_runtime *rt,
    int apps_enabled,
    int apps_level);
int netdata_cachestat_runtime_supports_core(void);
int netdata_cachestat_runtime_snapshot(
    struct netdata_ebpf_cachestat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_cachestat_snapshot *out);
int netdata_cachestat_runtime_snapshot_apps(
    struct netdata_ebpf_cachestat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_cachestat_pid_snapshot_list *out);
void netdata_cachestat_runtime_free_apps_snapshot(struct netdata_ebpf_cachestat_pid_snapshot_list *out);
int netdata_cachestat_runtime_delete_pid(struct netdata_ebpf_cachestat_runtime *rt, unsigned int pid);
int netdata_cachestat_runtime_delete_pids(
    struct netdata_ebpf_cachestat_runtime *rt,
    unsigned int *pids,
    size_t count);
int netdata_cachestat_runtime_pid_is_alive(unsigned int pid);
void netdata_cachestat_runtime_close(struct netdata_ebpf_cachestat_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type CachestatRuntime struct {
	ptr *C.struct_netdata_ebpf_cachestat_runtime

	// appsBuf is a persistent output buffer reused across SnapshotApps calls
	// so the per-cycle allocation pressure is zero in the steady state.
	// Each call resets it to len==0 and rebuilds; the slice's backing array
	// is preserved so growth is amortised.  Owned by the runtime; cleared
	// (set to nil) in Close().
	appsBuf []CachestatAppSnapshot
}

type CachestatRuntimeConfig struct {
	PidTableSize    uint32
	MapsPerCore     bool
	AccountFunction string
}

// NETDATA_APPS_LEVEL_ALL (2): BPF key = TID; values[i].tgid = process TGID.
// cgroup.procs contains TGIDs, so we must index shared memory by TGID, not by
// the parent's TGID that REAL_PARENT (0) would produce.
const cachestatAppsLevelAll = 2

func SupportsCore() bool {
	return C.netdata_cachestat_runtime_supports_core() != 0
}

func NewCachestatRuntime(path string, useCore bool) (*CachestatRuntime, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	cUseCore := C.int(0)
	if useCore {
		cUseCore = 1
	}

	rt := C.netdata_cachestat_runtime_open_mode(cpath, cUseCore)
	if rt == nil {
		return nil, fmt.Errorf("open cachestat object %q failed", path)
	}

	return &CachestatRuntime{ptr: rt}, nil
}

func (r *CachestatRuntime) Prepare(pidTableSize uint32, mapsPerCore bool, accountFunction string) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	cfn := C.CString(accountFunction)
	defer C.free(unsafe.Pointer(cfn))

	if ret := C.netdata_cachestat_runtime_prepare(r.ptr, C.uint(pidTableSize), cMapsPerCore, cfn); ret != 0 {
		return fmt.Errorf("prepare cachestat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) Load() error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_cachestat_runtime_load(r.ptr); ret != 0 {
		return fmt.Errorf("load cachestat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) Attach(accountFunction string) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cfn := C.CString(accountFunction)
	defer C.free(unsafe.Pointer(cfn))

	if ret := C.netdata_cachestat_runtime_attach(r.ptr, cfn); ret != 0 {
		return fmt.Errorf("attach cachestat runtime failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) UpdateController(appsEnabled bool, appsLevel int) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cAppsEnabled := C.int(0)
	if appsEnabled {
		cAppsEnabled = 1
	}

	if ret := C.netdata_cachestat_runtime_update_controller(r.ptr, cAppsEnabled, C.int(appsLevel)); ret != 0 {
		return fmt.Errorf("update cachestat controller failed: %d", int(ret))
	}

	return nil
}

func (r *CachestatRuntime) Snapshot(mapsPerCore bool) (CachestatSnapshot, error) {
	if r == nil || r.ptr == nil {
		return CachestatSnapshot{}, ErrDisabled
	}

	var cSnapshot C.struct_netdata_ebpf_cachestat_snapshot
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_cachestat_runtime_snapshot(r.ptr, cMapsPerCore, &cSnapshot); ret != 0 {
		return CachestatSnapshot{}, fmt.Errorf("snapshot cachestat runtime failed: %d", int(ret))
	}

	return CachestatSnapshot{
		MarkPageAccessed:   uint64(cSnapshot.mark_page_accessed),
		MarkBufferDirty:    uint64(cSnapshot.mark_buffer_dirty),
		AddToPageCacheLru:  uint64(cSnapshot.add_to_page_cache_lru),
		AccountPageDirtied: uint64(cSnapshot.account_page_dirtied),
	}, nil
}

func (r *CachestatRuntime) SnapshotApps(mapsPerCore bool) ([]CachestatAppSnapshot, error) {
	if r == nil || r.ptr == nil {
		return nil, ErrDisabled
	}

	var cList C.struct_netdata_ebpf_cachestat_pid_snapshot_list
	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_cachestat_runtime_snapshot_apps(r.ptr, cMapsPerCore, &cList); ret != 0 {
		return nil, fmt.Errorf("snapshot cachestat apps failed: %d", int(ret))
	}
	defer C.netdata_cachestat_runtime_free_apps_snapshot(&cList)

	// Reuse the persistent buffer.  Each cycle we reset to len==0 and
	// rebuild from the C-side items.  growth is amortised via the preserved
	// backing array; per-cycle allocation is zero in the steady state.
	out := r.appsBuf[:0]
	if cList.count == 0 || cList.items == nil {
		r.appsBuf = out
		return nil, nil
	}

	items := unsafe.Slice((*C.struct_netdata_ebpf_cachestat_pid_snapshot)(unsafe.Pointer(cList.items)), int(cList.count))
	if cap(out) < len(items) {
		out = make([]CachestatAppSnapshot, 0, len(items))
	}
	for _, item := range items {
		var comm [CachestatAppCommLen]byte
		copy(comm[:], unsafe.Slice((*byte)(unsafe.Pointer(&item.comm[0])), CachestatAppCommLen))
		out = append(out, CachestatAppSnapshot{
			Pid:                uint32(item.pid),
			Ppid:               uint32(item.ppid),
			Comm:               comm,
			Ct:                 uint64(item.ct),
			AddToPageCacheLru:  uint32(item.add_to_page_cache_lru),
			MarkPageAccessed:   uint32(item.mark_page_accessed),
			AccountPageDirtied: uint32(item.account_page_dirtied),
			MarkBufferDirty:    uint32(item.mark_buffer_dirty),
		})
	}
	r.appsBuf = out
	return out, nil
}

func (r *CachestatRuntime) DeletePid(pid uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_cachestat_runtime_delete_pid(r.ptr, C.uint(pid)); ret != 0 {
		return fmt.Errorf("delete pid %d from cstat_pid failed: %d", pid, int(ret))
	}

	return nil
}

// DeletePids removes a batch of stale PIDs in a single CGO call.  On
// kernel >= 5.6 the runtime uses bpf_map_delete_batch; older kernels fall
// back to a tight C loop of bpf_map_delete_elem.  Both paths also handle the
// buffer/arena accumulator eviction.  The C-side single round-trip
// replaces N CGO calls and amortises the cost of any per-call bookkeeping
// (e.g. acc_htable_rebuild for the accumulator).
//
// An empty input is a no-op and never errors.
func (r *CachestatRuntime) DeletePids(pids []uint32) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}
	if len(pids) == 0 {
		return nil
	}

	if ret := C.netdata_cachestat_runtime_delete_pids(
		r.ptr,
		(*C.uint)(unsafe.Pointer(&pids[0])),
		C.size_t(len(pids)),
	); ret != 0 {
		return fmt.Errorf("delete %d pids from cstat_pid failed: %d", len(pids), int(ret))
	}

	return nil
}

// PidIsAlive reports whether pid is currently a running process.  It uses
// the same kill(pid, 0) check the legacy C collector used to detect exited
// PIDs; this matches the historical semantics and avoids evicting PIDs
// that are merely idle (no BPF ct activity).
func PidIsAlive(pid uint32) bool {
	return C.netdata_cachestat_runtime_pid_is_alive(C.uint(pid)) != 0
}

func (r *CachestatRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_cachestat_runtime_close(r.ptr)
	r.ptr = nil
	r.appsBuf = nil
}
