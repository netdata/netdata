#include "adaptive_resortable_list.h"

// the default processor() of the ARL
// can be overwritten at arl_create()
inline void arl_callback_str2ull(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name;
    (void)hash;

    register unsigned long long *d = dst;
    *d = str2ull(value);
    // fprintf(stderr, "name '%s' with hash %u and value '%s' is %llu\n", name, hash, value, *d);
}

inline void arl_callback_str2kernel_uint_t(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name;
    (void)hash;

    register kernel_uint_t *d = dst;
    *d = str2kernel_uint_t(value);
    // fprintf(stderr, "name '%s' with hash %u and value '%s' is %llu\n", name, hash, value, (unsigned long long)*d);
}

// create a new ARL
ARL_BASE *arl_create(const char *name, void (*processor)(const char *, uint32_t, const char *, void *), size_t rechecks) {
    ARL_BASE *base = callocz(1, sizeof(ARL_BASE));

    base->name = strdupz(name);

    if(!processor)
        base->processor = arl_callback_str2ull;
    else
        base->processor = processor;

    base->rechecks = rechecks;

    return base;
}

void arl_free(ARL_BASE *arl_base) {
    if(unlikely(!arl_base))
        return;

    while(arl_base->head) {
        ARL_ENTRY *e = arl_base->head;
        arl_base->head = e->next;

        freez(e->name);
#ifdef NETDATA_INTERNAL_CHECKS
        memset(e, 0, sizeof(ARL_ENTRY));
#endif
        freez(e);
    }

    freez(arl_base->name);

#ifdef NETDATA_INTERNAL_CHECKS
    memset(arl_base, 0, sizeof(ARL_BASE));
#endif

    freez(arl_base);
}

void arl_begin(ARL_BASE *base) {

#ifdef NETDATA_INTERNAL_CHECKS
    if(likely(base->iteration > 10)) {
        // do these checks after the ARL has been sorted

        if(unlikely(base->relinkings > (base->expected + base->allocated)))
            info("ARL '%s' has %zu relinkings with %zu expected and %zu allocated entries. Is the source changing so fast?"
                 , base->name, base->relinkings, base->expected, base->allocated);

        if(unlikely(base->slow > base->fast))
            info("ARL '%s' has %zu fast searches and %zu slow searches. Is the source really changing so fast?"
                 , base->name, base->fast, base->slow);

        /*
        if(unlikely(base->iteration % 60 == 0)) {
            info("ARL '%s' statistics: iteration %zu, expected %zu, wanted %zu, allocated %zu, fred %zu, relinkings %zu, found %zu, added %zu, fast %zu, slow %zu"
                 , base->name
                 , base->iteration
                 , base->expected
                 , base->wanted
                 , base->allocated
                 , base->fred
                 , base->relinkings
                 , base->found
                 , base->added
                 , base->fast
                 , base->slow
            );
            // for(e = base->head; e; e = e->next) fprintf(stderr, "%s ", e->name);
            // fprintf(stderr, "\n");
        }
        */
    }
#endif

    if(unlikely(base->iteration > 0 && (base->added || (base->iteration % base->rechecks) == 0))) {
        int wanted_equals_expected = ((base->iteration % base->rechecks) == 0);

        // fprintf(stderr, "\n\narl_begin() rechecking, added %zu, iteration %zu, rechecks %zu, wanted_equals_expected %d\n\n\n", base->added, base->iteration, base->rechecks, wanted_equals_expected);

        base->added = 0;
        base->wanted = (wanted_equals_expected)?base->expected:0;

        ARL_ENTRY *e = base->head;
        while(e) {
            if(e->flags & ARL_ENTRY_FLAG_FOUND) {

                // remove the found flag
                e->flags &= ~ARL_ENTRY_FLAG_FOUND;

                // count it in wanted
                if(!wanted_equals_expected && e->flags & ARL_ENTRY_FLAG_EXPECTED)
                    base->wanted++;

            }
            else if(e->flags & ARL_ENTRY_FLAG_DYNAMIC && !(base->head == e && !e->next)) { // not last entry
                // we can remove this entry
                // it is not found, and it was created because
                // it was found in the source file

                // remember the next one
                ARL_ENTRY *t = e->next;

                // remove it from the list
                if(e->next) e->next->prev = e->prev;
                if(e->prev) e->prev->next = e->next;
                if(base->head == e) base->head = e->next;

                // free it
                freez(e->name);
                freez(e);

                // count it
                base->fred++;

                // continue
                e = t;
                continue;
            }

            e = e->next;
        }
    }

    if(unlikely(!base->head)) {
        // hm... no nodes at all in the list #1700
        // add a fake one to prevent a crash
        // this is better than checking for the existence of nodes all the time
        arl_expect(base, "a-really-not-existing-source-keyword", NULL);
    }

    base->iteration++;
    base->next_keyword = base->head;
    base->found = 0;

}

