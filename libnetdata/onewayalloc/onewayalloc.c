#include "onewayalloc.h"

static size_t OWA_NATURAL_PAGE_SIZE = 0;

// https://www.gnu.org/software/libc/manual/html_node/Aligned-Memory-Blocks.html
#define OWA_NATURAL_ALIGNMENT  (sizeof(void *) * 2)

typedef struct owa_page {
    size_t stats_pages;
    size_t stats_pages_size;
    size_t stats_mallocs_made;
    size_t stats_mallocs_size;
    size_t size;        // the total size of the page
    size_t offset;      // the first free byte of the page
    struct owa_page *next;     // the next page on the list
    struct owa_page *last;     // the last page on the list - we currently allocate on this
} OWA_PAGE;

// allocations need to be aligned to CPU register width
// https://en.wikipedia.org/wiki/Data_structure_alignment
static inline size_t natural_alignment(size_t size) {
    if(unlikely(size % OWA_NATURAL_ALIGNMENT))
        size = size + OWA_NATURAL_ALIGNMENT - (size % OWA_NATURAL_ALIGNMENT);

    return size;
}

// Create an OWA
// Once it is created, the called may call the onewayalloc_mallocz()
// any number of times, for any amount of memory.

static OWA_PAGE *onewayalloc_create_internal(OWA_PAGE *head, size_t size_hint) {
    if(unlikely(!OWA_NATURAL_PAGE_SIZE)) {
        long int page_size = sysconf(_SC_PAGE_SIZE);
        if (unlikely(page_size == -1))
            OWA_NATURAL_PAGE_SIZE = 4096;
        else
            OWA_NATURAL_PAGE_SIZE = page_size;
    }

    // our default page size
    size_t size = OWA_NATURAL_PAGE_SIZE;

    // make sure the new page will fit both the requested size
    // and the OWA_PAGE structure at its beginning
    size_hint += natural_alignment(sizeof(OWA_PAGE));

    // prefer the user size if it is bigger than our size
    if(size_hint > size) size = size_hint;

    // try to allocate half of the total we have allocated already
    if(likely(head)) {
        size_t optimal_size = head->stats_pages_size / 2;
        if(optimal_size > size) size = optimal_size;
    }

    // Make sure our allocations are always a multiple of the hardware page size
    if(size % OWA_NATURAL_PAGE_SIZE) size = size + OWA_NATURAL_PAGE_SIZE - (size % OWA_NATURAL_PAGE_SIZE);

    // OWA_PAGE *page = (OWA_PAGE *)netdata_mmap(NULL, size, MAP_ANONYMOUS|MAP_PRIVATE, 0);
    // if(unlikely(!page)) fatal("Cannot allocate onewayalloc buffer of size %zu", size);
    OWA_PAGE *page = (OWA_PAGE *)mallocz(size);

    page->size = size;
    page->offset = natural_alignment(sizeof(OWA_PAGE));
    page->next = page->last = NULL;

    if(unlikely(!head)) {
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

    return (ONEWAYALLOC *)page;
}

ONEWAYALLOC *onewayalloc_create(size_t size_hint) {
    return onewayalloc_create_internal(NULL, size_hint);
}

void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size) {
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
#ifdef NETDATA_INTERNAL_CHECKS
    // allow the caller to call us for a mallocz() allocation
    // so try to find it in our memory and if it is not there
    // log an error

    if (unlikely(!ptr))
        return;

    OWA_PAGE *head = (OWA_PAGE *)owa;
    OWA_PAGE *page;
    size_t seeking = (size_t)ptr;

    for(page = head; page ;page = page->next) {
        size_t start = (size_t)page;
        size_t end = start + page->size;

        if(seeking >= start && seeking <= end) {
            // found it - it is ours
            // just return to let the caller think we actually did something
            return;
        }
    }

    // not found - it is not ours
    // let's free it with the system allocator
    error("ONEWAYALLOC: request to free address 0x%p that is not allocated by this OWA", ptr);
#endif

    return;
}

void onewayalloc_destroy(ONEWAYALLOC *owa) {
    if(!owa) return;

    OWA_PAGE *head = (OWA_PAGE *)owa;

    //info("OWA: %zu allocations of %zu total bytes, in %zu pages of %zu total bytes",
    //     head->stats_mallocs_made, head->stats_mallocs_size,
    //     head->stats_pages, head->stats_pages_size);

    OWA_PAGE *page = head;
    while(page) {
        OWA_PAGE *p = page;
        page = page->next;
        // munmap(p, p->size);
        freez(p);
    }
}
