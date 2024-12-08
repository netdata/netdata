#include <cstdlib>
#include <memory>

#include "daemon/pulse/pulse-ml.h"

void *operator new(size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    pulse_ml_memory_allocated(size);
    return ptr;
}

void *operator new[](size_t size)
{
    void *ptr = malloc(size);
    if (!ptr)
        throw std::bad_alloc();

    pulse_ml_memory_allocated(size);
    return ptr;
}

void operator delete(void *ptr, size_t size) noexcept
{
    if (ptr) {
        pulse_ml_memory_freed(size);
        free(ptr);
    }
}

void operator delete[](void *ptr, size_t size) noexcept
{
    if (ptr) {
        pulse_ml_memory_freed(size);
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
