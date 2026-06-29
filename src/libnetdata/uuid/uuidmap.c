// SPDX-License-Identifier: GPL-3.0-or-later

#include "uuidmap.h"

struct uuidmap_entry {
    nd_uuid_t uuid;
    REFCOUNT refcount;
};

// each partition on its own cache line(s), so the heavily contended lock
// words of adjacent partitions do not false-share
struct uuidmap_partition {
    Pvoid_t uuid_to_id;         // JudyL: UUID string -> ID
    Pvoid_t id_to_uuid;         // JudyL: ID -> UUID binary
    UUIDMAP_ID next_id;         // Only use lower bits
    RW_SPINLOCK spinlock;

    int64_t memory;
    int32_t entries;
} __attribute__((aligned(64)));

static struct {
    struct uuidmap_partition p[UUIDMAP_PARTITIONS];
    ARAL *ar;
} uuid_map = { 0 };

static struct aral_statistics uuidmap_stats = { 0 };
struct aral_statistics *uuidmap_aral_statistics(void) { return &uuidmap_stats; }

size_t uuidmap_memory(void) {
    size_t memory = 0;

    for(size_t i = 0; i < _countof(uuid_map.p) ;i++) {
        rw_spinlock_read_lock(&uuid_map.p[i].spinlock);
        memory += uuid_map.p[i].memory;
        rw_spinlock_read_unlock(&uuid_map.p[i].spinlock);
    }

    return memory;
}

size_t uuidmap_free_bytes(void) {
    return aral_free_bytes_from_stats(&uuidmap_stats);
}

static void uuidmap_init_aral(void) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    if(!uuid_map.ar) {
        spinlock_lock(&spinlock);
        if(!uuid_map.ar) {
            uuid_map.ar = aral_create(
                "uuidmap",
                sizeof(struct uuidmap_entry),
                0,
                0,
                &uuidmap_stats,
                NULL, NULL, false, false, true);
        }
        spinlock_unlock(&spinlock);
    }
}

static UUIDMAP_ID get_next_id_unsafe(struct uuidmap_partition *partition) {
    // Check if we've reached the maximum ID value
    if (unlikely(partition->next_id >= UUIDMAP_ID_SEQ_MASK))
        fatal("UUIDMAP: Maximum ID limit reached for partition %u. UUIDs exhausted.",
              (unsigned int)(partition - uuid_map.p));

    // IDs are never reused, so the sequence space is lifetime capacity.
    // next_id is monotonic and only changes under the partition write lock,
    // so the equality check fires exactly once per partition.
    if (unlikely(partition->next_id == (UUIDMAP_ID_SEQ_MASK / 10) * 9))
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "UUIDMAP: partition %u has used 90%% of its lifetime ID space (%u of %u). "
               "When it is exhausted, netdata will exit. Restarting netdata resets it.",
               (unsigned int)(partition - uuid_map.p),
               (unsigned int)partition->next_id,
               (unsigned int)UUIDMAP_ID_SEQ_MASK);

    // Simply increment and return the next ID
    return uuidmap_make_id(partition - uuid_map.p, ++partition->next_id);
}

static inline UUIDMAP_ID uuidmap_acquire_by_uuid(const nd_uuid_t uuid) {
    UUIDMAP_ID id = 0;

    uint8_t partition = uuid_to_uuidmap_partition(uuid);

    // try to find it in the JudyHS - we may have it already
    rw_spinlock_read_lock(&uuid_map.p[partition].spinlock);
    Pvoid_t *PValue = JudyHSGet(uuid_map.p[partition].uuid_to_id, (void *)uuid, sizeof(nd_uuid_t));
    if(PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyHS array");

    if(PValue && *PValue) {
        // it is found

        id = (UUIDMAP_ID)(uintptr_t)*PValue;

        PValue = JudyLGet(uuid_map.p[partition].id_to_uuid, id, PJE0);
        if (!PValue || PValue == PJERR)
            fatal("UUIDMAP: corrupted JudyL array");

        struct uuidmap_entry *ue = *PValue;
        if(!refcount_acquire(&ue->refcount))
            id = 0;
    }

    rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);
    return id;
}

