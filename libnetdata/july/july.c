// SPDX-License-Identifier: GPL-3.0-or-later

#include "july.h"

#define JULYL_MIN_ENTRIES 10

struct JulyL_item {
    Word_t index;
    void *value;
};

struct JulyL {
    size_t entries;
    size_t used;

    // statistics
    size_t bytes;
    size_t bytes_moved;
    size_t reallocs;

    struct {
        struct JulyL *prev;
        struct JulyL *next;
    } cache;

    struct JulyL_item array[];
};

// ----------------------------------------------------------------------------
// JulyL cache

static struct {
    struct {
        SPINLOCK spinlock;
        struct JulyL *available_items;
        size_t available;
    } protected;

    struct {
        size_t bytes;
        size_t allocated;
        size_t bytes_moved;
        size_t reallocs;
    } atomics;
} julyl_globals = {
        .protected = {
                .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                .available_items = NULL,
                .available = 0,
        },
        .atomics = {
                .bytes = 0,
                .allocated = 0,
                .bytes_moved = 0,
                .reallocs = 0,
        },
};

void julyl_cleanup1(void) {
    struct JulyL *item = NULL;

    if(!netdata_spinlock_trylock(&julyl_globals.protected.spinlock))
        return;

    if(julyl_globals.protected.available_items && julyl_globals.protected.available > 10) {
        item = julyl_globals.protected.available_items;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(julyl_globals.protected.available_items, item, cache.prev, cache.next);
        julyl_globals.protected.available--;
    }

    netdata_spinlock_unlock(&julyl_globals.protected.spinlock);

    if(item) {
        size_t bytes = item->bytes;
        freez(item);
        __atomic_sub_fetch(&julyl_globals.atomics.bytes, bytes, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&julyl_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }
}

struct JulyL *julyl_get(void) {
    struct JulyL *j;

    netdata_spinlock_lock(&julyl_globals.protected.spinlock);

    j = julyl_globals.protected.available_items;
    if(likely(j)) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(julyl_globals.protected.available_items, j, cache.prev, cache.next);
        julyl_globals.protected.available--;
    }

    netdata_spinlock_unlock(&julyl_globals.protected.spinlock);

    if(unlikely(!j)) {
        size_t bytes = sizeof(struct JulyL) + JULYL_MIN_ENTRIES * sizeof(struct JulyL_item);
        j = mallocz(bytes);
        j->bytes = bytes;
        j->entries = JULYL_MIN_ENTRIES;
        __atomic_add_fetch(&julyl_globals.atomics.bytes, bytes, __ATOMIC_RELAXED);
        __atomic_add_fetch(&julyl_globals.atomics.allocated, 1, __ATOMIC_RELAXED);
    }

    j->used = 0;
    j->bytes_moved = 0;
    j->reallocs = 0;
    j->cache.next = j->cache.prev = NULL;
    return j;
}

