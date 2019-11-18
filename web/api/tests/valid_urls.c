// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../../libnetdata/libnetdata.h"
#include "../../../libnetdata/required_dummies.h"
#include "../../../database/rrd.h"
#include "../../../web/server/web_client.h"
#include <setjmp.h>
#include <cmocka.h>
#include <stdbool.h>

void repr(char *result, int result_size, char const *buf, int size)
{
    int n;
    char *end = result + result_size - 1;
    unsigned char const *ubuf = (unsigned char const*)buf;
    while (size && result_size > 0) {
        if (*ubuf <= 0x20 || *ubuf >= 0x80) {
            n = snprintf(result, result_size, "\\%02X", *ubuf);
        } else {
            *result = *ubuf;
            n = 1;
        }
        result += n;
        result_size -= n;
        ubuf++;
        size--;
    }
    if (result_size > 0)
        *(result++) = 0;
    else
        *end = 0;
}

// ---------------------------------- Mocking accesses from web_client ------------------------------------------------

ssize_t send(int sockfd, const void *buf, size_t len, int flags)
{
    info("Mocking send: %zu bytes\n", len);
    (void)sockfd;
    (void)buf;
    (void)flags;
    return len;
}

RRDHOST *__wrap_rrdhost_find_by_hostname(const char *hostname, uint32_t hash)
{
    (void)hostname;
    (void)hash;
    return NULL;
}

/* Note: we've got some intricate code inside the global statistics module, might be useful to pull it inside the
         test set instead of mocking it. */
void __wrap_finished_web_request_statistics(
    uint64_t dt, uint64_t bytes_received, uint64_t bytes_sent, uint64_t content_size, uint64_t compressed_content_size)
{
    (void)dt;
    (void)bytes_received;
    (void)bytes_sent;
    (void)content_size;
    (void)compressed_content_size;
}

char *__wrap_config_get(struct config *root, const char *section, const char *name, const char *default_value)
{
    if (!strcmp(section, CONFIG_SECTION_WEB) && !strcmp(name, "web files owner"))
        return "netdata";
    (void)root;
    (void)default_value;
    return "UNKNOWN FIX ME";
}

int __wrap_web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url)
{
    char url_repr[160];
    repr(url_repr, sizeof(url_repr), url, strlen(url));
    printf("web_client_api_request_v1(url=\"%s\")\n", url_repr);
    check_expected_ptr(host);
    check_expected_ptr(w);
    check_expected_ptr(url_repr);
    return HTTP_RESP_OK;
}

int __wrap_rrdpush_receiver_thread_spawn(RRDHOST *host, struct web_client *w, char *url)
{
    (void)host;
    (void)w;
    (void)url;
    return 0;
}

RRDHOST *__wrap_rrdhost_find_by_guid(const char *guid, uint32_t hash)
{
    (void)guid;
    (void)hash;
    printf("FIXME: rrdset_find_guid\n");
    return NULL;
}

RRDSET *__wrap_rrdset_find_byname(RRDHOST *host, const char *name)
{
    (void)host;
    (void)name;
    printf("FIXME: rrdset_find_byname\n");
    return NULL;
}

RRDSET *__wrap_rrdset_find(RRDHOST *host, const char *id)
{
    (void)host;
    (void)id;
    printf("FIXME: rrdset_find\n");
    return NULL;
}

// -------------------------------- Mocking the log - capture per-test ------------------------------------------------

char log_buffer[10240] = { 0 };
void __wrap_debug_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    sprintf(log_buffer + strlen(log_buffer), "  DEBUG: ");
    vsprintf(log_buffer + strlen(log_buffer), fmt, args);
    sprintf(log_buffer + strlen(log_buffer), "\n");
    va_end(args);
}

void __wrap_info_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    sprintf(log_buffer + strlen(log_buffer), "  INFO: ");
    vsprintf(log_buffer + strlen(log_buffer), fmt, args);
    sprintf(log_buffer + strlen(log_buffer), "\n");
    va_end(args);
}

void __wrap_error_int(
    const char *prefix, const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)prefix;
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    sprintf(log_buffer + strlen(log_buffer), "  ERROR: ");
    vsprintf(log_buffer + strlen(log_buffer), fmt, args);
    sprintf(log_buffer + strlen(log_buffer), "\n");
    va_end(args);
}

void __wrap_fatal_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    printf("FATAL: ");
    vprintf(fmt, args);
    printf("\n");
    va_end(args);
    fail();
}

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;
char *netdata_configured_web_dir = "UNKNOWN FIXME";
RRDHOST *localhost = NULL;

struct config netdata_config = { .sections = NULL,
                                 .mutex = NETDATA_MUTEX_INITIALIZER,
                                 .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                            .rwlock = AVL_LOCK_INITIALIZER } };


