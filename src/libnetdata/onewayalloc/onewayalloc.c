#include "onewayalloc.h"

typedef struct owa_page {
    size_t stats_pages;
    size_t stats_pages_size;
    size_t stats_mallocs_made;
    size_t stats_mallocs_size;
    size_t size;                // the total size of the page
    size_t offset;              // the first free byte of the page
    bool mmap;
    struct owa_page *next;      // the next page on the list
    struct owa_page *current;   // the allocation cursor (used only on the head page)
} OWA_PAGE;

static size_t onewayalloc_total_memory = 0;

size_t onewayalloc_allocated_memory(void) {
    return __atomic_load_n(&onewayalloc_total_memory, __ATOMIC_RELAXED);
}

static size_t onewayalloc_add_or_fatal(size_t size, size_t add, const char *context) {
    if(unlikely(add > SIZE_MAX - size))
        fatal("ONEWAYALLOC: cannot allocate %s size %zu with %zu additional bytes.", context, size, add);

    return size + add;
}

size_t onewayalloc_mul_or_fatal(size_t nmemb, size_t size, const char *context) {
    if(unlikely(size && nmemb > SIZE_MAX / size))
        fatal("ONEWAYALLOC: cannot allocate %s size %zu * %zu.", context, nmemb, size);

    return nmemb * size;
}

size_t onewayalloc_mul3_or_fatal(size_t nmemb1, size_t nmemb2, size_t size, const char *context) {
    size_t nmemb = onewayalloc_mul_or_fatal(nmemb1, nmemb2, context);
    return onewayalloc_mul_or_fatal(nmemb, size, context);
}

static size_t onewayalloc_natural_alignment_or_fatal(size_t size) {
    if(unlikely(size > SIZE_MAX - (SYSTEM_REQUIRED_ALIGNMENT - 1)))
        fatal("ONEWAYALLOC: cannot naturally align allocation size %zu.", size);

    return natural_alignment(size);
}

static size_t onewayalloc_page_alignment_or_fatal(size_t size, size_t page_size) {
    size_t remainder = size % page_size;

    if(remainder)
        size = onewayalloc_add_or_fatal(size, page_size - remainder, "page aligned");

    return size;
}

// Create an OWA
// Once it is created, the caller may call the onewayalloc_mallocz()
// any number of times, for any amount of memory.

static OWA_PAGE *onewayalloc_create_internal(OWA_PAGE *head, size_t size_hint) {
    size_t OWA_NATURAL_PAGE_SIZE = os_get_system_page_size();

    // our default page size
    size_t size = 32768;

    // make sure the new page will fit both the requested size
    // and the OWA_PAGE structure at its beginning
    size_hint = onewayalloc_add_or_fatal(size_hint, natural_alignment(sizeof(OWA_PAGE)), "page");

    // prefer the user size if it is bigger than our size
    if(size_hint > size)
        size = size_hint;

    if(head) {
        // double the current allocation
        size_t optimal_size = head->stats_pages_size;

        // cap it at 1 MiB
        if(optimal_size > 1ULL * 1024 * 1024)
            optimal_size = 1ULL * 1024 * 1024;

        // use the optimal if it is more than the required size
        if(optimal_size > size)
            size = optimal_size;
    }

    // Make sure our allocations are always a multiple of the hardware page size
    size = onewayalloc_page_alignment_or_fatal(size, OWA_NATURAL_PAGE_SIZE);

    // Use netdata_mmap instead of mallocz
    OWA_PAGE *page = (OWA_PAGE *)nd_mmap_advanced(NULL, size, MAP_ANONYMOUS | MAP_PRIVATE, 0, false, false, NULL);
    if(unlikely(!page)) {
        page = mallocz(size);
        page->mmap = false;
    }
    else
        page->mmap = true;

    __atomic_add_fetch(&onewayalloc_total_memory, size, __ATOMIC_RELAXED);

    page->size = size;
    page->offset = natural_alignment(sizeof(OWA_PAGE));
    page->next = page->current = NULL;

    if(!head) {
        // this is the first time we are called
        head = page;
        head->stats_pages = 0;
        head->stats_pages_size = 0;
        head->stats_mallocs_made = 0;
        head->stats_mallocs_size = 0;
    }
    else {
        // link this page into our existing linked list
        head->current->next = page;
    }

    head->current = page;
    head->stats_pages++;
    head->stats_pages_size += size;

    return page;
}

