#include "onewayalloc.h"

#define OWA_MIN_PAGE_SIZE (64 * 1024)
#define OWA_MAX_PAGE_SIZE (1024 * 1024)

typedef struct owa_page {
    size_t stats_pages;
    size_t stats_pages_size;
    size_t stats_mallocs_made;
    size_t stats_mallocs_size;
    size_t size;                // the total size of the page
    size_t offset;              // the first free byte of the page
    bool mmap;
    struct owa_page *prev;      // previous page; the head caches the tail here
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

    // Targets include the header and do not inherit the size of oversized pages.
    size_t size = OWA_MIN_PAGE_SIZE;
    for (size_t i = 0; head && i < head->stats_pages && size < OWA_MAX_PAGE_SIZE; i++)
        size *= 2;

    size_hint = onewayalloc_add_or_fatal(onewayalloc_natural_alignment_or_fatal(size_hint),
                                       natural_alignment(sizeof(OWA_PAGE)), "page");

    // prefer the user size if it is bigger than our size
    if(size_hint > size)
        size = size_hint;

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
    page->prev = page->next = page->current = NULL;

    if(!head) {
        // this is the first time we are called
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(head, page, prev, next);
        head->stats_pages = 0;
        head->stats_pages_size = 0;
        head->stats_mallocs_made = 0;
        head->stats_mallocs_size = 0;
    }
    else {
        OWA_PAGE *current = head->current;
        DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(head, current, page, prev, next);
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
        OWA_PAGE *current = page;
        size_t header = natural_alignment(sizeof(OWA_PAGE));

        // Keep skipped pages unused: only the selected page enters the used prefix.
        for (page = current->next; page; page = page->next) {
            if (page->size - header >= size)
                break;
        }

        if (page) {
            if (page != current->next) {
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(head, page, prev, next);
                DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(head, current, page, prev, next);
            }
            page->offset = header;
            head->current = page;
        }
        else
            page = onewayalloc_create_internal(head, size);
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

#ifndef FSANITIZE_ADDRESS
static int onewayalloc_list_unittest(OWA_PAGE *head) {
    size_t pages = 0, total = 0;
    bool found_current = false;
    OWA_PAGE *previous = NULL, *page = head;
    for (; page && pages < head->stats_pages; page = page->next) {
        if (previous && page->prev != previous)
            break;
        found_current = found_current || page == head->current;
        total += page->size;
        pages++;
        previous = page;
    }
    if (page || pages != head->stats_pages || total != head->stats_pages_size ||
        head->prev != previous || !found_current) {
        fprintf(stderr, "OWA: page list or accounting is inconsistent\n");
        return 1;
    }
    return 0;
}

static int onewayalloc_reuse_unittest(void) {
    size_t header = natural_alignment(sizeof(OWA_PAGE));
    OWA_PAGE *head = onewayalloc_create(0);
    OWA_PAGE *small = onewayalloc_create_internal(head, 0);
    OWA_PAGE *medium = onewayalloc_create_internal(head, 0);
    OWA_PAGE *large = onewayalloc_create_internal(head, 0);
    size_t retained = onewayalloc_allocated_memory();
    int errors = 0;

    onewayalloc_reset(head);
    unsigned char *large_ptr = onewayalloc_mallocz(head, large->size - header);
    errors += onewayalloc_list_unittest(head);
    memset(large_ptr, 0xa5, large->size - header);
    if (large_ptr != (unsigned char *)large + header || head->next != large ||
        large->next != small || small->next != medium || medium->next) {
        fprintf(stderr, "OWA: selection did not move only the fitting page\n");
        errors++;
    }

    void *small_ptr = onewayalloc_mallocz(head, small->size - header);
    errors += onewayalloc_list_unittest(head);
    void *medium_ptr = onewayalloc_mallocz(head, medium->size - header);
    errors += onewayalloc_list_unittest(head);
    if (small_ptr != (char *)small + header || medium_ptr != (char *)medium + header ||
        onewayalloc_allocated_memory() != retained) {
        fprintf(stderr, "OWA: skipped exact-fit pages were not reused\n");
        errors++;
    }
    for (size_t i = 0; i < large->size - header; i++) {
        if (large_ptr[i] != 0xa5) {
            fprintf(stderr, "OWA: page selection overwrote a live buffer\n");
            errors++;
            break;
        }
    }

    onewayalloc_reset(head);
    OWA_PAGE *unused = head->next;
    onewayalloc_mallocz(head, 2 * 1024 * 1024);
    OWA_PAGE *added = head->current;
    errors += onewayalloc_list_unittest(head);
    if (head->next != added || added->next != unused) {
        fprintf(stderr, "OWA: new page did not preserve the unused suffix\n");
        errors++;
    }
    if (added->size != onewayalloc_page_alignment_or_fatal(2 * 1024 * 1024 + header, os_get_system_page_size())) {
        fprintf(stderr, "OWA: oversized allocation was not sized exactly\n");
        errors++;
    }
    retained = onewayalloc_allocated_memory();
    if (onewayalloc_mallocz(head, unused->size - header) != (char *)unused + header ||
        onewayalloc_allocated_memory() != retained) {
        fprintf(stderr, "OWA: unused suffix was lost after a failed search\n");
        errors++;
    }

    errors += onewayalloc_list_unittest(head);

    onewayalloc_destroy(head);
    return errors;
}

static int onewayalloc_growth_unittest(void) {
    size_t header = natural_alignment(sizeof(OWA_PAGE));
    size_t os_page = os_get_system_page_size();
    OWA_PAGE *head = onewayalloc_create(0);
    int errors = 0;

    for (size_t i = 0; i < 8; i++) {
        size_t expected = onewayalloc_page_alignment_or_fatal((size_t)65536 << (i < 4 ? i : 4), os_page);
        OWA_PAGE *page = head->current;
        errors += onewayalloc_list_unittest(head);
        if (page->size != expected) {
            fprintf(stderr, "OWA: page %zu size %zu differs from target %zu\n", i, page->size, expected);
            errors++;
        }
        onewayalloc_mallocz(head, page->size - page->offset);
        if (i < 7)
            onewayalloc_mallocz(head, 1);
    }
    onewayalloc_destroy(head);

    const size_t requests[] = {
        65536 - header, 65536 - header + 1,
        1024 * 1024 - header, 1024 * 1024 - header + 1,
        1024 * 1024, 2 * 1024 * 1024 + 17,
    };
    for (size_t i = 0; i < sizeof(requests) / sizeof(requests[0]); i++) {
        size_t expected = onewayalloc_page_alignment_or_fatal(natural_alignment(requests[i]) + header, os_page);
        head = onewayalloc_create(requests[i]);
        errors += onewayalloc_list_unittest(head);
        if (head->size != expected) {
            fprintf(stderr, "OWA: size hint %zu produced %zu instead of %zu\n", requests[i], head->size, expected);
            errors++;
        }
        onewayalloc_mallocz(head, head->size - header);
        onewayalloc_mallocz(head, 1);
        expected = onewayalloc_page_alignment_or_fatal(128 * 1024, os_page);
        if (head->current->size != expected) {
            fprintf(stderr, "OWA: size hint inflated the next ordinary page\n");
            errors++;
        }
        onewayalloc_destroy(head);
    }
    return errors;
}
#endif

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

#ifndef FSANITIZE_ADDRESS
    errors += onewayalloc_reuse_unittest();
    errors += onewayalloc_growth_unittest();
#endif

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
                errors += onewayalloc_list_unittest(owa);
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
