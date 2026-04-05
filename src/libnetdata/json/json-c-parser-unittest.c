// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ============================================================================
// Wrapper functions
//
// All JSONC_PARSE_* macros contain "return false;" on error, so they must
// live inside functions returning bool.  Each wrapper isolates one macro
// call and returns true (success) or false (macro fired an error).
// ============================================================================

// --- BOOL ---
static bool wrap_parse_bool(json_object *jobj, const char *member,
                            bool *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, member, *dst, error, flags);
    return true;
}

// --- INT64 ---
static bool wrap_parse_int64(json_object *jobj, const char *member,
                             int64_t *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, member, *dst, error, flags);
    return true;
}

// --- UINT64 ---
static bool wrap_parse_uint64(json_object *jobj, const char *member,
                              uint64_t *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, member, *dst, error, flags);
    return true;
}

// --- DOUBLE ---
static bool wrap_parse_double(json_object *jobj, const char *member,
                              double *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, path, member, *dst, error, flags);
    return true;
}

// --- TXT2STRING ---
static bool wrap_parse_txt2string(json_object *jobj, const char *member,
                                  STRING **dst_ptr, BUFFER *error, int flags) {
    const char *path = "";
    STRING *dst = *dst_ptr;
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    *dst_ptr = dst;
    return true;
}

// --- TXT2STRDUPZ ---
static bool wrap_parse_txt2strdupz(json_object *jobj, const char *member,
                                   char **dst_ptr, BUFFER *error, int flags) {
    const char *path = "";
    const char *dst = *dst_ptr;
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    *dst_ptr = (char *)dst;
    return true;
}

// --- SCALAR2STRDUPZ ---
static bool wrap_parse_scalar2strdupz(json_object *jobj, const char *member,
                                      char **dst_ptr, BUFFER *error, int flags) {
    const char *path = "";
    const char *dst = *dst_ptr;
    JSONC_PARSE_SCALAR2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    *dst_ptr = (char *)dst;
    return true;
}

// --- TXT2CHAR ---
// dst must be a char array (sizeof used inside macro)
static bool wrap_parse_txt2char(json_object *jobj, const char *member,
                                char *out, BUFFER *error, int flags) {
    const char *path = "";
    char dst[256];
    dst[0] = '\0';
    JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    strncpyz(out, dst, 255);
    return true;
}

// --- TXT2BUFFER ---
static bool wrap_parse_txt2buffer(json_object *jobj, const char *member,
                                  BUFFER **dst_ptr, BUFFER *error, int flags) {
    const char *path = "";
    BUFFER *dst = *dst_ptr;
    JSONC_PARSE_TXT2BUFFER_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    *dst_ptr = dst;
    return true;
}

// --- TXT2UUID ---
static bool wrap_parse_txt2uuid(json_object *jobj, const char *member,
                                nd_uuid_t dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    return true;
}

// --- TXT2RFC3339 ---
static bool wrap_parse_txt2rfc3339(json_object *jobj, const char *member,
                                   usec_t *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_TXT2RFC3339_USEC_OR_ERROR_AND_RETURN(jobj, path, member, *dst, error, flags);
    return true;
}

// --- TXT2PATTERN ---
static bool wrap_parse_txt2pattern(json_object *jobj, const char *member,
                                   STRING **dst_ptr, BUFFER *error, int flags) {
    const char *path = "";
    STRING *dst = *dst_ptr;
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, flags);
    *dst_ptr = dst;
    return true;
}

// --- TXT2ENUM ---
static int dummy_enum_converter(const char *s) {
    if(strcmp(s, "alpha") == 0) return 1;
    if(strcmp(s, "beta") == 0)  return 2;
    return 0;
}

static bool wrap_parse_txt2enum(json_object *jobj, const char *member,
                                int *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, member, dummy_enum_converter, *dst, error, flags);
    return true;
}

// --- ARRAY_OF_TXT2BITMAP ---
static uint32_t dummy_bitmap_converter(const char *s) {
    if(strcmp(s, "read") == 0)  return 1;
    if(strcmp(s, "write") == 0) return 2;
    if(strcmp(s, "exec") == 0)  return 4;
    return 0;
}

static bool wrap_parse_array_of_txt2bitmap(json_object *jobj, const char *member,
                                           uint32_t *dst, BUFFER *error, int flags) {
    const char *path = "";
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, member, dummy_bitmap_converter, *dst, error, flags);
    return true;
}

// --- SUBOBJECT ---
static bool wrap_parse_subobject(json_object *jobj, const char *member,
                                 bool *entered, BUFFER *error, int flags) {
    char path[256] = "";
    *entered = false;
    JSONC_PARSE_SUBOBJECT(jobj, path, member, error, flags, {
        *entered = true;
    });
    return true;
}

// --- ARRAY ---
static bool wrap_parse_array(json_object *jobj, const char *member,
                             size_t *count, BUFFER *error, int flags) {
    char path[256] = "";
    *count = 0;
    JSONC_PARSE_ARRAY(jobj, path, member, error, flags, {
        *count = json_object_array_length(jobj);
    });
    return true;
}

// --- ARRAY_ITEM_OBJECT ---
static bool wrap_parse_array_item_object(json_object *jobj_in, size_t *count,
                                         BUFFER *error, int flags) {
    char path[256] = "";
    json_object *jobj = jobj_in;
    size_t index;
    *count = 0;
    JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, index, flags, {
        (*count)++;
    });
    return true;
}


// ============================================================================
// Test helpers
// ============================================================================

#define T(cond, msg) do { \
    if (!(cond)) { fprintf(stderr, "  FAILED: %s\n", msg); failed++; } \
} while(0)

#define R() buffer_flush(error)

// ============================================================================
// Test functions — each returns the number of failures (0 = all passed)
// ============================================================================

