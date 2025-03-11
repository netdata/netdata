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
    struct owa_page *last;      // the last page on the list - we currently allocate on this
} OWA_PAGE;

static size_t onewayalloc_total_memory = 0;

size_t onewayalloc_allocated_memory(void) {
    return __atomic_load_n(&onewayalloc_total_memory, __ATOMIC_RELAXED);
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
    size_hint += natural_alignment(sizeof(OWA_PAGE));

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
    if(size % OWA_NATURAL_PAGE_SIZE)
        size = size + OWA_NATURAL_PAGE_SIZE - (size % OWA_NATURAL_PAGE_SIZE);

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
    page->next = page->last = NULL;

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
        head->last->next = page;
    }

    head->last = page;
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
    OWA_PAGE *page = head->last;

    // update stats
    head->stats_mallocs_made++;
    head->stats_mallocs_size += size;

    // make sure the size is aligned
    size = natural_alignment(size);

    if(unlikely(page->size - page->offset < size)) {
        // we don't have enough space to fit the data
        // let's get another page
        page = onewayalloc_create_internal(head, (size > page->size)?size:page->size);
    }

    char *mem = (char *)page;
    mem = &mem[page->offset];
    page->offset += size;

    return (void *)mem;
}

void *onewayalloc_callocz(ONEWAYALLOC *owa, size_t nmemb, size_t size) {
    size_t total = nmemb * size;
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
    size_t newsize = oldsize * 2;
    void *dst = onewayalloc_mallocz(owa, newsize);
    memcpy(dst, src, oldsize);
    onewayalloc_freez(owa, src);
    return dst;
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
