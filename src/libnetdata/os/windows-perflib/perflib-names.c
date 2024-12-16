// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"

#if defined(OS_WINDOWS)
#define REGISTRY_KEY "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\Perflib\\009"

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

static struct {
    SPINLOCK spinlock;
    size_t size;
    perfLibRegistryEntry **array;
    struct simple_hashtable_PERFLIB hashtable;
    FILETIME lastWriteTime;
} names_globals = {
    .spinlock = SPINLOCK_INITIALIZER,
    .size = 0,
    .array = NULL,
};

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
    if(id >= names_globals.size) {
        // increase the size of the array

        size_t old_size = names_globals.size;

        if(!names_globals.size)
            names_globals.size = 20000;
        else
            names_globals.size *= 2;

        names_globals.array = reallocz(names_globals.array, names_globals.size * sizeof(perfLibRegistryEntry *));

        memset(names_globals.array + old_size, 0, (names_globals.size - old_size) * sizeof(perfLibRegistryEntry *));
   }

    perfLibRegistryEntry *entry = names_globals.array[id];
    if(!entry)
        entry = names_globals.array[id] = (perfLibRegistryEntry *)calloc(1, sizeof(perfLibRegistryEntry));

    bool add_to_hash = false;
    if(key && !entry->key) {
        entry->key = strdup(key);
        add_to_hash = true;
    }

    if(help && !entry->help)
        entry->help = strdup(help);

    entry->id = id;

    if(add_to_hash)
        RegistryAddToHashTable_unsafe(entry);
}

const char *RegistryFindNameByID(DWORD id) {
    const char *s = "";
    spinlock_lock(&names_globals.spinlock);

    if(id < names_globals.size) {
        perfLibRegistryEntry *titleEntry = names_globals.array[id];
        if(titleEntry && titleEntry->key)
            s = titleEntry->key;
    }

    spinlock_unlock(&names_globals.spinlock);
    return s;
}

const char *RegistryFindHelpByID(DWORD id) {
    const char *s = "";
    spinlock_lock(&names_globals.spinlock);

    if(id < names_globals.size) {
        perfLibRegistryEntry *titleEntry = names_globals.array[id];
        if(titleEntry && titleEntry->help)
            s = titleEntry->help;
    }

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
    while (*ptr) {
        TCHAR *sid = ptr;  // First string is the ID
        ptr += lstrlen(ptr) + 1; // Move to the next string
        TCHAR *name = ptr;  // Second string is the name
        ptr += lstrlen(ptr) + 1; // Move to the next pair

        DWORD id = strtoul(sid, NULL, 10);

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
    simple_hashtable_init_PERFLIB(&names_globals.hashtable, 20000);
    RegistryKeyModification(&names_globals.lastWriteTime);
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

#endif // OS_WINDOWS
