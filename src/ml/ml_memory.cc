#include <cstdlib>
#include <memory>

#include "daemon/telemetry/telemetry-ml.h"

void *operator new(size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    telemetry_ml_memory_allocated(size);
    return ptr;
}

void *operator new[](size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    telemetry_ml_memory_allocated(size);
    return ptr;
}

void operator delete(void *ptr, size_t size) noexcept
{
    if (ptr) {
        telemetry_ml_memory_freed(size);
        free(ptr);
    }
}

void operator delete[](void *ptr, size_t size) noexcept
{
    if (ptr) {
        telemetry_ml_memory_freed(size);
        free(ptr);
    }
}

void operator delete(void *ptr) noexcept
{
    if (ptr) {
        free(ptr);
    }
}

void operator delete[](void *ptr) noexcept
{
    if (ptr) {
        free(ptr);
    }
}
