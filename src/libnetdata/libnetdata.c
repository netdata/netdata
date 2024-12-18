// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata.h"

#define MALLOC_ALIGNMENT (sizeof(uintptr_t) * 2)
#define size_t_atomic_count(op, var, size) __atomic_## op ##_fetch(&(var), size, __ATOMIC_RELAXED)
#define size_t_atomic_bytes(op, var, size) __atomic_## op ##_fetch(&(var), ((size) % MALLOC_ALIGNMENT)?((size) + MALLOC_ALIGNMENT - ((size) % MALLOC_ALIGNMENT)):(size), __ATOMIC_RELAXED)

#if !defined(MADV_DONTFORK)
#define MADV_DONTFORK 0
#endif

#if !defined(O_NOATIME)
#define O_NOATIME 0
#endif

struct rlimit rlimit_nofile = { .rlim_cur = 1024, .rlim_max = 1024 };

#if defined(MADV_MERGEABLE)
int enable_ksm = CONFIG_BOOLEAN_AUTO;
#else
int enable_ksm = 0;
#endif

volatile sig_atomic_t netdata_exit = 0;

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

char *strdupz(const char *s) {
    char *t = strdup(s);
    if (unlikely(!t)) fatal("Cannot strdup() string '%s'", s);
    return t;
}

char *strndupz(const char *s, size_t len) {
    char *t = strndup(s, len);
    if (unlikely(!t)) fatal("Cannot strndup() string '%s' of len %zu", s, len);
    return t;
}

// If ptr is NULL, no operation is performed.
void freez(void *ptr) {
    if(likely(ptr)) free(ptr);
}

void *mallocz(size_t size) {
    void *p = malloc(size);
    if (unlikely(!p)) fatal("Cannot allocate %zu bytes of memory.", size);
    return p;
}

void *callocz(size_t nmemb, size_t size) {
    void *p = calloc(nmemb, size);
    if (unlikely(!p)) fatal("Cannot allocate %zu bytes of memory.", nmemb * size);
    return p;
}

void *reallocz(void *ptr, size_t size) {
    void *p = realloc(ptr, size);
    if (unlikely(!p)) fatal("Cannot re-allocate memory to %zu bytes.", size);
    return p;
}

void posix_memfree(void *ptr) {
    free(ptr);
}
#endif

void mallocz_release_as_much_memory_to_the_system(void) {
#if defined(HAVE_C_MALLOPT) || defined(HAVE_C_MALLOC_TRIM)
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

#ifdef HAVE_C_MALLOPT
    // the default is 128KiB
    size_t trim_threshold = 65ULL * 1024;
    mallopt(M_TRIM_THRESHOLD, (int)trim_threshold);
#endif

#ifdef HAVE_C_MALLOC_TRIM
    malloc_trim(0);
#endif

    spinlock_unlock(&spinlock);
#endif
}

// --------------------------------------------------------------------------------------------------------------------

void json_escape_string(char *dst, const char *src, size_t size) {
    const char *t;
    char *d = dst, *e = &dst[size - 1];

    for(t = src; *t && d < e ;t++) {
        if(unlikely(*t == '\\' || *t == '"')) {
            if(unlikely(d + 1 >= e)) break;
            *d++ = '\\';
        }
        *d++ = *t;
    }

    *d = '\0';
}

void json_fix_string(char *s) {
    unsigned char c;
    while((c = (unsigned char)*s)) {
        if(unlikely(c == '\\'))
            *s++ = '/';
        else if(unlikely(c == '"'))
            *s++ = '\'';
        else if(unlikely(isspace(c) || iscntrl(c)))
            *s++ = ' ';
        else if(unlikely(!isprint(c) || c > 127))
            *s++ = '_';
        else
            s++;
    }
}

