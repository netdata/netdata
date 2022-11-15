// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata.h"

#ifdef __APPLE__
#define INHERIT_NONE 0
#endif /* __APPLE__ */
#if defined(__FreeBSD__) || defined(__APPLE__)
#    define O_NOATIME     0
#    define MADV_DONTFORK INHERIT_NONE
#endif /* __FreeBSD__ || __APPLE__*/

struct rlimit rlimit_nofile = { .rlim_cur = 1024, .rlim_max = 1024 };

#ifdef MADV_MERGEABLE
int enable_ksm = 1;
#else
int enable_ksm = 0;
#endif

volatile sig_atomic_t netdata_exit = 0;
const char *program_version = VERSION;

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

Word_t JudyMalloc(Word_t Words) {
    Word_t Addr;

    Addr = (Word_t) mallocz(Words * sizeof(Word_t));
    return(Addr);
}
void JudyFree(void * PWord, Word_t Words) {
    (void)Words;
    freez(PWord);
}
Word_t JudyMallocVirtual(Word_t Words) {
    Word_t Addr;

    Addr = (Word_t) mallocz(Words * sizeof(Word_t));
    return(Addr);
}
void JudyFreeVirtual(void * PWord, Word_t Words) {
    (void)Words;
    freez(PWord);
}

#define MALLOC_ALIGNMENT (sizeof(uintptr_t) * 2)
#define size_t_atomic_count(op, var, size) __atomic_## op ##_fetch(&(var), size, __ATOMIC_RELAXED)
#define size_t_atomic_bytes(op, var, size) __atomic_## op ##_fetch(&(var), ((size) % MALLOC_ALIGNMENT)?((size) + MALLOC_ALIGNMENT - ((size) % MALLOC_ALIGNMENT)):(size), __ATOMIC_RELAXED)

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
    .rwlock = NETDATA_RWLOCK_INITIALIZER
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

static struct malloc_header *malloc_get_header(void *ptr, const char *caller, const char *file, const char *function, size_t line) {
    uint8_t *ret = (uint8_t *)ptr - malloc_header_size;
    struct malloc_header *t = (struct malloc_header *)ret;

