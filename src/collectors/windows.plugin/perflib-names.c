// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"
#include "windows-internals.h"

#define REGISTRY_KEY "SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\\Perflib\\009"

typedef struct perflib_registry {
    DWORD id;
    char *key;
    char *help;
} perfLibRegistryEntry;

static struct {
    SPINLOCK spinlock;
    size_t size;
    perfLibRegistryEntry **array;
    FILETIME lastWriteTime;
} names_globals = {
    .spinlock = NETDATA_SPINLOCK_INITIALIZER,
    .size = 0,
    .array = NULL,
};

static void RegistrySetData_unsafe(DWORD id, const char *key, const char *help) {
    if(id >= names_globals.size) {
        // increase the size of the array

        if(!names_globals.size)
            names_globals.size = 20000;
        else
            names_globals.size *= 2;

        names_globals.array = reallocz(names_globals.array, names_globals.size * sizeof(perfLibRegistryEntry *));
   }

    perfLibRegistryEntry *entry = names_globals.array[id];
    if(!entry)
        entry = names_globals.array[id] = (perfLibRegistryEntry *)calloc(1, sizeof(perfLibRegistryEntry));

    if(key && !entry->key)
        entry->key = strdup(key);

    if(help && !entry->help)
        entry->help = strdup(help);

    entry->id = id;
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

void RegistryInitialize(void) {
    spinlock_lock(&names_globals.spinlock);
    RegistryKeyModification(&names_globals.lastWriteTime);
    readRegistryKeys_unsafe(FALSE);
    readRegistryKeys_unsafe(TRUE);
    spinlock_unlock(&names_globals.spinlock);
}
void RegistryUpdate(void) {
    FILETIME lastWriteTime = { 0 };
    RegistryKeyModification(&lastWriteTime);

    if(CompareFileTime(&lastWriteTime, &names_globals.lastWriteTime) > 0)
        RegistryInitialize();
}