// Public read-only counterpart to uuidmap_create(): returns the existing ID
// (refcount incremented) or 0 if the UUID is not present. Never inserts.
// Caller must call uuidmap_free() on the returned ID.
UUIDMAP_ID uuidmap_acquire(const nd_uuid_t uuid) {
    return uuidmap_acquire_by_uuid(uuid);
}

UUIDMAP_ID uuidmap_create(const nd_uuid_t uuid) {
    UUIDMAP_ID id = uuidmap_acquire_by_uuid(uuid);
    if(id != 0) return id;

    uuidmap_init_aral();

    // we didn't find it - let's add it

    uint8_t partition = uuid_to_uuidmap_partition(uuid);

    JudyAllocThreadPulseReset();

    Pvoid_t *PValue;
    while(true) {
        rw_spinlock_write_lock(&uuid_map.p[partition].spinlock);

        PValue = JudyHSIns(&uuid_map.p[partition].uuid_to_id, (void *)uuid, sizeof(nd_uuid_t), PJE0);
        if (!PValue || PValue == PJERR)
            fatal("UUIDMAP: corrupted JudyHS array");

        // If value exists, return it
        if (*PValue != 0) {
            id = (UUIDMAP_ID)(uintptr_t)*PValue;

            PValue = JudyLGet(uuid_map.p[partition].id_to_uuid, id, PJE0);
            if (!PValue || PValue == PJERR)
                fatal("UUIDMAP: corrupted JudyL array");

            struct uuidmap_entry *ue = *PValue;
            if (!refcount_acquire(&ue->refcount)) {
                rw_spinlock_write_unlock(&uuid_map.p[partition].spinlock);
                continue;
            }

            uuid_map.p[partition].memory += JudyAllocThreadPulseGetAndReset();
            rw_spinlock_write_unlock(&uuid_map.p[partition].spinlock);
            return id;
        }
        else
            break;
    }

    id = get_next_id_unsafe(&uuid_map.p[partition]);
    *PValue = (Pvoid_t)(uintptr_t)id;

    // Store ID -> UUID mapping
    PValue = JudyLIns(&uuid_map.p[partition].id_to_uuid, id, PJE0);
    if (!PValue || PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyL array");

    struct uuidmap_entry *ue = aral_mallocz(uuid_map.ar);
    nd_uuid_copy(ue->uuid, uuid);
    ue->refcount = 1;
    *PValue = ue;

    uuid_map.p[partition].entries++;
    uuid_map.p[partition].memory += sizeof(*ue);

    uuid_map.p[partition].memory += JudyAllocThreadPulseGetAndReset();
    rw_spinlock_write_unlock(&uuid_map.p[partition].spinlock);
    return id;
}

static struct uuidmap_entry *get_entry_by_id_locked(UUIDMAP_ID id, uint8_t partition) {
    Pvoid_t *PValue = JudyLGet(uuid_map.p[partition].id_to_uuid, id, PJE0);
    if (PValue == PJERR)
        fatal("UUIDMAP: corrupted JudyL array");

    return PValue ? *PValue : NULL;
}

static ALWAYS_INLINE struct uuidmap_entry *get_entry_by_id_locked_and_acquire(UUIDMAP_ID id, uint8_t partition) {
    struct uuidmap_entry *ue = get_entry_by_id_locked(id, partition);
    if(ue && !refcount_acquire(&ue->refcount))
        ue = NULL;

    return ue;
}

static struct uuidmap_entry *get_entry_by_id_and_acquire(UUIDMAP_ID id) {
    if(id == 0) return NULL;

    uint8_t partition = uuidmap_id_to_partition(id);
    rw_spinlock_read_lock(&uuid_map.p[partition].spinlock);

    struct uuidmap_entry *ue = get_entry_by_id_locked_and_acquire(id, partition);

    rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);

    return ue;
}