    if(t->signature.magic != 0x0BADCAFE) {
        error("pointer %p is not our pointer (called %s() from %zu@%s, %s()).", ptr, caller, line, file, function);
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

// If ptr is NULL, no operation is performed.
void freez(void *ptr) {
    free(ptr);
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

unsigned char netdata_map_chart_names[256] = {
        [0] = '\0', //
        [1] = '_', //
        [2] = '_', //
        [3] = '_', //
        [4] = '_', //
        [5] = '_', //
        [6] = '_', //
        [7] = '_', //
        [8] = '_', //
        [9] = '_', //
        [10] = '_', //
        [11] = '_', //
        [12] = '_', //
        [13] = '_', //
        [14] = '_', //
        [15] = '_', //
        [16] = '_', //
        [17] = '_', //
        [18] = '_', //
        [19] = '_', //
        [20] = '_', //
        [21] = '_', //
        [22] = '_', //
        [23] = '_', //
        [24] = '_', //
        [25] = '_', //
        [26] = '_', //
        [27] = '_', //
        [28] = '_', //
        [29] = '_', //
        [30] = '_', //
        [31] = '_', //
        [32] = '_', //
        [33] = '_', // !
        [34] = '_', // "
        [35] = '_', // #
        [36] = '_', // $
        [37] = '_', // %
        [38] = '_', // &
        [39] = '_', // '
        [40] = '_', // (
        [41] = '_', // )
        [42] = '_', // *
        [43] = '_', // +
        [44] = '.', // ,
        [45] = '-', // -
        [46] = '.', // .
        [47] = '/', // /
        [48] = '0', // 0
        [49] = '1', // 1
        [50] = '2', // 2
        [51] = '3', // 3
        [52] = '4', // 4
        [53] = '5', // 5
        [54] = '6', // 6
        [55] = '7', // 7
        [56] = '8', // 8
        [57] = '9', // 9
        [58] = '_', // :
        [59] = '_', // ;
        [60] = '_', // <
        [61] = '_', // =
        [62] = '_', // >
        [63] = '_', // ?
        [64] = '_', // @
        [65] = 'a', // A
        [66] = 'b', // B
        [67] = 'c', // C
        [68] = 'd', // D
        [69] = 'e', // E
        [70] = 'f', // F
        [71] = 'g', // G
        [72] = 'h', // H
        [73] = 'i', // I
        [74] = 'j', // J
        [75] = 'k', // K
        [76] = 'l', // L
        [77] = 'm', // M
        [78] = 'n', // N
        [79] = 'o', // O
        [80] = 'p', // P
        [81] = 'q', // Q
        [82] = 'r', // R
        [83] = 's', // S
        [84] = 't', // T
        [85] = 'u', // U
        [86] = 'v', // V
        [87] = 'w', // W
        [88] = 'x', // X
        [89] = 'y', // Y
        [90] = 'z', // Z
        [91] = '_', // [
        [92] = '/', // backslash
        [93] = '_', // ]
        [94] = '_', // ^
        [95] = '_', // _
        [96] = '_', // `
        [97] = 'a', // a
        [98] = 'b', // b
        [99] = 'c', // c
        [100] = 'd', // d
        [101] = 'e', // e
        [102] = 'f', // f
        [103] = 'g', // g
        [104] = 'h', // h
        [105] = 'i', // i
        [106] = 'j', // j
        [107] = 'k', // k
        [108] = 'l', // l
        [109] = 'm', // m
        [110] = 'n', // n
        [111] = 'o', // o
        [112] = 'p', // p
        [113] = 'q', // q
        [114] = 'r', // r
        [115] = 's', // s
        [116] = 't', // t
        [117] = 'u', // u
        [118] = 'v', // v
        [119] = 'w', // w
        [120] = 'x', // x
        [121] = 'y', // y
        [122] = 'z', // z
        [123] = '_', // {
        [124] = '_', // |
        [125] = '_', // }
        [126] = '_', // ~
        [127] = '_', //
        [128] = '_', //
        [129] = '_', //
        [130] = '_', //
        [131] = '_', //
        [132] = '_', //
        [133] = '_', //
        [134] = '_', //
        [135] = '_', //
        [136] = '_', //
        [137] = '_', //
        [138] = '_', //
        [139] = '_', //
        [140] = '_', //
        [141] = '_', //
        [142] = '_', //
        [143] = '_', //
        [144] = '_', //
        [145] = '_', //
        [146] = '_', //
        [147] = '_', //
        [148] = '_', //
        [149] = '_', //
        [150] = '_', //
        [151] = '_', //
        [152] = '_', //
        [153] = '_', //
        [154] = '_', //
        [155] = '_', //
        [156] = '_', //
        [157] = '_', //
        [158] = '_', //
        [159] = '_', //
        [160] = '_', //
        [161] = '_', //
        [162] = '_', //
        [163] = '_', //
        [164] = '_', //
        [165] = '_', //
        [166] = '_', //
        [167] = '_', //
        [168] = '_', //
        [169] = '_', //
        [170] = '_', //
        [171] = '_', //
        [172] = '_', //
        [173] = '_', //
        [174] = '_', //
        [175] = '_', //
        [176] = '_', //
        [177] = '_', //
        [178] = '_', //
        [179] = '_', //
        [180] = '_', //
        [181] = '_', //
        [182] = '_', //
        [183] = '_', //
        [184] = '_', //
        [185] = '_', //
        [186] = '_', //
        [187] = '_', //
        [188] = '_', //
        [189] = '_', //
        [190] = '_', //
        [191] = '_', //
        [192] = '_', //
        [193] = '_', //
        [194] = '_', //
        [195] = '_', //
        [196] = '_', //
        [197] = '_', //
        [198] = '_', //
        [199] = '_', //
        [200] = '_', //
        [201] = '_', //
        [202] = '_', //
        [203] = '_', //
        [204] = '_', //
        [205] = '_', //
        [206] = '_', //
        [207] = '_', //
        [208] = '_', //
        [209] = '_', //
        [210] = '_', //
        [211] = '_', //
        [212] = '_', //
        [213] = '_', //
        [214] = '_', //
        [215] = '_', //
        [216] = '_', //
        [217] = '_', //
        [218] = '_', //
        [219] = '_', //
        [220] = '_', //
        [221] = '_', //
        [222] = '_', //
        [223] = '_', //
        [224] = '_', //
        [225] = '_', //
        [226] = '_', //
        [227] = '_', //
        [228] = '_', //
        [229] = '_', //
        [230] = '_', //
        [231] = '_', //
        [232] = '_', //
        [233] = '_', //
        [234] = '_', //
        [235] = '_', //
        [236] = '_', //
        [237] = '_', //
        [238] = '_', //
        [239] = '_', //
        [240] = '_', //
        [241] = '_', //
        [242] = '_', //
        [243] = '_', //
        [244] = '_', //
        [245] = '_', //
        [246] = '_', //
        [247] = '_', //
        [248] = '_', //
        [249] = '_', //
        [250] = '_', //
        [251] = '_', //
        [252] = '_', //
        [253] = '_', //
        [254] = '_', //
        [255] = '_'  //
};

// make sure the supplied string
// is good for a netdata chart/dimension ID/NAME
void netdata_fix_chart_name(char *s) {
    while ((*s = netdata_map_chart_names[(unsigned char) *s])) s++;
}

unsigned char netdata_map_chart_ids[256] = {
        [0] = '\0', //
        [1] = '_', //
        [2] = '_', //
        [3] = '_', //
        [4] = '_', //
        [5] = '_', //
        [6] = '_', //
        [7] = '_', //
        [8] = '_', //
        [9] = '_', //
        [10] = '_', //
        [11] = '_', //
        [12] = '_', //
        [13] = '_', //
        [14] = '_', //
        [15] = '_', //
        [16] = '_', //
        [17] = '_', //
        [18] = '_', //
        [19] = '_', //
        [20] = '_', //
        [21] = '_', //
        [22] = '_', //
        [23] = '_', //
        [24] = '_', //
        [25] = '_', //
        [26] = '_', //
        [27] = '_', //
        [28] = '_', //
        [29] = '_', //
        [30] = '_', //
        [31] = '_', //
        [32] = '_', //
        [33] = '_', // !
        [34] = '_', // "
        [35] = '_', // #
        [36] = '_', // $
        [37] = '_', // %
        [38] = '_', // &
        [39] = '_', // '
        [40] = '_', // (
        [41] = '_', // )
        [42] = '_', // *
        [43] = '_', // +
        [44] = '.', // ,
        [45] = '-', // -
        [46] = '.', // .
        [47] = '_', // /
        [48] = '0', // 0
        [49] = '1', // 1
        [50] = '2', // 2
        [51] = '3', // 3
        [52] = '4', // 4
        [53] = '5', // 5
        [54] = '6', // 6
        [55] = '7', // 7
        [56] = '8', // 8
        [57] = '9', // 9
        [58] = '_', // :
        [59] = '_', // ;
        [60] = '_', // <
        [61] = '_', // =
        [62] = '_', // >
        [63] = '_', // ?
        [64] = '_', // @
        [65] = 'a', // A
        [66] = 'b', // B
        [67] = 'c', // C
        [68] = 'd', // D
        [69] = 'e', // E
        [70] = 'f', // F
        [71] = 'g', // G
        [72] = 'h', // H
        [73] = 'i', // I
        [74] = 'j', // J
        [75] = 'k', // K
        [76] = 'l', // L
        [77] = 'm', // M
        [78] = 'n', // N
        [79] = 'o', // O
        [80] = 'p', // P
        [81] = 'q', // Q
        [82] = 'r', // R
        [83] = 's', // S
        [84] = 't', // T
        [85] = 'u', // U
        [86] = 'v', // V
        [87] = 'w', // W
        [88] = 'x', // X
        [89] = 'y', // Y
        [90] = 'z', // Z
        [91] = '_', // [
        [92] = '_', // backslash
        [93] = '_', // ]
        [94] = '_', // ^
        [95] = '_', // _
        [96] = '_', // `
        [97] = 'a', // a
        [98] = 'b', // b
        [99] = 'c', // c
        [100] = 'd', // d
        [101] = 'e', // e
        [102] = 'f', // f
        [103] = 'g', // g
        [104] = 'h', // h
        [105] = 'i', // i
        [106] = 'j', // j
        [107] = 'k', // k
        [108] = 'l', // l
        [109] = 'm', // m
        [110] = 'n', // n
        [111] = 'o', // o
        [112] = 'p', // p
        [113] = 'q', // q
        [114] = 'r', // r
        [115] = 's', // s
        [116] = 't', // t
        [117] = 'u', // u
        [118] = 'v', // v
        [119] = 'w', // w
        [120] = 'x', // x
        [121] = 'y', // y
        [122] = 'z', // z
        [123] = '_', // {
        [124] = '_', // |
        [125] = '_', // }
        [126] = '_', // ~
        [127] = '_', //
        [128] = '_', //
        [129] = '_', //
        [130] = '_', //
        [131] = '_', //
        [132] = '_', //
        [133] = '_', //
        [134] = '_', //
        [135] = '_', //
        [136] = '_', //
        [137] = '_', //
        [138] = '_', //
        [139] = '_', //
        [140] = '_', //
        [141] = '_', //
        [142] = '_', //
        [143] = '_', //
        [144] = '_', //
        [145] = '_', //
        [146] = '_', //
        [147] = '_', //
        [148] = '_', //
        [149] = '_', //
        [150] = '_', //
        [151] = '_', //
        [152] = '_', //
        [153] = '_', //
        [154] = '_', //
        [155] = '_', //
        [156] = '_', //
        [157] = '_', //
        [158] = '_', //
        [159] = '_', //
        [160] = '_', //
        [161] = '_', //
        [162] = '_', //
        [163] = '_', //
        [164] = '_', //
        [165] = '_', //
        [166] = '_', //
        [167] = '_', //
        [168] = '_', //
        [169] = '_', //
        [170] = '_', //
        [171] = '_', //
        [172] = '_', //
        [173] = '_', //
        [174] = '_', //
        [175] = '_', //
        [176] = '_', //
        [177] = '_', //
        [178] = '_', //
        [179] = '_', //
        [180] = '_', //
        [181] = '_', //
        [182] = '_', //
        [183] = '_', //
        [184] = '_', //
        [185] = '_', //
        [186] = '_', //
        [187] = '_', //
        [188] = '_', //
        [189] = '_', //
        [190] = '_', //
        [191] = '_', //
        [192] = '_', //
        [193] = '_', //
        [194] = '_', //
        [195] = '_', //
        [196] = '_', //
        [197] = '_', //
        [198] = '_', //
        [199] = '_', //
        [200] = '_', //
        [201] = '_', //
        [202] = '_', //
        [203] = '_', //
        [204] = '_', //
        [205] = '_', //
        [206] = '_', //
        [207] = '_', //
        [208] = '_', //
        [209] = '_', //
        [210] = '_', //
        [211] = '_', //
        [212] = '_', //
        [213] = '_', //
        [214] = '_', //
        [215] = '_', //
        [216] = '_', //
        [217] = '_', //
        [218] = '_', //
        [219] = '_', //
        [220] = '_', //
        [221] = '_', //
        [222] = '_', //
        [223] = '_', //
        [224] = '_', //
        [225] = '_', //
        [226] = '_', //
        [227] = '_', //
        [228] = '_', //
        [229] = '_', //
        [230] = '_', //
        [231] = '_', //
        [232] = '_', //
        [233] = '_', //
        [234] = '_', //
        [235] = '_', //
        [236] = '_', //
        [237] = '_', //
        [238] = '_', //
        [239] = '_', //
        [240] = '_', //
        [241] = '_', //
        [242] = '_', //
        [243] = '_', //
        [244] = '_', //
        [245] = '_', //
        [246] = '_', //
        [247] = '_', //
        [248] = '_', //
        [249] = '_', //
        [250] = '_', //
        [251] = '_', //
        [252] = '_', //
        [253] = '_', //
        [254] = '_', //
        [255] = '_'  //
};

// make sure the supplied string
// is good for a netdata chart/dimension ID/NAME
void netdata_fix_chart_id(char *s) {
    while ((*s = netdata_map_chart_ids[(unsigned char) *s])) s++;
}

/*
// http://stackoverflow.com/questions/7666509/hash-function-for-string
uint32_t simple_hash(const char *name)
{
    const char *s = name;
    uint32_t hash = 5381;
    int i;

    while((i = *s++)) hash = ((hash << 5) + hash) + i;

    // fprintf(stderr, "HASH: %lu %s\n", hash, name);

    return hash;
}
*/

/*
// http://isthe.com/chongo/tech/comp/fnv/#FNV-1a
uint32_t simple_hash(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hval = 0x811c9dc5;

    // FNV-1a algorithm
    while (*s) {
        // multiply by the 32 bit FNV magic prime mod 2^32
        // NOTE: No need to optimize with left shifts.
        //       GCC will use imul instruction anyway.
        //       Tested with 'gcc -O3 -S'
        //hval += (hval<<1) + (hval<<4) + (hval<<7) + (hval<<8) + (hval<<24);
        hval *= 16777619;

        // xor the bottom with the current octet
        hval ^= (uint32_t) *s++;
    }

    // fprintf(stderr, "HASH: %u = %s\n", hval, name);
    return hval;
}

uint32_t simple_uhash(const char *name) {
    unsigned char *s = (unsigned char *) name;
    uint32_t hval = 0x811c9dc5, c;

    // FNV-1a algorithm
    while ((c = *s++)) {
        if (unlikely(c >= 'A' && c <= 'Z')) c += 'a' - 'A';
        hval *= 16777619;
        hval ^= c;
    }
    return hval;
}
*/

/*
// http://eternallyconfuzzled.com/tuts/algorithms/jsw_tut_hashing.aspx
// one at a time hash
uint32_t simple_hash(const char *name) {
    unsigned char *s = (unsigned char *)name;
    uint32_t h = 0;

    while(*s) {
        h += *s++;
        h += (h << 10);
        h ^= (h >> 6);
    }

    h += (h << 3);
    h ^= (h >> 11);
    h += (h << 15);

    // fprintf(stderr, "HASH: %u = %s\n", h, name);

    return h;
}
*/

void strreverse(char *begin, char *end) {
    while (end > begin) {
        // clearer code.
        char aux = *end;
        *end-- = *begin;
        *begin++ = aux;
    }
}

char *strsep_on_1char(char **ptr, char c) {
    if(unlikely(!ptr || !*ptr))
        return NULL;

    // remember the position we started
    char *s = *ptr;

    // skip separators in front
    while(*s == c) s++;
    char *ret = s;

    // find the next separator
    while(*s++) {
        if(unlikely(*s == c)) {
            *s++ = '\0';
            *ptr = s;
            return ret;
        }
    }

    *ptr = NULL;
    return ret;
}

char *mystrsep(char **ptr, char *s) {
    char *p = "";
    while (p && !p[0] && *ptr) p = strsep(ptr, s);
    return (p);
}

char *trim(char *s) {
    // skip leading spaces
    while (*s && isspace(*s)) s++;
    if (!*s) return NULL;

    // skip tailing spaces
    // this way is way faster. Writes only one NUL char.
    ssize_t l = strlen(s);
    if (--l >= 0) {
        char *p = s + l;
        while (p > s && isspace(*p)) p--;
        *++p = '\0';
    }

    if (!*s) return NULL;

    return s;
}

inline char *trim_all(char *buffer) {
    char *d = buffer, *s = buffer;

    // skip spaces
    while(isspace(*s)) s++;

    while(*s) {
        // copy the non-space part
        while(*s && !isspace(*s)) *d++ = *s++;

        // add a space if we have to
        if(*s && isspace(*s)) {
            *d++ = ' ';
            s++;
        }

        // skip spaces
        while(isspace(*s)) s++;
    }

    *d = '\0';

    if(d > buffer) {
        d--;
        if(isspace(*d)) *d = '\0';
    }

    if(!buffer[0]) return NULL;
    return buffer;
}

static int memory_file_open(const char *filename, size_t size) {
    // info("memory_file_open('%s', %zu", filename, size);

    int fd = open(filename, O_RDWR | O_CREAT | O_NOATIME, 0664);
    if (fd != -1) {
        if (lseek(fd, size, SEEK_SET) == (off_t) size) {
            if (write(fd, "", 1) == 1) {
                if (ftruncate(fd, size))
                    error("Cannot truncate file '%s' to size %zu. Will use the larger file.", filename, size);
            }
            else error("Cannot write to file '%s' at position %zu.", filename, size);
        }
        else error("Cannot seek file '%s' to size %zu.", filename, size);
    }
    else error("Cannot create/open file '%s'.", filename);

    return fd;
}

static inline int madvise_sequential(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_SEQUENTIAL);

    if (ret != 0 && logger-- > 0) error("madvise(MADV_SEQUENTIAL) failed.");
    return ret;
}

static inline int madvise_dontfork(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTFORK);

    if (ret != 0 && logger-- > 0) error("madvise(MADV_DONTFORK) failed.");
    return ret;
}

static inline int madvise_willneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_WILLNEED);

    if (ret != 0 && logger-- > 0) error("madvise(MADV_WILLNEED) failed.");
    return ret;
}