// ----------------------------------------------------------------------------
// BOOL — branches:
//   key found: boolean, string(true/yes/on), string(false/no/off),
//              string(invalid)→ALWAYS error, int, double, null→false,
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_bool(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    bool dst, ok;
    char msg[256];

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = false; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == true, "bool: boolean true");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == false, "bool: boolean false");
    json_object_put(root);

    // --- type: string truthy (case-insensitive) ---
    {
        const char *vals[] = {"true", "yes", "on", "TRUE", "Yes", "ON", NULL};
        for (int i = 0; vals[i]; i++) {
            root = json_object_new_object();
            json_object_object_add(root, "k", json_object_new_string(vals[i]));
            dst = false; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
            snprintfz(msg, sizeof(msg), "bool: str '%s'→true", vals[i]);
            T(ok && dst == true, msg);
            json_object_put(root);
        }
    }

    // --- type: string falsy (case-insensitive) ---
    {
        const char *vals[] = {"false", "no", "off", "FALSE", "No", "OFF", NULL};
        for (int i = 0; vals[i]; i++) {
            root = json_object_new_object();
            json_object_object_add(root, "k", json_object_new_string(vals[i]));
            dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
            snprintfz(msg, sizeof(msg), "bool: str '%s'→false", vals[i]);
            T(ok && dst == false, msg);
            json_object_put(root);
        }
    }

    // --- type: string invalid → ALWAYS error regardless of flags ---
    {
        const char *vals[] = {"garbage", "1", "0", "maybe", "", NULL};
        for (int i = 0; vals[i]; i++) {
            root = json_object_new_object();
            json_object_object_add(root, "k", json_object_new_string(vals[i]));
            dst = false; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_OPTIONAL);
            snprintfz(msg, sizeof(msg), "bool: invalid str '%s'+OPT→error", vals[i]);
            T(!ok, msg);
            json_object_put(root);
        }
    }

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = false; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == true, "bool: int 42→true");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(0));
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == false, "bool: int 0→false");
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst = false; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == true, "bool: double 3.14→true");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(0.0));
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == false, "bool: double 0.0→false");
    json_object_put(root);

    // --- type: null → false ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, 0);
    T(ok && dst == false, "bool: null→false");
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "bool: %s+OPT→unchanged", wtn);
        T(ok && dst == true, msg);

        dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "bool: %s+REQ→error", wtn);
        T(!ok, msg);

        dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "bool: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == true, "bool: missing+OPT→unchanged");
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "bool: missing+REQ→error");
    dst = true; R(); ok = wrap_parse_bool(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == true, "bool: missing+STRICT→unchanged");
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// INT64 — branches:
//   key found: _j==NULL→0, int, double(truncate), boolean(0/1),
//              string(valid strtoll), string(invalid)→ALWAYS error,
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
//   NOTE: json_type_null is NOT explicitly handled — falls to "other type"
// ----------------------------------------------------------------------------
static int test_parse_int64(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    int64_t dst;
    bool ok;
    char msg[256];

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == 42, "int64: int 42");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(-1));
    dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == -1, "int64: int -1");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(0));
    dst = 99; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == 0, "int64: int 0");
    json_object_put(root);

    // --- type: double (truncated) ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.7));
    dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == 3, "int64: double 3.7→3");
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == 1, "int64: bool true→1");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = 99; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == 0, "int64: bool false→0");
    json_object_put(root);

    // --- type: string (valid) ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("123"));
    dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == 123, "int64: str '123'→123");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("-456"));
    dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, 0);
    T(ok && dst == -456, "int64: str '-456'→-456");
    json_object_put(root);

    // --- type: string (invalid) → ALWAYS error ---
    {
        const char *vals[] = {"abc", "12.5", "", "0x1F", NULL};
        for (int i = 0; vals[i]; i++) {
            root = json_object_new_object();
            json_object_object_add(root, "k", json_object_new_string(vals[i]));
            dst = 0; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_OPTIONAL);
            snprintfz(msg, sizeof(msg), "int64: invalid str '%s'+OPT→error", vals[i]);
            T(!ok, msg);
            json_object_put(root);
        }
    }

    // --- null (_j==NULL): unconditionally sets dst=0, before flag checks ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == 0, "int64: null+OPT→0");
    dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_REQUIRED);
    T(ok && dst == 0, "int64: null+REQ→0");
    dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == 0, "int64: null+STRICT→0");
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "int64: %s+OPT→unchanged", wtn);
        T(ok && dst == -999, msg);

        R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "int64: %s+REQ→error", wtn);
        T(!ok, msg);

        dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "int64: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == -999, "int64: missing+OPT→unchanged");
    R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "int64: missing+REQ→error");
    dst = -999; R(); ok = wrap_parse_int64(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == -999, "int64: missing+STRICT→unchanged");
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// UINT64 — same as INT64 plus:
//   string negative → ALWAYS error (before strtoull)
//   uses get_uint64 instead of get_int64
// ----------------------------------------------------------------------------
static int test_parse_uint64(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    uint64_t dst;
    bool ok;
    char msg[256];

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = 0; R(); ok = wrap_parse_uint64(root, "k", &dst, error, 0);
    T(ok && dst == 42, "uint64: int 42");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(0));
    dst = 99; R(); ok = wrap_parse_uint64(root, "k", &dst, error, 0);
    T(ok && dst == 0, "uint64: int 0");
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.7));
    dst = 0; R(); ok = wrap_parse_uint64(root, "k", &dst, error, 0);
    T(ok && dst == 3, "uint64: double 3.7→3");
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = 0; R(); ok = wrap_parse_uint64(root, "k", &dst, error, 0);
    T(ok && dst == 1, "uint64: bool true→1");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = 99; R(); ok = wrap_parse_uint64(root, "k", &dst, error, 0);
    T(ok && dst == 0, "uint64: bool false→0");
    json_object_put(root);

    // --- type: string valid ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("123"));
    dst = 0; R(); ok = wrap_parse_uint64(root, "k", &dst, error, 0);
    T(ok && dst == 123, "uint64: str '123'→123");
    json_object_put(root);

    // --- type: string negative → ALWAYS error ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("-5"));
    dst = 0; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_OPTIONAL);
    T(!ok, "uint64: str '-5'+OPT→error (negative)");
    json_object_put(root);

    // --- type: string invalid → ALWAYS error ---
    {
        const char *vals[] = {"abc", "", "12.5", NULL};
        for (int i = 0; vals[i]; i++) {
            root = json_object_new_object();
            json_object_object_add(root, "k", json_object_new_string(vals[i]));
            dst = 0; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_OPTIONAL);
            snprintfz(msg, sizeof(msg), "uint64: invalid str '%s'+OPT→error", vals[i]);
            T(!ok, msg);
            json_object_put(root);
        }
    }

    // --- null (_j==NULL): unconditionally sets dst=0, before flag checks ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == 0, "uint64: null+OPT→0");
    dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_REQUIRED);
    T(ok && dst == 0, "uint64: null+REQ→0");
    dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == 0, "uint64: null+STRICT→0");
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "uint64: %s+OPT→unchanged", wtn);
        T(ok && dst == 999, msg);

        R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "uint64: %s+REQ→error", wtn);
        T(!ok, msg);

        dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "uint64: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == 999, "uint64: missing+OPT→unchanged");
    R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "uint64: missing+REQ→error");
    dst = 999; R(); ok = wrap_parse_uint64(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == 999, "uint64: missing+STRICT→unchanged");
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// DOUBLE — branches:
//   key found: _j==NULL→NAN, double, int(cast), boolean(0.0/1.0),
//              string(valid strtod), string(invalid)→ALWAYS error,
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
//   NOTE: json_type_null falls to "other type" (no explicit handling)
// ----------------------------------------------------------------------------
static int test_parse_double(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    double dst;
    bool ok;
    char msg[256];

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst = 0; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && (dst > 3.13 && dst < 3.15), "double: double 3.14");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(0.0));
    dst = 99; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && dst == 0.0, "double: double 0.0");
    json_object_put(root);

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = 0; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && dst == 42.0, "double: int 42→42.0");
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = 0; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && dst == 1.0, "double: bool true→1.0");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = 99; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && dst == 0.0, "double: bool false→0.0");
    json_object_put(root);

    // --- type: string valid ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("3.14"));
    dst = 0; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && (dst > 3.13 && dst < 3.15), "double: str '3.14'→3.14");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("-1.5"));
    dst = 0; R(); ok = wrap_parse_double(root, "k", &dst, error, 0);
    T(ok && dst == -1.5, "double: str '-1.5'→-1.5");
    json_object_put(root);

    // --- type: string invalid → ALWAYS error ---
    {
        const char *vals[] = {"abc", "", "1.2.3", NULL};
        for (int i = 0; vals[i]; i++) {
            root = json_object_new_object();
            json_object_object_add(root, "k", json_object_new_string(vals[i]));
            dst = 0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_OPTIONAL);
            snprintfz(msg, sizeof(msg), "double: invalid str '%s'+OPT→error", vals[i]);
            T(!ok, msg);
            json_object_put(root);
        }
    }

    // --- null (_j==NULL): unconditionally sets dst=NAN, before flag checks ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && isnan(dst), "double: null+OPT→NAN");
    dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_REQUIRED);
    T(ok && isnan(dst), "double: null+REQ→NAN");
    dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_STRICT);
    T(ok && isnan(dst), "double: null+STRICT→NAN");
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "double: %s+OPT→unchanged", wtn);
        T(ok && dst == -999.0, msg);

        R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "double: %s+REQ→error", wtn);
        T(!ok, msg);

        dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "double: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == -999.0, "double: missing+OPT→unchanged");
    R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "double: missing+REQ→error");
    dst = -999.0; R(); ok = wrap_parse_double(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == -999.0, "double: missing+STRICT→unchanged");
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2STRING — branches:
//   key found: string, int(print_int64), double(print_netdata_double),
//              boolean("true"/"false"), null→NULL,
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_txt2string(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    STRING *dst;
    bool ok;
    char msg[256];

    // --- type: string ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("hello"));
    dst = NULL; R(); ok = wrap_parse_txt2string(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(string2str(dst), "hello") == 0, "txt2string: str 'hello'");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = NULL; R(); ok = wrap_parse_txt2string(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(string2str(dst), "42") == 0, "txt2string: int 42→'42'");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst = NULL; R(); ok = wrap_parse_txt2string(root, "k", &dst, error, 0);
    T(ok && dst != NULL, "txt2string: double 3.14→string");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = NULL; R(); ok = wrap_parse_txt2string(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(string2str(dst), "true") == 0, "txt2string: bool true→'true'");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = NULL; R(); ok = wrap_parse_txt2string(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(string2str(dst), "false") == 0, "txt2string: bool false→'false'");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: null → NULL ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = string_strdupz("sentinel");
    R(); ok = wrap_parse_txt2string(root, "k", &dst, error, 0);
    T(ok && dst == NULL, "txt2string: null→NULL");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = string_strdupz("sentinel"); R();
        ok = wrap_parse_txt2string(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "txt2string: %s+OPT→unchanged", wtn);
        T(ok && dst && strcmp(string2str(dst), "sentinel") == 0, msg);
        string_freez(dst); dst = NULL;

        dst = string_strdupz("sentinel"); R();
        ok = wrap_parse_txt2string(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "txt2string: %s+REQ→error", wtn);
        T(!ok, msg);
        string_freez(dst); dst = NULL;

        dst = string_strdupz("sentinel"); R();
        ok = wrap_parse_txt2string(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "txt2string: %s+STRICT→error", wtn);
        T(!ok, msg);
        string_freez(dst); dst = NULL;

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = string_strdupz("sentinel"); R();
    ok = wrap_parse_txt2string(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst && strcmp(string2str(dst), "sentinel") == 0, "txt2string: missing+OPT→unchanged");
    string_freez(dst);

    dst = string_strdupz("sentinel"); R();
    ok = wrap_parse_txt2string(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "txt2string: missing+REQ→error");
    string_freez(dst);

    dst = string_strdupz("sentinel"); R();
    ok = wrap_parse_txt2string(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst && strcmp(string2str(dst), "sentinel") == 0, "txt2string: missing+STRICT→unchanged");
    string_freez(dst);
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2STRDUPZ — same structure as TXT2STRING but uses strdupz/freez
// ----------------------------------------------------------------------------
static int test_parse_txt2strdupz(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    char *dst;
    bool ok;
    char msg[256];

    // --- type: string ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("hello"));
    dst = NULL; R(); ok = wrap_parse_txt2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "hello") == 0, "txt2strdupz: str 'hello'");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = NULL; R(); ok = wrap_parse_txt2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "42") == 0, "txt2strdupz: int 42→'42'");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst = NULL; R(); ok = wrap_parse_txt2strdupz(root, "k", &dst, error, 0);
    T(ok && dst != NULL, "txt2strdupz: double→string");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = NULL; R(); ok = wrap_parse_txt2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "true") == 0, "txt2strdupz: bool true→'true'");
    freez(dst); dst = NULL;
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = NULL; R(); ok = wrap_parse_txt2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "false") == 0, "txt2strdupz: bool false→'false'");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: null → NULL ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = strdupz("sentinel");
    R(); ok = wrap_parse_txt2strdupz(root, "k", &dst, error, 0);
    T(ok && dst == NULL, "txt2strdupz: null→NULL");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = strdupz("sentinel"); R();
        ok = wrap_parse_txt2strdupz(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "txt2strdupz: %s+OPT→unchanged", wtn);
        T(ok && dst && strcmp(dst, "sentinel") == 0, msg);
        freez(dst);

        dst = strdupz("sentinel"); R();
        ok = wrap_parse_txt2strdupz(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "txt2strdupz: %s+REQ→error", wtn);
        T(!ok, msg);
        freez(dst);

        dst = strdupz("sentinel"); R();
        ok = wrap_parse_txt2strdupz(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "txt2strdupz: %s+STRICT→error", wtn);
        T(!ok, msg);
        freez(dst);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = strdupz("sentinel"); R();
    ok = wrap_parse_txt2strdupz(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst && strcmp(dst, "sentinel") == 0, "txt2strdupz: missing+OPT→unchanged");
    freez(dst);

    dst = strdupz("sentinel"); R();
    ok = wrap_parse_txt2strdupz(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "txt2strdupz: missing+REQ→error");
    freez(dst);

    dst = strdupz("sentinel"); R();
    ok = wrap_parse_txt2strdupz(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst && strcmp(dst, "sentinel") == 0, "txt2strdupz: missing+STRICT→unchanged");
    freez(dst);
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// SCALAR2STRDUPZ — same as TXT2STRDUPZ but different error message
//   for array/object: "non-scalar type" instead of "cannot convert to string"
// ----------------------------------------------------------------------------
static int test_parse_scalar2strdupz(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    char *dst;
    bool ok;
    char msg[256];

    // --- type: string ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("hello"));
    dst = NULL; R(); ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "hello") == 0, "scalar2strdupz: str 'hello'");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = NULL; R(); ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "42") == 0, "scalar2strdupz: int 42→'42'");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst = NULL; R(); ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, 0);
    T(ok && dst != NULL, "scalar2strdupz: double→string");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = NULL; R(); ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "true") == 0, "scalar2strdupz: bool true→'true'");
    freez(dst); dst = NULL;
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = NULL; R(); ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(dst, "false") == 0, "scalar2strdupz: bool false→'false'");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: null → NULL ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = strdupz("sentinel");
    R(); ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, 0);
    T(ok && dst == NULL, "scalar2strdupz: null→NULL");
    freez(dst); dst = NULL;
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = strdupz("sentinel"); R();
        ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "scalar2strdupz: %s+OPT→unchanged", wtn);
        T(ok && dst && strcmp(dst, "sentinel") == 0, msg);
        freez(dst);

        dst = strdupz("sentinel"); R();
        ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "scalar2strdupz: %s+REQ→error", wtn);
        T(!ok, msg);
        freez(dst);

        dst = strdupz("sentinel"); R();
        ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "scalar2strdupz: %s+STRICT→error", wtn);
        T(!ok, msg);
        freez(dst);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = strdupz("sentinel"); R();
    ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst && strcmp(dst, "sentinel") == 0, "scalar2strdupz: missing+OPT→unchanged");
    freez(dst);

    dst = strdupz("sentinel"); R();
    ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "scalar2strdupz: missing+REQ→error");
    freez(dst);

    dst = strdupz("sentinel"); R();
    ok = wrap_parse_scalar2strdupz(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst && strcmp(dst, "sentinel") == 0, "scalar2strdupz: missing+STRICT→unchanged");
    freez(dst);
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2CHAR — branches:
//   key found: string, int(print_int64), double(print_netdata_double),
//              boolean("true"/"false"), null→"",
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: ALWAYS clears dst[0]='\0', then REQ→error
// ----------------------------------------------------------------------------
static int test_parse_txt2char(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    char dst[256];
    bool ok;
    char msg[256];

    // --- type: string ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("hello"));
    dst[0] = 0; R(); ok = wrap_parse_txt2char(root, "k", dst, error, 0);
    T(ok && strcmp(dst, "hello") == 0, "txt2char: str 'hello'");
    json_object_put(root);

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst[0] = 0; R(); ok = wrap_parse_txt2char(root, "k", dst, error, 0);
    T(ok && strcmp(dst, "42") == 0, "txt2char: int 42→'42'");
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst[0] = 0; R(); ok = wrap_parse_txt2char(root, "k", dst, error, 0);
    T(ok && dst[0] != '\0', "txt2char: double→string");
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst[0] = 0; R(); ok = wrap_parse_txt2char(root, "k", dst, error, 0);
    T(ok && strcmp(dst, "true") == 0, "txt2char: bool true→'true'");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst[0] = 0; R(); ok = wrap_parse_txt2char(root, "k", dst, error, 0);
    T(ok && strcmp(dst, "false") == 0, "txt2char: bool false→'false'");
    json_object_put(root);

    // --- type: null → "" ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    strncpyz(dst, "sentinel", sizeof(dst) - 1);
    R(); ok = wrap_parse_txt2char(root, "k", dst, error, 0);
    T(ok && dst[0] == '\0', "txt2char: null→empty");
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    // NOTE: wrapper always initializes its local dst to '\0'. On OPTIONAL
    // (macro skips), the wrapper copies empty string to out. On error
    // (macro returns false), the wrapper returns before copying, so out
    // is NOT updated.
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        strncpyz(dst, "sentinel", sizeof(dst) - 1); R();
        ok = wrap_parse_txt2char(root, "k", dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "txt2char: %s+OPT→no error", wtn);
        T(ok, msg);

        strncpyz(dst, "sentinel", sizeof(dst) - 1); R();
        ok = wrap_parse_txt2char(root, "k", dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "txt2char: %s+REQ→error", wtn);
        T(!ok, msg);

        strncpyz(dst, "sentinel", sizeof(dst) - 1); R();
        ok = wrap_parse_txt2char(root, "k", dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "txt2char: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key: macro ALWAYS clears dst, then REQ→error ---
    // On error, wrapper returns before copying → out unchanged
    root = json_object_new_object();

    strncpyz(dst, "sentinel", sizeof(dst) - 1); R();
    ok = wrap_parse_txt2char(root, "k", dst, error, JSONC_OPTIONAL);
    T(ok && dst[0] == '\0', "txt2char: missing+OPT→cleared");

    strncpyz(dst, "sentinel", sizeof(dst) - 1); R();
    ok = wrap_parse_txt2char(root, "k", dst, error, JSONC_REQUIRED);
    T(!ok, "txt2char: missing+REQ→error");

    strncpyz(dst, "sentinel", sizeof(dst) - 1); R();
    ok = wrap_parse_txt2char(root, "k", dst, error, JSONC_STRICT);
    T(ok && dst[0] == '\0', "txt2char: missing+STRICT→cleared");

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2BUFFER — branches:
//   key found: string(non-empty→buffer, empty→NULL), int, double,
//              boolean, null→NULL,
//              other+OPT→skip(_type_ok=false), other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_txt2buffer(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    BUFFER *dst;
    bool ok;
    char msg[256];

    // --- type: string non-empty ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("hello"));
    dst = NULL; R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(buffer_tostring(dst), "hello") == 0, "txt2buffer: str 'hello'");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- type: string empty → NULL ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string(""));
    dst = buffer_create(0, NULL);
    buffer_strcat(dst, "sentinel");
    R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst == NULL, "txt2buffer: str ''→NULL");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- type: int ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_int64(42));
    dst = NULL; R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(buffer_tostring(dst), "42") == 0, "txt2buffer: int 42→'42'");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- type: double ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_double(3.14));
    dst = NULL; R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst != NULL, "txt2buffer: double→buffer");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- type: boolean ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(1));
    dst = NULL; R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(buffer_tostring(dst), "true") == 0, "txt2buffer: bool true→'true'");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_boolean(0));
    dst = NULL; R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(buffer_tostring(dst), "false") == 0, "txt2buffer: bool false→'false'");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- type: null → NULL ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    dst = buffer_create(0, NULL);
    buffer_strcat(dst, "sentinel");
    R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst == NULL, "txt2buffer: null→NULL");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- type: string into existing buffer (flush+reuse) ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("new"));
    dst = buffer_create(0, NULL);
    buffer_strcat(dst, "old");
    R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(buffer_tostring(dst), "new") == 0, "txt2buffer: str overwrites existing");
    buffer_free(dst); dst = NULL;
    json_object_put(root);

    // --- wrong type (array, object) × 3 flags ---
    for (int wt = 0; wt < 2; wt++) {
        root = json_object_new_object();
        json_object_object_add(root, "k", wt == 0 ? json_object_new_array() : json_object_new_object());
        const char *wtn = wt == 0 ? "array" : "object";

        dst = buffer_create(0, NULL);
        buffer_strcat(dst, "sentinel");
        R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "txt2buffer: %s+OPT→unchanged", wtn);
        T(ok && dst && strcmp(buffer_tostring(dst), "sentinel") == 0, msg);
        buffer_free(dst);

        dst = buffer_create(0, NULL); R();
        ok = wrap_parse_txt2buffer(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "txt2buffer: %s+REQ→error", wtn);
        T(!ok, msg);
        buffer_free(dst);

        dst = buffer_create(0, NULL); R();
        ok = wrap_parse_txt2buffer(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "txt2buffer: %s+STRICT→error", wtn);
        T(!ok, msg);
        buffer_free(dst);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();

    dst = buffer_create(0, NULL);
    buffer_strcat(dst, "sentinel");
    R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst && strcmp(buffer_tostring(dst), "sentinel") == 0, "txt2buffer: missing+OPT→unchanged");
    buffer_free(dst);

    dst = buffer_create(0, NULL); R();
    ok = wrap_parse_txt2buffer(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "txt2buffer: missing+REQ→error");
    buffer_free(dst);

    dst = buffer_create(0, NULL);
    buffer_strcat(dst, "sentinel");
    R(); ok = wrap_parse_txt2buffer(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst && strcmp(buffer_tostring(dst), "sentinel") == 0, "txt2buffer: missing+STRICT→unchanged");
    buffer_free(dst);

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2UUID — branches:
//   key found: string+valid UUID→parsed, string+invalid UUID+OPT→uuid_clear,
//              string+invalid UUID+REQ→error, string+invalid UUID+STRICT→error,
//              null→uuid_clear,
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_txt2uuid(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    nd_uuid_t dst;
    bool ok;
    char msg[256];
    static const nd_uuid_t zero_uuid = { 0 };

    // --- type: string valid UUID ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("550e8400-e29b-41d4-a716-446655440000"));
    memset(dst, 0, sizeof(nd_uuid_t));
    R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, 0);
    T(ok && memcmp(dst, zero_uuid, sizeof(nd_uuid_t)) != 0, "txt2uuid: valid UUID parsed");
    json_object_put(root);

    // --- type: string invalid UUID + OPTIONAL → uuid_clear ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("not-a-uuid"));
    memset(dst, 0xFF, sizeof(nd_uuid_t));
    R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_OPTIONAL);
    T(ok && memcmp(dst, zero_uuid, sizeof(nd_uuid_t)) == 0, "txt2uuid: invalid UUID+OPT→uuid_clear");
    json_object_put(root);

    // --- type: string invalid UUID + REQUIRED → error ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("not-a-uuid"));
    R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_REQUIRED);
    T(!ok, "txt2uuid: invalid UUID+REQ→error");
    json_object_put(root);

    // --- type: string invalid UUID + STRICT → error ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("not-a-uuid"));
    R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_STRICT);
    T(!ok, "txt2uuid: invalid UUID+STRICT→error");
    json_object_put(root);

    // --- type: null → uuid_clear ---
    root = json_object_new_object();
    json_object_object_add(root, "k", NULL);
    memset(dst, 0xFF, sizeof(nd_uuid_t));
    R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, 0);
    T(ok && memcmp(dst, zero_uuid, sizeof(nd_uuid_t)) == 0, "txt2uuid: null→uuid_clear");
    json_object_put(root);

    // --- wrong type (int, array, object) × 3 flags ---
    {
        for (int wt = 0; wt < 3; wt++) {
            root = json_object_new_object();
            if (wt == 0) json_object_object_add(root, "k", json_object_new_int64(42));
            else if (wt == 1) json_object_object_add(root, "k", json_object_new_array());
            else json_object_object_add(root, "k", json_object_new_object());
            const char *wtn = (wt == 0) ? "int" : (wt == 1) ? "array" : "object";

            memset(dst, 0xFF, sizeof(nd_uuid_t)); R();
            ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_OPTIONAL);
            snprintfz(msg, sizeof(msg), "txt2uuid: %s+OPT→unchanged", wtn);
            T(ok, msg);

            R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_REQUIRED);
            snprintfz(msg, sizeof(msg), "txt2uuid: %s+REQ→error", wtn);
            T(!ok, msg);

            R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_STRICT);
            snprintfz(msg, sizeof(msg), "txt2uuid: %s+STRICT→error", wtn);
            T(!ok, msg);

            json_object_put(root);
        }
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    memset(dst, 0xFF, sizeof(nd_uuid_t)); R();
    ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_OPTIONAL);
    T(ok, "txt2uuid: missing+OPT→no error");

    R(); ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_REQUIRED);
    T(!ok, "txt2uuid: missing+REQ→error");

    memset(dst, 0xFF, sizeof(nd_uuid_t)); R();
    ok = wrap_parse_txt2uuid(root, "k", dst, error, JSONC_STRICT);
    T(ok, "txt2uuid: missing+STRICT→no error");

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2RFC3339 — branches:
//   key found: string → rfc3339_parse_ut,
//              other type → dst=0, then OPT→ok, REQ→error, STRICT→error
//   key missing: dst=0, then OPT→ok, REQ→error, STRICT→ok
//   NOTE: ALL non-string types (including null) → dst=0, flag check
// ----------------------------------------------------------------------------
static int test_parse_txt2rfc3339(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    usec_t dst;
    bool ok;
    char msg[256];

    // --- type: string valid RFC3339 ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("2024-01-15T10:30:00Z"));
    dst = 0; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, 0);
    T(ok && dst != 0, "txt2rfc3339: valid RFC3339");
    json_object_put(root);

    // --- non-string types → dst=0, flag check ---
    {
        struct { const char *name; int type_id; } types[] = {
            {"int", 0}, {"double", 1}, {"boolean", 2}, {"null", 3},
            {"array", 4}, {"object", 5},
        };
        for (int i = 0; i < 6; i++) {
            root = json_object_new_object();
            switch(i) {
                case 0: json_object_object_add(root, "k", json_object_new_int64(42)); break;
                case 1: json_object_object_add(root, "k", json_object_new_double(3.14)); break;
                case 2: json_object_object_add(root, "k", json_object_new_boolean(1)); break;
                case 3: json_object_object_add(root, "k", NULL); break;
                case 4: json_object_object_add(root, "k", json_object_new_array()); break;
                case 5: json_object_object_add(root, "k", json_object_new_object()); break;
            }

            dst = 999; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, JSONC_OPTIONAL);
            snprintfz(msg, sizeof(msg), "txt2rfc3339: %s+OPT→dst=0,ok", types[i].name);
            T(ok && dst == 0, msg);

            dst = 999; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, JSONC_REQUIRED);
            snprintfz(msg, sizeof(msg), "txt2rfc3339: %s+REQ→error", types[i].name);
            T(!ok && dst == 0, msg);

            dst = 999; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, JSONC_STRICT);
            snprintfz(msg, sizeof(msg), "txt2rfc3339: %s+STRICT→error", types[i].name);
            T(!ok && dst == 0, msg);

            json_object_put(root);
        }
    }

    // --- missing key: dst=0, flag check ---
    root = json_object_new_object();

    dst = 999; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == 0, "txt2rfc3339: missing+OPT→dst=0,ok");

    dst = 999; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok && dst == 0, "txt2rfc3339: missing+REQ→dst=0,error");

    dst = 999; R(); ok = wrap_parse_txt2rfc3339(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == 0, "txt2rfc3339: missing+STRICT→dst=0,ok");

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2PATTERN — branches:
//   key found: string "*"→NULL, string other→string_strdupz,
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_txt2pattern(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    STRING *dst;
    bool ok;
    char msg[256];

    // --- type: string normal ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("hello"));
    dst = NULL; R(); ok = wrap_parse_txt2pattern(root, "k", &dst, error, 0);
    T(ok && dst && strcmp(string2str(dst), "hello") == 0, "txt2pattern: str 'hello'");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- type: string "*" → NULL (wildcard) ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("*"));
    dst = string_strdupz("sentinel");
    R(); ok = wrap_parse_txt2pattern(root, "k", &dst, error, 0);
    T(ok && dst == NULL, "txt2pattern: str '*'→NULL (wildcard)");
    string_freez(dst); dst = NULL;
    json_object_put(root);

    // --- non-string types × 3 flags ---
    for (int wt = 0; wt < 3; wt++) {
        root = json_object_new_object();
        if (wt == 0) json_object_object_add(root, "k", json_object_new_int64(42));
        else if (wt == 1) json_object_object_add(root, "k", json_object_new_array());
        else json_object_object_add(root, "k", json_object_new_object());
        const char *wtn = (wt == 0) ? "int" : (wt == 1) ? "array" : "object";

        dst = string_strdupz("sentinel"); R();
        ok = wrap_parse_txt2pattern(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "txt2pattern: %s+OPT→unchanged", wtn);
        T(ok && dst && strcmp(string2str(dst), "sentinel") == 0, msg);
        string_freez(dst);

        dst = string_strdupz("sentinel"); R();
        ok = wrap_parse_txt2pattern(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "txt2pattern: %s+REQ→error", wtn);
        T(!ok, msg);
        string_freez(dst);

        dst = string_strdupz("sentinel"); R();
        ok = wrap_parse_txt2pattern(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "txt2pattern: %s+STRICT→error", wtn);
        T(!ok, msg);
        string_freez(dst);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = string_strdupz("sentinel"); R();
    ok = wrap_parse_txt2pattern(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst && strcmp(string2str(dst), "sentinel") == 0, "txt2pattern: missing+OPT→unchanged");
    string_freez(dst);

    dst = string_strdupz("sentinel"); R();
    ok = wrap_parse_txt2pattern(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "txt2pattern: missing+REQ→error");
    string_freez(dst);

    dst = string_strdupz("sentinel"); R();
    ok = wrap_parse_txt2pattern(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst && strcmp(string2str(dst), "sentinel") == 0, "txt2pattern: missing+STRICT→unchanged");
    string_freez(dst);

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// TXT2ENUM — branches:
//   key found: string → converter(str),
//              other+OPT→skip, other+REQ→error, other+STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_txt2enum(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    int dst;
    bool ok;
    char msg[256];

    // --- type: string known value ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("alpha"));
    dst = 0; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, 0);
    T(ok && dst == 1, "txt2enum: str 'alpha'→1");
    json_object_put(root);

    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("beta"));
    dst = 0; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, 0);
    T(ok && dst == 2, "txt2enum: str 'beta'→2");
    json_object_put(root);

    // --- type: string unknown → converter returns 0 ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_string("unknown"));
    dst = 99; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, 0);
    T(ok && dst == 0, "txt2enum: str 'unknown'→0");
    json_object_put(root);

    // --- non-string types × 3 flags ---
    for (int wt = 0; wt < 4; wt++) {
        root = json_object_new_object();
        if (wt == 0) json_object_object_add(root, "k", json_object_new_int64(1));
        else if (wt == 1) json_object_object_add(root, "k", json_object_new_boolean(1));
        else if (wt == 2) json_object_object_add(root, "k", json_object_new_array());
        else json_object_object_add(root, "k", json_object_new_object());
        const char *wtn = (wt == 0) ? "int" : (wt == 1) ? "bool" : (wt == 2) ? "array" : "object";

        dst = -999; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "txt2enum: %s+OPT→unchanged", wtn);
        T(ok && dst == -999, msg);

        R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "txt2enum: %s+REQ→error", wtn);
        T(!ok, msg);

        dst = -999; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "txt2enum: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = -999; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == -999, "txt2enum: missing+OPT→unchanged");
    R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "txt2enum: missing+REQ→error");
    dst = -999; R(); ok = wrap_parse_txt2enum(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == -999, "txt2enum: missing+STRICT→unchanged");
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// ARRAY_OF_TXT2BITMAP — branches:
//   key found + array: all string → OR bits, non-string item → ALWAYS error,
//                      unknown string (converter→0) → error msg but continues
//   key found + non-array: OPT→skip, REQ→error, STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_array_of_txt2bitmap(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    uint32_t dst;
    bool ok;
    char msg[256];

    // --- happy path: ["read","write"] → 3 ---
    {
        root = json_object_new_object();
        json_object *arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_string("read"));
        json_object_array_add(arr, json_object_new_string("write"));
        json_object_object_add(root, "k", arr);
        dst = 0; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, 0);
        T(ok && dst == 3, "bitmap: ['read','write']→3");
        json_object_put(root);
    }

    // --- happy path: ["exec"] → 4 ---
    {
        root = json_object_new_object();
        json_object *arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_string("exec"));
        json_object_object_add(root, "k", arr);
        dst = 0; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, 0);
        T(ok && dst == 4, "bitmap: ['exec']→4");
        json_object_put(root);
    }

    // --- empty array → dst=0 ---
    {
        root = json_object_new_object();
        json_object_object_add(root, "k", json_object_new_array());
        dst = 99; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, 0);
        T(ok && dst == 0, "bitmap: []→0");
        json_object_put(root);
    }

    // --- non-string item in array → ALWAYS error ---
    {
        root = json_object_new_object();
        json_object *arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_int64(42));
        json_object_object_add(root, "k", arr);
        dst = 0; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_OPTIONAL);
        T(!ok, "bitmap: non-string item+OPT→error");
        json_object_put(root);
    }

    // --- unknown string (converter returns 0) → error msg but no return false ---
    {
        root = json_object_new_object();
        json_object *arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_string("read"));
        json_object_array_add(arr, json_object_new_string("unknown"));
        json_object_object_add(root, "k", arr);
        dst = 0; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, 0);
        // error message written but return false is commented out, so ok=true
        T(ok && dst == 1, "bitmap: unknown string→error msg, continues, dst=1");
        json_object_put(root);
    }

    // --- non-array type × 3 flags ---
    for (int wt = 0; wt < 3; wt++) {
        root = json_object_new_object();
        if (wt == 0) json_object_object_add(root, "k", json_object_new_string("read"));
        else if (wt == 1) json_object_object_add(root, "k", json_object_new_int64(1));
        else json_object_object_add(root, "k", json_object_new_object());
        const char *wtn = (wt == 0) ? "string" : (wt == 1) ? "int" : "object";

        dst = 999; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "bitmap: %s+OPT→unchanged", wtn);
        T(ok && dst == 999, msg);

        R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "bitmap: %s+REQ→error", wtn);
        T(!ok, msg);

        dst = 999; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "bitmap: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();
    dst = 999; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_OPTIONAL);
    T(ok && dst == 999, "bitmap: missing+OPT→unchanged");
    R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_REQUIRED);
    T(!ok, "bitmap: missing+REQ→error");
    dst = 999; R(); ok = wrap_parse_array_of_txt2bitmap(root, "k", &dst, error, JSONC_STRICT);
    T(ok && dst == 999, "bitmap: missing+STRICT→unchanged");
    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// SUBOBJECT — branches:
