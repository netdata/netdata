// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_NATIVE_ENVIRONMENT_ACCESS 1
#include "libnetdata/libnetdata.h"

static size_t failures = 0;

static bool test_strings_equal(const char *left, const char *right) {
    return left && right && strcmp(left, right) == 0;
}

#if defined(OS_WINDOWS)
static char *windows_inherited_raw = NULL;

static wchar_t *test_windows_wide(const char *text, UINT code_page) {
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

static char *test_windows_narrow(const wchar_t *wide, UINT code_page) {
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

static char *test_windows_transcode(const char *text, UINT from, UINT to) {
    wchar_t *wide = test_windows_wide(text, from);
    if(!wide)
        return NULL;
    char *converted = test_windows_narrow(wide, to);
    freez(wide);
    return converted;
}

static bool test_windows_name_equal(const char *left, const char *right) {
    wchar_t *left_wide = test_windows_wide(left, CP_ACP);
    wchar_t *right_wide = test_windows_wide(right, CP_ACP);
    if(!left_wide || !right_wide) {
        freez(left_wide);
        freez(right_wide);
        return false;
    }

    bool equal = CompareStringOrdinal(left_wide, -1, right_wide, -1, TRUE) == CSTR_EQUAL;
    freez(left_wide);
    freez(right_wide);
    return equal;
}

static bool test_windows_path_list_name(const char *name) {
    return test_windows_name_equal(name, "PATH") ||
           test_windows_name_equal(name, "LD_LIBRARY_PATH") ||
           test_windows_name_equal(name, "ORIGINAL_PATH");
}

static bool test_windows_single_path_name(const char *name) {
    return test_windows_name_equal(name, "HOME") ||
           test_windows_name_equal(name, "SHELL") ||
           test_windows_name_equal(name, "TMPDIR") ||
           test_windows_name_equal(name, "TMP") ||
           test_windows_name_equal(name, "TEMP");
}

static char *test_windows_expected_raw(const char *name, const char *value) {
    char *name_oem = test_windows_transcode(name, CP_ACP, CP_OEMCP);
    char *value_oem = NULL;
    bool list = test_windows_path_list_name(name);
    bool path = test_windows_single_path_name(name);

    if(!list && !path)
        value_oem = test_windows_transcode(value, CP_ACP, CP_OEMCP);
    else if(!*value && test_windows_name_equal(name, "SHELL"))
        value_oem = test_windows_transcode(value, CP_ACP, CP_OEMCP);
    else {
        cygwin_conv_path_t conversion = CCP_POSIX_TO_WIN_W | (list ? CCP_RELATIVE : CCP_ABSOLUTE);
        ssize_t required = list
            ? cygwin_conv_path_list(conversion, value, NULL, 0)
            : cygwin_conv_path(conversion, value, NULL, 0);
        if(required > 0) {
            wchar_t *wide = mallocz((size_t)required);
            ssize_t rc = list
                ? cygwin_conv_path_list(conversion, value, wide, (size_t)required)
                : cygwin_conv_path(conversion, value, wide, (size_t)required);
            if(rc == 0)
                value_oem = test_windows_narrow(wide, CP_OEMCP);
            freez(wide);
        }
    }

    if(!name_oem || !value_oem) {
        freez(name_oem);
        freez(value_oem);
        return NULL;
    }

    size_t name_length = strlen(name_oem);
    size_t value_length = strlen(value_oem);
    char *raw = mallocz(name_length + value_length + 2);
    snprintfz(raw, name_length + value_length + 2, "%s=%s", name_oem, value_oem);
    freez(name_oem);
    freez(value_oem);
    return raw;
}

static bool test_windows_raw_name_equal(const char *raw, const char *name) {
    const char *equals = raw ? strchr(raw, '=') : NULL;
    if(!equals || equals == raw)
        return false;

    char *raw_name_oem = strndupz(raw, (size_t)(equals - raw));
    char *raw_name_acp = test_windows_transcode(raw_name_oem, CP_OEMCP, CP_ACP);
    bool equal = raw_name_acp && test_windows_name_equal(raw_name_acp, name);
    freez(raw_name_oem);
    freez(raw_name_acp);
    return equal;
}

static const char *test_windows_snapshot_raw(const ND_ENV_SNAPSHOT *snapshot, const char *name) {
    const char *const *envp = nd_environment_snapshot_envp(snapshot);
    for(size_t i = 0; envp && envp[i]; i++) {
        if(test_windows_raw_name_equal(envp[i], name))
            return envp[i];
    }
    return NULL;
}

static const char *test_windows_block_raw(const char *block, const char *name) {
    for(const char *entry = block; entry && *entry; entry += strlen(entry) + 1) {
        if(test_windows_raw_name_equal(entry, name))
            return entry;
    }
    return NULL;
}

static char *test_windows_native_raw_dup(const char *name) {
    LPCH block = GetEnvironmentStringsA();
    if(!block)
        return NULL;

    char *found = NULL;
    for(const char *entry = block; *entry; entry += strlen(entry) + 1) {
        if(test_windows_raw_name_equal(entry, name)) {
            found = strdupz(entry);
            break;
        }
    }
    FreeEnvironmentStringsA(block);
    return found;
}

static char *test_windows_native_value_dup(const char *name) {
    SetLastError(ERROR_SUCCESS);
    DWORD required = GetEnvironmentVariableA(name, NULL, 0);
    if(!required) {
        if(GetLastError() == ERROR_SUCCESS)
            return strdupz("");
        return NULL;
    }

    char *value = mallocz(required);
    SetLastError(ERROR_SUCCESS);
    DWORD copied = GetEnvironmentVariableA(name, value, required);
    if(!copied && GetLastError() != ERROR_SUCCESS) {
        freez(value);
        return NULL;
    }
    return value;
}

static bool test_windows_crt_drive_entry(const char *raw) {
    return raw && raw[0] == '!' && raw[1] && raw[2] && raw[3] == '=' &&
           ((isalpha((unsigned char)raw[1]) && raw[2] == ':') || (raw[1] == ':' && raw[2] == ':'));
}


static int test_windows_child_observation(void) {
    char *generic_value = test_windows_narrow(L"caf\u00e9", CP_ACP);
    char *path_value = test_windows_narrow(L"/c/t\u00e9st:/tmp", CP_UTF8);
    char *expected_generic = generic_value
        ? test_windows_expected_raw("ND_ENVIRONMENT_TEST_CHILD_GENERIC", generic_value)
        : NULL;
    char *expected_path = path_value
        ? test_windows_expected_raw("LD_LIBRARY_PATH", path_value)
        : NULL;
    char *actual_generic = test_windows_native_raw_dup("ND_ENVIRONMENT_TEST_CHILD_GENERIC");
    char *actual_path = test_windows_native_raw_dup("LD_LIBRARY_PATH");
    char *generic_native = test_windows_native_value_dup("ND_ENVIRONMENT_TEST_CHILD_GENERIC");
    char *path_native = test_windows_native_value_dup("LD_LIBRARY_PATH");
    char *expected_path_native = NULL;

    if(expected_path) {
        const char *equals = strchr(expected_path, '=');
        if(equals)
            expected_path_native = test_windows_transcode(equals + 1, CP_OEMCP, CP_ACP);
    }

    bool ok = generic_value && path_value && expected_generic && expected_path && actual_generic && actual_path &&
              generic_native && path_native && expected_path_native &&
              strcmp(expected_generic, actual_generic) == 0 && strcmp(expected_path, actual_path) == 0 &&
              strcmp(generic_value, generic_native) == 0 && strcmp(expected_path_native, path_native) == 0;
    if(!ok) {
        fprintf(stderr, "environment-unittest: Windows child environment observation failed\n");
        fprintf(stderr, "  generic raw expected='%s' actual='%s'\n",
                expected_generic ? expected_generic : "(null)", actual_generic ? actual_generic : "(null)");
        fprintf(stderr, "  path raw expected='%s' actual='%s'\n",
                expected_path ? expected_path : "(null)", actual_path ? actual_path : "(null)");
    }

    freez(generic_value);
    freez(path_value);
    freez(expected_generic);
    freez(expected_path);
    freez(actual_generic);
    freez(actual_path);
    freez(generic_native);
    freez(path_native);
    freez(expected_path_native);
    return ok ? 0 : 90;
}
#endif

#define CHECK(expression)                                                                                             \
    do {                                                                                                              \
        if(!(expression)) {                                                                                           \
            fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, #expression);                                  \
            failures++;                                                                                               \
        }                                                                                                             \
    } while(0)

static const char *snapshot_value(const ND_ENV_SNAPSHOT *snapshot, const char *name) {
    if(!snapshot || !name)
        return NULL;

    size_t name_length = strlen(name);
    const char *const *envp = nd_environment_snapshot_envp(snapshot);

    for(size_t i = 0; envp && envp[i]; i++) {
        if(strncmp(envp[i], name, name_length) == 0 && envp[i][name_length] == '=')
            return &envp[i][name_length + 1];
    }

    return NULL;
}

static void check_snapshot_exact(const ND_ENV_SNAPSHOT *snapshot, const char *const expected[]) {
    CHECK(snapshot != NULL);
    CHECK(expected != NULL);
    if(!snapshot || !expected)
        return;

    const char *const *envp = nd_environment_snapshot_envp(snapshot);
    CHECK(envp != NULL);
    if(!envp)
        return;

    size_t count = 0;
    while(expected[count]) {
        CHECK(envp[count] != NULL);
        if(envp[count])
            CHECK(test_strings_equal(envp[count], expected[count]));
        count++;
    }

    CHECK(nd_environment_snapshot_entries(snapshot) == count);
    CHECK(envp[count] == NULL);
}

static void test_preinit_contract(void) {
    errno = 0;
    CHECK(nd_environment_process() == NULL);
    CHECK(nd_environment_get_dup("PATH") == NULL);
    CHECK(errno == EPERM);

    errno = 0;
    CHECK(nd_environment_freeze_process() == -1);
    CHECK(errno == EPERM);

    errno = 0;
    CHECK(nd_environment_snapshot_acquire(NULL) == NULL);
    CHECK(errno == EPERM);

    errno = 0;
    CHECK(nd_environment_create_isolated(NULL) == NULL);
    CHECK(errno == EPERM);

    errno = 0;
    CHECK(nd_environment_set("ND_ENVIRONMENT_TEST_PREINIT", "forbidden", true) == -1);
    CHECK(errno == EPERM);

    errno = 0;
    CHECK(nd_environment_unset("ND_ENVIRONMENT_TEST_PREINIT") == -1);
    CHECK(errno == EPERM);
}

static void test_import_and_native_mirroring(void) {
    unsetenv("ND_ENVIRONMENT_TEST_BOOTSTRAP_WRAPPER");
#if defined(OS_WINDOWS)
    SetEnvironmentVariableA("ND_ENVIRONMENT_TEST_BOOTSTRAP_WRAPPER", NULL);

    char *inherited_value = test_windows_narrow(L"caf\u00e9", CP_ACP);
    CHECK(inherited_value != NULL);
    if(inherited_value) {
        nd_setenv("ND_ENVIRONMENT_TEST_INHERITED_OEM", inherited_value, 1);
        windows_inherited_raw = test_windows_native_raw_dup("ND_ENVIRONMENT_TEST_INHERITED_OEM");
        CHECK(windows_inherited_raw != NULL);
    }
    freez(inherited_value);
#endif

    nd_setenv("ND_ENVIRONMENT_TEST_NATIVE", "imported", 1);
    CHECK(test_strings_equal(getenv("ND_ENVIRONMENT_TEST_NATIVE"), "imported"));
    CHECK(nd_environment_init() == 0);
    CHECK(nd_environment_init() == 0);
    CHECK(nd_environment_is_initialized());
    CHECK(!nd_environment_is_process_frozen());
    CHECK(nd_environment_process() != NULL);

#if defined(OS_WINDOWS)
    ND_ENV_SNAPSHOT *initial = nd_environment_snapshot_acquire(nd_environment_process());
    CHECK(initial != NULL);
    if(initial) {
        const char *inherited = test_windows_snapshot_raw(initial, "ND_ENVIRONMENT_TEST_INHERITED_OEM");
        CHECK(inherited != NULL);
        if(inherited && windows_inherited_raw)
            CHECK(test_strings_equal(inherited, windows_inherited_raw));

        const char *const *envp = nd_environment_snapshot_envp(initial);
        for(size_t i = 0; envp && envp[i]; i++)
            CHECK(!test_windows_crt_drive_entry(envp[i]));
    }
    nd_environment_snapshot_release(initial);
#endif

    char *imported = nd_environment_get_dup("ND_ENVIRONMENT_TEST_NATIVE");
    CHECK(test_strings_equal(imported, "imported"));

    CHECK(nd_environment_set("ND_ENVIRONMENT_TEST_NATIVE", "initializing", true) == 0);
    CHECK(test_strings_equal(imported, "imported"));
    CHECK(test_strings_equal(getenv("ND_ENVIRONMENT_TEST_NATIVE"), "initializing"));
    freez(imported);

    nd_environment_test_fail_native_once();
    errno = 0;
    CHECK(nd_environment_set("ND_ENVIRONMENT_TEST_NATIVE", "must-not-publish", true) == -1);
    CHECK(errno == EIO);

    char *unchanged = nd_environment_get_dup("ND_ENVIRONMENT_TEST_NATIVE");
    CHECK(test_strings_equal(unchanged, "initializing"));
    CHECK(test_strings_equal(getenv("ND_ENVIRONMENT_TEST_NATIVE"), "initializing"));
    freez(unchanged);
}

static void test_windows_case_identity(void) {
#if defined(OS_WINDOWS)
    const char *const initial[] = { "MiXeD=first", NULL };
    ND_ENVIRONMENT *environment = nd_environment_create_isolated(initial);
    CHECK(environment != NULL);
    if(!environment)
        return;

    char *value = nd_environment_context_get_dup(environment, "mixed");
    CHECK(test_strings_equal(value, "first"));
    freez(value);

    CHECK(nd_environment_context_set(environment, "MIXED", "ignored", false) == 0);
    CHECK(nd_environment_context_set(environment, "mixed", "second", true) == 0);
    ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(environment);
    const char *const expected[] = { "MiXeD=second", NULL };
    check_snapshot_exact(snapshot, expected);

    CHECK(nd_environment_context_unset(environment, "mIxEd") == 0);
    char *missing = nd_environment_context_get_dup(environment, "MIXED");
    CHECK(missing == NULL);
    freez(missing);

    nd_environment_snapshot_release(snapshot);
    nd_environment_destroy(environment);
#endif
}

static void test_isolated_order_duplicates_and_opaque(void) {
    const char *const initial[] = {
        "OPAQUE",
        "A=first",
        "A=second",
        "",
        "B=",
        NULL,
    };
    ND_ENVIRONMENT *environment = nd_environment_create_isolated(initial);
    CHECK(environment != NULL);
    if(!environment)
        return;

    char *value = nd_environment_context_get_dup(environment, "A");
    CHECK(test_strings_equal(value, "first"));
    freez(value);

    ND_ENV_SNAPSHOT *original = nd_environment_snapshot_acquire(environment);
    CHECK(original != NULL);
    check_snapshot_exact(original, initial);
    uint64_t original_generation = nd_environment_snapshot_generation(original);

    CHECK(nd_environment_context_set(environment, "A", "ignored", false) == 0);
    ND_ENV_SNAPSHOT *no_overwrite = nd_environment_snapshot_acquire(environment);
    CHECK(no_overwrite == original);
    CHECK(nd_environment_snapshot_generation(no_overwrite) == original_generation);
    check_snapshot_exact(no_overwrite, initial);
    nd_environment_snapshot_release(no_overwrite);

    CHECK(nd_environment_context_set(environment, "A", "replacement", true) == 0);
    ND_ENV_SNAPSHOT *replaced = nd_environment_snapshot_acquire(environment);
    const char *const expected_replaced[] = {
        "OPAQUE",
        "A=replacement",
        "A=second",
        "",
        "B=",
        NULL,
    };
    check_snapshot_exact(replaced, expected_replaced);
    CHECK(nd_environment_snapshot_generation(replaced) != original_generation);

    // The old generation is still complete while a newer one is published.
    check_snapshot_exact(original, initial);

    CHECK(nd_environment_context_set(environment, "C", "last", true) == 0);
    CHECK(nd_environment_context_unset(environment, "A") == 0);
    ND_ENV_SNAPSHOT *removed = nd_environment_snapshot_acquire(environment);
    const char *const expected_removed[] = {
        "OPAQUE",
        "",
        "B=",
        "C=last",
        NULL,
    };
    check_snapshot_exact(removed, expected_removed);

    nd_environment_snapshot_release(original);
    nd_environment_snapshot_release(replaced);
    nd_environment_snapshot_release(removed);
    nd_environment_destroy(environment);
}

static void test_empty_and_owned_snapshot_destruction(void) {
    const char *const empty[] = { NULL };
    ND_ENVIRONMENT *environment = nd_environment_create_isolated(empty);
    CHECK(environment != NULL);
    if(!environment)
        return;

    ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(environment);
    CHECK(snapshot != NULL);
    if(!snapshot) {
        nd_environment_destroy(environment);
        return;
    }
    CHECK(nd_environment_snapshot_entries(snapshot) == 0);
    CHECK(nd_environment_snapshot_envp(snapshot)[0] == NULL);

    nd_environment_destroy(environment);
    CHECK(nd_environment_snapshot_entries(snapshot) == 0);
    CHECK(nd_environment_snapshot_envp(snapshot)[0] == NULL);
    nd_environment_snapshot_release(snapshot);
}

static void test_snapshot_failure_atomicity(void) {
    const char *const initial[] = { "FAILURE=old", NULL };
    ND_ENVIRONMENT *environment = nd_environment_create_isolated(initial);
    CHECK(environment != NULL);
    if(!environment)
        return;

    ND_ENV_SNAPSHOT *old = nd_environment_snapshot_acquire(environment);
    CHECK(old != NULL);
    if(!old) {
        nd_environment_destroy(environment);
        return;
    }

    CHECK(nd_environment_context_set(environment, "FAILURE", "new", true) == 0);
    nd_environment_test_fail_snapshot_once();
    errno = 0;
    CHECK(nd_environment_snapshot_acquire(environment) == NULL);
    CHECK(errno == ENOMEM);
    CHECK(test_strings_equal(snapshot_value(old, "FAILURE"), "old"));

    ND_ENV_SNAPSHOT *current = nd_environment_snapshot_acquire(environment);
    CHECK(current != NULL);
    CHECK(test_strings_equal(snapshot_value(current, "FAILURE"), "new"));
    CHECK(test_strings_equal(snapshot_value(old, "FAILURE"), "old"));

    nd_environment_snapshot_release(old);
    nd_environment_snapshot_release(current);
    nd_environment_destroy(environment);
}

static void test_windows_block_builder(void) {
    const char *const envp[] = {
        "z=last",
        "=C:=drive",
        "a=first",
        "A0=prefix",
        "B=middle",
        NULL,
    };
    const char *const expected[] = {
        "=C:=drive",
        "a=first",
        "A0=prefix",
        "B=middle",
        "z=last",
        NULL,
    };

    size_t size = 0;
    char *block = nd_environment_test_windows_block(envp, &size);
    CHECK(block != NULL);

    if(block) {
        const char *entry = block;
        size_t i = 0;
        while(*entry && expected[i]) {
            CHECK(test_strings_equal(entry, expected[i]));
            entry += strlen(entry) + 1;
            i++;
        }
        CHECK(*entry == '\0');
        CHECK(expected[i] == NULL);
        CHECK((size_t)(entry - block) + 1 == size);
        CHECK(entry > block);
        if(entry > block)
            CHECK(entry[-1] == '\0');
        CHECK(entry[0] == '\0');
    }
    freez(block);

    const char *const empty[] = { NULL };
    block = nd_environment_test_windows_block(empty, &size);
    CHECK(block != NULL);
    CHECK(size == 2);
    if(block && size >= 2)
        CHECK(block[0] == '\0' && block[1] == '\0');
    freez(block);
}

typedef struct environment_concurrency_test {
    ND_ENVIRONMENT *environment;
    bool stop;
    size_t errors;
    size_t acquisitions;
} ENVIRONMENT_CONCURRENCY_TEST;

static void environment_writer(void *data) {
    ENVIRONMENT_CONCURRENCY_TEST *test = data;
    for(size_t i = 0; i < 2000; i++) {
        char value[32];
        snprintfz(value, sizeof(value), "%zu", i);
        if(nd_environment_context_set(test->environment, "RACE", value, true) != 0)
            __atomic_add_fetch(&test->errors, 1, __ATOMIC_RELAXED);
    }
    __atomic_store_n(&test->stop, true, __ATOMIC_RELEASE);
}

static void environment_reader(void *data) {
    ENVIRONMENT_CONCURRENCY_TEST *test = data;
    do {
        ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(test->environment);
        if(!snapshot) {
            __atomic_add_fetch(&test->errors, 1, __ATOMIC_RELAXED);
            continue;
        }

        const char *value = snapshot_value(snapshot, "RACE");
        if(!value || !*value)
            __atomic_add_fetch(&test->errors, 1, __ATOMIC_RELAXED);
        else {
            char first = *value;
            uv_sleep(0);
            if(*value != first)
                __atomic_add_fetch(&test->errors, 1, __ATOMIC_RELAXED);
        }

        __atomic_add_fetch(&test->acquisitions, 1, __ATOMIC_RELAXED);
        nd_environment_snapshot_release(snapshot);
    } while(!__atomic_load_n(&test->stop, __ATOMIC_ACQUIRE));
}

static void test_concurrent_writers_and_snapshot_readers(void) {
    const char *const initial[] = { "RACE=0", NULL };
    ENVIRONMENT_CONCURRENCY_TEST test = {
        .environment = nd_environment_create_isolated(initial),
    };
    CHECK(test.environment != NULL);
    if(!test.environment)
        return;

    uv_thread_t writer;
    uv_thread_t readers[4];
    size_t readers_started = 0;
    for(; readers_started < _countof(readers); readers_started++) {
        int rc = uv_thread_create(&readers[readers_started], environment_reader, &test);
        CHECK(rc == 0);
        if(rc != 0)
            break;
    }

    bool writer_started = false;
    if(readers_started == _countof(readers)) {
        int rc = uv_thread_create(&writer, environment_writer, &test);
        CHECK(rc == 0);
        writer_started = rc == 0;
    }

    if(!writer_started)
        __atomic_store_n(&test.stop, true, __ATOMIC_RELEASE);

    if(writer_started)
        CHECK(uv_thread_join(&writer) == 0);
    for(size_t i = 0; i < readers_started; i++)
        CHECK(uv_thread_join(&readers[i]) == 0);

    if(writer_started) {
        CHECK(__atomic_load_n(&test.errors, __ATOMIC_RELAXED) == 0);
        CHECK(__atomic_load_n(&test.acquisitions, __ATOMIC_RELAXED) > 0);
    }
    nd_environment_destroy(test.environment);
}

static void test_process_freeze(void) {
    const char *const isolated_initial[] = { "ISOLATED=before", "REMOVE=present", NULL };
    ND_ENVIRONMENT *isolated = nd_environment_create_isolated(isolated_initial);
    CHECK(isolated != NULL);
    if(!isolated)
        return;

    CHECK(nd_environment_set("ND_ENVIRONMENT_TEST_UNSET", "native-must-remain", true) == 0);
    ND_ENV_SNAPSHOT *before = nd_environment_snapshot_acquire(nd_environment_process());
    CHECK(before != NULL);
    if(!before) {
        nd_environment_destroy(isolated);
        return;
    }

    CHECK(nd_environment_freeze_process() == 0);
    CHECK(nd_environment_freeze_process() == 0);
    CHECK(nd_environment_is_process_frozen());

    const char *native = getenv("ND_ENVIRONMENT_TEST_NATIVE");
    char *native_copy = native ? strdupz(native) : NULL;
#if defined(OS_WINDOWS)
    char *native_windows_copy = test_windows_native_raw_dup("ND_ENVIRONMENT_TEST_NATIVE");
    char *unset_windows_copy = test_windows_native_raw_dup("ND_ENVIRONMENT_TEST_UNSET");
    char *native_windows_value_copy = test_windows_native_value_dup("ND_ENVIRONMENT_TEST_NATIVE");
    char *unset_windows_value_copy = test_windows_native_value_dup("ND_ENVIRONMENT_TEST_UNSET");
#endif

    CHECK(nd_environment_set("ND_ENVIRONMENT_TEST_NATIVE", "managed-only", true) == 0);
    char *managed = nd_environment_get_dup("ND_ENVIRONMENT_TEST_NATIVE");
    CHECK(test_strings_equal(managed, "managed-only"));
    CHECK(test_strings_equal(native_copy, getenv("ND_ENVIRONMENT_TEST_NATIVE")));
#if defined(OS_WINDOWS)
    char *native_windows_after = test_windows_native_raw_dup("ND_ENVIRONMENT_TEST_NATIVE");
    char *native_windows_value_after = test_windows_native_value_dup("ND_ENVIRONMENT_TEST_NATIVE");
    CHECK(native_windows_copy != NULL);
    CHECK(native_windows_after != NULL);
    CHECK(native_windows_value_copy != NULL);
    CHECK(native_windows_value_after != NULL);
    if(native_windows_copy && native_windows_after)
        CHECK(test_strings_equal(native_windows_copy, native_windows_after));
    if(native_windows_value_copy && native_windows_value_after)
        CHECK(test_strings_equal(native_windows_value_copy, native_windows_value_after));
    freez(native_windows_after);
    freez(native_windows_value_after);
#endif

    CHECK(nd_environment_unset("ND_ENVIRONMENT_TEST_UNSET") == 0);
    CHECK(test_strings_equal(getenv("ND_ENVIRONMENT_TEST_UNSET"), "native-must-remain"));
#if defined(OS_WINDOWS)
    char *unset_windows_after = test_windows_native_raw_dup("ND_ENVIRONMENT_TEST_UNSET");
    char *unset_windows_value_after = test_windows_native_value_dup("ND_ENVIRONMENT_TEST_UNSET");
    CHECK(unset_windows_copy != NULL);
    CHECK(unset_windows_after != NULL);
    CHECK(unset_windows_value_copy != NULL);
    CHECK(unset_windows_value_after != NULL);
    if(unset_windows_copy && unset_windows_after)
        CHECK(test_strings_equal(unset_windows_copy, unset_windows_after));
    if(unset_windows_value_copy && unset_windows_value_after)
        CHECK(test_strings_equal(unset_windows_value_copy, unset_windows_value_after));
    freez(unset_windows_after);
    freez(unset_windows_value_after);
#endif
    char *removed = nd_environment_get_dup("ND_ENVIRONMENT_TEST_UNSET");
    CHECK(removed == NULL);
    freez(removed);

    CHECK(nd_environment_context_set(isolated, "ISOLATED", "after", true) == 0);
    CHECK(nd_environment_context_unset(isolated, "REMOVE") == 0);
    CHECK(nd_environment_context_set(isolated, "APPENDED", "yes", true) == 0);
    ND_ENV_SNAPSHOT *isolated_after = nd_environment_snapshot_acquire(isolated);
    const char *const isolated_expected[] = { "ISOLATED=after", "APPENDED=yes", NULL };
    check_snapshot_exact(isolated_after, isolated_expected);

    ND_ENV_SNAPSHOT *after = nd_environment_snapshot_acquire(nd_environment_process());
    CHECK(after != NULL);
    CHECK(test_strings_equal(snapshot_value(after, "ND_ENVIRONMENT_TEST_NATIVE"), "managed-only"));
    CHECK(test_strings_equal(snapshot_value(before, "ND_ENVIRONMENT_TEST_NATIVE"), "initializing"));
    CHECK(test_strings_equal(snapshot_value(before, "ND_ENVIRONMENT_TEST_UNSET"), "native-must-remain"));
    CHECK(snapshot_value(after, "ND_ENVIRONMENT_TEST_UNSET") == NULL);

    nd_setenv("ND_ENVIRONMENT_TEST_BOOTSTRAP_WRAPPER", "managed", 1);
    char *wrapper = nd_environment_get_dup("ND_ENVIRONMENT_TEST_BOOTSTRAP_WRAPPER");
    CHECK(test_strings_equal(wrapper, "managed"));
    CHECK(getenv("ND_ENVIRONMENT_TEST_BOOTSTRAP_WRAPPER") == NULL);

    freez(wrapper);
    freez(managed);
    freez(native_copy);
#if defined(OS_WINDOWS)
    freez(native_windows_copy);
    freez(unset_windows_copy);
    freez(native_windows_value_copy);
    freez(unset_windows_value_copy);
#endif
    nd_environment_snapshot_release(before);
    nd_environment_snapshot_release(after);
    nd_environment_snapshot_release(isolated_after);
    nd_environment_destroy(isolated);
}

#if defined(OS_WINDOWS)
static void test_windows_spawned_child_observation(void) {
    char *generic_value = test_windows_narrow(L"caf\u00e9", CP_ACP);
    char *path_value = test_windows_narrow(L"/c/t\u00e9st:/tmp", CP_UTF8);
    CHECK(generic_value != NULL);
    CHECK(path_value != NULL);
    if(!generic_value || !path_value) {
        freez(generic_value);
        freez(path_value);
        return;
    }

    CHECK(nd_environment_set("ND_ENVIRONMENT_TEST_CHILD_GENERIC", generic_value, true) == 0);
    CHECK(nd_environment_set("LD_LIBRARY_PATH", path_value, true) == 0);

    ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(nd_environment_process());
    CHECK(snapshot != NULL);
    if(!snapshot) {
        freez(generic_value);
        freez(path_value);
        return;
    }

    size_t block_size = 0;
    const char *block = nd_environment_snapshot_windows_block(snapshot, &block_size);
    CHECK(block != NULL);
    CHECK(block_size >= 2);
    if(!block || block_size < 2) {
        nd_environment_snapshot_release(snapshot);
        freez(generic_value);
        freez(path_value);
        return;
    }

    char executable[MAX_PATH + 1] = "";
    DWORD executable_length = GetModuleFileNameA(NULL, executable, (DWORD)_countof(executable));
    CHECK(executable_length > 0 && executable_length < _countof(executable));
    if(!executable_length || executable_length >= _countof(executable)) {
        nd_environment_snapshot_release(snapshot);
        freez(generic_value);
        freez(path_value);
        return;
    }

    size_t command_length = executable_length + strlen("\"\" --windows-environment-child") + 1;
    char *command = mallocz(command_length);
    snprintfz(command, command_length, "\"%s\" --windows-environment-child", executable);

    STARTUPINFOA startup = { .cb = sizeof(startup) };
    PROCESS_INFORMATION process = { 0 };
    BOOL created = CreateProcessA(
        executable, command, NULL, NULL, FALSE, 0, (LPVOID)block, NULL, &startup, &process);
    CHECK(created != FALSE);
    if(created) {
        CHECK(WaitForSingleObject(process.hProcess, 30000) == WAIT_OBJECT_0);
        DWORD exit_code = UINT32_MAX;
        CHECK(GetExitCodeProcess(process.hProcess, &exit_code) != FALSE);
        CHECK(exit_code == 0);
        CloseHandle(process.hThread);
        CloseHandle(process.hProcess);
    }

    freez(command);
    nd_environment_snapshot_release(snapshot);
    freez(generic_value);
    freez(path_value);
}

static void test_windows_managed_child_representation(void) {
    const char *const empty[] = { NULL };
    ND_ENVIRONMENT *environment = nd_environment_create_isolated(empty);
    CHECK(environment != NULL);
    if(!environment)
        return;

    char *utf8_path = test_windows_narrow(L"/c/t\u00e9st:/tmp", CP_UTF8);
    char *acp_value = test_windows_narrow(L"caf\u00e9", CP_ACP);
    CHECK(utf8_path != NULL);
    CHECK(acp_value != NULL);

    struct {
        const char *name;
        const char *value;
    } cases[] = {
        { "Path", utf8_path },
        { "LD_LIBRARY_PATH", "/c/lib:/usr/lib" },
        { "ORIGINAL_PATH", "/c/original:/bin" },
        { "HOME", "/home/netdata" },
        { "SHELL", "/usr/bin/bash" },
        { "TMPDIR", "/tmp/netdata-dir" },
        { "TMP", "/tmp/netdata-tmp" },
        { "TEMP", "/tmp/netdata-temp" },
        { "ND_ENVIRONMENT_TEST_NONASCII", acp_value },
    };

    for(size_t i = 0; i < _countof(cases); i++) {
        if(cases[i].value)
            CHECK(nd_environment_context_set(environment, cases[i].name, cases[i].value, true) == 0);
    }

    ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(environment);
    CHECK(snapshot != NULL);
    if(snapshot) {
        size_t block_size = 0;
        const char *block = nd_environment_snapshot_windows_block(snapshot, &block_size);
        CHECK(block != NULL);
        CHECK(block_size >= 2);
        if(block && block_size >= 2)
            CHECK(block[block_size - 1] == '\0' && block[block_size - 2] == '\0');

        for(size_t i = 0; i < _countof(cases); i++) {
            if(!cases[i].value)
                continue;

            char *expected = test_windows_expected_raw(cases[i].name, cases[i].value);
            const char *snapshot_raw = test_windows_snapshot_raw(snapshot, cases[i].name);
            const char *block_raw = test_windows_block_raw(block, cases[i].name);
            CHECK(expected != NULL);
            CHECK(snapshot_raw != NULL);
            CHECK(block_raw != NULL);
            if(expected && snapshot_raw)
                CHECK(test_strings_equal(snapshot_raw, expected));
            if(expected && block_raw)
                CHECK(test_strings_equal(block_raw, expected));
            freez(expected);
        }
    }
    nd_environment_snapshot_release(snapshot);

    CHECK(nd_environment_context_set(environment, "shell", "", true) == 0);
    snapshot = nd_environment_snapshot_acquire(environment);
    CHECK(snapshot != NULL);
    if(snapshot) {
        char *expected = test_windows_expected_raw("SHELL", "");
        const char *raw = test_windows_snapshot_raw(snapshot, "SHELL");
        CHECK(expected != NULL);
        CHECK(raw != NULL);
        if(expected && raw)
            CHECK(test_strings_equal(raw, expected));
        freez(expected);
    }
    nd_environment_snapshot_release(snapshot);

    freez(utf8_path);
    freez(acp_value);
    nd_environment_destroy(environment);
}
#endif

#if !defined(OS_WINDOWS)
static void test_fork_child_reset_and_replace(void) {
    pid_t child = fork();
    CHECK(child >= 0);
    if(child == 0) {
        if(nd_environment_fork_child_reset_from_native() != 0)
            _exit(10);
        if(nd_environment_is_process_frozen())
            _exit(11);

        const char *const replacement[] = {
            "ONLY=yes",
            "DUP=first",
            "DUP=second",
            "OPAQUE",
            NULL,
        };
        if(nd_environment_fork_child_replace(replacement) != 0)
            _exit(12);
        if(!getenv("ONLY") || strcmp(getenv("ONLY"), "yes") != 0)
            _exit(13);

        char *dup = nd_environment_get_dup("DUP");
        if(!dup || strcmp(dup, "first") != 0)
            _exit(14);
        freez(dup);

        ND_ENV_SNAPSHOT *snapshot = nd_environment_snapshot_acquire(nd_environment_process());
        if(!snapshot)
            _exit(15);
        const char *const expected[] = {
            "ONLY=yes",
            "DUP=first",
            "DUP=second",
            "OPAQUE",
            NULL,
        };
        const char *const *actual = nd_environment_snapshot_envp(snapshot);
        for(size_t i = 0; expected[i]; i++) {
            if(!actual[i] || strcmp(actual[i], expected[i]) != 0)
                _exit(16);
        }
        nd_environment_snapshot_release(snapshot);

        if(nd_environment_freeze_process() != 0 || !nd_environment_is_process_frozen())
            _exit(17);
        _exit(0);
    }

    if(child > 0) {
        int status = 0;
        CHECK(waitpid(child, &status, 0) == child);
        CHECK(WIFEXITED(status));
        CHECK(WEXITSTATUS(status) == 0);
    }
}
#endif

int main(int argc, char **argv) {
#if defined(OS_WINDOWS)
    if(argc == 2 && strcmp(argv[1], "--windows-environment-child") == 0)
        return test_windows_child_observation();
#else
    (void)argc;
    (void)argv;
#endif

    test_preinit_contract();
    test_import_and_native_mirroring();
    test_windows_case_identity();
    test_isolated_order_duplicates_and_opaque();
    test_empty_and_owned_snapshot_destruction();
    test_snapshot_failure_atomicity();
    test_windows_block_builder();
    test_process_freeze();
#if defined(OS_WINDOWS)
    test_windows_spawned_child_observation();
    test_windows_managed_child_representation();
#endif
#if !defined(OS_WINDOWS)
    test_fork_child_reset_and_replace();
#endif
    test_concurrent_writers_and_snapshot_readers();

#if defined(OS_WINDOWS)
    freez(windows_inherited_raw);
#endif

    if(failures) {
        fprintf(stderr, "environment-unittest: %zu failure(s)\n", failures);
        return 1;
    }

    fprintf(stderr, "environment-unittest: PASS\n");
    return 0;
}
