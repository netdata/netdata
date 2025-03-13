// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

out_of_memory_cb out_of_memory_callback = NULL;
void mallocz_register_out_of_memory_cb(out_of_memory_cb cb) {
    out_of_memory_callback = cb;
}

ALWAYS_INLINE NORETURN
void out_of_memory(const char *call, size_t size, const char *details) {
    exit_initiated_add(EXIT_REASON_OUT_OF_MEMORY);

    if(out_of_memory_callback)
        out_of_memory_callback();

#if defined(OS_LINUX) || defined(OS_WINDOWS)
    int rss_multiplier = 1024;
#else
    int rss_multiplier = 1;
#endif

    struct rusage usage = { 0 };
    if(getrusage(RUSAGE_SELF, &usage) != 0)
        usage.ru_maxrss = 0;

    char mem_available[64];
    char rss_used[64];

    OS_SYSTEM_MEMORY sm = os_last_reported_system_memory();
    size_snprintf(mem_available, sizeof(mem_available), sm.ram_available_bytes, "B", false);
    size_snprintf(rss_used, sizeof(rss_used), usage.ru_maxrss * rss_multiplier, "B", false);

    fatal("Out of memory on %s(%zu bytes)!\n"
          "System memory available: %s, while our max RSS usage is: %s\n"
          "O/S mmap limit: %llu, while our mmap count is: %zu\n"
          "Additional details: %s",
          call, size,
          mem_available, rss_used,
          os_mmap_limit(), __atomic_load_n(&nd_mmap_count, __ATOMIC_RELAXED),
          details ? details : "none");
}

// ----------------------------------------------------------------------------
// memory allocation functions that handle failures

// although netdata does not use memory allocations too often (netdata tries to
// maintain its memory footprint stable during runtime, i.e. all buffers are
// allocated during initialization and are adapted to current use throughout
// its lifetime), these can be used to override the default system allocation
// routines.

#ifdef NETDATA_TRACE_ALLOCATIONS
#warning NETDATA_TRACE_ALLOCATIONS ENABLED
#include "Judy.h"

#if defined(HAVE_DLSYM) && defined(ENABLE_DLSYM)
#include <dlfcn.h>

typedef void (*libc_function_t)(void);

static void *malloc_first_run(size_t size);
static void *(*libc_malloc)(size_t) = malloc_first_run;

static void *calloc_first_run(size_t n, size_t size);
static void *(*libc_calloc)(size_t, size_t) = calloc_first_run;

static void *realloc_first_run(void *ptr, size_t size);
static void *(*libc_realloc)(void *, size_t) = realloc_first_run;

static void free_first_run(void *ptr);
static void (*libc_free)(void *) = free_first_run;

static char *strdup_first_run(const char *s);
static char *(*libc_strdup)(const char *) = strdup_first_run;

static char *strndup_first_run(const char *s, size_t len);
static char *(*libc_strndup)(const char *, size_t) = strndup_first_run;

static size_t malloc_usable_size_first_run(void *ptr);
#ifdef HAVE_MALLOC_USABLE_SIZE
static size_t (*libc_malloc_usable_size)(void *) = malloc_usable_size_first_run;
#else
static size_t (*libc_malloc_usable_size)(void *) = NULL;
#endif

static void link_system_library_function(libc_function_t *func_pptr, const char *name, bool required) {
    *func_pptr = dlsym(RTLD_NEXT, name);
    if(!*func_pptr && required) {
        fprintf(stderr, "FATAL: Cannot find system's %s() function.\n", name);
        abort();
    }
}

static void *malloc_first_run(size_t size) {
    link_system_library_function((libc_function_t *) &libc_malloc, "malloc", true);
    return libc_malloc(size);
}

static void *calloc_first_run(size_t n, size_t size) {
    link_system_library_function((libc_function_t *) &libc_calloc, "calloc", true);
    return libc_calloc(n, size);
}

static void *realloc_first_run(void *ptr, size_t size) {
    link_system_library_function((libc_function_t *) &libc_realloc, "realloc", true);
    return libc_realloc(ptr, size);
}

static void free_first_run(void *ptr) {
    link_system_library_function((libc_function_t *) &libc_free, "free", true);
    libc_free(ptr);
}

static char *strdup_first_run(const char *s) {
    link_system_library_function((libc_function_t *) &libc_strdup, "strdup", true);
    return libc_strdup(s);
}

static char *strndup_first_run(const char *s, size_t len) {
    link_system_library_function((libc_function_t *) &libc_strndup, "strndup", true);
    return libc_strndup(s, len);
}

static size_t malloc_usable_size_first_run(void *ptr) {
    link_system_library_function((libc_function_t *) &libc_malloc_usable_size, "malloc_usable_size", false);

    if(libc_malloc_usable_size)
        return libc_malloc_usable_size(ptr);
    else
        return 0;
}

void *malloc(size_t size) {
    return mallocz(size);
}

void *calloc(size_t n, size_t size) {
    return callocz(n, size);
}

void *realloc(void *ptr, size_t size) {
    return reallocz(ptr, size);
}