//   key found + object → enter block
//   key found + non-object: OPT→skip, REQ→error, STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_subobject(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    bool entered, ok;
    char msg[256];

    // --- happy path: object present → block entered ---
    root = json_object_new_object();
    json_object_object_add(root, "k", json_object_new_object());
    R(); ok = wrap_parse_subobject(root, "k", &entered, error, 0);
    T(ok && entered, "subobject: object→entered");
    json_object_put(root);

    // --- non-object type × 3 flags ---
    for (int wt = 0; wt < 3; wt++) {
        root = json_object_new_object();
        if (wt == 0) json_object_object_add(root, "k", json_object_new_string("str"));
        else if (wt == 1) json_object_object_add(root, "k", json_object_new_int64(42));
        else json_object_object_add(root, "k", json_object_new_array());
        const char *wtn = (wt == 0) ? "string" : (wt == 1) ? "int" : "array";

        R(); ok = wrap_parse_subobject(root, "k", &entered, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "subobject: %s+OPT→not entered,ok", wtn);
        T(ok && !entered, msg);

        R(); ok = wrap_parse_subobject(root, "k", &entered, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "subobject: %s+REQ→error", wtn);
        T(!ok, msg);

        R(); ok = wrap_parse_subobject(root, "k", &entered, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "subobject: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();

    R(); ok = wrap_parse_subobject(root, "k", &entered, error, JSONC_OPTIONAL);
    T(ok && !entered, "subobject: missing+OPT→not entered,ok");

    R(); ok = wrap_parse_subobject(root, "k", &entered, error, JSONC_REQUIRED);
    T(!ok, "subobject: missing+REQ→error");

    R(); ok = wrap_parse_subobject(root, "k", &entered, error, JSONC_STRICT);
    T(ok && !entered, "subobject: missing+STRICT→not entered,ok");

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// ARRAY — branches:
//   key found + array → enter block
//   key found + non-array: OPT→skip, REQ→error, STRICT→error
//   key missing: OPT→skip, REQ→error, STRICT→skip
// ----------------------------------------------------------------------------
static int test_parse_array(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *root;
    size_t count;
    bool ok;
    char msg[256];

    // --- happy path: array present → block entered, correct length ---
    {
        root = json_object_new_object();
        json_object *arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_int64(1));
        json_object_array_add(arr, json_object_new_int64(2));
        json_object_array_add(arr, json_object_new_int64(3));
        json_object_object_add(root, "k", arr);
        R(); ok = wrap_parse_array(root, "k", &count, error, 0);
        T(ok && count == 3, "array: present→entered, len=3");
        json_object_put(root);
    }

    // --- empty array ---
    {
        root = json_object_new_object();
        json_object_object_add(root, "k", json_object_new_array());
        R(); ok = wrap_parse_array(root, "k", &count, error, 0);
        T(ok && count == 0, "array: empty→entered, len=0");
        json_object_put(root);
    }

    // --- non-array type × 3 flags ---
    for (int wt = 0; wt < 3; wt++) {
        root = json_object_new_object();
        if (wt == 0) json_object_object_add(root, "k", json_object_new_string("str"));
        else if (wt == 1) json_object_object_add(root, "k", json_object_new_int64(42));
        else json_object_object_add(root, "k", json_object_new_object());
        const char *wtn = (wt == 0) ? "string" : (wt == 1) ? "int" : "object";

        count = 999; R(); ok = wrap_parse_array(root, "k", &count, error, JSONC_OPTIONAL);
        snprintfz(msg, sizeof(msg), "array: %s+OPT→not entered,ok", wtn);
        T(ok && count == 0, msg);

        R(); ok = wrap_parse_array(root, "k", &count, error, JSONC_REQUIRED);
        snprintfz(msg, sizeof(msg), "array: %s+REQ→error", wtn);
        T(!ok, msg);

        R(); ok = wrap_parse_array(root, "k", &count, error, JSONC_STRICT);
        snprintfz(msg, sizeof(msg), "array: %s+STRICT→error", wtn);
        T(!ok, msg);

        json_object_put(root);
    }

    // --- missing key × 3 flags ---
    root = json_object_new_object();

    count = 999; R(); ok = wrap_parse_array(root, "k", &count, error, JSONC_OPTIONAL);
    T(ok && count == 0, "array: missing+OPT→not entered,ok");

    R(); ok = wrap_parse_array(root, "k", &count, error, JSONC_REQUIRED);
    T(!ok, "array: missing+REQ→error");

    count = 999; R(); ok = wrap_parse_array(root, "k", &count, error, JSONC_STRICT);
    T(ok && count == 0, "array: missing+STRICT→not entered,ok");

    json_object_put(root);

    buffer_free(error);
    return failed;
}

// ----------------------------------------------------------------------------
// ARRAY_ITEM_OBJECT — branches:
//   item is object → enter block
//   item is non-object: OPT→skip, REQ→error, STRICT→error
//   empty array → no iterations
// ----------------------------------------------------------------------------
static int test_parse_array_item_object(void) {
    int failed = 0;
    BUFFER *error = buffer_create(0, NULL);
    json_object *arr;
    size_t count;
    bool ok;

    // --- all items are objects ---
    {
        arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_object());
        json_object_array_add(arr, json_object_new_object());
        json_object_array_add(arr, json_object_new_object());
        R(); ok = wrap_parse_array_item_object(arr, &count, error, 0);
        T(ok && count == 3, "array_item_object: 3 objects→count=3");
        json_object_put(arr);
    }

    // --- empty array ---
    {
        arr = json_object_new_array();
        R(); ok = wrap_parse_array_item_object(arr, &count, error, 0);
        T(ok && count == 0, "array_item_object: empty→count=0");
        json_object_put(arr);
    }

    // --- non-object item + OPTIONAL → skipped ---
    {
        arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_object());
        json_object_array_add(arr, json_object_new_string("not_obj"));
        json_object_array_add(arr, json_object_new_object());
        R(); ok = wrap_parse_array_item_object(arr, &count, error, JSONC_OPTIONAL);
        T(ok && count == 2, "array_item_object: non-obj+OPT→skipped, count=2");
        json_object_put(arr);
    }

    // --- non-object item + REQUIRED → error ---
    {
        arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_string("not_obj"));
        R(); ok = wrap_parse_array_item_object(arr, &count, error, JSONC_REQUIRED);
        T(!ok, "array_item_object: non-obj+REQ→error");
        json_object_put(arr);
    }

    // --- non-object item + STRICT → error ---
    {
        arr = json_object_new_array();
        json_object_array_add(arr, json_object_new_int64(42));
        R(); ok = wrap_parse_array_item_object(arr, &count, error, JSONC_STRICT);
        T(!ok, "array_item_object: non-obj+STRICT→error");
        json_object_put(arr);
    }

    buffer_free(error);
    return failed;
}

