// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"
#include "database/rrd.h"
#include "web/server/web_client.h"
#include <setjmp.h>
#include <cmocka.h>
#include <stdbool.h>

void free_temporary_host(RRDHOST *host)
{
    (void) host;
}

void *__wrap_free_temporary_host(RRDHOST *host)
{
    (void) host;
    return NULL;
}

void repr(char *result, int result_size, char const *buf, int size)
{
    int n;
    char *end = result + result_size - 1;
    unsigned char const *ubuf = (unsigned char const *)buf;
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
    netdata_log_info("Mocking send: %zu bytes\n", len);
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

char *__wrap_inicfg_get(&netdata_config, struct config *root, const char *section, const char *name, const char *default_value)
{
    (void)root;
    (void)section;
    (void)name;
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

int __wrap_mysendfile(struct web_client *w, char *filename)
{
    (void)w;
    printf("mysendfile(filename=\"%s\"\n", filename);
    check_expected_ptr(filename);
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

// -------------------------------- Mocking the log - dump straight through --------------------------------------------

void __wrap_debug_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    printf("  DEBUG: ");
    printf(fmt, args);
    printf("\n");
    va_end(args);
}

void __wrap_info_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    printf("  INFO: ");
    printf(fmt, args);
    printf("\n");
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
    printf("  ERROR: ");
    printf(fmt, args);
    printf("\n");
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
    printf(fmt, args);
    printf("\n");
    va_end(args);
    fail();
}

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;
char *netdata_configured_web_dir = "UNKNOWN FIXME";
RRDHOST *localhost = NULL;

struct config netdata_config = { .first_section = NULL,
                                 .last_section = NULL,
                                 .mutex = NETDATA_MUTEX_INITIALIZER,
                                 .index = { .avl_tree = { .root = NULL, .compar = inicfg_section_compare },
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

//////////////////////////// Test cases ///////////////////////////////////////////////////////////////////////////////

static void only_root(void **state)
{
    (void)state;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET / HTTP/1.1\r\n\r\n");

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("-> \"%s\"\n", debug);

    //char expected_url_repr[4096];
    //repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_string(__wrap_mysendfile, filename, "/");

    web_client_process_request(w);

    //assert_string_equal(w->decoded_query_string, def->query_out);
    destroy_web_client(w);
    free(localhost);
    localhost = NULL;
}

static void two_slashes(void **state)
{
    (void)state;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET // HTTP/1.1\r\n\r\n");

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("-> \"%s\"\n", debug);

    //char expected_url_repr[4096];
    //repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_string(__wrap_mysendfile, filename, "//");

    web_client_process_request(w);

    //assert_string_equal(w->decoded_query_string, def->query_out);
    destroy_web_client(w);
    free(localhost);
    localhost = NULL;
}

static void absolute_url(void **state)
{
    (void)state;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET http://localhost:19999/api/v1/info HTTP/1.1\r\n\r\n");

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("-> \"%s\"\n", debug);

    //char expected_url_repr[4096];
    //repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_string(__wrap_web_client_api_request_v1, url_repr, "info");

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, "?blah");
    destroy_web_client(w);
    free(localhost);
    localhost = NULL;
}

static void valid_url(void **state)
{
    (void)state;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET /api/v1/info?blah HTTP/1.1\r\n\r\n");

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("-> \"%s\"\n", debug);

    //char expected_url_repr[4096];
    //repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_string(__wrap_web_client_api_request_v1, url_repr, "info");

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, "?blah");
    destroy_web_client(w);
    free(localhost);
    localhost = NULL;
}

/* RFC2616, section 4.1:

   In the interest of robustness, servers SHOULD ignore any empty
   line(s) received where a Request-Line is expected. In other words, if
   the server is reading the protocol stream at the beginning of a
   message and receives a CRLF first, it should ignore the CRLF.
*/
static void leading_blanks(void **state)
{
    (void)state;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "\r\n\r\nGET /api/v1/info?blah HTTP/1.1\r\n\r\n");

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("-> \"%s\"\n", debug);

    //char expected_url_repr[4096];
    //repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_string(__wrap_web_client_api_request_v1, url_repr, "info");

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, "?blah");
    destroy_web_client(w);
    free(localhost);
    localhost = NULL;
}

