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
int netdata_cachestat_runtime_prepare(struct netdata_ebpf_cachestat_runtime *rt, unsigned int pid_table_size, int maps_per_core);
int netdata_cachestat_runtime_load(struct netdata_ebpf_cachestat_runtime *rt);
int netdata_cachestat_runtime_attach(struct netdata_ebpf_cachestat_runtime *rt, const char *account_function);
int netdata_cachestat_runtime_snapshot(
    struct netdata_ebpf_cachestat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_cachestat_snapshot *out);
int netdata_cachestat_runtime_snapshot_apps(
    struct netdata_ebpf_cachestat_runtime *rt,
    int maps_per_core,
    struct netdata_ebpf_cachestat_pid_snapshot_list *out);
void netdata_cachestat_runtime_free_apps_snapshot(struct netdata_ebpf_cachestat_pid_snapshot_list *out);
void netdata_cachestat_runtime_close(struct netdata_ebpf_cachestat_runtime *rt);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type CachestatRuntime struct {
	ptr *C.struct_netdata_ebpf_cachestat_runtime
}

type CachestatRuntimeConfig struct {
	PidTableSize    uint32
	MapsPerCore     bool
	AccountFunction string
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

func (r *CachestatRuntime) Prepare(pidTableSize uint32, mapsPerCore bool) error {
	if r == nil || r.ptr == nil {
		return ErrDisabled
	}

	cMapsPerCore := C.int(0)
	if mapsPerCore {
		cMapsPerCore = 1
	}

	if ret := C.netdata_cachestat_runtime_prepare(r.ptr, C.uint(pidTableSize), cMapsPerCore); ret != 0 {
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

	if cList.count == 0 || cList.items == nil {
		return nil, nil
	}

	items := unsafe.Slice((*C.struct_netdata_ebpf_cachestat_pid_snapshot)(unsafe.Pointer(cList.items)), int(cList.count))
	out := make([]CachestatAppSnapshot, 0, len(items))
	for _, item := range items {
		var comm [CachestatAppCommLen]byte
		copy(comm[:], C.GoBytes(unsafe.Pointer(&item.comm[0]), C.int(CachestatAppCommLen)))
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

	return out, nil
}

func (r *CachestatRuntime) Close() {
	if r == nil || r.ptr == nil {
		return
	}

	C.netdata_cachestat_runtime_close(r.ptr)
	r.ptr = nil
}