// ============================================================================
// Entry point
// ============================================================================

#undef T
#undef R

int json_c_parser_unittest(void) {
    struct {
        const char *name;
        int (*func)(void);
    } tests[] = {
        { "BOOL",                test_parse_bool },
        { "INT64",               test_parse_int64 },
        { "UINT64",              test_parse_uint64 },
        { "DOUBLE",              test_parse_double },
        { "TXT2STRING",          test_parse_txt2string },
        { "TXT2STRDUPZ",        test_parse_txt2strdupz },
        { "SCALAR2STRDUPZ",     test_parse_scalar2strdupz },
        { "TXT2CHAR",           test_parse_txt2char },
        { "TXT2BUFFER",         test_parse_txt2buffer },
        { "TXT2UUID",           test_parse_txt2uuid },
        { "TXT2RFC3339",        test_parse_txt2rfc3339 },
        { "TXT2PATTERN",        test_parse_txt2pattern },
        { "TXT2ENUM",           test_parse_txt2enum },
        { "ARRAY_OF_TXT2BITMAP", test_parse_array_of_txt2bitmap },
        { "SUBOBJECT",          test_parse_subobject },
        { "ARRAY",              test_parse_array },
        { "ARRAY_ITEM_OBJECT",  test_parse_array_item_object },
        { NULL, NULL }
    };

    int total_failed = 0;
    fprintf(stderr, "\n%s\n", "JSON-C Parser Unit Tests");
    fprintf(stderr, "%s\n", "========================");

    for (int i = 0; tests[i].name; i++) {
        int f = tests[i].func();
        if (f)
            fprintf(stderr, "  %-25s FAILED (%d failures)\n", tests[i].name, f);
        else
            fprintf(stderr, "  %-25s PASSED\n", tests[i].name);
        total_failed += f;
    }

    fprintf(stderr, "\nTotal: %d failures\n\n", total_failed);
    return total_failed;
}