/* Note: this is not a CMocka group_test_setup/teardown pair. This is performed per-test.
*/
static struct web_client *setup_fresh_web_client()
{
    struct web_client *w = (struct web_client *)malloc(sizeof(struct web_client));
    memset(w, 0, sizeof(struct web_client));
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    w->response.header = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    w->response.header_output = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    strcpy(w->origin, "*"); // Simulate web_client_create_on_fd()
    w->cookie1[0] = 0;      // Simulate web_client_create_on_fd()
    w->cookie2[0] = 0;      // Simulate web_client_create_on_fd()
    w->acl = 0x1f;          // Everything on
    return w;
}

static void destroy_web_client(struct web_client *w)
{
    buffer_free(w->response.data);
    buffer_free(w->response.header);
    buffer_free(w->response.header_output);
    free(w);
}

static void build_request(struct web_buffer *wb, const char *url, bool use_cr, size_t num_headers)
{
    buffer_reset(wb);
    buffer_strcat(wb, "GET ");
    buffer_strcat(wb, url);
    buffer_strcat(wb, " HTTP/1.1\r\n\r\n");
}

// ---------------------------------- Parameterized test-families -----------------------------------------------------
// There is no way to pass a parameter block into the setup fixture, we would have to patch CMocka and maintain it
// locally. (The void **current_state in _run_group_tests would be set from a parameter). This is unfortunate as a
// parameteric unit-tester needs to be to pass parameters to the fixtures. We are faking this by calculating the
// space of tests in the launcher, passing an array of identical unit-tests to CMocka and then counting through the
// parameters in the shared state passed between tests. To initialise this counter structure we use this global to
// pass from the launcher (test-builder) to the setup-fixture.

struct valid_url_test_def {
    char name[80];
    char url_in[1024];
    char url_out_repr[1024];
    char query_out[1024];
};

struct valid_url_test_def valid_url_tests[] = {
    { "legal_query", "/api/v1/info?blah", "info", "?blah" },
    { "root_only", "/", "", "" },
    { "", "", "", "" }
};

size_t shared_test_state = 0;
bool test_completed = false;

static void valid_url(void **state)
{
    (void)state;
    struct valid_url_test_def *def = &valid_url_tests[shared_test_state];
    shared_test_state ++;

    if (shared_test_state>0 && !test_completed && strlen(log_buffer) > 0) {
        printf("Log of failing case %s:\n", (def-1)->name);
        puts(log_buffer);
    }
    test_completed = false;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    build_request(w->response.data, def->url_in, true, 0);

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[4096];
    repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    // expect_any(__wrap_web_client_api_request_v1, url_repr);
    expect_string(__wrap_web_client_api_request_v1, url_repr, expected_url_repr);       // FIXME: pre-repr in def?

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, def->query_out);
    free(localhost);
    localhost = NULL;
    test_completed = true;
    log_buffer[0] = 0;

}

int valid_url_launcher()
{
    size_t num_tests = 0;
    for(size_t i=0; valid_url_tests[i].name[0]!=0; i++)
        num_tests++;

    struct CMUnitTest *tests = calloc(num_tests, sizeof(struct CMUnitTest));
    for (size_t i = 0; i < num_tests; i++) {
        tests[i].name = valid_url_tests[i].name;
        tests[i].test_func = valid_url;
        tests[i].setup_func = NULL;
        tests[i].teardown_func = NULL;
        tests[i].initial_state = NULL;
    }
    shared_test_state = valid_url_tests;
    int fails = _cmocka_run_group_tests("valid_urls", tests, num_tests, NULL, NULL);
    free(tests);
    return fails;
}

static void legal_query(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    build_request(w->response.data, "/api/v1/info?blah", true, 0);

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "info?blah", 6);

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_any(__wrap_web_client_api_request_v1, url_repr);
    //    expect_string(__wrap_web_client_api_request_v1, url_repr, expected_url_repr);

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, "?blah");
    free(localhost);
}

static void not_a_query(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    build_request(w->response.data, "/api/v1/info%3fblah%3f", true, 0);

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "info?blah?", 10);

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_string(__wrap_web_client_api_request_v1, url_repr, expected_url_repr);

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, "");
    free(localhost);
}

static void newline_in_url(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    build_request(w->response.data, "/api/v1/inf\no\t?blah", true, 0);

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_string(__wrap_web_client_api_request_v1, url_repr, expected_url_repr);

    web_client_process_request(w);

    printf("decoded: %s\n", w->decoded_query_string);
    free(localhost);
}

int main(void)
{
    debug_flags = 0xffffffffffff;
    int fails = 0;

    struct CMUnitTest static_tests[] = { cmocka_unit_test(newline_in_url), cmocka_unit_test(legal_query),
                                         cmocka_unit_test(not_a_query) };
    fails += cmocka_run_group_tests_name("static_tests", static_tests, NULL, NULL);

    fails += valid_url_launcher();

    return fails;
}

