// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_MALLOCZ_H
#define NETDATA_ND_MALLOCZ_H

#include "../common.h"

// memory allocation functions that handle failures
#ifdef NETDATA_TRACE_ALLOCATIONS
struct malloc_trace {
    avl_t avl;

    const char *function;
    const char *file;
    size_t line;

    size_t malloc_calls;
    size_t calloc_calls;
    size_t realloc_calls;
    size_t strdup_calls;
    size_t free_calls;

    size_t mmap_calls;
    size_t munmap_calls;

    size_t allocations;
    size_t bytes;

    struct rrddim *rd_bytes;
    struct rrddim *rd_allocations;
    struct rrddim *rd_avg_alloc;
    struct rrddim *rd_ops;
};

int malloc_trace_walkthrough(int (*callback)(void *item, void *data), void *data);

#define strdupz(s) strdupz_int(s, __FILE__, __FUNCTION__, __LINE__)
#define strndupz(s, len) strndupz_int(s, len, __FILE__, __FUNCTION__, __LINE__)
#define callocz(nmemb, size) callocz_int(nmemb, size, __FILE__, __FUNCTION__, __LINE__)
#define mallocz(size) mallocz_int(size, __FILE__, __FUNCTION__, __LINE__)
#define reallocz(ptr, size) reallocz_int(ptr, size, __FILE__, __FUNCTION__, __LINE__)
#define freez(ptr) freez_int(ptr, __FILE__, __FUNCTION__, __LINE__)
#define mallocz_usable_size(ptr) mallocz_usable_size_int(ptr, __FILE__, __FUNCTION__, __LINE__)

char *strdupz_int(const char *s, const char *file, const char *function, size_t line);
char *strndupz_int(const char *s, size_t len, const char *file, const char *function, size_t line);
void *callocz_int(size_t nmemb, size_t size, const char *file, const char *function, size_t line);
void *mallocz_int(size_t size, const char *file, const char *function, size_t line);
void *reallocz_int(void *ptr, size_t size, const char *file, const char *function, size_t line);
void freez_int(void *ptr, const char *file, const char *function, size_t line);
size_t mallocz_usable_size_int(void *ptr, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS
char *strdupz(const char *s) MALLOCLIKE NEVERNULL WARNUNUSED;
char *strndupz(const char *s, size_t len) MALLOCLIKE NEVERNULL WARNUNUSED;
void *callocz(size_t nmemb, size_t size) MALLOCLIKE NEVERNULL WARNUNUSED;
void *mallocz(size_t size) MALLOCLIKE NEVERNULL WARNUNUSED;
void *reallocz(void *ptr, size_t size) MALLOCLIKE NEVERNULL WARNUNUSED;
void freez(void *ptr);
#endif // NETDATA_TRACE_ALLOCATIONS

void mallocz_release_as_much_memory_to_the_system(void);

int posix_memalignz(void **memptr, size_t alignment, size_t size);
void posix_memalign_freez(void *ptr);

typedef void (*out_of_memory_cb)(void);
void mallocz_register_out_of_memory_cb(out_of_memory_cb cb);

NORETURN
void out_of_memory(const char *call, size_t size, const char *details);

#endif //NETDATA_ND_MALLOCZ_H