static int memory_file_open(const char *filename, size_t size) {
    // netdata_log_info("memory_file_open('%s', %zu", filename, size);

    int fd = open(filename, O_RDWR | O_CREAT | O_NOATIME | O_CLOEXEC, 0664);
    if (fd != -1) {
        if (lseek(fd, size, SEEK_SET) == (off_t) size) {
            if (write(fd, "", 1) == 1) {
                if (ftruncate(fd, size))
                    netdata_log_error("Cannot truncate file '%s' to size %zu. Will use the larger file.", filename, size);
            }
            else
                netdata_log_error("Cannot write to file '%s' at position %zu.", filename, size);
        }
        else
            netdata_log_error("Cannot seek file '%s' to size %zu.", filename, size);
    }
    else
        netdata_log_error("Cannot create/open file '%s'.", filename);

    return fd;
}

inline int madvise_sequential(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_SEQUENTIAL);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_SEQUENTIAL) of size %zu, failed.", len);
    return ret;
}

inline int madvise_random(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_RANDOM);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_RANDOM) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontfork(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTFORK);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_DONTFORK) of size %zu, failed.", len);
    return ret;
}

inline int madvise_willneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_WILLNEED);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_WILLNEED) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTNEED);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_DONTNEED) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontdump(void *mem __maybe_unused, size_t len __maybe_unused) {
#if __linux__
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTDUMP);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_DONTDUMP) of size %zu, failed.", len);
    return ret;
#else
    return 0;
#endif
}

inline int madvise_mergeable(void *mem __maybe_unused, size_t len __maybe_unused) {
#ifdef MADV_MERGEABLE
    static int logger = 1;
    int ret = madvise(mem, len, MADV_MERGEABLE);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_MERGEABLE) of size %zu, failed.", len);
    return ret;
#else
    return 0;
#endif
}

void *netdata_mmap(const char *filename, size_t size, int flags, int ksm, bool read_only, int *open_fd)
{
    // netdata_log_info("netdata_mmap('%s', %zu", filename, size);

    // MAP_SHARED is used in memory mode map
    // MAP_PRIVATE is used in memory mode ram and save

    if(unlikely(!(flags & MAP_SHARED) && !(flags & MAP_PRIVATE)))
        fatal("Neither MAP_SHARED or MAP_PRIVATE were given to netdata_mmap()");

    if(unlikely((flags & MAP_SHARED) && (flags & MAP_PRIVATE)))
        fatal("Both MAP_SHARED and MAP_PRIVATE were given to netdata_mmap()");

    if(unlikely((flags & MAP_SHARED) && (!filename || !*filename)))
        fatal("MAP_SHARED requested, without a filename to netdata_mmap()");

    // don't enable ksm is the global setting is disabled
    if(unlikely(!enable_ksm)) ksm = 0;

    // KSM only merges anonymous (private) pages, never pagecache (file) pages
    // but MAP_PRIVATE without MAP_ANONYMOUS it fails too, so we need it always
    if((flags & MAP_PRIVATE)) flags |= MAP_ANONYMOUS;

    int fd = -1;
    void *mem = MAP_FAILED;

    if(filename && *filename) {
        // open/create the file to be used
        fd = memory_file_open(filename, size);
        if(fd == -1) goto cleanup;
    }

    int fd_for_mmap = fd;
    if(fd != -1 && (flags & MAP_PRIVATE)) {
        // this is MAP_PRIVATE allocation
        // no need for mmap() to use our fd
        // we will copy the file into the memory allocated
        fd_for_mmap = -1;
    }

    mem = mmap(NULL, size, read_only ? PROT_READ : PROT_READ | PROT_WRITE, flags, fd_for_mmap, 0);
    if (mem != MAP_FAILED) {

#ifdef NETDATA_TRACE_ALLOCATIONS
        malloc_trace_mmap(size);
#endif

        // if we have a file open, but we didn't give it to mmap(),
        // we have to read the file into the memory block we allocated
        if(fd != -1 && fd_for_mmap == -1) {
            if (lseek(fd, 0, SEEK_SET) == 0) {
                if (read(fd, mem, size) != (ssize_t) size)
                    netdata_log_info("Cannot read from file '%s'", filename);
            }
            else netdata_log_info("Cannot seek to beginning of file '%s'.", filename);
        }

        // madvise_sequential(mem, size);
        madvise_dontfork(mem, size);
        madvise_dontdump(mem, size);
        // if(flags & MAP_SHARED) madvise_willneed(mem, size);
        if(ksm) madvise_mergeable(mem, size);
    }

cleanup:
    if(fd != -1) {
        if (open_fd)
            *open_fd = fd;
        else
            close(fd);
    }
    if(mem == MAP_FAILED) return NULL;
    errno_clear();
    return mem;
}

