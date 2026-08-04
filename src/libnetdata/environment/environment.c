// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_NATIVE_ENVIRONMENT_ACCESS 1
#include "libnetdata/libnetdata.h"

#if defined(OS_MACOS)
#include <crt_externs.h>
#endif

#if !defined(OS_MACOS)
extern char **environ;
#endif

#define ND_ENVIRONMENT_MAGIC UINT64_C(0x4e44454e5649524f)

typedef struct nd_env_entry {
    struct nd_env_entry *prev;
    struct nd_env_entry *next;

    char *raw;
    char *name;
    char *value;
#if defined(OS_WINDOWS)
    bool managed_from_crt;
#endif
} ND_ENV_ENTRY;

typedef struct nd_env_index {
    ND_ENV_ENTRY *first;
} ND_ENV_INDEX;

struct nd_env_snapshot {
    size_t references;
    uint64_t generation;

    size_t entries;
    char **envp;

    char *windows_block;
    size_t windows_block_size;
};

struct nd_environment {
    uint64_t magic;
    bool process_global;
    bool mirrors_native;

    netdata_mutex_t mutation_mutex;
    SPINLOCK publication_lock;

    DICTIONARY *index;
    ND_ENV_ENTRY *entries;
    size_t entries_count;

    uint64_t generation;
    ND_ENV_SNAPSHOT *published;
};

typedef enum nd_environment_process_state {
    ND_ENVIRONMENT_UNINITIALIZED = 0,
    ND_ENVIRONMENT_IMPORTING = 1,
    ND_ENVIRONMENT_INITIALIZING = 2,
    ND_ENVIRONMENT_PROCESS_FROZEN = 3,
} ND_ENVIRONMENT_PROCESS_STATE;

static ND_ENVIRONMENT *process_environment = NULL;
static int process_environment_state = ND_ENVIRONMENT_UNINITIALIZED;

static int test_fail_native_once = 0;
static int test_fail_snapshot_once = 0;

// --------------------------------------------------------------------------------------------------------------------
// Platform identity and native enumeration

static char **environment_native_environ(void) {
#if defined(OS_MACOS)
    return *_NSGetEnviron();
#else
    return environ;
#endif
}

static bool environment_name_is_valid(const char *name) {
    return name && *name && !strchr(name, '=');
}

static bool environment_raw_parse(const char *raw, const char **value, size_t *name_length) {
    if(!raw || !*raw)
        return false;

    const char *equals = strchr(raw, '=');
    if(!equals || equals == raw)
        return false;

    if(value)
        *value = equals + 1;
    if(name_length)
        *name_length = (size_t)(equals - raw);

    return true;
}

static char *environment_ascii_casefold(const char *name) {
    size_t length = strlen(name);
    char *key = mallocz(length + 1);

    for(size_t i = 0; i < length; i++)
        key[i] = (char)tolower((unsigned char)name[i]);

    key[length] = '\0';
    return key;
}

#if defined(OS_WINDOWS)
static wchar_t *environment_windows_wide(const char *text, UINT code_page) {
    int length = MultiByteToWideChar(code_page, 0, text, -1, NULL, 0);
    if(length <= 0)
        return NULL;

    wchar_t *wide = mallocz((size_t)length * sizeof(*wide));
    if(MultiByteToWideChar(code_page, 0, text, -1, wide, length) <= 0) {
        freez(wide);
        return NULL;
    }

    return wide;
}

static char *environment_windows_narrow(const wchar_t *wide, UINT code_page) {
    int length = WideCharToMultiByte(code_page, 0, wide, -1, NULL, 0, NULL, NULL);
    if(length <= 0)
        return NULL;

    char *text = mallocz((size_t)length);
    if(WideCharToMultiByte(code_page, 0, wide, -1, text, length, NULL, NULL) <= 0) {
        freez(text);
        return NULL;
    }

    return text;
}

static char *environment_windows_transcode(const char *text, UINT from, UINT to) {
    wchar_t *wide = environment_windows_wide(text, from);
    if(!wide)
        return NULL;

    char *converted = environment_windows_narrow(wide, to);
    freez(wide);
    return converted;
}

static char *environment_key(const char *name) {
    wchar_t *wide = environment_windows_wide(name, CP_ACP);
    if(!wide)
        return environment_ascii_casefold(name);

    int wide_length = (int)wcslen(wide) + 1;
    wchar_t *lower = mallocz((size_t)wide_length * sizeof(*lower));
    int mapped = LCMapStringEx(
        LOCALE_NAME_INVARIANT, LCMAP_LOWERCASE, wide, wide_length, lower, wide_length, NULL, NULL, 0);
    freez(wide);

    if(mapped <= 0) {
        freez(lower);
        return environment_ascii_casefold(name);
    }

    int key_length = WideCharToMultiByte(CP_UTF8, 0, lower, -1, NULL, 0, NULL, NULL);
    if(key_length <= 0) {
        freez(lower);
        return environment_ascii_casefold(name);
    }

    char *key = mallocz((size_t)key_length);
    if(WideCharToMultiByte(CP_UTF8, 0, lower, -1, key, key_length, NULL, NULL) <= 0) {
        freez(lower);
        freez(key);
        return environment_ascii_casefold(name);
    }

    freez(lower);
    return key;
}
#else
static char *environment_key(const char *name) {
    return strdupz(name);
}
#endif