void uuidmap_free(UUIDMAP_ID id) {
    if(id == 0) return;

    uint8_t partition = uuidmap_id_to_partition(id);
    struct uuidmap_entry *ue = NULL;
    bool should_delete = false;

    rw_spinlock_write_lock(&uuid_map.p[partition].spinlock);

    ue = get_entry_by_id_locked(id, partition);
    if(ue && refcount_release_and_acquire_for_deletion(&ue->refcount)) {
        JudyAllocThreadPulseReset();

        int rc;
        rc = JudyHSDel(&uuid_map.p[partition].uuid_to_id, (void *)ue->uuid, sizeof(nd_uuid_t), PJE0);
        if(unlikely(!rc))
            fatal("UUIDMAP: cannot delete UUID from JudyHS");

        rc = JudyLDel(&uuid_map.p[partition].id_to_uuid, id, PJE0);
        if(unlikely(!rc))
            fatal("UUIDMAP: cannot delete ID from JudyL");

        uuid_map.p[partition].memory -= sizeof(*ue);
        uuid_map.p[partition].entries--;

        uuid_map.p[partition].memory += JudyAllocThreadPulseGetAndReset();
        should_delete = true;
    }

    rw_spinlock_write_unlock(&uuid_map.p[partition].spinlock);

    if(should_delete)
        aral_freez(uuid_map.ar, ue);
}

nd_uuid_t *uuidmap_uuid_ptr(UUIDMAP_ID id) {
    if(id == 0) return NULL;

    uint8_t partition = uuidmap_id_to_partition(id);
    rw_spinlock_read_lock(&uuid_map.p[partition].spinlock);

    struct uuidmap_entry *ue = get_entry_by_id_locked(id, partition);
    nd_uuid_t *uuid = ue ? &ue->uuid : NULL;

    rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);

    return uuid;
}

nd_uuid_t *uuidmap_uuid_ptr_and_dup(UUIDMAP_ID id) {
    struct uuidmap_entry *ue = get_entry_by_id_and_acquire(id);
    return ue ? &ue->uuid : NULL;
}

bool uuidmap_uuid(UUIDMAP_ID id, nd_uuid_t out_uuid) {
    if(id == 0) {
        nd_uuid_clear(out_uuid);
        return false;
    }

    uint8_t partition = uuidmap_id_to_partition(id);
    rw_spinlock_read_lock(&uuid_map.p[partition].spinlock);

    struct uuidmap_entry *ue = get_entry_by_id_locked(id, partition);
    if(!ue) {
        nd_uuid_clear(out_uuid);
        rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);
        return false;
    }

    uuid_copy(out_uuid, ue->uuid);
    rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);

    return true;
}

ND_UUID uuidmap_get(UUIDMAP_ID id) {
    ND_UUID uuid;
    uuidmap_uuid(id, uuid.uuid);
    return uuid;
}

UUIDMAP_ID uuidmap_dup(UUIDMAP_ID id) {
    struct uuidmap_entry *ue = get_entry_by_id_and_acquire(id);

    if(!ue)
        fatal("UUIDMAP: id %u does not exist, or cannot be acquired, in %s", id, __FUNCTION__ );

    return id;
}

