// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/stat.h>
#include <unistd.h>

// Include the implementation; the test drives it only through api_v3_settings().
#include "../v3/api_v3_settings.c"

RRDHOST *localhost = NULL;

static char settings_test_root[FILENAME_MAX];

static int settings_test_expect(bool condition, const char *name, const char *detail) {
    if(condition)
        return 0;

    fprintf(stderr, "FAILED: %s%s%s\n", name, detail ? ": " : "", detail ? detail : "");
    return 1;
}

static void settings_test_path(char out[FILENAME_MAX], const char *file) {
    snprintfz(out, FILENAME_MAX, "%s/settings/%s", settings_test_root, file);
}

static void settings_test_reset(void) {
    const char *files[] = { "default", "default.new", "my-dashboard" };
    for(size_t i = 0; i < sizeof(files) / sizeof(files[0]); i++) {
        char path[FILENAME_MAX];
        settings_test_path(path, files[i]);
        (void)unlink(path);
    }
}

static void settings_test_store(const char *json) {
    char path[FILENAME_MAX];
    settings_test_path(path, "");
    (void)mkdir(path, 0700);
    settings_test_path(path, "default");
    FILE *fp = fopen(path, "w");
    if(fp) {
        fputs(json, fp);
        fclose(fp);
    }
}

static bool settings_test_exists(const char *file) {
    char path[FILENAME_MAX];
    settings_test_path(path, file);
    struct stat st;
    return stat(path, &st) == 0;
}

// run one request; the response body is left in *body (caller frees)
static int settings_test_call(HTTP_REQUEST_MODE mode, bool bearer, const char *query, const char *payload, BUFFER **body) {
    struct web_client w;
    memset(&w, 0, sizeof(w));
    w.mode = mode;
    w.user_auth.method = bearer ? USER_AUTH_METHOD_BEARER : USER_AUTH_METHOD_NONE;
    w.response.data = buffer_create(0, NULL);
    if(payload) {
        w.payload = buffer_create(0, NULL);
        buffer_strcat(w.payload, payload);
    }

    char url[1024];
    strncpyz(url, query, sizeof(url) - 1);

    int code = api_v3_settings(localhost, &w, url);

    buffer_free(w.payload);
    *body = w.response.data;
    return code;
}

static int settings_test_code(const char *name, HTTP_REQUEST_MODE mode, bool bearer, const char *query,
                              const char *payload, int expected) {
    BUFFER *body = NULL;
    int code = settings_test_call(mode, bearer, query, payload, &body);

    char detail[256];
    snprintfz(detail, sizeof(detail), "expected HTTP %d, got %d, body '%s'", expected, code, buffer_tostring(body));
    buffer_free(body);
    return settings_test_expect(code == expected, name, detail);
}

// GET the object and compare its version and the value of member "marker" (NULL = must be absent)
static int settings_test_stored(const char *name, int64_t version, const char *marker) {
    BUFFER *body = NULL;
    int code = settings_test_call(HTTP_REQUEST_MODE_GET, false, "file=default", NULL, &body);

    int64_t got_version = -1;
    const char *got_marker = NULL;
    bool version_is_int = false;
    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(buffer_tostring(body));
    struct json_object *o;
    if(jobj && json_object_object_get_ex(jobj, "version", &o)) {
        version_is_int = json_object_is_type(o, json_type_int);
        got_version = json_object_get_int64(o);
    }
    if(jobj && json_object_object_get_ex(jobj, "marker", &o))
        got_marker = json_object_get_string(o);

    bool ok = code == HTTP_RESP_OK && version_is_int && got_version == version &&
              ((!marker && !got_marker) || (marker && got_marker && !strcmp(marker, got_marker)));

    char detail[512];
    snprintfz(detail, sizeof(detail), "expected version %lld marker '%s', got HTTP %d body '%s'",
              (long long)version, marker ? marker : "(none)", code, buffer_tostring(body));
    buffer_free(body);
    return settings_test_expect(ok, name, detail);
}

static int settings_test_default_object(void) {
    int errors = 0;
    settings_test_reset();

    errors += settings_test_stored("a missing object reads as version 1", 1, NULL);
    errors += settings_test_code("GET without file is the default object", HTTP_REQUEST_MODE_GET, false, "", NULL, HTTP_RESP_OK);
    errors += settings_test_code("GET with an empty file is the default object", HTTP_REQUEST_MODE_GET, false, "file=", NULL, HTTP_RESP_OK);
    errors += settings_test_code("anonymous PUT of default", HTTP_REQUEST_MODE_PUT, false, "file=default",
                                 "{\"version\":1,\"marker\":\"a\"}", HTTP_RESP_OK);
    errors += settings_test_stored("PUT increments the version", 2, "a");
    errors += settings_test_code("stale version conflicts", HTTP_REQUEST_MODE_PUT, false, "file=default",
                                 "{\"version\":1,\"marker\":\"stale\"}", HTTP_RESP_CONFLICT);
    errors += settings_test_code("bearer PUT of default without file", HTTP_REQUEST_MODE_PUT, true, "",
                                 "{\"version\":2,\"marker\":\"b\"}", HTTP_RESP_OK);
    errors += settings_test_stored("anonymous and bearer callers share default", 3, "b");
    errors += settings_test_code("PUT without payload", HTTP_REQUEST_MODE_PUT, false, "file=default", NULL, HTTP_RESP_BAD_REQUEST);
    errors += settings_test_code("PUT without version", HTTP_REQUEST_MODE_PUT, false, "file=default",
                                 "{\"marker\":\"x\"}", HTTP_RESP_BAD_REQUEST);
    errors += settings_test_stored("rejected PUTs leave the object unchanged", 3, "b");
    return errors;
}

