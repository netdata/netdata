#include <cstdlib>
#include <memory>

#if defined(__linux__)
  #include <malloc.h>
#elif defined(__APPLE__)
  #include <malloc/malloc.h>
  #define malloc_usable_size(p) malloc_size(p)
#elif defined(__FreeBSD__)
  #include <malloc_np.h>
#else
  // On unknown platforms, fall back to a zero return for the unsized delete
  // path. The counter will not decrement; matches pre-fix behavior on
  // those platforms.
  static inline size_t malloc_usable_size(void *) { return 0; }
#endif

#include "ml_memory.h"
#include "daemon/pulse/pulse-ml.h"

// The six operator overrides below replace the global C++ allocator hooks
// for the netdata binary (only compiled when ENABLE_MIMALLOC is OFF).
// They unconditionally call malloc/free; the pulse_ml_memory_* counters are
// updated only when ml_alloc_active is true on the calling thread, so that
// the ML memory chart reflects ML allocations and not unrelated C++ work
// (notably ACLK protobuf serialization) that happens to share the same
// global operator new/delete.
//
// All six paths count the allocator block size returned by
// malloc_usable_size(ptr), not the size argument to operator new/delete.
// This is what makes the counter balance regardless of whether the
// compiler emits sized delete calls: when -fsized-deallocation is off,
// every free routes through the unsized overload; when it is on, GCC may
// emit either form depending on type knowledge. By using
// malloc_usable_size(ptr) on every path, the per-pointer alloc/free pair
// is symmetric independent of which delete form the compiler chose.
//
// Side effect: the reported byte total reflects actual allocator block
// sizes (including alignment/rounding slack), not requested sizes. This
// is a more accurate measure of real RAM consumption than requested-size
// accounting.

void *operator new(size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    if (ml_alloc_active)
        pulse_ml_memory_allocated(malloc_usable_size(ptr));
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void *operator new[](size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    if (ml_alloc_active)
        pulse_ml_memory_allocated(malloc_usable_size(ptr));
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void operator delete(void *ptr, size_t /*size*/) noexcept
{
    if (ptr) {
        if (ml_alloc_active)
            pulse_ml_memory_freed(malloc_usable_size(ptr));
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, size_t /*size*/) noexcept
{
    if (ptr) {
        if (ml_alloc_active)
            pulse_ml_memory_freed(malloc_usable_size(ptr));
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete(void *ptr) noexcept
{
    if (ptr) {
        if (ml_alloc_active)
            pulse_ml_memory_freed(malloc_usable_size(ptr));
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr) noexcept
{
    if (ptr) {
        if (ml_alloc_active)
            pulse_ml_memory_freed(malloc_usable_size(ptr));
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}
