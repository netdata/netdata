// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"

#if defined(OS_WINDOWS)
#define REGISTRY_KEY "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\Perflib\\009"
#include "libnetdata/libjudy/judyl-typed.h" // Judy array for efficient storage of sparse registry IDs

typedef struct perflib_registry {
    DWORD id;
    char *key;
    char *help;
} perfLibRegistryEntry;

static inline bool compare_perfLibRegistryEntry(const char *k1, const char *k2) {
    return strcmp(k1, k2) == 0;
}

static inline const char *value2key_perfLibRegistryEntry(perfLibRegistryEntry *entry) {
    return entry->key;
}

#define SIMPLE_HASHTABLE_COMPARE_KEYS_FUNCTION compare_perfLibRegistryEntry
#define SIMPLE_HASHTABLE_VALUE2KEY_FUNCTION value2key_perfLibRegistryEntry
#define SIMPLE_HASHTABLE_KEY_TYPE const char
#define SIMPLE_HASHTABLE_VALUE_TYPE perfLibRegistryEntry *
#define SIMPLE_HASHTABLE_NAME _PERFLIB
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// Define type for Judy array of perfLibRegistryEntry pointers
DEFINE_JUDYL_TYPED(PERFLIB_ENTRIES, perfLibRegistryEntry *);

static struct {
    SPINLOCK spinlock;
    struct simple_hashtable_PERFLIB hashtable;
    FILETIME lastWriteTime;
    PERFLIB_ENTRIES_JudyLSet registry_entries;
} names_globals = {
    .spinlock = SPINLOCK_INITIALIZER,
};

// Helper functions for registry entry access using Judy arrays

// Get entry for ID - returns NULL if not found
static inline perfLibRegistryEntry* registry_get_entry(DWORD id) {
    return PERFLIB_ENTRIES_GET(&names_globals.registry_entries, (Word_t)id);
}

// Set entry for ID - returns entry pointer
static inline perfLibRegistryEntry* registry_ensure_entry(DWORD id) {
    perfLibRegistryEntry *entry = PERFLIB_ENTRIES_GET(&names_globals.registry_entries, (Word_t)id);
    
    if(!entry) {
        entry = (perfLibRegistryEntry *)callocz(1, sizeof(perfLibRegistryEntry));
        
        if(!PERFLIB_ENTRIES_SET(&names_globals.registry_entries, (Word_t)id, entry)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Failed to store registry entry in Judy array");
            freez(entry);
            return NULL;
        }
    }
    
    return entry;
}

DWORD RegistryFindIDByName(const char *name) {
    DWORD rc = PERFLIB_REGISTRY_NAME_NOT_FOUND;

    spinlock_lock(&names_globals.spinlock);
    XXH64_hash_t hash = XXH3_64bits((void *)name, strlen(name));
    SIMPLE_HASHTABLE_SLOT_PERFLIB *sl = simple_hashtable_get_slot_PERFLIB(&names_globals.hashtable, hash, name, false);
    perfLibRegistryEntry *e = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(e) rc = e->id;
    spinlock_unlock(&names_globals.spinlock);

    return rc;
}

static inline void RegistryAddToHashTable_unsafe(perfLibRegistryEntry *entry) {
    XXH64_hash_t hash = XXH3_64bits((void *)entry->key, strlen(entry->key));
    SIMPLE_HASHTABLE_SLOT_PERFLIB *sl = simple_hashtable_get_slot_PERFLIB(&names_globals.hashtable, hash, entry->key, true);
    perfLibRegistryEntry *e = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!e || e->id > entry->id)
        simple_hashtable_set_slot_PERFLIB(&names_globals.hashtable, sl, hash, entry);
}

static void RegistrySetData_unsafe(DWORD id, const char *key, const char *help) {
    perfLibRegistryEntry *entry = registry_ensure_entry(id);
    if (!entry)
        return;  // ID was too large or allocation failed

    bool add_to_hash = false;
    if(key) {
        // Always update the key if provided
        if(entry->key) {
            // Only if the key actually changes, we need to update hash
            if(strcmp(entry->key, key) != 0) {
                freez(entry->key);
                entry->key = strdupz(key);
                add_to_hash = true;
            }
        }
        else {
            entry->key = strdupz(key);
            add_to_hash = true;
        }
    }

    if(help) {
        if(entry->help)
            freez(entry->help);
        entry->help = strdupz(help);
    }

    entry->id = id;

    if(add_to_hash)
        RegistryAddToHashTable_unsafe(entry);
}

