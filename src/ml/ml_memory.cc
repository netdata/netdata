#include <cstdlib>
#include <memory>

#if defined(__linux__)
  #include <malloc.h>
  #define ML_HAVE_MALLOC_USABLE_SIZE 1
#elif defined(__APPLE__)
  #include <malloc/malloc.h>
  #define malloc_usable_size(p) malloc_size(p)
  #define ML_HAVE_MALLOC_USABLE_SIZE 1
#elif defined(__FreeBSD__)
  #include <malloc_np.h>
  #define ML_HAVE_MALLOC_USABLE_SIZE 1
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
// On platforms with malloc_usable_size() (Linux/macOS/FreeBSD), every path
// reports the allocator block size returned by malloc_usable_size(ptr),
// not the size argument. This makes alloc/free symmetric regardless of
// which delete form the compiler emits: when -fsized-deallocation is off,
// every free routes through the unsized overload; when it is on, GCC may
// emit either form depending on type knowledge. By using
// malloc_usable_size(ptr) on every path, the per-pointer alloc/free pair
// is symmetric independent of which delete form the compiler chose. The
// reported byte total also reflects actual allocator block sizes
// (including alignment/rounding slack), not requested sizes -- a more
// accurate measure of real RAM consumption.
//
// On platforms without malloc_usable_size() (everything that is not
// Linux/macOS/FreeBSD), the sized paths attribute the requested size to
// keep the counter meaningful instead of flat-zero. The unsized delete
// overloads have no size to fall back to; they decrement zero and rely
// on the saturating pulse_ml_memory_freed() to prevent underflow. Net
// effect on these platforms: a small persistent over-count proportional
// to the share of frees that route through unsized delete.

void *operator new(size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_allocated(malloc_usable_size(ptr));
#else
        pulse_ml_memory_allocated(size);
#endif
    }
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void *operator new[](size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_allocated(malloc_usable_size(ptr));
#else
        pulse_ml_memory_allocated(size);
#endif
    }
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void operator delete(void *ptr, size_t size) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
            (void)size;
#else
            pulse_ml_memory_freed(size);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, size_t size) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
            (void)size;
#else
            pulse_ml_memory_freed(size);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete(void *ptr) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(0);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(0);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}