int netdata_munmap(void *ptr, size_t size) {
#ifdef NETDATA_TRACE_ALLOCATIONS
    malloc_trace_munmap(size);
#endif
    return munmap(ptr, size);
}

char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len) {
    char *s = fgets(buf, (int)buf_size, fp);
    if (!s) return NULL;

    char *t = s;
    if (*t != '\0') {
        // find the string end
        while (*++t != '\0');

        // trim trailing spaces/newlines/tabs
        while (--t > s && *t == '\n')
            *t = '\0';
    }

    if (len)
        *len = t - s + 1;

    return s;
}

// vsnprintfz() returns the number of bytes actually written - after possible truncation
int vsnprintfz(char *dst, size_t n, const char *fmt, va_list args) {
    if(unlikely(!n)) return 0;

    int size = vsnprintf(dst, n, fmt, args);
    dst[n - 1] = '\0';

    if (unlikely((size_t) size >= n)) size = (int)(n - 1);

    return size;
}

// snprintfz() returns the number of bytes actually written - after possible truncation
int snprintfz(char *dst, size_t n, const char *fmt, ...) {
    va_list args;

    va_start(args, fmt);
    int ret = vsnprintfz(dst, n, fmt, args);
    va_end(args);

    return ret;
}

// Returns the number of bytes read from the file if file_size is not NULL.
// The actual buffer has an extra byte set to zero (not included in the count).
char *read_by_filename(const char *filename, long *file_size)
{
    FILE *f = fopen(filename, "r");
    if (!f)
        return NULL;

    if (fseek(f, 0, SEEK_END) < 0) {
        fclose(f);
        return NULL;
    }

    long size = ftell(f);
    if (size <= 0 || fseek(f, 0, SEEK_END) < 0) {
        fclose(f);
        return NULL;
    }

    char *contents = callocz(size + 1, 1);
    if (fseek(f, 0, SEEK_SET) < 0) {
        fclose(f);
        freez(contents);
        return NULL;
    }

    size_t res = fread(contents, 1, size, f);
    if ( res != (size_t)size) {
        freez(contents);
        fclose(f);
        return NULL;
    }

    fclose(f);

    if (file_size)
        *file_size = size;

    return contents;
}

char *find_and_replace(const char *src, const char *find, const char *replace, const char *where)
{
    size_t size = strlen(src) + 1;
    size_t find_len = strlen(find);
    size_t repl_len = strlen(replace);
    char *value, *dst;

    if (likely(where))
        size += (repl_len - find_len);

    value = mallocz(size);
    dst = value;

    if (likely(where)) {
        size_t count = where - src;

        memmove(dst, src, count);
        src += count;
        dst += count;

        memmove(dst, replace, repl_len);
        src += find_len;
        dst += repl_len;
    }

    strcpy(dst, src);

    return value;
}

BUFFER *run_command_and_get_output_to_buffer(const char *command, int max_line_length) {
    BUFFER *wb = buffer_create(0, NULL);

    POPEN_INSTANCE *pi = spawn_popen_run(command);
    if(pi) {
        char buffer[max_line_length + 1];
        while (fgets(buffer, max_line_length, spawn_popen_stdout(pi))) {
            buffer[max_line_length] = '\0';
            buffer_strcat(wb, buffer);
        }
        spawn_popen_kill(pi, 0);
    }
    else {
        buffer_free(wb);
        netdata_log_error("Failed to execute command '%s'.", command);
        return NULL;
    }

    return wb;
}