// register an expected keyword to the ARL
// together with its destination ( i.e. the output of the processor() )
ARL_ENTRY *arl_expect_custom(ARL_BASE *base, const char *keyword, void (*processor)(const char *name, uint32_t hash, const char *value, void *dst), void *dst) {
    ARL_ENTRY *e = callocz(1, sizeof(ARL_ENTRY));
    e->name = strdupz(keyword);
    e->hash = simple_hash(e->name);
    e->processor = (processor)?processor:base->processor;
    e->dst = dst;
    e->flags = ARL_ENTRY_FLAG_EXPECTED;
    e->prev = NULL;
    e->next = base->head;

    if(base->head) base->head->prev = e;
    else base->next_keyword = e;

    base->head = e;
    base->expected++;
    base->allocated++;

    base->wanted = base->expected;

    return e;
}

int arl_find_or_create_and_relink(ARL_BASE *base, const char *s, const char *value) {
    ARL_ENTRY *e;

    uint32_t hash = simple_hash(s);

    // find if it already exists in the data
    for(e = base->head; e ; e = e->next)
        if(e->hash == hash && !strcmp(e->name, s))
            break;

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(base->next_keyword && e == base->next_keyword))
        fatal("Internal Error: e == base->last");
#endif

    if(e) {
        // found it in the keywords

        base->relinkings++;

        // run the processor for it
        if(unlikely(e->dst)) {
            e->processor(e->name, hash, value, e->dst);
            base->found++;
        }

        // unlink it - we will relink it below
        if(e->next) e->next->prev = e->prev;
        if(e->prev) e->prev->next = e->next;

        // make sure the head is properly linked
        if(base->head == e)
            base->head = e->next;
    }
    else {
        // not found

        // create it
        e = callocz(1, sizeof(ARL_ENTRY));
        e->name = strdupz(s);
        e->hash = hash;
        e->flags = ARL_ENTRY_FLAG_DYNAMIC;

        base->allocated++;
        base->added++;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(base->iteration % 60 == 0 && e->flags & ARL_ENTRY_FLAG_FOUND))
        info("ARL '%s': entry '%s' is already found. Did you forget to call arl_begin()?", base->name, s);
#endif

    e->flags |= ARL_ENTRY_FLAG_FOUND;

    // link it here
    e->next = base->next_keyword;
    if(base->next_keyword) {
        e->prev = base->next_keyword->prev;
        base->next_keyword->prev = e;

        if(e->prev)
            e->prev->next = e;

        if(base->head == base->next_keyword)
            base->head = e;
    }
    else {
        e->prev = NULL;

        if(!base->head)
            base->head = e;
    }

    // prepare the next iteration
    base->next_keyword = e->next;
    if(unlikely(!base->next_keyword))
        base->next_keyword = base->head;

    if(unlikely(base->found == base->wanted)) {
        // fprintf(stderr, "FOUND ALL WANTED 1: found = %zu, wanted = %zu, expected %zu\n", base->found, base->wanted, base->expected);
        return 1;
    }

    return 0;
}