const char *RegistryFindNameByID(DWORD id) {
    const char *s = "";
    spinlock_lock(&names_globals.spinlock);

    perfLibRegistryEntry *titleEntry = registry_get_entry(id);
    if(titleEntry && titleEntry->key)
        s = titleEntry->key;

    spinlock_unlock(&names_globals.spinlock);
    return s;
}

const char *RegistryFindHelpByID(DWORD id) {
    const char *s = "";
    spinlock_lock(&names_globals.spinlock);

    perfLibRegistryEntry *titleEntry = registry_get_entry(id);
    if(titleEntry && titleEntry->help)
        s = titleEntry->help;

    spinlock_unlock(&names_globals.spinlock);
    return s;
}

// ----------------------------------------------------------

static inline void readRegistryKeys_unsafe(BOOL helps) {
    TCHAR *pData = NULL;

    HKEY hKey;
    DWORD dwType;
    DWORD dwSize = 0;
    LONG lStatus;

    LPCSTR valueName;
    if(helps)
        valueName = TEXT("help");
    else
        valueName = TEXT("CounterDefinition");

    // Open the key for the English counters
    lStatus = RegOpenKeyEx(HKEY_LOCAL_MACHINE, TEXT(REGISTRY_KEY), 0, KEY_READ, &hKey);
    if (lStatus != ERROR_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Failed to open registry key HKEY_LOCAL_MACHINE, subkey '%s', error %ld\n", REGISTRY_KEY, (long)lStatus);
        return;
    }

    // Get the size of the 'Counters' data
    lStatus = RegQueryValueEx(hKey, valueName, NULL, &dwType, NULL, &dwSize);
    if (lStatus != ERROR_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Failed to get registry key HKEY_LOCAL_MACHINE, subkey '%s', value '%s', size of data, error %ld\n",
               REGISTRY_KEY, (const char *)valueName, (long)lStatus);
        goto cleanup;
    }

    // Allocate memory for the data
    pData = mallocz(dwSize);

    // Read the 'Counters' data
    lStatus = RegQueryValueEx(hKey, valueName, NULL, &dwType, (LPBYTE)pData, &dwSize);
    if (lStatus != ERROR_SUCCESS || dwType != REG_MULTI_SZ) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Failed to get registry key HKEY_LOCAL_MACHINE, subkey '%s', value '%s', data, error %ld\n",
               REGISTRY_KEY, (const char *)valueName, (long)lStatus);
        goto cleanup;
    }

    // Process the counter data
    TCHAR *ptr = pData;
    TCHAR *end_ptr = pData + dwSize;
    while (*ptr && ptr < end_ptr - 1) {
        TCHAR *sid = ptr;  // First string is the ID
        size_t sid_len = lstrlen(ptr);
        
        // Check for valid ID string
        if (sid_len == 0) {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Empty registry ID found, skipping");
            break;
        }
        
        // Check for buffer overrun
        if (ptr + sid_len + 1 >= end_ptr) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Registry data truncated after ID, aborting");
            break;
        }
        
        ptr += sid_len + 1; // Move to the next string
        
        TCHAR *name = ptr;  // Second string is the name
        size_t name_len = lstrlen(ptr);
        
        // Check for empty name
        if (name_len == 0) {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Empty registry name found, skipping");
            // Skip to next pair if possible
            ptr += 1;
            continue;
        }
        
        // Check for buffer overrun
        if (ptr + name_len + 1 > end_ptr) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Registry data truncated after name, aborting");
            break;
        }
        
        ptr += name_len + 1; // Move to the next pair
        
        // Convert ID to number with validation
        char *endptr;
        DWORD id = strtoul(sid, &endptr, 10);
        
        // Validate conversion was successful
        if (endptr == sid || *endptr != '\0') {
            nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Invalid registry ID format: '%s', skipping", sid);
            continue;
        }

        if(helps)
            RegistrySetData_unsafe(id, NULL, name);
        else
            RegistrySetData_unsafe(id, name, NULL);
    }

cleanup:
    if(pData) freez(pData);
    RegCloseKey(hKey);
}