bool run_command_and_copy_output_to_stdout(const char *command, int max_line_length) {
    POPEN_INSTANCE *pi = spawn_popen_run(command);
    if(pi) {
        char buffer[max_line_length + 1];

        while (fgets(buffer, max_line_length, spawn_popen_stdout(pi)))
            fprintf(stdout, "%s", buffer);

        spawn_popen_kill(pi, 0);
    }
    else {
        netdata_log_error("Failed to execute command '%s'.", command);
        return false;
    }

    return true;
}

struct timing_steps {
    const char *name;
    usec_t time;
    size_t count;
} timing_steps[TIMING_STEP_MAX + 1] = {
        [TIMING_STEP_INTERNAL] = { .name = "internal", .time = 0, },

        [TIMING_STEP_BEGIN2_PREPARE] = { .name = "BEGIN2 prepare", .time = 0, },
        [TIMING_STEP_BEGIN2_FIND_CHART] = { .name = "BEGIN2 find chart", .time = 0, },
        [TIMING_STEP_BEGIN2_PARSE] = { .name = "BEGIN2 parse", .time = 0, },
        [TIMING_STEP_BEGIN2_ML] = { .name = "BEGIN2 ml", .time = 0, },
        [TIMING_STEP_BEGIN2_PROPAGATE] = { .name = "BEGIN2 propagate", .time = 0, },
        [TIMING_STEP_BEGIN2_STORE] = { .name = "BEGIN2 store", .time = 0, },

        [TIMING_STEP_SET2_PREPARE] = { .name = "SET2 prepare", .time = 0, },
        [TIMING_STEP_SET2_LOOKUP_DIMENSION] = { .name = "SET2 find dimension", .time = 0, },
        [TIMING_STEP_SET2_PARSE] = { .name = "SET2 parse", .time = 0, },
        [TIMING_STEP_SET2_ML] = { .name = "SET2 ml", .time = 0, },
        [TIMING_STEP_SET2_PROPAGATE] = { .name = "SET2 propagate", .time = 0, },
        [TIMING_STEP_RRDSET_STORE_METRIC] = { .name = "SET2 rrdset store", .time = 0, },
        [TIMING_STEP_DBENGINE_FIRST_CHECK] = { .name = "db 1st check", .time = 0, },
        [TIMING_STEP_DBENGINE_CHECK_DATA] = { .name = "db check data", .time = 0, },
        [TIMING_STEP_DBENGINE_PACK] = { .name = "db pack", .time = 0, },
        [TIMING_STEP_DBENGINE_PAGE_FIN] = { .name = "db page fin", .time = 0, },
        [TIMING_STEP_DBENGINE_MRG_UPDATE] = { .name = "db mrg update", .time = 0, },
        [TIMING_STEP_DBENGINE_PAGE_ALLOC] = { .name = "db page alloc", .time = 0, },
        [TIMING_STEP_DBENGINE_CREATE_NEW_PAGE] = { .name = "db new page", .time = 0, },
        [TIMING_STEP_DBENGINE_FLUSH_PAGE] = { .name = "db page flush", .time = 0, },
        [TIMING_STEP_SET2_STORE] = { .name = "SET2 store", .time = 0, },

        [TIMING_STEP_END2_PREPARE] = { .name = "END2 prepare", .time = 0, },
        [TIMING_STEP_END2_PUSH_V1] = { .name = "END2 push v1", .time = 0, },
        [TIMING_STEP_END2_ML] = { .name = "END2 ml", .time = 0, },
        [TIMING_STEP_END2_RRDSET] = { .name = "END2 rrdset", .time = 0, },
        [TIMING_STEP_END2_PROPAGATE] = { .name = "END2 propagate", .time = 0, },
        [TIMING_STEP_END2_STORE] = { .name = "END2 store", .time = 0, },

        [TIMING_STEP_DBENGINE_EVICT_LOCK]                       = { .name = "EVC_LOCK", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_SELECT]                     = { .name = "EVC_SELECT", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_SELECT_PAGE ]               = { .name = "EVT_SELECT_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_RELOCATE_PAGE ]             = { .name = "EVT_RELOCATE_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_SORT]                       = { .name = "EVC_SORT", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_DEINDEX]                    = { .name = "EVC_DEINDEX", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_DEINDEX_PAGE]               = { .name = "EVC_DEINDEX_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FINISHED]                   = { .name = "EVC_FINISHED", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_LOOP]                  = { .name = "EVC_FREE_LOOP", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_PAGE]                  = { .name = "EVC_FREE_PAGE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS]               = { .name = "EVC_FREE_ATOMICS", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_CB]                    = { .name = "EVC_FREE_CB", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS2]              = { .name = "EVC_FREE_ATOMICS2", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_ARAL]                  = { .name = "EVC_FREE_ARAL", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_DATA]         = { .name = "EVC_FREE_PGD_DATA", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_ARAL]         = { .name = "EVC_FREE_PGD_ARAL", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_TIER1_ARAL]   = { .name = "EVC_FREE_MAIN_T1ARL", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GLIVE]        = { .name = "EVC_FREE_MAIN_GLIVE", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GWORKER]      = { .name = "EVC_FREE_MAIN_GWORK", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_OPEN]                  = { .name = "EVC_FREE_OPEN", .time = 0, },
        [TIMING_STEP_DBENGINE_EVICT_FREE_EXTENT]                = { .name = "EVC_FREE_EXTENT", .time = 0, },

        // terminator
        [TIMING_STEP_MAX] = { .name = NULL, .time = 0, },
};

