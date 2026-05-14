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

// The four operator overrides below replace the global C++ allocator hooks
// for the netdata binary (only compiled when ENABLE_MIMALLOC is OFF).
// They unconditionally call malloc/free; the pulse_ml_memory_* counters are
// updated only when ml_alloc_active is true on the calling thread, so that
// the ML memory chart reflects ML allocations and not unrelated C++ work
// (notably ACLK protobuf serialization) that happens to share the same
// global operator new/delete.
//
// The two unsized delete overloads recover the allocator's block size via
// malloc_usable_size() so the counter decrements match the increments
// produced by the new overloads. The returned size may be slightly larger
// than the originally requested size (allocator rounding); the asymmetry
// versus the sized delete overloads is bounded and acceptable for a
// gauge-grade signal.

void *operator new(size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    if (ml_alloc_active)
        pulse_ml_memory_allocated(size);
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void *operator new[](size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    if (ml_alloc_active)
        pulse_ml_memory_allocated(size);
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void operator delete(void *ptr, size_t size) noexcept
{
    if (ptr) {
        if (ml_alloc_active)
            pulse_ml_memory_freed(size);
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, size_t size) noexcept
{
    if (ptr) {
        if (ml_alloc_active)
            pulse_ml_memory_freed(size);
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