#if __linux__
static inline int madvise_dontdump(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTDUMP);

    if (ret != 0 && logger-- > 0) error("madvise(MADV_DONTDUMP) failed.");
    return ret;
}
#else
static inline int madvise_dontdump(void *mem, size_t len) {
    UNUSED(mem);
    UNUSED(len);

    return 0;
}
#endif

static inline int madvise_mergeable(void *mem, size_t len) {
#ifdef MADV_MERGEABLE
    static int logger = 1;
    int ret = madvise(mem, len, MADV_MERGEABLE);

    if (ret != 0 && logger-- > 0) error("madvise(MADV_MERGEABLE) failed.");
    return ret;
#else
    UNUSED(mem);
    UNUSED(len);
    
    return 0;
#endif
}

void *netdata_mmap(const char *filename, size_t size, int flags, int ksm, bool read_only)
{
    // info("netdata_mmap('%s', %zu", filename, size);

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
                    info("Cannot read from file '%s'", filename);
            }
            else info("Cannot seek to beginning of file '%s'.", filename);
        }

        madvise_sequential(mem, size);
        madvise_dontfork(mem, size);
        madvise_dontdump(mem, size);
        if(flags & MAP_SHARED) madvise_willneed(mem, size);
        if(ksm) madvise_mergeable(mem, size);
    }