static bool environment_names_equal(const char *left, const char *right) {
#if defined(OS_WINDOWS)
    char *left_key = environment_key(left);
    char *right_key = environment_key(right);
    bool equal = strcmp(left_key, right_key) == 0;
    freez(left_key);
    freez(right_key);
    return equal;
#else
    return strcmp(left, right) == 0;
#endif
}

#if defined(OS_WINDOWS)
static bool environment_windows_path_list(const char *name) {
    return environment_names_equal(name, "PATH") ||
           environment_names_equal(name, "LD_LIBRARY_PATH") ||
           environment_names_equal(name, "ORIGINAL_PATH");
}

static bool environment_windows_single_path(const char *name) {
    return environment_names_equal(name, "HOME") ||
           environment_names_equal(name, "SHELL") ||
           environment_names_equal(name, "TMPDIR") ||
           environment_names_equal(name, "TMP") ||
           environment_names_equal(name, "TEMP");
}
#endif

static char *environment_spawn_value_from_managed(const char *name, const char *value) {
#if defined(OS_WINDOWS)
    bool list = environment_windows_path_list(name);
    bool path = environment_windows_single_path(name);

    if(!list && !path)
        return environment_windows_transcode(value, CP_ACP, CP_OEMCP);

    // MSYS leaves an empty SHELL unchanged instead of converting it.
    if(!*value && environment_names_equal(name, "SHELL"))
        return environment_windows_transcode(value, CP_ACP, CP_OEMCP);

    cygwin_conv_path_t conversion = CCP_POSIX_TO_WIN_W | (list ? CCP_RELATIVE : CCP_ABSOLUTE);
    ssize_t required = list
        ? cygwin_conv_path_list(conversion, value, NULL, 0)
        : cygwin_conv_path(conversion, value, NULL, 0);
    if(required <= 0)
        return NULL;

    wchar_t *wide = mallocz((size_t)required);
    ssize_t result = list
        ? cygwin_conv_path_list(conversion, value, wide, (size_t)required)
        : cygwin_conv_path(conversion, value, wide, (size_t)required);
    if(result != 0) {
        freez(wide);
        return NULL;
    }

    char *converted = environment_windows_narrow(wide, CP_OEMCP);
    freez(wide);
    return converted;
#else
    (void)name;
    return strdupz(value);
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// Owned ordered entries and index

static ND_ENV_ENTRY *environment_entry_from_raw(const char *raw) {
    ND_ENV_ENTRY *entry = callocz(1, sizeof(*entry));
    entry->raw = strdupz(raw ? raw : "");

    const char *value = NULL;
    size_t name_length = 0;
    if(environment_raw_parse(entry->raw, &value, &name_length)) {
        entry->name = strndupz(entry->raw, name_length);
        entry->value = strdupz(value);
    }

    return entry;
}

#if defined(OS_WINDOWS)
static ND_ENV_ENTRY *environment_entry_from_windows_native_raw(const char *raw) {
    ND_ENV_ENTRY *entry = callocz(1, sizeof(*entry));
    entry->raw = strdupz(raw ? raw : "");

    const char *value_oem = NULL;
    size_t name_length = 0;
    if(environment_raw_parse(entry->raw, &value_oem, &name_length)) {
        char *name_oem = strndupz(entry->raw, name_length);
        entry->name = environment_windows_transcode(name_oem, CP_OEMCP, CP_ACP);
        entry->value = environment_windows_transcode(value_oem, CP_OEMCP, CP_ACP);
        freez(name_oem);

        if(!entry->name || !entry->value) {
            freez(entry->name);
            freez(entry->value);
            entry->name = NULL;
            entry->value = NULL;
        }
    }

    return entry;
}
#endif

static char *environment_compose_raw(const char *name, const char *value) {
    size_t name_length = strlen(name);
    size_t value_length = strlen(value);

    if(unlikely(name_length > SIZE_MAX - value_length - 2)) {
        errno = EOVERFLOW;
        return NULL;
    }

    char *raw = mallocz(name_length + value_length + 2);
    memcpy(raw, name, name_length);
    raw[name_length] = '=';
    memcpy(&raw[name_length + 1], value, value_length + 1);
    return raw;
}

static char *environment_spawn_raw_from_managed(const char *name, const char *value) {
#if defined(OS_WINDOWS)
    char *name_oem = environment_windows_transcode(name, CP_ACP, CP_OEMCP);
    char *value_oem = environment_spawn_value_from_managed(name, value);
    if(!name_oem || !value_oem) {
        freez(name_oem);
        freez(value_oem);
        return NULL;
    }

    char *raw = environment_compose_raw(name_oem, value_oem);
    freez(name_oem);
    freez(value_oem);
    return raw;
#else
    return environment_compose_raw(name, value);
#endif
}

static ND_ENV_ENTRY *environment_entry_from_name_values(
    const char *name, const char *managed_value, char *raw) {
    if(!raw)
        return NULL;

    ND_ENV_ENTRY *entry = callocz(1, sizeof(*entry));
    entry->raw = raw;
    entry->name = strdupz(name);
    entry->value = strdupz(managed_value);
    return entry;
}

static void environment_entry_free(ND_ENV_ENTRY *entry) {
    if(!entry)
        return;

    freez(entry->raw);
    freez(entry->name);
    freez(entry->value);
    freez(entry);
}

static void environment_entry_append(ND_ENVIRONMENT *environment, ND_ENV_ENTRY *entry) {
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(environment->entries, entry, prev, next);
    environment->entries_count++;
}

static void environment_entries_clear(ND_ENVIRONMENT *environment) {
    ND_ENV_ENTRY *entry = environment->entries;
    while(entry) {
        ND_ENV_ENTRY *next = entry->next;
        environment_entry_free(entry);
        entry = next;
    }

    environment->entries = NULL;
    environment->entries_count = 0;
}

static DICTIONARY *environment_index_create(void) {
    return dictionary_create_advanced(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
        NULL,
        sizeof(ND_ENV_INDEX));
}

static void environment_index_rebuild(ND_ENVIRONMENT *environment) {
    DICTIONARY *index = environment_index_create();

    for(ND_ENV_ENTRY *entry = environment->entries; entry; entry = entry->next) {
        if(!entry->name)
            continue;

        char *key = environment_key(entry->name);
        ND_ENV_INDEX value = { .first = entry };
        dictionary_set(index, key, &value, sizeof(value));
        freez(key);
    }

    DICTIONARY *old = environment->index;
    environment->index = index;
    if(old)
        dictionary_destroy(old);
}

static ND_ENV_ENTRY *environment_index_get(ND_ENVIRONMENT *environment, const char *name) {
    char *key = environment_key(name);
    ND_ENV_INDEX *indexed = dictionary_get(environment->index, key);
    freez(key);
    return indexed ? indexed->first : NULL;
}

static ND_ENV_ENTRY *environment_find_linear(ND_ENVIRONMENT *environment, const char *name) {
    for(ND_ENV_ENTRY *entry = environment->entries; entry; entry = entry->next) {
        if(entry->name && environment_names_equal(entry->name, name))
            return entry;
    }

    return NULL;
}

static int environment_import_vector(ND_ENVIRONMENT *environment, const char *const envp[]) {
    if(!envp)
        return 0;

    for(size_t i = 0; envp[i]; i++) {
        ND_ENV_ENTRY *entry = environment_entry_from_raw(envp[i]);
#if defined(OS_WINDOWS)
        if(entry->name) {
            char *raw = environment_spawn_raw_from_managed(entry->name, entry->value);
            if(!raw) {
                environment_entry_free(entry);
                return -1;
            }

            freez(entry->raw);
            entry->raw = raw;
        }
#endif
        environment_entry_append(environment, entry);
    }

    return 0;
}

#if defined(OS_WINDOWS)
static bool environment_windows_crt_drive_entry(const char *raw) {
    return raw && raw[0] == '!' && raw[1] && raw[2] && raw[3] == '=' &&
           ((isalpha((unsigned char)raw[1]) && raw[2] == ':') || (raw[1] == ':' && raw[2] == ':'));
}

static int environment_import_windows_native(ND_ENVIRONMENT *environment) {
    LPCH block = GetEnvironmentStringsA();
    if(!block) {
        errno = EIO;
        return -1;
    }

    for(const char *entry = block; *entry; entry += strlen(entry) + 1)
        environment_entry_append(environment, environment_entry_from_windows_native_raw(entry));

    FreeEnvironmentStringsA(block);

    // MSYS keeps a separate POSIX environment. Append ordinary variables absent
    // from the Win32 block while retaining native spelling, order, and path values.
    char **crt = environment_native_environ();
    if(crt) {
        for(size_t i = 0; crt[i]; i++) {
            if(environment_windows_crt_drive_entry(crt[i]))
                continue;

            const char *value = NULL;
            size_t name_length = 0;
            if(!environment_raw_parse(crt[i], &value, &name_length))
                continue;

            char *name = strndupz(crt[i], name_length);
            ND_ENV_ENTRY *native_entry = environment_find_linear(environment, name);
            if(!native_entry) {
                char *raw = environment_spawn_raw_from_managed(name, value);
                if(!raw) {
                    freez(name);
                    errno = EILSEQ;
                    return -1;
                }
                native_entry = environment_entry_from_name_values(name, value, raw);
                native_entry->managed_from_crt = true;
                environment_entry_append(environment, native_entry);
            }
            else if(!native_entry->managed_from_crt && strcmp(native_entry->value, value) != 0) {
                // Managed reads follow the MSYS/CRT representation. The native
                // raw entry remains the child-facing Win32 representation.
                freez(native_entry->value);
                native_entry->value = strdupz(value);
            }
            native_entry->managed_from_crt = true;
            freez(name);
        }
    }

    return 0;
}
#endif

static ND_ENVIRONMENT *environment_create_empty(bool process_global, bool mirrors_native) {
    ND_ENVIRONMENT *environment = callocz(1, sizeof(*environment));
    if(netdata_mutex_init(&environment->mutation_mutex) != 0) {
        freez(environment);
        errno = EAGAIN;
        return NULL;
    }

    spinlock_init(&environment->publication_lock);
    environment->index = environment_index_create();
    environment->process_global = process_global;
    environment->mirrors_native = mirrors_native;
    environment->generation = 1;
    environment->magic = ND_ENVIRONMENT_MAGIC;
    return environment;
}

static ND_ENVIRONMENT *environment_create_from_vector(
    const char *const envp[], bool process_global, bool mirrors_native) {
    ND_ENVIRONMENT *environment = environment_create_empty(process_global, mirrors_native);
    if(!environment)
        return NULL;

    if(environment_import_vector(environment, envp) != 0) {
        dictionary_destroy(environment->index);
        netdata_mutex_destroy(&environment->mutation_mutex);
        environment_entries_clear(environment);
        freez(environment);
        return NULL;
    }
    environment_index_rebuild(environment);
    return environment;
}

static ND_ENVIRONMENT *environment_create_process_from_native(void) {
#if defined(OS_WINDOWS)
    ND_ENVIRONMENT *environment = environment_create_empty(true, true);
    if(!environment)
        return NULL;

    if(environment_import_windows_native(environment) != 0) {
        dictionary_destroy(environment->index);
        netdata_mutex_destroy(&environment->mutation_mutex);
        environment_entries_clear(environment);
        freez(environment);
        return NULL;
    }

    environment_index_rebuild(environment);
    return environment;
#else
    return environment_create_from_vector((const char *const *)environment_native_environ(), true, true);
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// Native mutation during the initialization phase

#if defined(OS_WINDOWS)
static int environment_windows_native_get_dup(const char *name, bool *present, char **value) {
    *present = false;
    *value = NULL;

    SetLastError(ERROR_SUCCESS);
    DWORD required = GetEnvironmentVariableA(name, NULL, 0);
    if(!required) {
        DWORD error = GetLastError();
        if(error == ERROR_ENVVAR_NOT_FOUND)
            return 0;

        if(error != ERROR_SUCCESS) {
            errno = EIO;
            return -1;
        }

        *present = true;
        *value = strdupz("");
        return 0;
    }

    char *copy = mallocz(required);
    SetLastError(ERROR_SUCCESS);
    DWORD copied = GetEnvironmentVariableA(name, copy, required);
    if(!copied && GetLastError() != ERROR_SUCCESS) {
        freez(copy);
        errno = EIO;
        return -1;
    }

    *present = true;
    *value = copy;
    return 0;
}

static int environment_windows_native_raw_get_dup(const char *name, char **raw) {
    *raw = NULL;

    LPCH block = GetEnvironmentStringsA();
    if(!block) {
        errno = EIO;
        return -1;
    }

    for(const char *candidate = block; *candidate; candidate += strlen(candidate) + 1) {
        size_t name_length = 0;
        if(!environment_raw_parse(candidate, NULL, &name_length))
            continue;

        char *name_oem = strndupz(candidate, name_length);
        char *name_acp = environment_windows_transcode(name_oem, CP_OEMCP, CP_ACP);
        freez(name_oem);

        bool matches = name_acp && environment_names_equal(name_acp, name);
        freez(name_acp);
        if(matches) {
            *raw = strdupz(candidate);
            break;
        }
    }

    FreeEnvironmentStringsA(block);
    if(!*raw) {
        errno = ENOENT;
        return -1;
    }

    return 0;
}

static void environment_windows_restore_crt(const char *name, const char *old_value) {
    if(old_value)
        setenv(name, old_value, 1);
    else
        unsetenv(name);
}

static int environment_native_set(const char *name, const char *value, char **managed_value, char **spawn_raw) {
    if(__atomic_exchange_n(&test_fail_native_once, 0, __ATOMIC_ACQ_REL)) {
        errno = EIO;
        return -1;
    }

    const char *crt_borrowed = getenv(name);
    char *old_crt = crt_borrowed ? strdupz(crt_borrowed) : NULL;
    bool old_native_present = false;
    char *old_native = NULL;
    if(environment_windows_native_get_dup(name, &old_native_present, &old_native) != 0) {
        freez(old_crt);
        return -1;
    }

    if(!SetEnvironmentVariableA(name, value)) {
        freez(old_crt);
        freez(old_native);
        errno = EIO;
        return -1;
    }

    if(setenv(name, value, 1) != 0) {
        int saved_errno = errno ? errno : EIO;
        environment_windows_restore_crt(name, old_crt);
        SetEnvironmentVariableA(name, old_native_present ? old_native : NULL);
        freez(old_crt);
        freez(old_native);
        errno = saved_errno;
        return -1;
    }

    const char *crt_current = getenv(name);
    if(!crt_current) {
        environment_windows_restore_crt(name, old_crt);
        SetEnvironmentVariableA(name, old_native_present ? old_native : NULL);
        freez(old_crt);
        freez(old_native);
        errno = EIO;
        return -1;
    }

    bool now_present = false;
    char *native_value = NULL;
    char *verified_raw = NULL;
    if(environment_windows_native_get_dup(name, &now_present, &native_value) != 0 || !now_present ||
       environment_windows_native_raw_get_dup(name, &verified_raw) != 0) {
        int saved_errno = errno ? errno : EIO;
        environment_windows_restore_crt(name, old_crt);
        SetEnvironmentVariableA(name, old_native_present ? old_native : NULL);
        freez(old_crt);
        freez(old_native);
        freez(native_value);
        freez(verified_raw);
        errno = saved_errno;
        return -1;
    }

    if(environment_windows_path_list(name) || environment_windows_single_path(name)) {
        *spawn_raw = environment_spawn_raw_from_managed(name, crt_current);
        if(!*spawn_raw) {
            int saved_errno = errno ? errno : EILSEQ;
            environment_windows_restore_crt(name, old_crt);
            SetEnvironmentVariableA(name, old_native_present ? old_native : NULL);
            freez(old_crt);
            freez(old_native);
            freez(native_value);
            freez(verified_raw);
            errno = saved_errno;
            return -1;
        }
        freez(verified_raw);
    }
    else
        *spawn_raw = verified_raw;

    freez(old_crt);
    freez(old_native);
    freez(native_value);
    *managed_value = strdupz(crt_current);
    return 0;
}

static int environment_native_unset(const char *name) {
    if(__atomic_exchange_n(&test_fail_native_once, 0, __ATOMIC_ACQ_REL)) {
        errno = EIO;
        return -1;
    }

    const char *crt_borrowed = getenv(name);
    char *old_crt = crt_borrowed ? strdupz(crt_borrowed) : NULL;
    bool old_native_present = false;
    char *old_native = NULL;
    if(environment_windows_native_get_dup(name, &old_native_present, &old_native) != 0) {
        freez(old_crt);
        return -1;
    }

    if(!SetEnvironmentVariableA(name, NULL)) {
        freez(old_crt);
        freez(old_native);
        errno = EIO;
        return -1;
    }

    if(unsetenv(name) != 0) {
        int saved_errno = errno ? errno : EIO;
        environment_windows_restore_crt(name, old_crt);
        SetEnvironmentVariableA(name, old_native_present ? old_native : NULL);
        freez(old_crt);
        freez(old_native);
        errno = saved_errno;
        return -1;
    }

    freez(old_crt);
    freez(old_native);
    return 0;
}
#else
static int environment_native_set(const char *name, const char *value, char **managed_value, char **spawn_raw) {
    if(__atomic_exchange_n(&test_fail_native_once, 0, __ATOMIC_ACQ_REL)) {
        errno = EIO;
        return -1;
    }

    if(setenv(name, value, 1) != 0)
        return -1;

    *managed_value = strdupz(value);
    *spawn_raw = environment_compose_raw(name, value);
    if(!*spawn_raw) {
        freez(*managed_value);
        *managed_value = NULL;
        return -1;
    }
    return 0;
}

static int environment_native_unset(const char *name) {
    if(__atomic_exchange_n(&test_fail_native_once, 0, __ATOMIC_ACQ_REL)) {
        errno = EIO;
        return -1;
    }

    return unsetenv(name);
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// Windows environment block builder

typedef struct nd_environment_sort_item {
    const char *raw;
    size_t name_length;
#if defined(OS_WINDOWS)
    wchar_t *wide_name;
#endif
} ND_ENVIRONMENT_SORT_ITEM;

static size_t environment_windows_name_length(const char *raw) {
    const char *equals = strchr(raw + (raw[0] == '='), '=');
    return equals ? (size_t)(equals - raw) : strlen(raw);
}

static int environment_ascii_name_casecmp(
    const char *left, size_t left_length, const char *right, size_t right_length) {
    size_t common = MIN(left_length, right_length);
    for(size_t i = 0; i < common; i++) {
        unsigned char l = (unsigned char)tolower((unsigned char)left[i]);
        unsigned char r = (unsigned char)tolower((unsigned char)right[i]);
        if(l < r)
            return -1;
        if(l > r)
            return 1;
    }

    return (left_length > right_length) - (left_length < right_length);
}

static int environment_windows_block_compare(const void *left_ptr, const void *right_ptr) {
    const ND_ENVIRONMENT_SORT_ITEM *left = left_ptr;
    const ND_ENVIRONMENT_SORT_ITEM *right = right_ptr;

    int result = 0;
#if defined(OS_WINDOWS)
    if(left->wide_name && right->wide_name) {
        int compared = CompareStringOrdinal(
            left->wide_name, -1,
            right->wide_name, -1,
            TRUE);
        if(compared == CSTR_LESS_THAN)
            result = -1;
        else if(compared == CSTR_GREATER_THAN)
            result = 1;
    }
    else
#endif
        result = environment_ascii_name_casecmp(
            left->raw, left->name_length, right->raw, right->name_length);

    if(!result)
        result = strcmp(left->raw, right->raw);

    return result;
}

static char *environment_windows_block_build(
    const char *const envp[], size_t entries, size_t *block_size) {
    size_t included = 0;
    for(size_t i = 0; i < entries; i++) {
        if(envp[i] && *envp[i])
            included++;
    }

    ND_ENVIRONMENT_SORT_ITEM *sorted = included ? callocz(included, sizeof(*sorted)) : NULL;
    size_t position = 0;
    for(size_t i = 0; i < entries; i++) {
        if(!envp[i] || !*envp[i])
            continue;

        sorted[position].raw = envp[i];
        sorted[position].name_length = environment_windows_name_length(envp[i]);
#if defined(OS_WINDOWS)
        char *name = strndupz(envp[i], sorted[position].name_length);
        sorted[position].wide_name = environment_windows_wide(name, CP_OEMCP);
        freez(name);
#endif
        position++;
    }

    if(included > 1)
        qsort(sorted, included, sizeof(*sorted), environment_windows_block_compare);

    size_t required = included ? 1 : 2;
    for(size_t i = 0; i < included; i++) {
        size_t length = strlen(sorted[i].raw) + 1;
        if(unlikely(required > SIZE_MAX - length)) {
#if defined(OS_WINDOWS)
            for(size_t j = 0; j < included; j++)
                freez(sorted[j].wide_name);
#endif
            freez(sorted);
            errno = EOVERFLOW;
            return NULL;
        }
        required += length;
    }

    char *block = callocz(required, 1);
    char *dst = block;
    for(size_t i = 0; i < included; i++) {
        size_t length = strlen(sorted[i].raw) + 1;
        memcpy(dst, sorted[i].raw, length);
        dst += length;
    }

#if defined(OS_WINDOWS)
    for(size_t i = 0; i < included; i++)
        freez(sorted[i].wide_name);
#endif
    freez(sorted);

    if(block_size)
        *block_size = required;
    return block;
}

// --------------------------------------------------------------------------------------------------------------------
// Immutable snapshots

static void environment_snapshot_free(ND_ENV_SNAPSHOT *snapshot) {
    if(!snapshot)
        return;

    for(size_t i = 0; i < snapshot->entries; i++)
        freez(snapshot->envp[i]);
    freez(snapshot->envp);
    freez(snapshot->windows_block);
    freez(snapshot);
}

static void environment_snapshot_reference(ND_ENV_SNAPSHOT *snapshot) {
    size_t references = __atomic_add_fetch(&snapshot->references, 1, __ATOMIC_ACQUIRE);
    internal_fatal(!references, "ENVIRONMENT: snapshot reference count overflow");
}

void nd_environment_snapshot_release(ND_ENV_SNAPSHOT *snapshot) {
    if(!snapshot)
        return;

    size_t references = __atomic_sub_fetch(&snapshot->references, 1, __ATOMIC_ACQ_REL);
    if(!references)
        environment_snapshot_free(snapshot);
}

static ND_ENV_SNAPSHOT *environment_snapshot_build_locked(ND_ENVIRONMENT *environment) {
    if(__atomic_exchange_n(&test_fail_snapshot_once, 0, __ATOMIC_ACQ_REL)) {
        errno = ENOMEM;
        return NULL;
    }

    if(unlikely(environment->entries_count == SIZE_MAX)) {
        errno = EOVERFLOW;
        return NULL;
    }

    ND_ENV_SNAPSHOT *snapshot = callocz(1, sizeof(*snapshot));
    snapshot->references = 1; // context-owned publication reference
    snapshot->generation = __atomic_load_n(&environment->generation, __ATOMIC_RELAXED);
    snapshot->entries = environment->entries_count;
    snapshot->envp = callocz(snapshot->entries + 1, sizeof(*snapshot->envp));

    size_t i = 0;
    for(ND_ENV_ENTRY *entry = environment->entries; entry; entry = entry->next)
        snapshot->envp[i++] = strdupz(entry->raw);
    internal_fatal(i != snapshot->entries, "ENVIRONMENT: ordered entry count changed while snapshotting");

#if defined(OS_WINDOWS)
    snapshot->windows_block = environment_windows_block_build(
        (const char *const *)snapshot->envp, snapshot->entries, &snapshot->windows_block_size);
    if(!snapshot->windows_block) {
        environment_snapshot_free(snapshot);
        return NULL;
    }
#endif

    return snapshot;
}

static int environment_snapshot_publish_locked(ND_ENVIRONMENT *environment) {
    uint64_t generation = __atomic_load_n(&environment->generation, __ATOMIC_RELAXED);

    spinlock_lock(&environment->publication_lock);
    bool current = environment->published && environment->published->generation == generation;
    spinlock_unlock(&environment->publication_lock);
    if(current)
        return 0;

    ND_ENV_SNAPSHOT *snapshot = environment_snapshot_build_locked(environment);
    if(!snapshot)
        return -1;

    spinlock_lock(&environment->publication_lock);
    ND_ENV_SNAPSHOT *old = environment->published;
    environment->published = snapshot;
    spinlock_unlock(&environment->publication_lock);

    nd_environment_snapshot_release(old);
    return 0;
}

ND_ENV_SNAPSHOT *nd_environment_snapshot_acquire(ND_ENVIRONMENT *environment) {
    if(!nd_environment_is_initialized() || !environment || environment->magic != ND_ENVIRONMENT_MAGIC) {
        errno = EPERM;
        return NULL;
    }

    uint64_t generation = __atomic_load_n(&environment->generation, __ATOMIC_ACQUIRE);
    spinlock_lock(&environment->publication_lock);
    ND_ENV_SNAPSHOT *snapshot = environment->published;
    if(snapshot && snapshot->generation == generation) {
        environment_snapshot_reference(snapshot);
        spinlock_unlock(&environment->publication_lock);
        return snapshot;
    }
    spinlock_unlock(&environment->publication_lock);

    netdata_mutex_lock(&environment->mutation_mutex);

    generation = __atomic_load_n(&environment->generation, __ATOMIC_RELAXED);
    spinlock_lock(&environment->publication_lock);
    snapshot = environment->published;
    if(snapshot && snapshot->generation == generation) {
        environment_snapshot_reference(snapshot);
        spinlock_unlock(&environment->publication_lock);
        netdata_mutex_unlock(&environment->mutation_mutex);
        return snapshot;
    }
    spinlock_unlock(&environment->publication_lock);

    if(environment_snapshot_publish_locked(environment) != 0) {
        netdata_mutex_unlock(&environment->mutation_mutex);
        return NULL;
    }

    spinlock_lock(&environment->publication_lock);
    snapshot = environment->published;
    environment_snapshot_reference(snapshot);
    spinlock_unlock(&environment->publication_lock);

    netdata_mutex_unlock(&environment->mutation_mutex);
    return snapshot;
}

const char *const *nd_environment_snapshot_envp(const ND_ENV_SNAPSHOT *snapshot) {
    return snapshot ? (const char *const *)snapshot->envp : NULL;
}

size_t nd_environment_snapshot_entries(const ND_ENV_SNAPSHOT *snapshot) {
    return snapshot ? snapshot->entries : 0;
}

uint64_t nd_environment_snapshot_generation(const ND_ENV_SNAPSHOT *snapshot) {
    return snapshot ? snapshot->generation : 0;
}

const char *nd_environment_snapshot_windows_block(const ND_ENV_SNAPSHOT *snapshot, size_t *size) {
    if(size)
        *size = snapshot ? snapshot->windows_block_size : 0;
    return snapshot ? snapshot->windows_block : NULL;
}

// --------------------------------------------------------------------------------------------------------------------
// Managed reads and mutations

int nd_environment_context_set(
    ND_ENVIRONMENT *environment, const char *name, const char *value, bool overwrite) {
    if(!nd_environment_is_initialized() || !environment || environment->magic != ND_ENVIRONMENT_MAGIC) {
        errno = EPERM;
        return -1;
    }
    if(!environment_name_is_valid(name) || !value) {
        errno = EINVAL;
        return -1;
    }

    netdata_mutex_lock(&environment->mutation_mutex);
    ND_ENV_ENTRY *entry = environment_index_get(environment, name);
    if(entry && !overwrite) {
        netdata_mutex_unlock(&environment->mutation_mutex);
        return 0;
    }

    char *managed_value = NULL;
    char *spawn_raw = NULL;
    const char *display_name = entry ? entry->name : name;
    if(environment->mirrors_native) {
        if(environment_native_set(display_name, value, &managed_value, &spawn_raw) != 0) {
            netdata_mutex_unlock(&environment->mutation_mutex);
            return -1;
        }
    }
    else {
        managed_value = strdupz(value);
        spawn_raw = environment_spawn_raw_from_managed(display_name, value);
        if(!spawn_raw) {
            freez(managed_value);
            netdata_mutex_unlock(&environment->mutation_mutex);
            return -1;
        }
    }

    bool changed = !entry || strcmp(entry->value, managed_value) != 0 || strcmp(entry->raw, spawn_raw) != 0;
    if(changed && entry) {
        freez(entry->raw);
        freez(entry->value);
        entry->raw = spawn_raw;
        entry->value = managed_value;
        spawn_raw = NULL;
        managed_value = NULL;
    }
    else if(changed) {
        ND_ENV_ENTRY *new_entry = environment_entry_from_name_values(name, managed_value, spawn_raw);
        if(!new_entry) {
            freez(managed_value);
            freez(spawn_raw);
            netdata_mutex_unlock(&environment->mutation_mutex);
            return -1;
        }
        spawn_raw = NULL;
        environment_entry_append(environment, new_entry);
    }

    freez(managed_value);
    freez(spawn_raw);

    if(changed) {
        environment_index_rebuild(environment);
        __atomic_add_fetch(&environment->generation, 1, __ATOMIC_RELEASE);
    }

    netdata_mutex_unlock(&environment->mutation_mutex);
    return 0;
}

int nd_environment_context_unset(ND_ENVIRONMENT *environment, const char *name) {
    if(!nd_environment_is_initialized() || !environment || environment->magic != ND_ENVIRONMENT_MAGIC) {
        errno = EPERM;
        return -1;
    }
    if(!environment_name_is_valid(name)) {
        errno = EINVAL;
        return -1;
    }

    netdata_mutex_lock(&environment->mutation_mutex);
    if(!environment_index_get(environment, name)) {
        netdata_mutex_unlock(&environment->mutation_mutex);
        return 0;
    }

    if(environment->mirrors_native && environment_native_unset(name) != 0) {
        netdata_mutex_unlock(&environment->mutation_mutex);
        return -1;
    }

    ND_ENV_ENTRY *entry = environment->entries;
    while(entry) {
        ND_ENV_ENTRY *next = entry->next;
        if(entry->name && environment_names_equal(entry->name, name)) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(environment->entries, entry, prev, next);
            environment->entries_count--;
            environment_entry_free(entry);
        }
        entry = next;
    }

    environment_index_rebuild(environment);
    __atomic_add_fetch(&environment->generation, 1, __ATOMIC_RELEASE);
    netdata_mutex_unlock(&environment->mutation_mutex);
    return 0;
}

char *nd_environment_context_get_dup(ND_ENVIRONMENT *environment, const char *name) {
    if(!nd_environment_is_initialized() || !environment || environment->magic != ND_ENVIRONMENT_MAGIC) {
        errno = EPERM;
        return NULL;
    }
    if(!environment_name_is_valid(name)) {
        errno = EINVAL;
        return NULL;
    }

    netdata_mutex_lock(&environment->mutation_mutex);
    ND_ENV_ENTRY *entry = environment_index_get(environment, name);
    char *value = entry ? strdupz(entry->value) : NULL;
    netdata_mutex_unlock(&environment->mutation_mutex);

    errno = 0;
    return value;
}

int nd_environment_set(const char *name, const char *value, bool overwrite) {
    ND_ENVIRONMENT *environment = nd_environment_process();
    if(!environment) {
        errno = EPERM;
        return -1;
    }

    return nd_environment_context_set(environment, name, value, overwrite);
}

int nd_environment_unset(const char *name) {
    ND_ENVIRONMENT *environment = nd_environment_process();
    if(!environment) {
        errno = EPERM;
        return -1;
    }

    return nd_environment_context_unset(environment, name);
}

char *nd_environment_get_dup(const char *name) {
    ND_ENVIRONMENT *environment = nd_environment_process();
    if(!environment) {
        errno = EPERM;
        return NULL;
    }

    return nd_environment_context_get_dup(environment, name);
}

// --------------------------------------------------------------------------------------------------------------------
// Lifecycle and context ownership

bool nd_environment_is_initialized(void) {
    return __atomic_load_n(&process_environment_state, __ATOMIC_ACQUIRE) >= ND_ENVIRONMENT_INITIALIZING;
}

bool nd_environment_is_process_frozen(void) {
    return __atomic_load_n(&process_environment_state, __ATOMIC_ACQUIRE) == ND_ENVIRONMENT_PROCESS_FROZEN;
}

ND_ENVIRONMENT *nd_environment_process(void) {
    if(!nd_environment_is_initialized())
        return NULL;

    return __atomic_load_n(&process_environment, __ATOMIC_ACQUIRE);
}

int nd_environment_init(void) {
    int expected = ND_ENVIRONMENT_UNINITIALIZED;
    if(!__atomic_compare_exchange_n(
           &process_environment_state,
           &expected,
           ND_ENVIRONMENT_IMPORTING,
           false,
           __ATOMIC_ACQ_REL,
           __ATOMIC_ACQUIRE)) {
        if(expected >= ND_ENVIRONMENT_INITIALIZING)
            return 0;

        errno = EBUSY;
        return -1;
    }

    ND_ENVIRONMENT *environment = environment_create_process_from_native();
    if(!environment) {
        __atomic_store_n(&process_environment_state, ND_ENVIRONMENT_UNINITIALIZED, __ATOMIC_RELEASE);
        return -1;
    }

    __atomic_store_n(&process_environment, environment, __ATOMIC_RELEASE);
    __atomic_store_n(&process_environment_state, ND_ENVIRONMENT_INITIALIZING, __ATOMIC_RELEASE);
    return 0;
}

int nd_environment_freeze_process(void) {
    int state = __atomic_load_n(&process_environment_state, __ATOMIC_ACQUIRE);
    if(state == ND_ENVIRONMENT_PROCESS_FROZEN)
        return 0;
    if(state != ND_ENVIRONMENT_INITIALIZING) {
        errno = state == ND_ENVIRONMENT_IMPORTING ? EBUSY : EPERM;
        return -1;
    }

    ND_ENVIRONMENT *environment = __atomic_load_n(&process_environment, __ATOMIC_ACQUIRE);
    if(!environment) {
        errno = EPERM;
        return -1;
    }

    netdata_mutex_lock(&environment->mutation_mutex);
    if(environment_snapshot_publish_locked(environment) != 0) {
        netdata_mutex_unlock(&environment->mutation_mutex);
        return -1;
    }

    environment->mirrors_native = false;
    __atomic_store_n(&process_environment_state, ND_ENVIRONMENT_PROCESS_FROZEN, __ATOMIC_RELEASE);
    netdata_mutex_unlock(&environment->mutation_mutex);
    return 0;
}

ND_ENVIRONMENT *nd_environment_create_isolated(const char *const envp[]) {
    if(!nd_environment_is_initialized()) {
        errno = EPERM;
        return NULL;
    }

    return environment_create_from_vector(envp, false, false);
}

void nd_environment_destroy(ND_ENVIRONMENT *environment) {
    if(!environment || environment->magic != ND_ENVIRONMENT_MAGIC)
        return;

    if(environment->process_global) {
        errno = EPERM;
        return;
    }

    netdata_mutex_lock(&environment->mutation_mutex);
    environment->magic = 0;

    spinlock_lock(&environment->publication_lock);
    ND_ENV_SNAPSHOT *snapshot = environment->published;
    environment->published = NULL;
    spinlock_unlock(&environment->publication_lock);

    dictionary_destroy(environment->index);
    environment->index = NULL;
    environment_entries_clear(environment);
    netdata_mutex_unlock(&environment->mutation_mutex);
    netdata_mutex_destroy(&environment->mutation_mutex);

    nd_environment_snapshot_release(snapshot);
    freez(environment);
}

// --------------------------------------------------------------------------------------------------------------------
// Post-fork child reset

static void environment_fork_child_abandon_process(void) {
    __atomic_store_n(&process_environment, NULL, __ATOMIC_RELAXED);
    __atomic_store_n(&process_environment_state, ND_ENVIRONMENT_UNINITIALIZED, __ATOMIC_RELAXED);
}

int nd_environment_fork_child_reset_from_native(void) {
    environment_fork_child_abandon_process();
    return nd_environment_init();
}

int nd_environment_fork_child_replace(const char *const envp[]) {
    if(!envp) {
        errno = EINVAL;
        return -1;
    }

#if defined(OS_WINDOWS)
    errno = ENOTSUP;
    return -1;
#else
    size_t entries = 0;
    while(envp[entries]) {
        if(unlikely(entries == SIZE_MAX - 1)) {
            errno = EOVERFLOW;
            return -1;
        }
        entries++;
    }

    char **native = callocz(entries + 1, sizeof(*native));
    for(size_t i = 0; i < entries; i++)
        native[i] = strdupz(envp[i]);

#if defined(OS_MACOS)
    *_NSGetEnviron() = native;
#else
    environ = native;
#endif

    environment_fork_child_abandon_process();
    return nd_environment_init();
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// Focused test hooks

void nd_environment_test_fail_native_once(void) {
    __atomic_store_n(&test_fail_native_once, 1, __ATOMIC_RELEASE);
}

void nd_environment_test_fail_snapshot_once(void) {
    __atomic_store_n(&test_fail_snapshot_once, 1, __ATOMIC_RELEASE);
}

char *nd_environment_test_windows_block(const char *const envp[], size_t *size) {
    if(!envp) {
        errno = EINVAL;
        return NULL;
    }

    size_t entries = 0;
    while(envp[entries])
        entries++;

    return environment_windows_block_build(envp, entries, size);
}