size_t uuidmap_destroy(void) {
    size_t referenced = 0;

    // Traverse all partitions
    for (size_t partition = 0; partition < UUIDMAP_PARTITIONS; partition++) {
        // Lock the partition to prevent new entries while we're cleaning up
        rw_spinlock_write_lock(&uuid_map.p[partition].spinlock);

        Pvoid_t uuid_to_id = uuid_map.p[partition].uuid_to_id;
        Pvoid_t id_to_uuid = uuid_map.p[partition].id_to_uuid;

        // Process all entries in the id_to_uuid map
        Word_t id_index = 0;
        Pvoid_t *id_pvalue;

        for (id_pvalue = JudyLFirst(id_to_uuid, &id_index, PJE0);
             id_pvalue != NULL && id_pvalue != PJERR;
             id_pvalue = JudyLNext(id_to_uuid, &id_index, PJE0)) {

            if (!(*id_pvalue))
                continue;

            struct uuidmap_entry *ue = *id_pvalue;

            // Try to acquire for deletion
            if (!refcount_acquire_for_deletion(&ue->refcount))
                referenced++;

            aral_freez(uuid_map.ar, ue);
        }

        // Free all Judy arrays
        JudyHSFreeArray(&uuid_to_id, PJE0);
        JudyLFreeArray(&id_to_uuid, PJE0);

        // Reset partition data
        memset(&uuid_map.p[partition], 0, sizeof(uuid_map.p[partition]));

        rw_spinlock_write_unlock(&uuid_map.p[partition].spinlock);
    }

    // Destroy ARAL
    if (uuid_map.ar) {
        aral_destroy(uuid_map.ar);
        uuid_map.ar = NULL;
    }

    memset(&uuid_map, 0, sizeof(uuid_map));
    return referenced;
}

// --------------------------------------------------------------------------------------------------------------------

static volatile bool stop_flag = false;

typedef struct thread_stats {
    size_t creates;
    size_t finds;
    size_t dups;
    size_t frees;
    size_t cycles;
} THREAD_STATS;

typedef struct uuidmap_delete_gate {
    UUIDMAP_ID id;
    uint8_t partition;
    bool writer_blocked;
    bool writer_unexpectedly_acquired;
    bool allow_delete;
    bool delete_done;
} UUIDMAP_DELETE_GATE;

static void uuidmap_deferred_free_thread(void *arg) {
    UUIDMAP_DELETE_GATE *gate = arg;

    if(rw_spinlock_trywrite_lock(&uuid_map.p[gate->partition].spinlock)) {
        __atomic_store_n(&gate->writer_unexpectedly_acquired, true, __ATOMIC_RELEASE);
        rw_spinlock_write_unlock(&uuid_map.p[gate->partition].spinlock);
    }
    else
        __atomic_store_n(&gate->writer_blocked, true, __ATOMIC_RELEASE);

    while(!__atomic_load_n(&gate->allow_delete, __ATOMIC_ACQUIRE))
        tinysleep();

    uuidmap_free(gate->id);
    __atomic_store_n(&gate->delete_done, true, __ATOMIC_RELEASE);
}