cleanup:
    if(fd != -1) close(fd);
    if(mem == MAP_FAILED) return NULL;
    errno = 0;
    return mem;
}

int netdata_munmap(void *ptr, size_t size) {
#ifdef NETDATA_TRACE_ALLOCATIONS
    malloc_trace_munmap(size);
#endif
    return munmap(ptr, size);
}

int memory_file_save(const char *filename, void *mem, size_t size) {
    char tmpfilename[FILENAME_MAX + 1];

    snprintfz(tmpfilename, FILENAME_MAX, "%s.%ld.tmp", filename, (long) getpid());

    int fd = open(tmpfilename, O_RDWR | O_CREAT | O_NOATIME, 0664);
    if (fd < 0) {
        error("Cannot create/open file '%s'.", filename);
        return -1;
    }

    if (write(fd, mem, size) != (ssize_t) size) {
        error("Cannot write to file '%s' %ld bytes.", filename, (long) size);
        close(fd);
        return -1;
    }

    close(fd);

    if (rename(tmpfilename, filename)) {
        error("Cannot rename '%s' to '%s'", tmpfilename, filename);
        return -1;
    }

    return 0;
}

int fd_is_valid(int fd) {
    return fcntl(fd, F_GETFD) != -1 || errno != EBADF;
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

int vsnprintfz(char *dst, size_t n, const char *fmt, va_list args) {
    if(unlikely(!n)) return 0;

    int size = vsnprintf(dst, n, fmt, args);
    dst[n - 1] = '\0';

    if (unlikely((size_t) size > n)) size = (int)n;

    return size;
}

int snprintfz(char *dst, size_t n, const char *fmt, ...) {
    va_list args;

    va_start(args, fmt);
    int ret = vsnprintfz(dst, n, fmt, args);
    va_end(args);

    return ret;
}

/*
// poor man cycle counting
static unsigned long tsc;
void begin_tsc(void) {
    unsigned long a, d;
    asm volatile ("cpuid\nrdtsc" : "=a" (a), "=d" (d) : "0" (0) : "ebx", "ecx");
    tsc = ((unsigned long)d << 32) | (unsigned long)a;
}
unsigned long end_tsc(void) {
    unsigned long a, d;
    asm volatile ("rdtscp" : "=a" (a), "=d" (d) : : "ecx");
    return (((unsigned long)d << 32) | (unsigned long)a) - tsc;
}
*/

int recursively_delete_dir(const char *path, const char *reason) {
    DIR *dir = opendir(path);
    if(!dir) {
        error("Cannot read %s directory to be deleted '%s'", reason?reason:"", path);
        return -1;
    }

    int ret = 0;
    struct dirent *de = NULL;
    while((de = readdir(dir))) {
        if(de->d_type == DT_DIR
           && (
                   (de->d_name[0] == '.' && de->d_name[1] == '\0')
                   || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
           ))
            continue;

        char fullpath[FILENAME_MAX + 1];
        snprintfz(fullpath, FILENAME_MAX, "%s/%s", path, de->d_name);

        if(de->d_type == DT_DIR) {
            int r = recursively_delete_dir(fullpath, reason);
            if(r > 0) ret += r;
            continue;
        }

        info("Deleting %s file '%s'", reason?reason:"", fullpath);
        if(unlikely(unlink(fullpath) == -1))
            error("Cannot delete %s file '%s'", reason?reason:"", fullpath);
        else
            ret++;
    }

    info("Deleting empty directory '%s'", path);
    if(unlikely(rmdir(path) == -1))
        error("Cannot delete empty directory '%s'", path);
    else
        ret++;

    closedir(dir);

    return ret;
}

static int is_virtual_filesystem(const char *path, char **reason) {

#if defined(__APPLE__) || defined(__FreeBSD__)
    (void)path;
    (void)reason;
#else
    struct statfs stat;
    // stat.f_fsid.__val[0] is a file system id
    // stat.f_fsid.__val[1] is the inode
    // so their combination uniquely identifies the file/dir

    if (statfs(path, &stat) == -1) {
        if(reason) *reason = "failed to statfs()";
        return -1;
    }

    if(stat.f_fsid.__val[0] != 0 || stat.f_fsid.__val[1] != 0) {
        errno = EINVAL;
        if(reason) *reason = "is not a virtual file system";
        return -1;
    }
#endif

    return 0;
}

int verify_netdata_host_prefix() {
    if(!netdata_configured_host_prefix)
        netdata_configured_host_prefix = "";

    if(!*netdata_configured_host_prefix)
        return 0;

    char buffer[FILENAME_MAX + 1];
    char *path = netdata_configured_host_prefix;
    char *reason = "unknown reason";
    errno = 0;

    struct stat sb;
    if (stat(path, &sb) == -1) {
        reason = "failed to stat()";
        goto failed;
    }

    if((sb.st_mode & S_IFMT) != S_IFDIR) {
        errno = EINVAL;
        reason = "is not a directory";
        goto failed;
    }

    path = buffer;
    snprintfz(path, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
    if(is_virtual_filesystem(path, &reason) == -1)
        goto failed;

    snprintfz(path, FILENAME_MAX, "%s/sys", netdata_configured_host_prefix);
    if(is_virtual_filesystem(path, &reason) == -1)
        goto failed;

    if(netdata_configured_host_prefix && *netdata_configured_host_prefix)
        info("Using host prefix directory '%s'", netdata_configured_host_prefix);

    return 0;

failed:
    error("Ignoring host prefix '%s': path '%s' %s", netdata_configured_host_prefix, path, reason);
    netdata_configured_host_prefix = "";
    return -1;
}

char *strdupz_path_subpath(const char *path, const char *subpath) {
    if(unlikely(!path || !*path)) path = ".";
    if(unlikely(!subpath)) subpath = "";

    // skip trailing slashes in path
    size_t len = strlen(path);
    while(len > 0 && path[len - 1] == '/') len--;

    // skip leading slashes in subpath
    while(subpath[0] == '/') subpath++;

    // if the last character in path is / and (there is a subpath or path is now empty)
    // keep the trailing slash in path and remove the additional slash
    char *slash = "/";
    if(path[len] == '/' && (*subpath || len == 0)) {
        slash = "";
        len++;
    }
    else if(!*subpath) {
        // there is no subpath
        // no need for trailing slash
        slash = "";
    }

    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%.*s%s%s", (int)len, path, slash, subpath);
    return strdupz(buffer);
}

int path_is_dir(const char *path, const char *subpath) {
    char *s = strdupz_path_subpath(path, subpath);

    size_t max_links = 100;

    int is_dir = 0;
    struct stat statbuf;
    while(max_links-- && stat(s, &statbuf) == 0) {
        if((statbuf.st_mode & S_IFMT) == S_IFDIR) {
            is_dir = 1;
            break;
        }
        else if((statbuf.st_mode & S_IFMT) == S_IFLNK) {
            char buffer[FILENAME_MAX + 1];
            ssize_t l = readlink(s, buffer, FILENAME_MAX);
            if(l > 0) {
                buffer[l] = '\0';
                freez(s);
                s = strdupz(buffer);
                continue;
            }
            else {
                is_dir = 0;
                break;
            }
        }
        else {
            is_dir = 0;
            break;
        }
    }

    freez(s);
    return is_dir;
}

int path_is_file(const char *path, const char *subpath) {
    char *s = strdupz_path_subpath(path, subpath);

    size_t max_links = 100;

    int is_file = 0;
    struct stat statbuf;
    while(max_links-- && stat(s, &statbuf) == 0) {
        if((statbuf.st_mode & S_IFMT) == S_IFREG) {
            is_file = 1;
            break;
        }
        else if((statbuf.st_mode & S_IFMT) == S_IFLNK) {
            char buffer[FILENAME_MAX + 1];
            ssize_t l = readlink(s, buffer, FILENAME_MAX);
            if(l > 0) {
                buffer[l] = '\0';
                freez(s);
                s = strdupz(buffer);
                continue;
            }
            else {
                is_file = 0;
                break;
            }
        }
        else {
            is_file = 0;
            break;
        }
    }

    freez(s);
    return is_file;
}

void recursive_config_double_dir_load(const char *user_path, const char *stock_path, const char *subpath, int (*callback)(const char *filename, void *data), void *data, size_t depth) {
    if(depth > 3) {
        error("CONFIG: Max directory depth reached while reading user path '%s', stock path '%s', subpath '%s'", user_path, stock_path, subpath);
        return;
    }

    char *udir = strdupz_path_subpath(user_path, subpath);
    char *sdir = strdupz_path_subpath(stock_path, subpath);

    debug(D_HEALTH, "CONFIG traversing user-config directory '%s', stock config directory '%s'", udir, sdir);

    DIR *dir = opendir(udir);
    if (!dir) {
        error("CONFIG cannot open user-config directory '%s'.", udir);
    }
    else {
        struct dirent *de = NULL;
        while((de = readdir(dir))) {
            if(de->d_type == DT_DIR || de->d_type == DT_LNK) {
                if( !de->d_name[0] ||
                    (de->d_name[0] == '.' && de->d_name[1] == '\0') ||
                    (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                        ) {
                    debug(D_HEALTH, "CONFIG ignoring user-config directory '%s/%s'", udir, de->d_name);
                    continue;
                }

                if(path_is_dir(udir, de->d_name)) {
                    recursive_config_double_dir_load(udir, sdir, de->d_name, callback, data, depth + 1);
                    continue;
                }
            }

            if(de->d_type == DT_UNKNOWN || de->d_type == DT_REG || de->d_type == DT_LNK) {
                size_t len = strlen(de->d_name);
                if(path_is_file(udir, de->d_name) &&
                   len > 5 && !strcmp(&de->d_name[len - 5], ".conf")) {
                    char *filename = strdupz_path_subpath(udir, de->d_name);
                    debug(D_HEALTH, "CONFIG calling callback for user file '%s'", filename);
                    callback(filename, data);
                    freez(filename);
                    continue;
                }
            }

            debug(D_HEALTH, "CONFIG ignoring user-config file '%s/%s' of type %d", udir, de->d_name, (int)de->d_type);
        }

        closedir(dir);
    }

    debug(D_HEALTH, "CONFIG traversing stock config directory '%s', user config directory '%s'", sdir, udir);

    dir = opendir(sdir);
    if (!dir) {
        error("CONFIG cannot open stock config directory '%s'.", sdir);
    }
    else {
        if (strcmp(udir, sdir)) {
            struct dirent *de = NULL;
            while((de = readdir(dir))) {
                if(de->d_type == DT_DIR || de->d_type == DT_LNK) {
                    if( !de->d_name[0] ||
                        (de->d_name[0] == '.' && de->d_name[1] == '\0') ||
                        (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                        ) {
                        debug(D_HEALTH, "CONFIG ignoring stock config directory '%s/%s'", sdir, de->d_name);
                        continue;
                    }

                    if(path_is_dir(sdir, de->d_name)) {
                        // we recurse in stock subdirectory, only when there is no corresponding
                        // user subdirectory - to avoid reading the files twice

                        if(!path_is_dir(udir, de->d_name))
                            recursive_config_double_dir_load(udir, sdir, de->d_name, callback, data, depth + 1);

                        continue;
                    }
                }

                if(de->d_type == DT_UNKNOWN || de->d_type == DT_REG || de->d_type == DT_LNK) {
                    size_t len = strlen(de->d_name);
                    if(path_is_file(sdir, de->d_name) && !path_is_file(udir, de->d_name) &&
                        len > 5 && !strcmp(&de->d_name[len - 5], ".conf")) {
                        char *filename = strdupz_path_subpath(sdir, de->d_name);
                        debug(D_HEALTH, "CONFIG calling callback for stock file '%s'", filename);
                        callback(filename, data);
                        freez(filename);
                        continue;
                    }

                }

                debug(D_HEALTH, "CONFIG ignoring stock-config file '%s/%s' of type %d", udir, de->d_name, (int)de->d_type);
            }
        }
        closedir(dir);
    }

    debug(D_HEALTH, "CONFIG done traversing user-config directory '%s', stock config directory '%s'", udir, sdir);

    freez(udir);
    freez(sdir);
}

// Returns the number of bytes read from the file if file_size is not NULL.
// The actual buffer has an extra byte set to zero (not included in the count).
char *read_by_filename(char *filename, long *file_size)
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
    if (!contents) {
        fclose(f);
        return NULL;
    }
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

inline int pluginsd_space(char c) {
    switch(c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case '=':
            return 1;

        default:
            return 0;
    }
}

inline int config_isspace(char c)
{
    switch (c) {
        case ' ':
        case '\t':
        case '\r':
        case '\n':
        case ',':
            return 1;

        default:
            return 0;
    }
}

// split a text into words, respecting quotes
inline size_t quoted_strings_splitter(char *str, char **words, size_t max_words, int (*custom_isspace)(char), char *recover_input, char **recover_location, int max_recover)
{
    char *s = str, quote = 0;
    size_t i = 0;
    int rec = 0;
    char *recover = recover_input;

    // skip all white space
    while (unlikely(custom_isspace(*s)))
        s++;

    // check for quote
    if (unlikely(*s == '\'' || *s == '"')) {
        quote = *s; // remember the quote
        s++;        // skip the quote
    }

    // store the first word
    words[i++] = s;

    // while we have something
    while (likely(*s)) {
        // if it is escape
        if (unlikely(*s == '\\' && s[1])) {
            s += 2;
            continue;
        }

        // if it is quote
        else if (unlikely(*s == quote)) {
            quote = 0;
            if (recover && rec < max_recover) {
                recover_location[rec++] = s;
                *recover++ = *s;
            }
            *s = ' ';
            continue;
        }

        // if it is a space
        else if (unlikely(quote == 0 && custom_isspace(*s))) {
            // terminate the word
            if (recover && rec < max_recover) {
                if (!rec || recover_location[rec-1] != s) {
                    recover_location[rec++] = s;
                    *recover++ = *s;
                }
            }
            *s++ = '\0';

            // skip all white space
            while (likely(custom_isspace(*s)))
                s++;

            // check for quote
            if (unlikely(*s == '\'' || *s == '"')) {
                quote = *s; // remember the quote
                s++;        // skip the quote
            }

            // if we reached the end, stop
            if (unlikely(!*s))
                break;

            // store the next word
            if (likely(i < max_words))
                words[i++] = s;
            else
                break;
        }

        // anything else
        else
            s++;
    }

    if (i < max_words)
        words[i] = NULL;

    return i;
}

inline size_t pluginsd_split_words(char *str, char **words, size_t max_words, char *recover_input, char **recover_location, int max_recover)
{
    return quoted_strings_splitter(str, words, max_words, pluginsd_space, recover_input, recover_location, max_recover);
}

bool bitmap256_get_bit(BITMAP256 *ptr, uint8_t idx) {
    if (unlikely(!ptr))
        return false;
    return (ptr->data[idx / 64] & (1ULL << (idx % 64)));
}

void bitmap256_set_bit(BITMAP256 *ptr, uint8_t idx, bool value) {
    if (unlikely(!ptr))
        return;
    if (likely(value))
        ptr->data[idx / 64] |= (1ULL << (idx % 64));
    else
        ptr->data[idx / 64] &= ~(1ULL << (idx % 64));
}

bool run_command_and_copy_output_to_stdout(const char *command, int max_line_length) {
    pid_t pid;
    FILE *fp = netdata_popen(command, &pid, NULL);

    if(fp) {
        char buffer[max_line_length + 1];
        while (fgets(buffer, max_line_length, fp))
            fprintf(stdout, "%s", buffer);
    }
    else {
        error("Failed to execute command '%s'.", command);
        return false;
    }

    netdata_pclose(NULL, fp, pid);
    return true;
}
