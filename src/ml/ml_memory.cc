#include <cstdlib>
#include <new>

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

// The operator overrides below replace the global C++ allocator hooks for
// the netdata binary (only compiled when ENABLE_MIMALLOC is OFF). They
// unconditionally call malloc/free (or posix_memalign for over-aligned
// allocations); the pulse_ml_memory_* counters are updated only when
// ml_alloc_active is true on the calling thread, so that the ML memory
// chart reflects ML allocations and not unrelated C++ work (notably ACLK
// protobuf serialization) that happens to share the same global operator
// new/delete.
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
// to the share of frees that route through unsized delete. We accept
// this over a size-prefix scheme that would impose a per-allocation
// header on every build.
//
// Over-aligned types (alignof(T) > __STDCPP_DEFAULT_NEW_ALIGNMENT__)
// route through the std::align_val_t-tagged overloads below; without
// these, such allocations would silently bypass ML accounting.
//
// Known limitation: this is a thread-local accounting scheme. Whether an
// allocation contributes to pulse_ml_memory_* is decided at free time by
// the freeing thread's ml_alloc_active flag, not by the flag state at
// the time the matching new ran. The two mismatch directions are:
//
//   * alloc under MlAllocScope on thread A, free on thread B outside any
//     scope -- the allocation was counted, the matching free is not, so
//     ml_memory_consumption is permanently over-counted.
//   * alloc outside any scope, free on a thread inside MlAllocScope --
//     the free attempts a decrement with no matching increment; the
//     saturating atomic CAS in pulse_ml_memory_freed() clamps to zero.
//
// In practice the ML subsystem keeps allocations thread-local (worker
// threads own their km_contexts, training queues, sample buffers, etc.)
// and the user-facing entrypoints scope both allocation and destruction.
// A size-prefix header on every C++ allocation would close this gap
// fully at the cost of an unconditional per-allocation header; the
// current implementation accepts the small residual skew instead.

// Helper: posix_memalign requires alignment >= sizeof(void*) and a power
// of two. std::align_val_t is always a power of two, but in principle
// can be smaller than sizeof(void*); clamp upward for safety.
static inline size_t ml_min_posix_alignment(size_t alignment) noexcept
{
    return alignment < sizeof(void *) ? sizeof(void *) : alignment;
}

void *operator new(size_t size)
{
    // C++ requires operator new to return a non-null pointer even for a
    // zero-byte request; malloc(0) is implementation-defined and may
    // return nullptr.
    if (size == 0)
        size = 1;

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
    if (size == 0)
        size = 1;

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

void operator delete(void *ptr, [[maybe_unused]] size_t size) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(size);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, [[maybe_unused]] size_t size) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
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

// Over-aligned overloads (C++17). Triggered when alignof(T) exceeds
// __STDCPP_DEFAULT_NEW_ALIGNMENT__ (typically 16). posix_memalign is
// available on every platform with malloc_usable_size() above and on
// most POSIX systems otherwise; the resulting pointer is freed with
// plain free().

void *operator new(size_t size, std::align_val_t al)
{
    if (size == 0)
        size = 1;

    void *ptr = nullptr;
    if (posix_memalign(&ptr, ml_min_posix_alignment(static_cast<size_t>(al)), size) != 0 || !ptr)
        throw std::bad_alloc();

    if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_allocated(malloc_usable_size(ptr));
#else
        pulse_ml_memory_allocated(size);
#endif
    }
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN);
    return ptr;
}

void *operator new[](size_t size, std::align_val_t al)
{
    if (size == 0)
        size = 1;

    void *ptr = nullptr;
    if (posix_memalign(&ptr, ml_min_posix_alignment(static_cast<size_t>(al)), size) != 0 || !ptr)
        throw std::bad_alloc();

    if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_allocated(malloc_usable_size(ptr));
#else
        pulse_ml_memory_allocated(size);
#endif
    }
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN);
    return ptr;
}

void operator delete(void *ptr, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(0);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(0);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

void operator delete(void *ptr, [[maybe_unused]] size_t size, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(size);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, [[maybe_unused]] size_t size, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
        if (ml_alloc_active) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
            pulse_ml_memory_freed(malloc_usable_size(ptr));
#else
            pulse_ml_memory_freed(size);
#endif
        }
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

// std::nothrow_t variants. The standard library would otherwise resolve
// these to the default implementations and bypass ML accounting. Each
// wrapper forwards to the corresponding throwing form and translates the
// std::bad_alloc into a nullptr return.

void *operator new(size_t size, const std::nothrow_t &) noexcept
{
    try {
        return ::operator new(size);
    } catch (...) {
        return nullptr;
    }
}

void *operator new[](size_t size, const std::nothrow_t &) noexcept
{
    try {
        return ::operator new[](size);
    } catch (...) {
        return nullptr;
    }
}

void operator delete(void *ptr, const std::nothrow_t &) noexcept
{
    ::operator delete(ptr);
}

void operator delete[](void *ptr, const std::nothrow_t &) noexcept
{
    ::operator delete[](ptr);
}

void *operator new(size_t size, std::align_val_t al, const std::nothrow_t &) noexcept
{
    try {
        return ::operator new(size, al);
    } catch (...) {
        return nullptr;
    }
}

void *operator new[](size_t size, std::align_val_t al, const std::nothrow_t &) noexcept
{
    try {
        return ::operator new[](size, al);
    } catch (...) {
        return nullptr;
    }
}

void operator delete(void *ptr, std::align_val_t al, const std::nothrow_t &) noexcept
{
    ::operator delete(ptr, al);
}

void operator delete[](void *ptr, std::align_val_t al, const std::nothrow_t &) noexcept
{
    ::operator delete[](ptr, al);
}