static int uuidmap_locked_lookup_delete_interleaving_unittest(void) {
    fprintf(stderr, "\nTesting UUID Map locked lookup/delete interleaving...\n");

    nd_uuid_t test_uuid = {
        0xf0, 0xde, 0xbc, 0x9a,
        0x78, 0x56, 0x34, 0x12,
        0xf0, 0xde, 0xbc, 0x9a,
        0x78, 0x56, 0x34, 0x11
    };

    int errors = 0;
    UUIDMAP_ID id = uuidmap_create(test_uuid);
    if(!id) {
        fprintf(stderr, "ERROR: Cannot create UUID for locked lookup/delete interleaving test\n");
        return 1;
    }

    uint8_t partition = uuidmap_id_to_partition(id);
    UUIDMAP_DELETE_GATE gate = {
        .id = id,
        .partition = partition,
    };

    rw_spinlock_read_lock(&uuid_map.p[partition].spinlock);

    struct uuidmap_entry *ue = get_entry_by_id_locked(id, partition);
    if(!ue) {
        fprintf(stderr, "ERROR: Cannot find UUID after create in locked lookup/delete interleaving test\n");
        errors++;
    }

    ND_THREAD *thread = nd_thread_create(
        "UUID-DELETE-GATE",
        NETDATA_THREAD_OPTION_DONT_LOG,
        uuidmap_deferred_free_thread,
        &gate);
    if(!thread) {
        fprintf(stderr, "ERROR: Cannot create delete helper thread in locked lookup/delete interleaving test\n");
        rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);
        uuidmap_free(id);
        return errors + 1;
    }

    usec_t deadline = now_monotonic_usec() + 5 * USEC_PER_SEC;
    while(!__atomic_load_n(&gate.writer_blocked, __ATOMIC_ACQUIRE) &&
          !__atomic_load_n(&gate.writer_unexpectedly_acquired, __ATOMIC_ACQUIRE) &&
          now_monotonic_usec() < deadline)
        tinysleep();

    if(__atomic_load_n(&gate.writer_unexpectedly_acquired, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "ERROR: UUID delete writer entered while lookup reader was locked\n");
        errors++;
    }

    if(!__atomic_load_n(&gate.writer_blocked, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "ERROR: UUID delete writer did not reach the blocked interleaving window\n");
        errors++;
    }

    bool acquired_ref = false;
    if(ue) {
        struct uuidmap_entry *acquired = get_entry_by_id_locked_and_acquire(id, partition);
        if(acquired != ue) {
            fprintf(stderr, "ERROR: Locked lookup/acquire returned unexpected UUID entry\n");
            errors++;
        }
        else
            acquired_ref = true;
    }

    rw_spinlock_read_unlock(&uuid_map.p[partition].spinlock);

    __atomic_store_n(&gate.allow_delete, true, __ATOMIC_RELEASE);
    nd_thread_join(thread);

    if(!__atomic_load_n(&gate.delete_done, __ATOMIC_ACQUIRE)) {
        fprintf(stderr, "ERROR: UUID delete helper did not complete\n");
        errors++;
    }

    if(acquired_ref) {
        nd_uuid_t found_uuid;
        if(!uuidmap_uuid(id, found_uuid)) {
            fprintf(stderr, "ERROR: UUID disappeared despite locked refcount acquire\n");
            errors++;
        }
        else if(uuid_compare(found_uuid, test_uuid) != 0) {
            fprintf(stderr, "ERROR: UUID changed after locked lookup/delete interleaving\n");
            errors++;
        }

        uuidmap_free(id);
    }

    nd_uuid_t found_uuid;
    if(uuidmap_uuid(id, found_uuid)) {
        fprintf(stderr, "ERROR: UUID still exists after releasing locked interleaving reference\n");
        errors++;
        uuidmap_free(id);
    }

    if(errors)
        fprintf(stderr, "UUID Map locked lookup/delete interleaving test: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "UUID Map locked lookup/delete interleaving test: OK\n");

    return errors;
}

static void concurrent_test_thread(void *arg) {
    THREAD_STATS *stats = arg;
    nd_uuid_t test_uuid = {
        0x12, 0x34, 0x56, 0x78,
        0x9a, 0xbc, 0xde, 0xf0,
        0x12, 0x34, 0x56, 0x78,
        0x9a, 0xbc, 0xde, 0xf0
    };

    while(!__atomic_load_n(&stop_flag, __ATOMIC_RELAXED)) {
        // 1. Create UUID (refcount 1)
        UUIDMAP_ID id = uuidmap_create(test_uuid);
        if(!id) continue;
        stats->creates++;

        // 2. Find its pointer
        nd_uuid_t *uuid_ptr = uuidmap_uuid_ptr(id);
        if(!uuid_ptr) {
            fprintf(stderr, "ERROR: Cannot find UUID we just created\n");
            break;
        }
        stats->finds++;

        // 3. Dup it (refcount 2)
        UUIDMAP_ID id2 = uuidmap_dup(id);
        if(!id2) {
            fprintf(stderr, "ERROR: Cannot dup UUID\n");
            break;
        }
        stats->dups++;

        // 4. Free it once (refcount 1)
        uuidmap_free(id);
        stats->frees++;

        // 5. Find its pointer again
        uuid_ptr = uuidmap_uuid_ptr(id2);
        if(!uuid_ptr) {
            fprintf(stderr, "ERROR: Cannot find UUID after first free\n");
            break;
        }
        stats->finds++;

        // 6. Free it twice (should delete)
        uuidmap_free(id2);
        stats->frees++;

        stats->cycles++;
    }
}