void *reallocarray(void *ptr, size_t n, size_t size) {
    return reallocz(ptr, n * size);
}

void free(void *ptr) {
    freez(ptr);
}

char *strdup(const char *s) {
    return strdupz(s);
}

char *strndup(const char *s, size_t len) {
    return strndupz(s, len);
}

size_t malloc_usable_size(void *ptr) {
    return mallocz_usable_size(ptr);
}
#else // !HAVE_DLSYM

static void *(*libc_malloc)(size_t) = malloc;
static void *(*libc_calloc)(size_t, size_t) = calloc;
static void *(*libc_realloc)(void *, size_t) = realloc;
static void (*libc_free)(void *) = free;

#ifdef HAVE_MALLOC_USABLE_SIZE
static size_t (*libc_malloc_usable_size)(void *) = malloc_usable_size;
#else
static size_t (*libc_malloc_usable_size)(void *) = NULL;
#endif

#endif // HAVE_DLSYM


void posix_memfree(void *ptr) {
    libc_free(ptr);
}

struct malloc_header_signature {
    uint32_t magic;
    uint32_t size;
    struct malloc_trace *trace;
};

struct malloc_header {
    struct malloc_header_signature signature;
    uint8_t padding[(sizeof(struct malloc_header_signature) % MALLOC_ALIGNMENT) ? MALLOC_ALIGNMENT - (sizeof(struct malloc_header_signature) % MALLOC_ALIGNMENT) : 0];
    uint8_t data[];
};

static size_t malloc_header_size = sizeof(struct malloc_header);

int malloc_trace_compare(void *A, void *B) {
    struct malloc_trace *a = A;
    struct malloc_trace *b = B;
    return strcmp(a->function, b->function);
}

static avl_tree_lock malloc_trace_index = {
    .avl_tree = {
        .root = NULL,
        .compar = malloc_trace_compare},
    .rwlock = AVL_LOCK_INITIALIZER
};

int malloc_trace_walkthrough(int (*callback)(void *item, void *data), void *data) {
    return avl_traverse_lock(&malloc_trace_index, callback, data);
}

NEVERNULL WARNUNUSED
    static struct malloc_trace *malloc_trace_find_or_create(const char *file, const char *function, size_t line) {
    struct malloc_trace tmp = {
        .line = line,
        .function = function,
        .file = file,
    };

    struct malloc_trace *t = (struct malloc_trace *)avl_search_lock(&malloc_trace_index, (avl_t *)&tmp);
    if(!t) {
        t = libc_calloc(1, sizeof(struct malloc_trace));
        if(!t) fatal("No memory");
        t->line = line;
        t->function = function;
        t->file = file;

        struct malloc_trace *t2 = (struct malloc_trace *)avl_insert_lock(&malloc_trace_index, (avl_t *)t);
        if(t2 != t)
            free(t);

        t = t2;
    }

    if(!t)
        fatal("Cannot insert to AVL");

    return t;
}

void malloc_trace_mmap(size_t size) {
    struct malloc_trace *p = malloc_trace_find_or_create("unknown", "netdata_mmap", 1);
    size_t_atomic_count(add, p->mmap_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);
}

void malloc_trace_munmap(size_t size) {
    struct malloc_trace *p = malloc_trace_find_or_create("unknown", "netdata_mmap", 1);
    size_t_atomic_count(add, p->munmap_calls, 1);
    size_t_atomic_count(sub, p->allocations, 1);
    size_t_atomic_bytes(sub, p->bytes, size);
}

