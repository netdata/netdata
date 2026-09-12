#include <cstdlib>
#include <new>

// libnetdata.h is included first so that, when NETDATA_TRACE_ALLOCATIONS
// is defined, this translation unit sees the same view of malloc /
// malloc_usable_size as the rest of the codebase. nd-mallocz.c overrides
// the libc symbols in that mode and prepends a Netdata header to every
// allocation; ml_usable_size() below must route through that shim, not
// through Apple's malloc_size(), to read the correct accounted size.
#include "libnetdata/libnetdata.h"

#if defined(__linux__)
  #include <malloc.h>
  #define ML_HAVE_MALLOC_USABLE_SIZE 1
#elif defined(__APPLE__)
  #include <malloc/malloc.h>
  #define ML_HAVE_MALLOC_USABLE_SIZE 1
#elif defined(__FreeBSD__)
  #include <malloc_np.h>
  #define ML_HAVE_MALLOC_USABLE_SIZE 1
#endif

#include "daemon/pulse/pulse-ml.h"

#ifdef ML_HAVE_MALLOC_USABLE_SIZE
// Return the allocator's view of the block size for ptr. When
// NETDATA_TRACE_ALLOCATIONS is on, we route through nd-mallocz's
// mallocz_usable_size(), which reads the Netdata allocation header so
// size reporting stays consistent with the matching malloc() (also
// overridden in that mode). We call the declared wrapper rather than the
// libc-override symbol malloc_usable_size: the override is defined in
// nd-mallocz.c but never declared in a header, and on macOS
// <malloc/malloc.h> exposes only malloc_size(), so calling
// malloc_usable_size() here would not compile in C++ under trace mode.
// When NETDATA_TRACE_ALLOCATIONS is off, we call the platform-native
// function directly: malloc_usable_size on Linux/FreeBSD, malloc_size on
// macOS (Apple does not expose malloc_usable_size).
static inline size_t ml_usable_size(void *ptr) noexcept
{
#if defined(NETDATA_TRACE_ALLOCATIONS)
    return mallocz_usable_size(ptr);
#elif defined(__APPLE__)
    return malloc_size(ptr);
#else
    return malloc_usable_size(ptr);
#endif
}
#endif

// Global C++ operator new/delete overrides for the netdata binary (only
// compiled when ENABLE_MIMALLOC is OFF). They wrap malloc/free (or
// posix_memalign for over-aligned allocations) and account every C++
// allocation to the pulse_ml_memory_* counters that back the ML memory
// line on netdata.memory.
//
// Scope: these are the *global* operators, so they capture all C++ heap
// activity in the process, not ML alone (notably ACLK protobuf
// serialization). In this binary the C++ heap is dominated by ML, and
// counting every allocation is what keeps the accounting symmetric: every
// counted new has a matching counted delete. A thread-scoped filter could
// not guarantee that -- a block allocated on one thread and freed on
// another could be counted on only one side, making the counter drift and
// the chart flicker -- so we deliberately account unconditionally.
//
// On platforms with malloc_usable_size() (Linux/macOS/FreeBSD), every path
// reports the allocator block size returned by ml_usable_size(ptr), not the
// size argument. This makes alloc/free symmetric regardless of which delete
// form the compiler emits: when -fsized-deallocation is off, every free
// routes through the unsized overload (which has no size argument); when it
// is on, the compiler may emit either form depending on type knowledge. By
// reading the size from the pointer on every path, the per-pointer
// alloc/free pair is balanced independent of which delete form was chosen.
// The reported byte total also reflects actual allocator block sizes
// (including alignment/rounding slack), not requested sizes -- a more
// accurate measure of real RAM consumption.
//
// On platforms without malloc_usable_size() (everything that is not
// Linux/macOS/FreeBSD), the sized paths attribute the requested size to
// keep the counter meaningful instead of flat-zero. The unsized delete
// overloads have no size to fall back to; they decrement zero and rely on
// the saturating pulse_ml_memory_freed() to prevent underflow. Net effect
// on these platforms: a small persistent over-count proportional to the
// share of frees that route through unsized delete. We accept this over a
// size-prefix scheme that would impose a per-allocation header on every
// build.
//
// Over-aligned types (alignof(T) > __STDCPP_DEFAULT_NEW_ALIGNMENT__)
// route through the std::align_val_t-tagged overloads below; without
// these, such allocations would silently bypass accounting.

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

#ifdef ML_HAVE_MALLOC_USABLE_SIZE
    pulse_ml_memory_allocated(ml_usable_size(ptr));
#else
    pulse_ml_memory_allocated(size);
#endif
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

#ifdef ML_HAVE_MALLOC_USABLE_SIZE
    pulse_ml_memory_allocated(ml_usable_size(ptr));
#else
    pulse_ml_memory_allocated(size);
#endif
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    return ptr;
}

void operator delete(void *ptr, [[maybe_unused]] size_t size) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(size);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, [[maybe_unused]] size_t size) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(size);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete(void *ptr) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(0);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(0);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

// Over-aligned overloads (C++17). Triggered when alignof(T) exceeds
// __STDCPP_DEFAULT_NEW_ALIGNMENT__ (typically 16). posix_memalign is
// available on every platform with malloc_usable_size() above and on
// most POSIX systems otherwise; the resulting pointer is freed with
// plain free().
//
// Guarded behind __cpp_aligned_new because some C++17-capable toolchains
// (older libstdc++ shipped with RHEL/CentOS RPM builds, in particular)
// declare the language standard as C++17 but ship a <new> header that
// does not define std::align_val_t. The ML subsystem does not introduce
// any over-aligned types today, so on those toolchains the aligned
// overloads simply fall back to the default global new/delete, which
// matches the pre-existing project behavior on those platforms.

#if defined(__cpp_aligned_new) && __cpp_aligned_new >= 201606L

void *operator new(size_t size, std::align_val_t al)
{
    if (size == 0)
        size = 1;

    void *ptr = nullptr;
    if (posix_memalign(&ptr, ml_min_posix_alignment(static_cast<size_t>(al)), size) != 0 || !ptr)
        throw std::bad_alloc();

#ifdef ML_HAVE_MALLOC_USABLE_SIZE
    pulse_ml_memory_allocated(ml_usable_size(ptr));
#else
    pulse_ml_memory_allocated(size);
#endif
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

#ifdef ML_HAVE_MALLOC_USABLE_SIZE
    pulse_ml_memory_allocated(ml_usable_size(ptr));
#else
    pulse_ml_memory_allocated(size);
#endif
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN);
    return ptr;
}

void operator delete(void *ptr, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(0);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(0);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

void operator delete(void *ptr, [[maybe_unused]] size_t size, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(size);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

void operator delete[](void *ptr, [[maybe_unused]] size_t size, std::align_val_t /*al*/) noexcept
{
    if (ptr) {
#ifdef ML_HAVE_MALLOC_USABLE_SIZE
        pulse_ml_memory_freed(ml_usable_size(ptr));
#else
        pulse_ml_memory_freed(size);
#endif
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
        free(ptr);
    }
}

#endif // __cpp_aligned_new

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

#if defined(__cpp_aligned_new) && __cpp_aligned_new >= 201606L

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

#endif // __cpp_aligned_new