static int uuidmap_concurrent_unittest(void) {
    enum { UUIDMAP_UNITTEST_THREADS = 4, UUIDMAP_UNITTEST_SECONDS = 5 };
    fprintf(stderr, "\nTesting concurrent UUID Map access with %d threads for %d seconds...\n", UUIDMAP_UNITTEST_THREADS, UUIDMAP_UNITTEST_SECONDS);
    int errors = 0;

    THREAD_STATS stats[UUIDMAP_UNITTEST_THREADS];
    memset(stats, 0, sizeof(stats));

    ND_THREAD *threads[UUIDMAP_UNITTEST_THREADS];

    // Start threads
    __atomic_store_n(&stop_flag, false, __ATOMIC_RELAXED);

    for(int i = 0; i < UUIDMAP_UNITTEST_THREADS; i++) {
        char thread_name[32];
        snprintf(thread_name, sizeof(thread_name), "UUID-TEST-%d", i);
        threads[i] = nd_thread_create(
            thread_name,
            NETDATA_THREAD_OPTION_DONT_LOG,
            concurrent_test_thread,
            &stats[i]);
    }

    // Let it run for 5 seconds
    sleep_usec(UUIDMAP_UNITTEST_SECONDS * USEC_PER_SEC);

    // Stop threads
    __atomic_store_n(&stop_flag, true, __ATOMIC_RELEASE);

    // Wait for threads
    for(int i = 0; i < UUIDMAP_UNITTEST_THREADS; i++)
        nd_thread_join(threads[i]);

    // Print statistics
    size_t total_cycles = 0;
    for(int i = 0; i < UUIDMAP_UNITTEST_THREADS; i++) {
        fprintf(stderr, "Thread %d stats:\n"
                        "  Cycles completed : %zu\n"
                        "  Creates         : %zu\n"
                        "  Finds           : %zu\n"
                        "  Dups            : %zu\n"
                        "  Frees           : %zu\n",
                i,
                stats[i].cycles,
                stats[i].creates,
                stats[i].finds,
                stats[i].dups,
                stats[i].frees);

        total_cycles += stats[i].cycles;
    }

    fprintf(stderr, "\nTotal cycles completed: %zu (%.2f cycles/sec)\n",
            total_cycles,
            (double)total_cycles / 5.0);

    return errors;
}