static BOOL RegistryKeyModification(FILETIME *lastWriteTime) {
    HKEY hKey;
    LONG lResult;
    BOOL ret = FALSE;

    // Open the registry key
    lResult = RegOpenKeyEx(HKEY_LOCAL_MACHINE, TEXT(REGISTRY_KEY), 0, KEY_READ, &hKey);
    if (lResult != ERROR_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Failed to open registry key HKEY_LOCAL_MACHINE, subkey '%s', error %ld\n", REGISTRY_KEY, (long)lResult);
        return FALSE;
    }

    // Get the last write time
    lResult = RegQueryInfoKey(hKey, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, lastWriteTime);
    if (lResult != ERROR_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Failed to query registry key HKEY_LOCAL_MACHINE, subkey '%s', last write time, error %ld\n", REGISTRY_KEY, (long)lResult);
        ret = FALSE;
    }
    else
        ret = TRUE;

    RegCloseKey(hKey);
    return ret;
}

static inline void RegistryFetchAll_unsafe(void) {
    readRegistryKeys_unsafe(FALSE);
    readRegistryKeys_unsafe(TRUE);
}

void PerflibNamesRegistryInitialize(void) {
    spinlock_lock(&names_globals.spinlock);
    
    // Initialize the hashtable
    simple_hashtable_init_PERFLIB(&names_globals.hashtable, 20000);

    // Initialize Judy array for registry entries
    PERFLIB_ENTRIES_INIT(&names_globals.registry_entries);
    
    if(!RegistryKeyModification(&names_globals.lastWriteTime)) {
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to get registry last modification time");
        // Continue despite this error - we can still try to fetch registry data
    }
    
    RegistryFetchAll_unsafe();
    
    spinlock_unlock(&names_globals.spinlock);
}

void PerflibNamesRegistryUpdate(void) {
    FILETIME lastWriteTime = { 0 };
    RegistryKeyModification(&lastWriteTime);

    if(CompareFileTime(&lastWriteTime, &names_globals.lastWriteTime) > 0) {
        spinlock_lock(&names_globals.spinlock);
        if(CompareFileTime(&lastWriteTime, &names_globals.lastWriteTime) > 0) {
            names_globals.lastWriteTime = lastWriteTime;
            RegistryFetchAll_unsafe();
        }
        spinlock_unlock(&names_globals.spinlock);
    }
}

// Helper to free registry entry memory when Judy array is freed
static void free_registry_entry(Word_t idx, perfLibRegistryEntry *entry, void *data) {
    (void)idx;
    (void)data;
    
    if(entry) {
        if(entry->key) freez(entry->key);
        if(entry->help) freez(entry->help);
        freez(entry);
    }
}

// Cleanup function to be called during shutdown to free allocated resources
void PerflibNamesRegistryCleanup(void) {
    spinlock_lock(&names_globals.spinlock);
    
    // Free the Judy array and all registry entries
    PERFLIB_ENTRIES_FREE(&names_globals.registry_entries, free_registry_entry, NULL);
    
    // Free the hashtable
    simple_hashtable_destroy_PERFLIB(&names_globals.hashtable);
    
    spinlock_unlock(&names_globals.spinlock);
}

// Callback for collecting statistics from Judy array
struct judy_stats {
    DWORD count;
    DWORD min_id;
    DWORD max_id;
    uint64_t sum_id;
    DWORD *all_ids;
    size_t ids_capacity;
    size_t ids_count;
};

static void collect_judy_stats(Word_t idx, perfLibRegistryEntry *entry, void *data) {
    struct judy_stats *stats = (struct judy_stats *)data;
    
    // Skip null entries
    if (!entry)
        return;
    
    stats->count++;
    
    // Update min/max
    DWORD id = (DWORD)idx;
    if (stats->count == 1 || id < stats->min_id)
        stats->min_id = id;
    if (id > stats->max_id)
        stats->max_id = id;
    
    // Update sum for average calculation
    stats->sum_id += id;
    
    // Store ID if there's space
    if (stats->all_ids && stats->ids_count < stats->ids_capacity) {
        stats->all_ids[stats->ids_count++] = id;
    }
}