ONEWAYALLOC *onewayalloc_create(size_t size_hint) {
    return (ONEWAYALLOC *)onewayalloc_create_internal(NULL, size_hint);
}

void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size) {
#ifdef FSANITIZE_ADDRESS
    return mallocz(size);
#endif

    OWA_PAGE *head = (OWA_PAGE *)owa;
    OWA_PAGE *page = head->current;

    // update stats
    head->stats_mallocs_made++;
    head->stats_mallocs_size += size;

    // make sure the size is aligned
    size = onewayalloc_natural_alignment_or_fatal(size);

    if(unlikely(page->size - page->offset < size)) {
        // Pages after the cursor have not been used since reset. Rewind lazily.
        while(page->next) {
            page = page->next;
            page->offset = natural_alignment(sizeof(OWA_PAGE));
            if(page->size - page->offset >= size)
                break;
        }
        head->current = page;

        if(page->size - page->offset < size)
            page = onewayalloc_create_internal(head, (size > page->size)?size:page->size);
    }

    char *mem = (char *)page;
    mem = &mem[page->offset];
    page->offset += size;

    return (void *)mem;
}

void *onewayalloc_callocz(ONEWAYALLOC *owa, size_t nmemb, size_t size) {
    size_t total = onewayalloc_mul_or_fatal(nmemb, size, "calloc");
    void *mem = onewayalloc_mallocz(owa, total);
    memset(mem, 0, total);
    return mem;
}

char *onewayalloc_strdupz(ONEWAYALLOC *owa, const char *s) {
    size_t size = strlen(s) + 1;
    char *d = onewayalloc_mallocz((OWA_PAGE *)owa, size);
    memcpy(d, s, size);
    return d;
}

void *onewayalloc_memdupz(ONEWAYALLOC *owa, const void *src, size_t size) {
    void *mem = onewayalloc_mallocz((OWA_PAGE *)owa, size);
    // memcpy() is way faster than strcpy() since it does not check for '\0'
    memcpy(mem, src, size);
    return mem;
}

void onewayalloc_freez(ONEWAYALLOC *owa __maybe_unused, const void *ptr __maybe_unused) {
#ifdef FSANITIZE_ADDRESS
    freez((void *)ptr);
    return;
#endif

#ifdef NETDATA_INTERNAL_CHECKS
    // allow the caller to call us for a mallocz() allocation
    // so try to find it in our memory and if it is not there
    // log an error

    if (unlikely(!ptr))
        return;

    OWA_PAGE *head = (OWA_PAGE *)owa;
    OWA_PAGE *page;
    uintptr_t seeking = (uintptr_t)ptr;

    for(page = head; page ;page = page->next) {
        uintptr_t start = (uintptr_t)page;
        uintptr_t end = start + page->size;

        if(seeking >= start && seeking <= end) {
            // found it - it is ours
            // just return to let the caller think we actually did something
            return;
        }
    }

    // not found - it is not ours
    // let's free it with the system allocator
    netdata_log_error("ONEWAYALLOC: request to free address 0x%p that is not allocated by this OWA", ptr);
#endif
}

void *onewayalloc_doublesize(ONEWAYALLOC *owa, const void *src, size_t oldsize) {
    size_t newsize = onewayalloc_mul_or_fatal(oldsize, 2, "doubled");
    void *dst = onewayalloc_mallocz(owa, newsize);
    memcpy(dst, src, oldsize);
    onewayalloc_freez(owa, src);
    return dst;
}

void onewayalloc_reset(ONEWAYALLOC *owa) {
    if (!owa) return;

#ifdef FSANITIZE_ADDRESS
    // Under the sanitizer path, onewayalloc_mallocz goes straight to the
    // system allocator and nothing is tracked in the owa page list — there
    // is nothing to reset. Individual allocations are released by callers
    // via onewayalloc_freez() (which calls freez() under the sanitizer).
    return;
#endif

    OWA_PAGE *head = (OWA_PAGE *)owa;

    // Keep all pages until destroy; allocation rewinds the unused suffix on demand.
    head->current = head;
    head->offset = natural_alignment(sizeof(OWA_PAGE));
}