static void empty_url(void **state)
{
    (void)state;

    if (localhost != NULL)
        free(localhost);
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET  HTTP/1.1\r\n\r\n");

    char debug[4096];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("-> \"%s\"\n", debug);

    //char expected_url_repr[4096];
    //repr(expected_url_repr, sizeof(expected_url_repr), def->url_out_repr, strlen(def->url_out_repr));

    expect_value(__wrap_web_client_api_request_v1, host, localhost);
    expect_value(__wrap_web_client_api_request_v1, w, w);
    expect_string(__wrap_web_client_api_request_v1, url_repr, "info");

    web_client_process_request(w);

    assert_string_equal(w->decoded_query_string, "?blah");
    destroy_web_client(w);
    free(localhost);
    localhost = NULL;
}

/* If the %-escape is being performed at the correct time then the url should not be treated as a query, but instead
   as a path "/api/v1/info?blah?" which should dispatch into the API with the given values.
*/
static void not_a_query(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET /api/v1/info%3fblah%3f HTTP/1.1\r\n\r\n");

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
    destroy_web_client(w);
    free(localhost);
}

static void cr_in_url(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET /api/v1/inf\ro\t?blah HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}
static void newline_in_url(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET /api/v1/inf\no\t?blah HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void bad_version(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET /api/v1/info?blah HTTP/1.2\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void pathless_query(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET ?blah HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void pathless_fragment(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET #blah HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void short_percent(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET % HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void short_percent2(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET %0 HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void short_percent3(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET %");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void percent_nulls(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET %00%00%00%00%00%00 HTTP/1.1\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void percent_invalid(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET /%x%x%x%x%x%x HTTP/1.1\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void space_in_url(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET / / HTTP/1.1\r\n\r\n");

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void random_sploit1(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    // FIXME: Encoding probably needs to go through printf
    buffer_need_bytes(w->response.data, 55);
    memcpy(
        w->response.data->buffer,
        "GET \x03\x00\x00/*\xE0\x00\x00\x00\x00\x00Cookie: mstshash=Administr HTTP/1.1\r\n\r\n", 54);
    w->response.data->len = 54;

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

static void null_in_url(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET / / HTTP/1.1\r\n\r\n");
    w->response.data->buffer[5] = 0;

    char debug[160];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}
static void many_ands(void **state)
{
    (void)state;
    localhost = malloc(sizeof(RRDHOST));

    struct web_client *w = setup_fresh_web_client();
    buffer_strcat(w->response.data, "GET foo?");
    for (size_t i = 0; i < 600; i++)
        buffer_strcat(w->response.data, "&");
    buffer_strcat(w->response.data, " HTTP/1.1\r\n\r\n");

    char debug[2048];
    repr(debug, sizeof(debug), w->response.data->buffer, w->response.data->len);
    printf("->%s\n", debug);

    char expected_url_repr[160];
    repr(expected_url_repr, sizeof(expected_url_repr), "inf\no\t", 6);

    web_client_process_request(w);

    assert_int_equal(w->response.code, HTTP_RESP_BAD_REQUEST);

    destroy_web_client(w);
    free(localhost);
}

int main(void)
{
    debug_flags = 0xffffffffffff;
    int fails = 0;

    struct CMUnitTest static_tests[] = {
        cmocka_unit_test(only_root), cmocka_unit_test(two_slashes), cmocka_unit_test(valid_url),
        cmocka_unit_test(leading_blanks), cmocka_unit_test(empty_url), cmocka_unit_test(newline_in_url),
        cmocka_unit_test(not_a_query), cmocka_unit_test(cr_in_url), cmocka_unit_test(pathless_query),
        cmocka_unit_test(pathless_fragment), cmocka_unit_test(short_percent), cmocka_unit_test(short_percent2),
        cmocka_unit_test(short_percent3), cmocka_unit_test(percent_nulls), cmocka_unit_test(percent_invalid),
        cmocka_unit_test(space_in_url), cmocka_unit_test(random_sploit1), cmocka_unit_test(null_in_url),
        cmocka_unit_test(absolute_url),
        //        cmocka_unit_test(many_ands),      CMocka cannot recover after this crash
        cmocka_unit_test(bad_version)
    };
    (void)many_ands;

    fails += cmocka_run_group_tests_name("static_tests", static_tests, NULL, NULL);
    return fails;
}