// Unit test for perflib-names functionality
int perflibnamestest_main(void) {
    fprintf(stderr, "Running perflib-names unit tests...\n");
    
    int errors = 0;
    
    // PART 1: Analyze real registry data
    // Initialize the registry - this loads actual Windows registry data
    fprintf(stderr, "\n--- Real Registry Data Analysis ---\n");
    PerflibNamesRegistryInitialize();
    
    // Collect statistics about the real registry data
    struct judy_stats real_stats = {
        .count = 0,
        .min_id = 0,
        .max_id = 0,
        .sum_id = 0,
        .all_ids = (DWORD *)mallocz(100 * sizeof(DWORD)),  // Allocate space for up to 100 IDs
        .ids_capacity = 100,
        .ids_count = 0
    };
    
    fprintf(stderr, "Analyzing real Windows registry performance counter data...\n");
    spinlock_lock(&names_globals.spinlock);
    PERFLIB_ENTRIES_FREE(&names_globals.registry_entries, collect_judy_stats, &real_stats);
    spinlock_unlock(&names_globals.spinlock);
    
    // Print the real-world statistics
    fprintf(stderr, "Real Registry Statistics:\n");
    fprintf(stderr, "  Total entries: %u\n", real_stats.count);
    fprintf(stderr, "  ID range: %u to %u\n", real_stats.min_id, real_stats.max_id);
    
    // Calculate sparseness metrics for real data
    if (real_stats.count > 0) {
        double avg_id = (double)real_stats.sum_id / real_stats.count;
        double theoretical_density = (double)real_stats.count / (real_stats.max_id - real_stats.min_id + 1) * 100.0;
        
        fprintf(stderr, "  Average ID: %.2f\n", avg_id);
        fprintf(stderr, "  Range width: %u\n", real_stats.max_id - real_stats.min_id + 1);
        fprintf(stderr, "  Density: %.2f%%\n", theoretical_density);
        fprintf(stderr, "  Sparseness: %.2f%%\n", 100.0 - theoretical_density);
        
        // Print sample of real IDs to show distribution
        fprintf(stderr, "  Sample IDs (up to 100): ");
        for (size_t i = 0; i < real_stats.ids_count && i < 100; i++) {
            fprintf(stderr, "%u ", real_stats.all_ids[i]);
        }
        fprintf(stderr, "\n");
    }
    
    // Free allocated memory
    freez(real_stats.all_ids);
    
    // PART 2: Clean registry and run isolated tests
    fprintf(stderr, "\n--- Isolated Test Environment ---\n");
    
    // Clean up previous registry data
    PerflibNamesRegistryCleanup();
    
    // Initialize a fresh, empty registry
    spinlock_lock(&names_globals.spinlock);
    simple_hashtable_init_PERFLIB(&names_globals.hashtable, 20000);
    PERFLIB_ENTRIES_INIT(&names_globals.registry_entries);
    spinlock_unlock(&names_globals.spinlock);
    
    // Test 1: Add and retrieve registry entries
    fprintf(stderr, "Test 1: Adding and retrieving registry entries...\n");
    spinlock_lock(&names_globals.spinlock);
    
    // Use test IDs
    const DWORD test_id1 = 1001;
    const char *test_key1 = "TestKey1";
    const char *test_help1 = "TestHelp1";
    RegistrySetData_unsafe(test_id1, test_key1, test_help1);
    
    // Test with another ID
    const DWORD test_id2 = 2001;
    const char *test_key2 = "TestKey2";
    const char *test_help2 = "TestHelp2";
    RegistrySetData_unsafe(test_id2, test_key2, test_help2);
    
    // Add a few more entries to demonstrate sparseness
    RegistrySetData_unsafe(5001, "Key5001", "Help5001");
    RegistrySetData_unsafe(10001, "Key10001", "Help10001");
    RegistrySetData_unsafe(50001, "Key50001", "Help50001");
    RegistrySetData_unsafe(100001, "Key100001", "Help100001");
    
    spinlock_unlock(&names_globals.spinlock);
    
    // Test lookup by ID
    const char *result_key1 = RegistryFindNameByID(test_id1);
    if (strcmp(result_key1, test_key1) != 0) {
        fprintf(stderr, "FAILED: RegistryFindNameByID(%u) returned '%s', expected '%s'\n", 
                (unsigned)test_id1, result_key1, test_key1);
        errors++;
    }
    
    const char *result_help1 = RegistryFindHelpByID(test_id1);
    if (strcmp(result_help1, test_help1) != 0) {
        fprintf(stderr, "FAILED: RegistryFindHelpByID(%u) returned '%s', expected '%s'\n", 
                (unsigned)test_id1, result_help1, test_help1);
        errors++;
    }
    
    // Test lookup of second ID
    const char *result_key2 = RegistryFindNameByID(test_id2);
    if (strcmp(result_key2, test_key2) != 0) {
        fprintf(stderr, "FAILED: RegistryFindNameByID(%u) returned '%s', expected '%s'\n", 
                (unsigned)test_id2, result_key2, test_key2);
        errors++;
    }
    
    // Test 2: Lookup by name
    fprintf(stderr, "Test 2: Looking up registry entries by name...\n");
    DWORD result_id1 = RegistryFindIDByName(test_key1);
    if (result_id1 != test_id1) {
        fprintf(stderr, "FAILED: RegistryFindIDByName('%s') returned %u, expected %u\n", 
                test_key1, (unsigned)result_id1, (unsigned)test_id1);
        errors++;
    }
    
    // Test 3: Lookup non-existent entry
    fprintf(stderr, "Test 3: Looking up non-existent entries...\n");
    const char *result_nonexistent = RegistryFindNameByID(999999);
    if (strcmp(result_nonexistent, "") != 0) {
        fprintf(stderr, "FAILED: RegistryFindNameByID(999999) returned '%s', expected ''\n", 
                result_nonexistent);
        errors++;
    }
    
    DWORD result_id_nonexistent = RegistryFindIDByName("NonExistentKey");
    if (result_id_nonexistent != PERFLIB_REGISTRY_NAME_NOT_FOUND) {
        fprintf(stderr, "FAILED: RegistryFindIDByName('NonExistentKey') returned %u, expected %u\n", 
                (unsigned)result_id_nonexistent, (unsigned)PERFLIB_REGISTRY_NAME_NOT_FOUND);
        errors++;
    }
    
    // Test 4: Update entry
    fprintf(stderr, "Test 4: Updating existing entries...\n");
    spinlock_lock(&names_globals.spinlock);
    const char *test_help1_updated = "UpdatedHelp1";
    RegistrySetData_unsafe(test_id1, NULL, test_help1_updated);
    spinlock_unlock(&names_globals.spinlock);
    
    const char *result_help1_updated = RegistryFindHelpByID(test_id1);
    if (strcmp(result_help1_updated, test_help1_updated) != 0) {
        fprintf(stderr, "FAILED: RegistryFindHelpByID(%u) after update returned '%s', expected '%s'\n", 
                (unsigned)test_id1, result_help1_updated, test_help1_updated);
        errors++;
    }
    
    // Collect and print statistics about our test data Judy array
    fprintf(stderr, "\nTest Judy Array Statistics:\n");
    struct judy_stats test_stats = {
        .count = 0,
        .min_id = 0,
        .max_id = 0,
        .sum_id = 0,
        .all_ids = (DWORD *)mallocz(100 * sizeof(DWORD)),  // Allocate space for up to 100 IDs
        .ids_capacity = 100,
        .ids_count = 0
    };
    
    spinlock_lock(&names_globals.spinlock);
    PERFLIB_ENTRIES_FREE(&names_globals.registry_entries, collect_judy_stats, &test_stats);
    spinlock_unlock(&names_globals.spinlock);
    
    // Print the collected statistics
    fprintf(stderr, "  Total entries: %u\n", test_stats.count);
    fprintf(stderr, "  ID range: %u to %u\n", test_stats.min_id, test_stats.max_id);
    
    // Calculate sparseness metrics
    if (test_stats.count > 0) {
        double avg_id = (double)test_stats.sum_id / test_stats.count;
        double theoretical_density = (double)test_stats.count / (test_stats.max_id - test_stats.min_id + 1) * 100.0;
        
        fprintf(stderr, "  Average ID: %.2f\n", avg_id);
        fprintf(stderr, "  Range width: %u\n", test_stats.max_id - test_stats.min_id + 1);
        fprintf(stderr, "  Density: %.2f%%\n", theoretical_density);
        fprintf(stderr, "  Sparseness: %.2f%%\n", 100.0 - theoretical_density);
        
        // Print all IDs to show distribution
        fprintf(stderr, "  IDs in array: ");
        for (size_t i = 0; i < test_stats.ids_count; i++) {
            fprintf(stderr, "%u ", test_stats.all_ids[i]);
        }
        fprintf(stderr, "\n");
    }
    
    // Free allocated memory
    freez(test_stats.all_ids);
    
    // Clean up
    PerflibNamesRegistryCleanup();
    
    // Report results
    if (errors == 0) {
        fprintf(stderr, "\nAll perflib-names tests passed!\n");
        return 0;
    } else {
        fprintf(stderr, "\n%d perflib-names tests failed.\n", errors);
        return 1;
    }
}

#endif // OS_WINDOWS