static void julyl_release(struct JulyL *j) {
    if(unlikely(!j)) return;

    __atomic_add_fetch(&julyl_globals.atomics.bytes_moved, j->bytes_moved, __ATOMIC_RELAXED);
    __atomic_add_fetch(&julyl_globals.atomics.reallocs, j->reallocs, __ATOMIC_RELAXED);

    netdata_spinlock_lock(&julyl_globals.protected.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(julyl_globals.protected.available_items, j, cache.prev, cache.next);
    julyl_globals.protected.available++;
    netdata_spinlock_unlock(&julyl_globals.protected.spinlock);
}

size_t julyl_cache_size(void) {
    return __atomic_load_n(&julyl_globals.atomics.bytes, __ATOMIC_RELAXED);
}

size_t julyl_bytes_moved(void) {
    return __atomic_load_n(&julyl_globals.atomics.bytes_moved, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// JulyL

size_t JulyLGet_binary_search_position_of_index(const struct JulyL *July, Word_t Index) {
    // return the position of the first item >= Index

    size_t left = 0;
    size_t right = July->used;
    while(left < right) {
        size_t middle = (left + right) >> 1;

        if(July->array[middle].index > Index)
            right = middle;

        else
            left = middle + 1;
    }

    internal_fatal(left > July->used, "JULY: invalid position returned");

    if(left > 0 && July->array[left - 1].index == Index)
        return left - 1;

    internal_fatal( (left < July->used && July->array[left].index < Index) ||
                    (left > 0 && July->array[left - 1].index >= Index)
                   , "JULY: wrong item returned");

    return left;
}

PPvoid_t JulyLGet(Pcvoid_t PArray, Word_t Index, PJError_t PJError __maybe_unused) {
    const struct JulyL *July = PArray;
    if(!July)
        return NULL;

    size_t pos = JulyLGet_binary_search_position_of_index(July, Index);

    if(unlikely(pos >= July->used || July->array[pos].index != Index))
        return NULL;

    return (PPvoid_t)&July->array[pos].value;
}

PPvoid_t JulyLIns(PPvoid_t PPArray, Word_t Index, PJError_t PJError __maybe_unused) {
    struct JulyL *July = *PPArray;
    if(unlikely(!July)) {
        July = julyl_get();
        July->used = 0;
        *PPArray = July;
    }

    size_t pos = JulyLGet_binary_search_position_of_index(July, Index);

    if((pos == July->used || July->array[pos].index != Index)) {
        // we have to add this entry

        if (unlikely(July->used == July->entries)) {
            // we have to expand the array
            size_t bytes = sizeof(struct JulyL) + July->entries * 2 * sizeof(struct JulyL_item);
            __atomic_add_fetch(&julyl_globals.atomics.bytes, bytes - July->bytes, __ATOMIC_RELAXED);
            July = reallocz(July, bytes);
            July->bytes = bytes;
            July->entries *= 2;
            July->reallocs++;
            *PPArray = July;
        }

        if (unlikely(pos != July->used)) {
            // we have to shift some members to make room
            size_t size = (July->used - pos) * sizeof(struct JulyL_item);
            memmove(&July->array[pos + 1], &July->array[pos], size);
            July->bytes_moved += size;
        }

        July->used++;
        July->array[pos].value = NULL;
        July->array[pos].index = Index;
    }

    return &July->array[pos].value;
}

PPvoid_t JulyLFirst(Pcvoid_t PArray, Word_t *Index, PJError_t PJError __maybe_unused) {
    const struct JulyL *July = PArray;
    if(!July)
        return NULL;

    size_t pos = JulyLGet_binary_search_position_of_index(July, *Index);
    // pos is >= Index

    if(unlikely(pos == July->used))
        return NULL;

    *Index = July->array[pos].index;
    return (PPvoid_t)&July->array[pos].value;
}

PPvoid_t JulyLNext(Pcvoid_t PArray, Word_t *Index, PJError_t PJError __maybe_unused) {
    const struct JulyL *July = PArray;
    if(!July)
        return NULL;

    size_t pos = JulyLGet_binary_search_position_of_index(July, *Index);
    // pos is >= Index

    if(unlikely(pos == July->used))
        return NULL;

    if(July->array[pos].index == *Index) {
        pos++;

        if(unlikely(pos == July->used))
            return NULL;
    }

    *Index = July->array[pos].index;
    return (PPvoid_t)&July->array[pos].value;
}

PPvoid_t JulyLLast(Pcvoid_t PArray, Word_t *Index, PJError_t PJError __maybe_unused) {
    const struct JulyL *July = PArray;
    if(!July)
        return NULL;

    size_t pos = JulyLGet_binary_search_position_of_index(July, *Index);
    // pos is >= Index

    if(pos > 0 && (pos == July->used || July->array[pos].index > *Index))
        pos--;

    if(unlikely(pos == 0 && July->array[0].index > *Index))
        return NULL;

    *Index = July->array[pos].index;
    return (PPvoid_t)&July->array[pos].value;
}

PPvoid_t JulyLPrev(Pcvoid_t PArray, Word_t *Index, PJError_t PJError __maybe_unused) {
    const struct JulyL *July = PArray;
    if(!July)
        return NULL;

    size_t pos = JulyLGet_binary_search_position_of_index(July, *Index);
    // pos is >= Index

    if(unlikely(pos == 0 || July->used == 0))
        return NULL;

    // get the previous one
    pos--;

    *Index = July->array[pos].index;
    return (PPvoid_t)&July->array[pos].value;
}

Word_t JulyLFreeArray(PPvoid_t PPArray, PJError_t PJError __maybe_unused) {
    struct JulyL *July = *PPArray;
    if(unlikely(!July))
        return 0;

    size_t bytes = July->bytes;
    julyl_release(July);
    *PPArray = NULL;
    return bytes;
}

// ----------------------------------------------------------------------------
// unittest

#define item_index(i) (((i) * 2) + 100)

int julytest(void) {
    Word_t entries = 10000;
    Pvoid_t array = NULL;

    // test additions
    for(Word_t i = 0; i < entries ;i++) {
        Pvoid_t *PValue = JulyLIns(&array, item_index(i), PJE0);
        if(!PValue)
            fatal("JULY: cannot insert item %lu", item_index(i));

        *PValue = (void *)(item_index(i));
    }

    // test successful finds
    for(Word_t i = 0; i < entries ;i++) {
        Pvoid_t *PValue = JulyLGet(array, item_index(i), PJE0);
        if(!PValue)
            fatal("JULY: cannot find item %lu", item_index(i));

        if(*PValue != (void *)(item_index(i)))
            fatal("JULY: item %lu has the value %lu", item_index(i), (unsigned long)(*PValue));
    }

    // test finding the first item
    for(Word_t i = 0; i < entries ;i++) {
        Word_t index = item_index(i);
        Pvoid_t *PValue = JulyLFirst(array, &index, PJE0);
        if(!PValue)
            fatal("JULY: cannot find first item %lu", item_index(i));

        if(*PValue != (void *)(item_index(i)))
            fatal("JULY: item %lu has the value %lu", item_index(i), (unsigned long)(*PValue));

        if(index != item_index(i))
            fatal("JULY: item %lu has index %lu", item_index(i), index);
    }

    // test finding the next item
    for(Word_t i = 0; i < entries - 1 ;i++) {
        Word_t index = item_index(i);
        Pvoid_t *PValue = JulyLNext(array, &index, PJE0);
        if(!PValue)
            fatal("JULY: cannot find next item %lu", item_index(i));

        if(*PValue != (void *)(item_index(i + 1)))
            fatal("JULY: item %lu next has the value %lu", item_index(i), (unsigned long)(*PValue));

        if(index != item_index(i + 1))
            fatal("JULY: item %lu next has index %lu", item_index(i), index);
    }

    // test finding the last item
    for(Word_t i = 0; i < entries ;i++) {
        Word_t index = item_index(i);
        Pvoid_t *PValue = JulyLLast(array, &index, PJE0);
        if(!PValue)
            fatal("JULY: cannot find last item %lu", item_index(i));

        if(*PValue != (void *)(item_index(i)))
            fatal("JULY: item %lu has the value %lu", item_index(i), (unsigned long)(*PValue));

        if(index != item_index(i))
            fatal("JULY: item %lu has index %lu", item_index(i), index);
    }

    // test finding the prev item
    for(Word_t i = 1; i < entries ;i++) {
        Word_t index = item_index(i);
        Pvoid_t *PValue = JulyLPrev(array, &index, PJE0);
        if(!PValue)
            fatal("JULY: cannot find prev item %lu", item_index(i));

        if(*PValue != (void *)(item_index(i - 1)))
            fatal("JULY: item %lu prev has the value %lu", item_index(i), (unsigned long)(*PValue));

        if(index != item_index(i - 1))
            fatal("JULY: item %lu prev has index %lu", item_index(i), index);
    }

    // test full traversal forward
    {
        Word_t i = 0;
        Word_t index = 0;
        bool first = true;
        Pvoid_t *PValue;
        while((PValue = JulyLFirstThenNext(array, &index, &first))) {
            if(*PValue != (void *)(item_index(i)))
                fatal("JULY: item %lu traversal has the value %lu", item_index(i), (unsigned long)(*PValue));

            if(index != item_index(i))
                fatal("JULY: item %lu traversal has index %lu", item_index(i), index);

            i++;
        }

        if(i != entries)
            fatal("JULY: expected to forward traverse %lu entries, but traversed %lu", entries, i);
    }

    // test full traversal backward
    {
        Word_t i = 0;
        Word_t index = (Word_t)(-1);
        bool first = true;
        Pvoid_t *PValue;
        while((PValue = JulyLLastThenPrev(array, &index, &first))) {
            if(*PValue != (void *)(item_index(entries - i - 1)))
                fatal("JULY: item %lu traversal has the value %lu", item_index(i), (unsigned long)(*PValue));

            if(index != item_index(entries - i - 1))
                fatal("JULY: item %lu traversal has index %lu", item_index(i), index);

            i++;
        }

        if(i != entries)
            fatal("JULY: expected to back traverse %lu entries, but traversed %lu", entries, i);
    }

    // test finding non-existing first item
    for(Word_t i = 0; i < entries ;i++) {
        Word_t index = item_index(i) - 1;
        Pvoid_t *PValue = JulyLFirst(array, &index, PJE0);
        if(!PValue)
            fatal("JULY: cannot find first item %lu", item_index(i) - 1);

        if(*PValue != (void *)(item_index(i)))
            fatal("JULY: item %lu has the value %lu", item_index(i), (unsigned long)(*PValue));

        if(index != item_index(i))
            fatal("JULY: item %lu has index %lu", item_index(i), index);
    }

    // test finding non-existing last item
    for(Word_t i = 0; i < entries ;i++) {
        Word_t index = item_index(i) + 1;
        Pvoid_t *PValue = JulyLLast(array, &index, PJE0);
        if(!PValue)
            fatal("JULY: cannot find last item %lu", item_index(i) + 1);

        if(*PValue != (void *)(item_index(i)))
            fatal("JULY: item %lu has the value %lu", item_index(i), (unsigned long)(*PValue));

        if(index != item_index(i))
            fatal("JULY: item %lu has index %lu", item_index(i), index);
    }

    JulyLFreeArray(&array, PJE0);

    return 0;
}


