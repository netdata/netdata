// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

out_of_memory_cb out_of_memory_callback = NULL;
void mallocz_register_out_of_memory_cb(out_of_memory_cb cb) {
    out_of_memory_callback = cb;
}

static __thread bool out_of_memory_running = false;

ALWAYS_INLINE NORETURN
void out_of_memory(const char *call, size_t size, const char *details) {
    int errno_saved = errno;

    if(unlikely(out_of_memory_running)) {
        errno = errno_saved;
        fatal("Out of memory on %s(%zu bytes) while already handling out-of-memory.\n"
              "Additional details: %s",
              call, size,
              details ? details : "none");
    }

    out_of_memory_running = true;

    exit_initiated_add(EXIT_REASON_OUT_OF_MEMORY);

    if(out_of_memory_callback)
        out_of_memory_callback();

    OS_PROCESS_MEMORY proc_mem = os_process_memory(0);
    uint64_t max_rss = OS_PROCESS_MEMORY_OK(proc_mem) ? proc_mem.max_rss : 0;

    char mem_available[64];
    char rss_used[64];

    OS_SYSTEM_MEMORY sm = os_last_reported_system_memory();
    size_snprintf(mem_available, sizeof(mem_available), sm.ram_available_bytes, "B", false);
    size_snprintf(rss_used, sizeof(rss_used), max_rss, "B", false);

    errno = errno_saved;
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

#ifndef MALLOC_ALIGNMENT
#define MALLOC_ALIGNMENT (sizeof(uintptr_t) * 2)
#endif

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

#define DLSYM_BOOTSTRAP_BUFFER_SIZE (1024 * 1024)
#define DLSYM_BOOTSTRAP_MAGIC 0xD157B007U

struct dlsym_bootstrap_header {
    size_t size;
    uint32_t magic;
};

#define DLSYM_BOOTSTRAP_HEADER_SIZE \
    (((sizeof(struct dlsym_bootstrap_header) + MALLOC_ALIGNMENT - 1) / MALLOC_ALIGNMENT) * MALLOC_ALIGNMENT)

static uint8_t dlsym_bootstrap_buffer[DLSYM_BOOTSTRAP_BUFFER_SIZE]
    __attribute__((aligned(sizeof(uintptr_t) * 2)));
static size_t dlsym_bootstrap_used = 0;
static __thread size_t dlsym_bootstrap_depth = 0;

static size_t dlsym_bootstrap_align_size(size_t size) {
    size_t rem = size % MALLOC_ALIGNMENT;
    return rem ? size + MALLOC_ALIGNMENT - rem : size;
}

static bool dlsym_bootstrap_active(void) {
    return dlsym_bootstrap_depth != 0;
}

static void dlsym_bootstrap_abort(void) {
    static const char msg[] = "FATAL: dlsym() bootstrap allocator exhausted.\n";
    if(write(STDERR_FILENO, msg, sizeof(msg) - 1) < 0) {
        // best effort
    }
    abort();
}

static bool dlsym_bootstrap_contains(const void *ptr) {
    uintptr_t addr = (uintptr_t)ptr;
    uintptr_t base = (uintptr_t)dlsym_bootstrap_buffer;
    return addr >= base + DLSYM_BOOTSTRAP_HEADER_SIZE && addr < base + sizeof(dlsym_bootstrap_buffer);
}

static struct dlsym_bootstrap_header *dlsym_bootstrap_header(void *ptr) {
    if(!dlsym_bootstrap_contains(ptr))
        return NULL;

    struct dlsym_bootstrap_header *hdr =
        (struct dlsym_bootstrap_header *)((uint8_t *)ptr - DLSYM_BOOTSTRAP_HEADER_SIZE);

    return hdr->magic == DLSYM_BOOTSTRAP_MAGIC ? hdr : NULL;
}

static void *dlsym_bootstrap_malloc(size_t size) {
    if(!size)
        size = 1;

    if(size > SIZE_MAX - DLSYM_BOOTSTRAP_HEADER_SIZE)
        dlsym_bootstrap_abort();

    size_t requested = DLSYM_BOOTSTRAP_HEADER_SIZE + size;
    if(requested > SIZE_MAX - MALLOC_ALIGNMENT)
        dlsym_bootstrap_abort();

    size_t total = dlsym_bootstrap_align_size(requested);

    while(true) {
        size_t used = __atomic_load_n(&dlsym_bootstrap_used, __ATOMIC_RELAXED);
        size_t aligned_used = dlsym_bootstrap_align_size(used);

        if(aligned_used > DLSYM_BOOTSTRAP_BUFFER_SIZE || total > DLSYM_BOOTSTRAP_BUFFER_SIZE - aligned_used)
            dlsym_bootstrap_abort();

        size_t next = aligned_used + total;
        if(__atomic_compare_exchange_n(&dlsym_bootstrap_used, &used, next, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
            struct dlsym_bootstrap_header *hdr =
                (struct dlsym_bootstrap_header *)&dlsym_bootstrap_buffer[aligned_used];
            hdr->size = size;
            hdr->magic = DLSYM_BOOTSTRAP_MAGIC;
            return &dlsym_bootstrap_buffer[aligned_used + DLSYM_BOOTSTRAP_HEADER_SIZE];
        }
    }
}

static void *dlsym_bootstrap_calloc(size_t n, size_t size) {
    if(size && n > SIZE_MAX / size)
        dlsym_bootstrap_abort();

    size_t bytes = n * size;
    void *ptr = dlsym_bootstrap_malloc(bytes);
    memset(ptr, 0, bytes);
    return ptr;
}

static void *dlsym_bootstrap_realloc(void *ptr, size_t size) {
    if(!ptr)
        return dlsym_bootstrap_malloc(size);

    struct dlsym_bootstrap_header *hdr = dlsym_bootstrap_header(ptr);
    if(!hdr)
        dlsym_bootstrap_abort();

    void *new_ptr = dlsym_bootstrap_malloc(size);
    memcpy(new_ptr, ptr, MIN(hdr->size, size));
    return new_ptr;
}

static char *dlsym_bootstrap_strdup(const char *s) {
    size_t len = strnlen(s, SIZE_MAX - 1);
    if(unlikely(len == SIZE_MAX - 1))
        dlsym_bootstrap_abort();

    size_t size = len + 1;
    char *ptr = dlsym_bootstrap_malloc(size);
    memcpy(ptr, s, size);
    return ptr;
}

static char *dlsym_bootstrap_strndup(const char *s, size_t len) {
    size_t bytes = strnlen(s, len);
    if(unlikely(bytes == SIZE_MAX))
        dlsym_bootstrap_abort();

    char *ptr = dlsym_bootstrap_malloc(bytes + 1);
    memcpy(ptr, s, bytes);
    ptr[bytes] = '\0';
    return ptr;
}

static bool dlsym_bootstrap_free(void *ptr) {
    return dlsym_bootstrap_header(ptr) != NULL;
}

static size_t dlsym_bootstrap_usable_size(void *ptr) {
    struct dlsym_bootstrap_header *hdr = dlsym_bootstrap_header(ptr);
    return hdr ? hdr->size : 0;
}

static void link_system_library_function(libc_function_t *func_pptr, const char *name, bool required) {
    dlsym_bootstrap_depth++;
    libc_function_t func = dlsym(RTLD_NEXT, name);
    dlsym_bootstrap_depth--;

    if(!func && required) {
        // Report without allocating, then abort. fprintf() may call malloc(),
        // which at this point (bootstrap inactive, libc allocator still
        // unresolved and *func_pptr about to be NULL) would recurse into the
        // first-run resolver or dereference a NULL libc function pointer.
        // Mirror dlsym_bootstrap_abort() and use write(2). name is always a
        // string literal, so strlen() is safe and allocation-free here.
        static const char pre[] = "FATAL: Cannot find system's ";
        static const char post[] = "() function.\n";
        if(write(STDERR_FILENO, pre, sizeof(pre) - 1) < 0) { /* best effort */ }
        if(write(STDERR_FILENO, name, strlen(name)) < 0) { /* best effort */ }
        if(write(STDERR_FILENO, post, sizeof(post) - 1) < 0) { /* best effort */ }
        abort();
    }

    *func_pptr = func;
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
    if(unlikely(dlsym_bootstrap_active()))
        return dlsym_bootstrap_malloc(size);

    return mallocz(size);
}

void *calloc(size_t n, size_t size) {
    if(unlikely(dlsym_bootstrap_active()))
        return dlsym_bootstrap_calloc(n, size);

    return callocz(n, size);
}

void *realloc(void *ptr, size_t size) {
    if(unlikely(dlsym_bootstrap_contains(ptr)))
        return dlsym_bootstrap_realloc(ptr, size);

    if(unlikely(dlsym_bootstrap_active())) {
        if(!ptr)
            return dlsym_bootstrap_realloc(ptr, size);

        if(libc_realloc != realloc_first_run)
            return libc_realloc(ptr, size);

        dlsym_bootstrap_abort();
    }

    return reallocz(ptr, size);
}

void *reallocarray(void *ptr, size_t n, size_t size) {
    if(unlikely(size && n > SIZE_MAX / size)) {
        if(unlikely(dlsym_bootstrap_active()))
            dlsym_bootstrap_abort();

        fatal("reallocarray() cannot allocate %zu members of %zu bytes.", n, size);
    }

    size_t bytes = n * size;

    if(unlikely(dlsym_bootstrap_contains(ptr)))
        return dlsym_bootstrap_realloc(ptr, bytes);

    if(unlikely(dlsym_bootstrap_active())) {
        if(!ptr)
            return dlsym_bootstrap_realloc(ptr, bytes);

        if(libc_realloc != realloc_first_run)
            return libc_realloc(ptr, bytes);

        dlsym_bootstrap_abort();
    }

    return reallocz(ptr, bytes);
}

void free(void *ptr) {
    if(unlikely(dlsym_bootstrap_free(ptr)))
        return;

    if(unlikely(dlsym_bootstrap_active())) {
        if(libc_free != free_first_run)
            libc_free(ptr);

        return;
    }

    freez(ptr);
}

char *strdup(const char *s) {
    if(unlikely(dlsym_bootstrap_active()))
        return dlsym_bootstrap_strdup(s);

    return strdupz(s);
}

char *strndup(const char *s, size_t len) {
    if(unlikely(dlsym_bootstrap_active()))
        return dlsym_bootstrap_strndup(s, len);

    return strndupz(s, len);
}

size_t malloc_usable_size(void *ptr) {
    if(unlikely(dlsym_bootstrap_contains(ptr)))
        return dlsym_bootstrap_usable_size(ptr);

    if(unlikely(dlsym_bootstrap_active()))
        return 0;

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

#define MALLOC_HEADER_MAGIC 0x0BADCAFEU

static const uint8_t malloc_header_magic_salt;

static uint32_t malloc_header_magic(const struct malloc_header *t) {
    uintptr_t value = (uintptr_t)t ^ (uintptr_t)&malloc_header_magic_salt;

#if UINTPTR_MAX > UINT32_MAX
    value ^= value >> 32;
#endif

    return (uint32_t)value ^ MALLOC_HEADER_MAGIC;
}

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
    if(unlikely(size > SIZE_MAX - malloc_header_size))
        fatal("mallocz() cannot allocate %zu bytes of memory.", size);

    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);

    size_t_atomic_count(add, p->malloc_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_malloc(malloc_header_size + size);
    if (unlikely(!t)) fatal("mallocz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.trace = p;
    t->signature.size = size;
    t->signature.magic = malloc_header_magic(t);

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    return (void *)&t->data;
}

void *callocz_int(size_t nmemb, size_t size, const char *file, const char *function, size_t line) {
    if(unlikely(size && nmemb > (SIZE_MAX - malloc_header_size) / size))
        fatal("callocz() cannot allocate %zu members of %zu bytes.", nmemb, size);

    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size = nmemb * size;

    size_t_atomic_count(add, p->calloc_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_calloc(1, malloc_header_size + size);
    if (unlikely(!t)) fatal("mallocz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.trace = p;
    t->signature.size = size;
    t->signature.magic = malloc_header_magic(t);

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    return &t->data;
}

char *strdupz_int(const char *s, const char *file, const char *function, size_t line) {
    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size_t size = strlen(s) + 1;
    if(unlikely(size > SIZE_MAX - malloc_header_size))
        fatal("strdupz() cannot allocate %zu bytes of memory.", size);

    size_t_atomic_count(add, p->strdup_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_malloc(malloc_header_size + size);
    if (unlikely(!t)) fatal("strdupz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.trace = p;
    t->signature.size = size;
    t->signature.magic = malloc_header_magic(t);

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    memcpy(&t->data, s, size);
    return (char *)&t->data;
}

char *strndupz_int(const char *s, size_t len, const char *file, const char *function, size_t line) {
    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size_t bytes = strnlen(s, len);
    if (unlikely(bytes >= SIZE_MAX - malloc_header_size))
        fatal("strndupz() cannot allocate %zu bytes of memory.", bytes);

    size_t size = bytes + 1;

    size_t_atomic_count(add, p->strdup_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    struct malloc_header *t = (struct malloc_header *)libc_malloc(malloc_header_size + size);
    if (unlikely(!t)) fatal("strndupz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.trace = p;
    t->signature.size = size;
    t->signature.magic = malloc_header_magic(t);

#ifdef NETDATA_INTERNAL_CHECKS
    for(ssize_t i = 0; i < (ssize_t)sizeof(t->padding) ;i++) // signed to avoid compiler warning when zero-padded
        t->padding[i] = 0xFF;
#endif

    memcpy(&t->data, s, bytes);
    t->data[bytes] = '\0';
    return (char *)&t->data;
}

static struct malloc_header *malloc_get_header(void *ptr, const char *caller, const char *file, const char *function, size_t line) {
    if(!ptr)
        return NULL;

    uint8_t *ret = (uint8_t *)ptr - malloc_header_size;
    struct malloc_header *t = (struct malloc_header *)ret;

    if(t->signature.magic != malloc_header_magic(t)) {
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
    if(unlikely(size > SIZE_MAX - malloc_header_size))
        fatal("reallocz() cannot allocate %zu bytes of memory.", size);

    size_t_atomic_count(add, t->signature.trace->free_calls, 1);
    size_t_atomic_count(sub, t->signature.trace->allocations, 1);
    size_t_atomic_bytes(sub, t->signature.trace->bytes, t->signature.size);

    struct malloc_trace *p = malloc_trace_find_or_create(file, function, line);
    size_t_atomic_count(add, p->realloc_calls, 1);
    size_t_atomic_count(add, p->allocations, 1);
    size_t_atomic_bytes(add, p->bytes, size);

    t = (struct malloc_header *)libc_realloc(t, malloc_header_size + size);
    if (unlikely(!t)) fatal("reallocz() cannot allocate %zu bytes of memory (%zu with header).", size, malloc_header_size + size);
    t->signature.trace = p;
    t->signature.size = size;
    t->signature.magic = malloc_header_magic(t);

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
