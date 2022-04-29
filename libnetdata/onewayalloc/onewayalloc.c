#include "onewayalloc.h"

static size_t PAGE_SIZE = 0;

typedef struct owa_page {
    size_t pages;
    size_t size;        // the total size of the page
    size_t offset;      // the first free byte of the page
    struct owa_page *next;     // the next page on the list
    struct owa_page *last;     // the last page on the list - we currently allocate on this
} OWA_PAGE;

static inline size_t alignment(size_t size) {
    size_t wanted = sizeof(size_t);

    if(unlikely(size % wanted))
        size = size + wanted - (size % wanted);

    return size;
}

ONEWAYALLOC *onewayalloc_create(size_t size_hint) {
    if(unlikely(!PAGE_SIZE)) PAGE_SIZE = sysconf(_SC_PAGE_SIZE);

    size_t size = PAGE_SIZE;
    if(size_hint > size) size = ((PAGE_SIZE / size_hint) + 1) * PAGE_SIZE;

    void *mem = netdata_mmap(NULL, size, MAP_ANONYMOUS|MAP_PRIVATE, 0);
    if(!mem) fatal("Cannot allocate onewayalloc buffer of size %zu", size);

    OWA_PAGE *page = (OWA_PAGE *)mem;
    page->size = size;
    page->offset = alignment(sizeof(OWA_PAGE));
    page->next = NULL;
    page->pages = 1;
    page->last = page;

    return (ONEWAYALLOC *)page;
}

void *onewayalloc_mallocz(ONEWAYALLOC *owa, size_t size) {
    OWA_PAGE *head = (OWA_PAGE *)owa;
    OWA_PAGE *page = head->last;

    // make sure the size is aligned
    size = alignment(size);

    if(unlikely(page->size - page->offset < size)) {
        // we don't have enough space to fit the data
        // let's get another page

        page->next = onewayalloc_create((size > page->size)?size:page->size);
        page->last = page->next;

        head->last = page->next;
        head->pages++;

        page = head->last;
    }

    char *mem = (char *)page;
    mem = &mem[page->offset];
    page->offset += size;

    return (void *)mem;
}

char *onewayalloc_strdupz(ONEWAYALLOC *owa, const char *s) {
    char *d = onewayalloc_mallocz((OWA_PAGE *)owa, strlen(s) + 1);
    strcpy(d, s);
    return d;
}

void *onewayalloc_memdupz(ONEWAYALLOC *owa, const void *src, size_t size) {
    void *mem = onewayalloc_mallocz((OWA_PAGE *)owa, size);
    memcpy(mem, src, size);
    return mem;
}

void onewayalloc_destroy(ONEWAYALLOC *ptr) {
    OWA_PAGE *page = (OWA_PAGE *)ptr;
    if(page->next)
        onewayalloc_destroy((ONEWAYALLOC *)page->next);

    munmap(page, page->size);
}


void onewayalloc_unittest(void) {
    ONEWAYALLOC *owa = onewayalloc_create(0);

    // strdupz
    char *buffer, *s, *mem = (char *)owa;
    int i, size = 200;

    buffer = mallocz(size);
    for(i = 0; i < size ;i++) buffer[i] = 'A' + (i % 26);
    s = onewayalloc_strdupz(owa, buffer);

    if(s - mem != sizeof(OWA_PAGE))
        printf("allocation is not in place mem=0x%08x, buffer=0x%08X, delta = %zu, expected %zu\n", mem, s, (size_t)s - (size_t)mem, sizeof(OWA_PAGE));

    for(i = 0; i < size ;i++) if(s[i] != buffer[i]) printf("onewayalloc_strdupz() check failed\n");


}