void timing_action(TIMING_ACTION action, TIMING_STEP step) {
    static __thread usec_t last_action_time = 0;
    static struct timing_steps timings2[TIMING_STEP_MAX + 1] = {};

    switch(action) {
        case TIMING_ACTION_INIT:
            last_action_time = now_monotonic_usec();
            break;

        case TIMING_ACTION_STEP: {
            if(!last_action_time)
                return;

            usec_t now = now_monotonic_usec();
            __atomic_add_fetch(&timing_steps[step].time, now - last_action_time, __ATOMIC_RELAXED);
            __atomic_add_fetch(&timing_steps[step].count, 1, __ATOMIC_RELAXED);
            last_action_time = now;
            break;
        }

        case TIMING_ACTION_FINISH: {
            if(!last_action_time)
                return;

            usec_t expected = __atomic_load_n(&timing_steps[TIMING_STEP_INTERNAL].time, __ATOMIC_RELAXED);
            if(last_action_time - expected < 10 * USEC_PER_SEC) {
                last_action_time = 0;
                return;
            }

            if(!__atomic_compare_exchange_n(&timing_steps[TIMING_STEP_INTERNAL].time, &expected, last_action_time, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST)) {
                last_action_time = 0;
                return;
            }

            struct timing_steps timings3[TIMING_STEP_MAX + 1];
            memcpy(timings3, timing_steps, sizeof(timings3));

            size_t total_reqs = 0;
            usec_t total_usec = 0;
            for(size_t t = 1; t < TIMING_STEP_MAX ; t++) {
                total_usec += timings3[t].time - timings2[t].time;
                total_reqs += timings3[t].count - timings2[t].count;
            }

            BUFFER *wb = buffer_create(1024, NULL);

            for(size_t t = 1; t < TIMING_STEP_MAX ; t++) {
                size_t requests = timings3[t].count - timings2[t].count;
                if(!requests) continue;

                buffer_sprintf(wb, "TIMINGS REPORT: [%3zu. %-20s]: # %10zu, t %11.2f ms (%6.2f %%), avg %6.2f usec/run\n",
                               t,
                               timing_steps[t].name ? timing_steps[t].name : "x",
                               requests,
                               (double) (timings3[t].time - timings2[t].time) / (double)USEC_PER_MS,
                               (double) (timings3[t].time - timings2[t].time) * 100.0 / (double) total_usec,
                               (double) (timings3[t].time - timings2[t].time) / (double)requests
                );
            }

            netdata_log_info("TIMINGS REPORT:\n%sTIMINGS REPORT:                        total # %10zu, t %11.2f ms",
                 buffer_tostring(wb), total_reqs, (double)total_usec / USEC_PER_MS);

            memcpy(timings2, timings3, sizeof(timings2));

            last_action_time = 0;
            buffer_free(wb);
        }
    }
}