int uuidmap_unittest(void) {
    fprintf(stderr, "\nTesting UUID Map...\n");

    const size_t ENTRIES = 100000;
    int errors = uuidmap_locked_lookup_delete_interleaving_unittest();
    errors += uuidmap_concurrent_unittest();

    struct test_entry {
        nd_uuid_t uuid;
        UUIDMAP_ID id;
    };

    struct test_entry *entries = mallocz(sizeof(struct test_entry) * ENTRIES);

    fprintf(stderr, "Generating and testing %zu entries...\n", ENTRIES);

    usec_t start_time = now_monotonic_usec();
    size_t step = ENTRIES / 100;
    size_t next_step = step;

    for(size_t i = 0; i < ENTRIES; i++) {
        if (i >= next_step) {
            fprintf(stderr, ".");
            next_step += step;
        }

        uuid_generate_random(entries[i].uuid);
        char uuid_str[UUID_STR_LEN];
        uuid_unparse_lower(entries[i].uuid, uuid_str);

        // Test 1: Should not exist yet
        UUIDMAP_ID id = uuidmap_acquire_by_uuid(entries[i].uuid);
        if(id != 0) {
            fprintf(stderr, "\nERROR [%zu]: UUID found before adding it"
                            "\n  UUID: %s"
                            "\n  Got ID: %u (expected: 0)\n",
                    i, uuid_str, id);
            errors++;
        }

        // Test 2: Create it
        id = uuidmap_create(entries[i].uuid);
        if(id == 0) {
            fprintf(stderr, "\nERROR [%zu]: Failed to create UUID mapping"
                            "\n  UUID: %s\n",
                    i, uuid_str);
            errors++;
            continue;
        }

        // Test 3: Create again, should return same id
        UUIDMAP_ID id2 = uuidmap_create(entries[i].uuid);
        if(id2 != id) {
            fprintf(stderr, "\nERROR [%zu]: Second create returned different ID"
                            "\n  UUID: %s"
                            "\n  First ID: %u"
                            "\n  Second ID: %u\n",
                    i, uuid_str, id, id2);
            errors++;
        }

        // Test 4: Get UUID and verify
        nd_uuid_t test_uuid;
        if(!uuidmap_uuid(id, test_uuid)) {
            fprintf(stderr, "\nERROR [%zu]: Failed to get UUID for valid ID"
                            "\n  UUID: %s"
                            "\n  ID: %u\n",
                    i, uuid_str, id);
            errors++;
        }
        else {
            char test_uuid_str[UUID_STR_LEN];
            uuid_unparse_lower(test_uuid, test_uuid_str);
            if(uuid_compare(test_uuid, entries[i].uuid) != 0) {
                fprintf(stderr, "\nERROR [%zu]: Retrieved UUID doesn't match original"
                                "\n  Original UUID: %s"
                                "\n  Retrieved UUID: %s"
                                "\n  ID: %u\n",
                        i, uuid_str, test_uuid_str, id);
                errors++;
            }
        }

        // Test 5: Free once (decrease refcount)
        uuidmap_free(id);

        // Test 6: Should still exist
        if(!uuidmap_uuid(id, test_uuid)) {
            fprintf(stderr, "\nERROR [%zu]: UUID disappeared after first free"
                            "\n  UUID: %s"
                            "\n  ID: %u\n",
                    i, uuid_str, id);
            errors++;
        }
        else {
            char test_uuid_str[UUID_STR_LEN];
            uuid_unparse_lower(test_uuid, test_uuid_str);
            if(uuid_compare(test_uuid, entries[i].uuid) != 0) {
                fprintf(stderr, "\nERROR [%zu]: Retrieved UUID doesn't match after first free"
                                "\n  Original UUID: %s"
                                "\n  Retrieved UUID: %s"
                                "\n  ID: %u\n",
                        i, uuid_str, test_uuid_str, id);
                errors++;
            }
        }

        // Test 7: Free again (should delete)
        uuidmap_free(id);

        // Test 8: Should be gone
        if(uuidmap_uuid_ptr(id) != NULL) {
            char curr_uuid_str[UUID_STR_LEN];
            nd_uuid_t *curr_uuid = uuidmap_uuid_ptr(id);
            if(curr_uuid)
                uuid_unparse_lower(*curr_uuid, curr_uuid_str);

            fprintf(stderr, "\nERROR [%zu]: UUID still exists after second free"
                            "\n  Original UUID: %s"
                            "\n  Current UUID: %s"
                            "\n  ID: %u\n",
                    i, uuid_str, curr_uuid_str, id);

            errors++;
        }

        // Test 9: Create again for phase 2
        id = uuidmap_create(entries[i].uuid);
        if(id == 0) {
            fprintf(stderr, "\nERROR [%zu]: Failed to recreate UUID mapping"
                            "\n  UUID: %s\n",
                    i, uuid_str);
            errors++;
            continue;
        }

        entries[i].id = id;
    }

    usec_t end_time = now_monotonic_usec();
    fprintf(stderr, "\nPhase 1 completed in %.2f seconds with %d errors\n",
            (double)(end_time - start_time) / (double)USEC_PER_SEC, errors);

    // BENCHMARK while we have all entries loaded
    if(errors == 0) {
        fprintf(stderr, "\nBenchmarking UUID retrievals...\n");

        // First benchmark: uuidmap_uuid_ptr()
        size_t successful = 0;
        usec_t start_ut = now_monotonic_usec();

        for(size_t i = 0; i < ENTRIES; i++) {
            nd_uuid_t *uuid_ptr = uuidmap_uuid_ptr(entries[i].id);
            if(uuid_ptr && uuid_compare(*uuid_ptr, entries[i].uuid) == 0)
                successful++;
        }

        usec_t end_ut = now_monotonic_usec();
        double secs = (double)(end_ut - start_ut) / USEC_PER_SEC;
        double ops = (double)successful / secs;

        fprintf(stderr, "uuidmap_uuid_ptr()   : %.2f ops/sec (%.2f usec/op)\n",
                ops, (double)(end_ut - start_ut) / (double)successful);

        // Second benchmark: uuidmap_get_by_uuid()
        successful = 0;
        start_ut = now_monotonic_usec();

        for(size_t i = 0; i < ENTRIES; i++) {
            UUIDMAP_ID id = uuidmap_acquire_by_uuid(entries[i].uuid);
            if(id != 0) {
                successful++;
                uuidmap_free(id);  // Must free since get_by_uuid increases refcount
            }
        }

        end_ut = now_monotonic_usec();
        secs = (double)(end_ut - start_ut) / USEC_PER_SEC;
        ops = (double)successful / secs;

        fprintf(stderr, "uuidmap_acquire_by_uuid(): %.2f ops/sec (%.2f usec/op)\n",
                ops, (double)(end_ut - start_ut) / (double)successful);
    }

    // Phase 2: Delete everything
    fprintf(stderr, "\nDeleting all entries...\n");
    start_time = now_monotonic_usec();
    next_step = step;

    for(size_t i = 0; i < ENTRIES; i++) {
        if (i >= next_step) {
            fprintf(stderr, ".");
            next_step += step;
        }

        UUIDMAP_ID id = entries[i].id;
        char uuid_str[UUID_STR_LEN];
        uuid_unparse_lower(entries[i].uuid, uuid_str);

        // Test 1: Should exist
        nd_uuid_t *uuid_ptr = uuidmap_uuid_ptr(id);
        if(!uuid_ptr) {
            fprintf(stderr, "\nERROR [%zu]: UUID not found before deletion"
                            "\n  UUID: %s"
                            "\n  ID: %u\n",
                    i, uuid_str, id);
            errors++;
            continue;
        }

        char current_uuid_str[UUID_STR_LEN];
        uuid_unparse_lower(*uuid_ptr, current_uuid_str);
        if(uuid_compare(*uuid_ptr, entries[i].uuid) != 0) {
            fprintf(stderr, "\nERROR [%zu]: Retrieved UUID doesn't match before deletion"
                            "\n  Original UUID: %s"
                            "\n  Current UUID: %s"
                            "\n  ID: %u\n",
                    i, uuid_str, current_uuid_str, id);
            errors++;
        }

        // Test 2: Create again
        UUIDMAP_ID id2 = uuidmap_create(entries[i].uuid);
        if(id2 != id) {
            fprintf(stderr, "\nERROR [%zu]: Recreation returned different ID"
                            "\n  UUID: %s"
                            "\n  Original ID: %u"
                            "\n  New ID: %u\n",
                    i, uuid_str, id, id2);
            errors++;
        }

        // Test 3 & 4: Free three times (one extra from benchmark)
        uuidmap_free(id);
        uuidmap_free(id);
        uuidmap_free(id);

        // Test 5: Should be gone
        uuid_ptr = uuidmap_uuid_ptr(id);
        if(uuid_ptr != NULL) {
            char remaining_uuid_str[UUID_STR_LEN];
            uuid_unparse_lower(*uuid_ptr, remaining_uuid_str);
            fprintf(stderr, "\nERROR [%zu]: UUID still exists after final deletion"
                            "\n  Original UUID: %s"
                            "\n  Remaining UUID: %s"
                            "\n  ID: %u\n",
                    i, uuid_str, remaining_uuid_str, id);
            errors++;
        }
    }

    end_time = now_monotonic_usec();
    fprintf(stderr, "\nPhase 2 completed in %.2f seconds with %d errors\n",
            (double)(end_time - start_time) / (double)USEC_PER_SEC, errors);

    freez(entries);

    fprintf(stderr, "\nUUID Map test completed with %d total errors\n", errors);
    return errors;
}