static int settings_test_only_default(void) {
    int errors = 0;
    settings_test_reset();

    errors += settings_test_code("bearer PUT of a named file is rejected", HTTP_REQUEST_MODE_PUT, true, "file=my-dashboard",
                                 "{\"version\":1,\"marker\":\"n\"}", HTTP_RESP_BAD_REQUEST);
    errors += settings_test_expect(!settings_test_exists("my-dashboard"), "a rejected named PUT creates no file", NULL);
    errors += settings_test_code("bearer GET of a named file is rejected", HTTP_REQUEST_MODE_GET, true, "file=my-dashboard",
                                 NULL, HTTP_RESP_BAD_REQUEST);
    errors += settings_test_code("anonymous GET of a named file is rejected", HTTP_REQUEST_MODE_GET, false, "file=my-dashboard",
                                 NULL, HTTP_RESP_BAD_REQUEST);
    errors += settings_test_code("a path in file is rejected", HTTP_REQUEST_MODE_GET, true, "file=../default",
                                 NULL, HTTP_RESP_BAD_REQUEST);
    errors += settings_test_code("any non-default file parameter is rejected", HTTP_REQUEST_MODE_GET, true,
                                 "file=my-dashboard&file=default", NULL, HTTP_RESP_BAD_REQUEST);
    return errors;
}

static int settings_test_version_types(void) {
    int errors = 0;
    settings_test_reset();

    const char *bad[] = {
        "{\"version\":\"1\",\"marker\":\"s\"}",
        "{\"version\":1.0,\"marker\":\"d\"}",
        "{\"version\":true,\"marker\":\"t\"}",
        "{\"version\":null,\"marker\":\"n\"}",
        "{\"version\":0,\"marker\":\"z\"}",
        "{\"version\":-1,\"marker\":\"m\"}",
        "{\"version\":9223372036854775807,\"marker\":\"max\"}",
        "{\"version\":18446744073709551615,\"marker\":\"u64\"}",
        "[1]",
    };
    for(size_t i = 0; i < sizeof(bad) / sizeof(bad[0]); i++) {
        char name[256];
        snprintfz(name, sizeof(name), "invalid version is rejected: %s", bad[i]);
        errors += settings_test_code(name, HTTP_REQUEST_MODE_PUT, false, "file=default", bad[i], HTTP_RESP_BAD_REQUEST);
    }
    errors += settings_test_expect(!settings_test_exists("default"), "invalid versions create no object", NULL);

    // stored versions follow the same rule; a large valid one is kept and incremented as int64
    settings_test_store("{\"version\":\"5\",\"marker\":\"hand-edited\"}");
    errors += settings_test_stored("a stored non-integer version reads as a new object", 1, NULL);

    settings_test_store("{\"version\":4294967296,\"marker\":\"big\"}");
    errors += settings_test_stored("a stored 64-bit version is served", 4294967296LL, "big");
    errors += settings_test_code("PUT on a 64-bit version", HTTP_REQUEST_MODE_PUT, false, "file=default",
                                 "{\"version\":4294967296,\"marker\":\"big2\"}", HTTP_RESP_OK);
    errors += settings_test_stored("a 64-bit version increments", 4294967297LL, "big2");

    // the largest valid stored version is served but cannot be incremented into an invalid one
    settings_test_store("{\"version\":9223372036854775806,\"marker\":\"top\"}");
    errors += settings_test_stored("the largest valid version is served", INT64_MAX - 1, "top");
    errors += settings_test_code("the largest valid version cannot be incremented", HTTP_REQUEST_MODE_PUT, false,
                                 "file=default", "{\"version\":9223372036854775806,\"marker\":\"over\"}",
                                 HTTP_RESP_BAD_REQUEST);
    errors += settings_test_stored("a refused increment keeps the object", INT64_MAX - 1, "top");
    return errors;
}

int main(void) {
    static RRDHOST test_host;
    memset(&test_host, 0, sizeof(test_host));
    localhost = &test_host;

    // created in the current directory, not in a shared temporary directory
    char root_template[] = "netdata-api-v3-settings-test-XXXXXX";
    char *root = mkdtemp(root_template);
    if(!root) {
        fprintf(stderr, "cannot create a temporary directory\n");
        return 1;
    }
    strncpyz(settings_test_root, root, sizeof(settings_test_root) - 1);
    netdata_configured_varlib_dir = settings_test_root;

    int errors = 0;
    errors += settings_test_default_object();
    errors += settings_test_only_default();
    errors += settings_test_version_types();

    settings_test_reset();
    char path[FILENAME_MAX];
    settings_test_path(path, "");
    (void)rmdir(path);
    (void)rmdir(settings_test_root);

    if(errors) {
        fprintf(stderr, "api v3 settings tests failed with %d error(s)\n", errors);
        return 1;
    }

    fprintf(stderr, "api v3 settings tests passed\n");
    return 0;
}