void *mallocz_int(size_t size, const char *file, const char *function, size_t line) {
    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);

    size_t_atomic_count(add, p->malloc_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_malloc(malloc_header_size + size);
    if (unlikely(!t)) fatal("mallocz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.magic = 0x0BADCAFE;
    t->signature.trace = p;
    t->signature.size = size;

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    return (void *)&t->data;
}

void *callocz_int(size_t nmemb, size_t size, const char *file, const char *function, size_t line) {
    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size = nmemb * size;

    size_t_atomic_count(add, p->calloc_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_calloc(1, malloc_header_size + size);
    if (unlikely(!t)) fatal("mallocz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.magic = 0x0BADCAFE;
    t->signature.trace = p;
    t->signature.size = size;

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    return &t->data;
}

char *strdupz_int(const char *s, const char *file, const char *function, size_t line) {
    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size_t size = strlen(s) + 1;

    size_t_atomic_count(add, p->strdup_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_malloc(malloc_header_size + size);
    if (unlikely(!t)) fatal("strdupz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.magic = 0x0BADCAFE;
    t->signature.trace = p;
    t->signature.size = size;

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    memcpy(&t->data, s, size);
    return (char *)&t->data;
}

char *strndupz_int(const char *s, size_t len, const char *file, const char *function, size_t line) {
    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size_t size = len + 1;

    size_t_atomic_count(add, p->strdup_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_malloc(malloc_header_size + size);
    if (unlikely(!t)) fatal("strndupz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.magic = 0x0BADCAFE;
    t->signature.trace = p;
    t->signature.size = size;

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    memcpy(&t->data, s, size);
    t->data[len] = '\0';
    return (char *)&t->data;
}

static struct malloc_header *malloc_get_header(void *ptr, const char *caller, const char *file, const char *function, size_t line) {
    uint8_t *ret = (uint8_t *)ptr - malloc_header_size;
    struct malloc_header *t = (struct malloc_header *)ret;

    if(t->signature.magic != 0x0BADCAFE) {
        netdata_log_error("pointer %p is not our pointer (called %s() from %zu@%s, %s()).", ptr, caller, line, file, function);
        return NULL;
    }

    return t;
}

void *reallocz_int(void *ptr, size_t size, const char *file, const char *function, size_t line) {
    if(!ptr) return mallocz_int(size, file, function, line);

    struct malloc_header *t = malloc_get_header(ptr, __FUNCTION__, file, function, line);
    if(!t)
        return libc_realloc(ptr, size);

    if(t->signature.size == size) return ptr;
    size_t_atomic_count(add, t->signature.trace->free_calls, 1);
    size_t_atomic_count(sub, t->signature.trace->allocations, 1);
    size_t_atomic_bytes(sub, t->signature.trace->bytes, t->signature.size);

    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size_t_atomic_count(add, p->realloc_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    t = (struct malloc_header *)libc_realloc(t, malloc_header_size + size);
    if (unlikely(!t)) fatal("reallocz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.magic = 0x0BADCAFE;
    t->signature.trace = p;
    t->signature.size = size;

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    return (void *)&t->data;
}

size_t mallocz_usable_size_int(void *ptr, const char *file, const char *function, size_t line) {
    if(unlikely(!ptr)) return 0;

    struct malloc_header *t = malloc_get_header(ptr, __FUNCTION__, file, function, line);
    if(!t) {
        if(libc_malloc_usable_size)
            return libc_malloc_usable_size(ptr);
        else
            return 0;
    }

    return t->signature.size;
}

void freez_int(void *ptr, const char *file, const char *function, size_t line) {
    if(unlikely(!ptr)) return;

    struct malloc_header *t = malloc_get_header(ptr, __FUNCTION__, file, function, line);
    if(!t) {
        libc_free(ptr);
        return;
    }

    size_t_atomic_count(add, t->signature.trace->free_calls, 1);
    size_t_atomic_count(sub, t->signature.trace->allocations, 1);
    size_t_atomic_bytes(sub, t->signature.trace->bytes, t->signature.size);

#ifdef NETDATA_INTERNAL_CHECKS
    // it should crash if it is used after freeing it
    memset(t, 0, malloc_header_size + t->signature.size);
#endif

    libc_free(t);
}
#else

ALWAYS_INLINE MALLOCLIKE NEVERNULL WARNUNUSED
char *strdupz(const char *s) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_STRDUP);

    char *t = strdup(s);
    if (unlikely(!t))
        out_of_memory(__FUNCTION__ , strlen(s) + 1, NULL);

    return t;
}

ALWAYS_INLINE MALLOCLIKE NEVERNULL WARNUNUSED
char *strndupz(const char *s, size_t len) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_STRNDUP);

    char *t = strndup(s, len);
    if (unlikely(!t))
        out_of_memory(__FUNCTION__ , len + 1, NULL);

    return t;
}

// If ptr is NULL, no operation is performed.
ALWAYS_INLINE
void freez(void *ptr) {
    if(likely(ptr)) {
        workers_memory_call(WORKERS_MEMORY_CALL_LIBC_FREE);
        free(ptr);
    }
}

ALWAYS_INLINE MALLOCLIKE NEVERNULL WARNUNUSED
void *mallocz(size_t size) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_MALLOC);
    void *p = malloc(size);
    if (unlikely(!p))
        out_of_memory(__FUNCTION__, size, NULL);

    return p;
}

ALWAYS_INLINE MALLOCLIKE NEVERNULL WARNUNUSED
void *callocz(size_t nmemb, size_t size) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_CALLOC);
    void *p = calloc(nmemb, size);
    if (unlikely(!p))
        out_of_memory(__FUNCTION__, nmemb * size, NULL);

    return p;
}

ALWAYS_INLINE MALLOCLIKE NEVERNULL WARNUNUSED
void *reallocz(void *ptr, size_t size) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_REALLOC);
    void *p = realloc(ptr, size);
    if (unlikely(!p))
        out_of_memory(__FUNCTION__, size, NULL);

    return p;
}

ALWAYS_INLINE
int posix_memalignz(void **memptr, size_t alignment, size_t size) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN);
    int rc = posix_memalign(memptr, alignment, size);
    if(unlikely(rc))
        out_of_memory(__FUNCTION__, size, NULL);

    return rc;
}

ALWAYS_INLINE
void posix_memalign_freez(void *ptr) {
    workers_memory_call(WORKERS_MEMORY_CALL_LIBC_POSIX_MEMALIGN_FREE);
    free(ptr);
}
#endif

void mallocz_release_as_much_memory_to_the_system(void) {
#if defined(HAVE_C_MALLOC_TRIM)
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    if(spinlock_trylock(&spinlock)) {
        malloc_trim(0);
        spinlock_unlock(&spinlock);
    }
#endif
}