void onewayalloc_destroy(ONEWAYALLOC *owa) {
    if(!owa) return;

    OWA_PAGE *head = (OWA_PAGE *)owa;

    //netdata_log_info("OWA: %zu allocations of %zu total bytes, in %zu pages of %zu total bytes",
    //     head->stats_mallocs_made, head->stats_mallocs_size,
    //     head->stats_pages, head->stats_pages_size);

    size_t total_size = 0;
    OWA_PAGE *page = head;
    while(page) {
        total_size += page->size;

        OWA_PAGE *p = page;
        page = page->next;

        // Use netdata_munmap instead of freez
        if(p->mmap)
            nd_munmap(p, p->size);
        else
            freez(p);
    }

    __atomic_sub_fetch(&onewayalloc_total_memory, total_size, __ATOMIC_RELAXED);
}

int onewayalloc_unittest(void) {
    const size_t cases[][9] = {
        {0, 1, 15, 32768, 65537, 4000, 2 * 1024 * 1024, 1, 100000},
        {2 * 1024 * 1024, 65537, 32768, 1, 17, 3, 4000, 0, 100000},
        {1, 3, 17, 0, 31, 64, 7, 9, 1},
        {32768, 65537, 4 * 1024 * 1024, 17, 4000, 1, 0, 3, 100000},
    };
    size_t initial_memory = onewayalloc_allocated_memory();
    ONEWAYALLOC *owa = onewayalloc_create(0);
    int errors = 0;

    onewayalloc_reset(NULL);
    for (size_t c = 0; c < sizeof(cases) / sizeof(cases[0]); c++) {
#ifndef FSANITIZE_ADDRESS
        uintptr_t first_addresses[9];
        size_t first_memory = 0;
#endif
        for (size_t repeat = 0; repeat < 8; repeat++) {
            size_t before_reset = onewayalloc_allocated_memory();
            onewayalloc_reset(owa);
            onewayalloc_reset(owa);
            if (onewayalloc_allocated_memory() != before_reset) {
                fprintf(stderr, "OWA: reset did not retain allocated pages\n");
                errors++;
                goto cleanup;
            }

            unsigned char *ptrs[9];
            for (size_t i = 0; i < 9; i++) {
                size_t size = cases[c][i];
                ptrs[i] = repeat % 2 ? onewayalloc_callocz(owa, size, 1) : onewayalloc_mallocz(owa, size);
                if ((uintptr_t)ptrs[i] % SYSTEM_REQUIRED_ALIGNMENT) {
                    fprintf(stderr, "OWA: allocation is not naturally aligned\n");
                    errors++;
                }
                if (repeat % 2) {
                    for (size_t j = 0; j < size; j++) {
                        if (ptrs[i][j] != 0) {
                            fprintf(stderr, "OWA: calloc did not clear reused memory\n");
                            errors++;
                            break;
                        }
                    }
                }
                memset(ptrs[i], (int)i + 1, size);
#ifndef FSANITIZE_ADDRESS
                if (!repeat)
                    first_addresses[i] = (uintptr_t)ptrs[i];
                else if (first_addresses[i] != (uintptr_t)ptrs[i]) {
                    fprintf(stderr, "OWA: repeated allocation did not reuse its address\n");
                    errors++;
                }
#endif
            }

            // Check all live buffers after allocation to detect overlapping bump cursors.
            for (size_t i = 0; i < 9; i++) {
                for (size_t j = 0; j < cases[c][i]; j++) {
                    if (ptrs[i][j] != i + 1) {
                        fprintf(stderr, "OWA: live allocation was overwritten\n");
                        errors++;
                        break;
                    }
                }
                onewayalloc_freez(owa, ptrs[i]);
            }
#ifndef FSANITIZE_ADDRESS
            if (!repeat)
                first_memory = onewayalloc_allocated_memory();
            else if (first_memory != onewayalloc_allocated_memory()) {
                fprintf(stderr, "OWA: repeated allocation sequence grew the arena\n");
                errors++;
            }
#endif
        }
    }

cleanup:
    onewayalloc_destroy(owa);
    if (onewayalloc_allocated_memory() != initial_memory) {
        fprintf(stderr, "OWA: destroy did not release all pages\n");
        errors++;
    }
    fprintf(stderr, "OWA tests: %s\n", errors ? "FAILED" : "PASSED");
    return errors ? 1 : 0;
}
