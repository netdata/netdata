#include <cstdlib>
#include <memory>

#include "daemon/pulse/pulse-ml.h"

void *operator new(size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    pulse_ml_memory_allocated(size);
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void *operator new[](size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    pulse_ml_memory_allocated(size);
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void operator delete(void *ptr, size_t size) noexcept
{
    if (ptr) {
        pulse_ml_memory_freed(size);
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, size_t size) noexcept
{
    if (ptr) {
        pulse_ml_memory_freed(size);
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

// The two unsized operator delete overloads below do NOT call
// pulse_ml_memory_freed(), so they neither subtract the bytes nor count the
// free. This is deliberate, not an oversight:
//
//   1. They have no size parameter, so there is nothing to subtract. Recovering
//      it means asking the allocator: malloc_usable_size() on glibc,
//      malloc_np.h on FreeBSD, malloc_size() on macOS, _msize() on Windows,
//      crossed with ENABLE_JEMALLOC / ENABLE_MIMALLOC which change what
//      "usable size" means. mallocz_usable_size() is not usable here either --
//      it is declared only under NETDATA_TRACE_ALLOCATIONS, and netdata's own
//      global malloc_usable_size() exists only under HAVE_DLSYM. That is a
//      per-platform shim in the allocation hot path.
//
//   2. It buys nothing today. With C++17 the compiler emits SIZED deallocation
//      for everything ML does: `delete dim` / `delete chart` on complete types
//      with non-virtual destructors, and std::vector / std::queue /
//      unordered_map deallocating through std::allocator, which calls
//      operator delete(p, n). Unsized delete is emitted mainly for deletion
//      through a base pointer without a virtual destructor, or on an incomplete
//      type -- neither of which occurs in ML.
//
// There is a standing check for this. pulse_ml_memory_freed() increments the
// "delete" dimension of netdata.ml_memory_ops, and it is called only from the
// sized overloads, so on that chart:
//
//      new - delete == the rate of unsized frees
//
// Measured on a 786-node production parent it is exactly zero (779.2/s vs
// 779.2/s), and netdata.ml_memory_used is flat over 24h. If ML ever gains
// polymorphic deletion that gap goes non-zero and netdata.ml_memory_used starts
// climbing without bound -- at which point the shim above becomes worth its
// cost. Until then it is not.

void operator delete(void *ptr) noexcept
{
    if (ptr) {
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr) noexcept
{
    if (ptr) {
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}