int hash256_string(const unsigned char *string, size_t size, char *hash) {
    EVP_MD_CTX *ctx;
    ctx = EVP_MD_CTX_create();

    if (!ctx)
        return 0;

    if (!EVP_DigestInit(ctx, EVP_sha256())) {
        EVP_MD_CTX_destroy(ctx);
        return 0;
    }

    if (!EVP_DigestUpdate(ctx, string, size)) {
        EVP_MD_CTX_destroy(ctx);
        return 0;
    }

    if (!EVP_DigestFinal(ctx, (unsigned char *)hash, NULL)) {
        EVP_MD_CTX_destroy(ctx);
        return 0;
    }
    EVP_MD_CTX_destroy(ctx);
    return 1;
}


bool rrdr_relative_window_to_absolute(time_t *after, time_t *before, time_t now) {
    if(!now) now = now_realtime_sec();

    int absolute_period_requested = -1;
    time_t before_requested = *before;
    time_t after_requested = *after;

    // allow relative for before (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(before_requested) <= API_RELATIVE_TIME_MAX) {
        // if the user asked for a positive relative time,
        // flip it to a negative
        if(before_requested > 0)
            before_requested = -before_requested;

        before_requested = now + before_requested;
        absolute_period_requested = 0;
    }

    // allow relative for after (smaller than API_RELATIVE_TIME_MAX)
    if(ABS(after_requested) <= API_RELATIVE_TIME_MAX) {
        if(after_requested > 0)
            after_requested = -after_requested;

        // if the user didn't give an after, use the number of points
        // to give a sane default
        if(after_requested == 0)
            after_requested = -600;

        // since the query engine now returns inclusive timestamps
        // it is awkward to return 6 points when after=-5 is given
        // so for relative queries we add 1 second, to give
        // more predictable results to users.
        after_requested = before_requested + after_requested + 1;
        absolute_period_requested = 0;
    }

    if(absolute_period_requested == -1)
        absolute_period_requested = 1;

    // check if the parameters are flipped
    if(after_requested > before_requested) {
        long long t = before_requested;
        before_requested = after_requested;
        after_requested = t;
    }

    // if the query requests future data
    // shift the query back to be in the present time
    // (this may also happen because of the rules above)
    if(before_requested > now) {
        time_t delta = before_requested - now;
        before_requested -= delta;
        after_requested  -= delta;
    }

    *before = before_requested;
    *after = after_requested;

    return (absolute_period_requested != 1);
}

// Returns 1 if an absolute period was requested or 0 if it was a relative period
bool rrdr_relative_window_to_absolute_query(time_t *after, time_t *before, time_t *now_ptr, bool unittest) {
    time_t now = now_realtime_sec() - 1;

    if(now_ptr)
        *now_ptr = now;

    time_t before_requested = *before;
    time_t after_requested = *after;

    int absolute_period_requested = rrdr_relative_window_to_absolute(&after_requested, &before_requested, now);

    time_t absolute_minimum_time = now - (10 * 365 * 86400);
    time_t absolute_maximum_time = now + (1 * 365 * 86400);

    if (after_requested < absolute_minimum_time && !unittest)
        after_requested = absolute_minimum_time;

    if (after_requested > absolute_maximum_time && !unittest)
        after_requested = absolute_maximum_time;

    if (before_requested < absolute_minimum_time && !unittest)
        before_requested = absolute_minimum_time;

    if (before_requested > absolute_maximum_time && !unittest)
        before_requested = absolute_maximum_time;

    *before = before_requested;
    *after = after_requested;

    return (absolute_period_requested != 1);
}


#if defined(OPENSSL_VERSION_NUMBER) && OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110
static inline EVP_ENCODE_CTX *EVP_ENCODE_CTX_new(void)
{
    EVP_ENCODE_CTX *ctx = OPENSSL_malloc(sizeof(*ctx));

    if (ctx != NULL) {
        memset(ctx, 0, sizeof(*ctx));
    }
    return ctx;
}

static void EVP_ENCODE_CTX_free(EVP_ENCODE_CTX *ctx)
{
	OPENSSL_free(ctx);
}
#endif

int netdata_base64_decode(unsigned char *out, const unsigned char *in, const int in_len)
{
    int outl;
    unsigned char remaining_data[256];

    EVP_ENCODE_CTX *ctx = EVP_ENCODE_CTX_new();
    EVP_DecodeInit(ctx);
    EVP_DecodeUpdate(ctx, out, &outl, in, in_len);
    int remainder = 0;
    EVP_DecodeFinal(ctx, remaining_data, &remainder);
    EVP_ENCODE_CTX_free(ctx);
    if (remainder)
        return -1;

    return outl;
}

int netdata_base64_encode(unsigned char *encoded, const unsigned char *input, size_t input_size)
{
    return EVP_EncodeBlock(encoded, input, input_size);
}

// Keep internal implementation
// int netdata_base64_decode_internal(const char *encoded, char *decoded, size_t decoded_size) {
//     static const unsigned char base64_table[256] = {
//             ['A'] = 0, ['B'] = 1, ['C'] = 2, ['D'] = 3, ['E'] = 4, ['F'] = 5, ['G'] = 6, ['H'] = 7,
//             ['I'] = 8, ['J'] = 9, ['K'] = 10, ['L'] = 11, ['M'] = 12, ['N'] = 13, ['O'] = 14, ['P'] = 15,
//             ['Q'] = 16, ['R'] = 17, ['S'] = 18, ['T'] = 19, ['U'] = 20, ['V'] = 21, ['W'] = 22, ['X'] = 23,
//             ['Y'] = 24, ['Z'] = 25, ['a'] = 26, ['b'] = 27, ['c'] = 28, ['d'] = 29, ['e'] = 30, ['f'] = 31,
//             ['g'] = 32, ['h'] = 33, ['i'] = 34, ['j'] = 35, ['k'] = 36, ['l'] = 37, ['m'] = 38, ['n'] = 39,
//             ['o'] = 40, ['p'] = 41, ['q'] = 42, ['r'] = 43, ['s'] = 44, ['t'] = 45, ['u'] = 46, ['v'] = 47,
//             ['w'] = 48, ['x'] = 49, ['y'] = 50, ['z'] = 51, ['0'] = 52, ['1'] = 53, ['2'] = 54, ['3'] = 55,
//             ['4'] = 56, ['5'] = 57, ['6'] = 58, ['7'] = 59, ['8'] = 60, ['9'] = 61, ['+'] = 62, ['/'] = 63,
//             [0 ... '+' - 1] = 255,
//             ['+' + 1 ... '/' - 1] = 255,
//             ['9' + 1 ... 'A' - 1] = 255,
//             ['Z' + 1 ... 'a' - 1] = 255,
//             ['z' + 1 ... 255] = 255
//     };
//
//     size_t count = 0;
//     unsigned int tmp = 0;
//     int i, bit;
//
//     if (decoded_size < 1)
//         return 0; // Buffer size must be at least 1 for null termination
//
//     for (i = 0, bit = 0; encoded[i]; i++) {
//         unsigned char value = base64_table[(unsigned char)encoded[i]];
//         if (value > 63)
//             return -1; // Invalid character in input
//
//         tmp = tmp << 6 | value;
//         if (++bit == 4) {
//             if (count + 3 >= decoded_size) break; // Stop decoding if buffer is full
//             decoded[count++] = (tmp >> 16) & 0xFF;
//             decoded[count++] = (tmp >> 8) & 0xFF;
//             decoded[count++] = tmp & 0xFF;
//             tmp = 0;
//             bit = 0;
//         }
//     }
//
//     if (bit > 0 && count + 1 < decoded_size) {
//         tmp <<= 6 * (4 - bit);
//         if (bit > 2 && count + 1 < decoded_size) decoded[count++] = (tmp >> 16) & 0xFF;
//         if (bit > 3 && count + 1 < decoded_size) decoded[count++] = (tmp >> 8) & 0xFF;
//     }
//
//     decoded[count] = '\0'; // Null terminate the output string
//     return count;
// }
