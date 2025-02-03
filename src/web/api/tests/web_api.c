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
    netdata_log_info("web_client_api_request_v1(url=\"%s\")\n", url_repr);
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
    size_t cur = strlen(log_buffer);
    snprintf(log_buffer + cur, sizeof(log_buffer) - cur, "  DEBUG: ");
    cur = strlen(log_buffer);
    vsnprintf(log_buffer + cur, sizeof(log_buffer) - cur, fmt, args);
    cur = strlen(log_buffer);
    snprintf(log_buffer + cur, sizeof(log_buffer) - cur, "\n");
    va_end(args);
}

void __wrap_info_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    (void)file;
    (void)function;
    (void)line;
    va_list args;
    va_start(args, fmt);
    size_t cur = strlen(log_buffer);
    snprintf(log_buffer + cur, sizeof(log_buffer) - cur, "  INFO: ");
    cur = strlen(log_buffer);
    vsnprintf(log_buffer + cur, sizeof(log_buffer) - cur, fmt, args);
    cur = strlen(log_buffer);
    snprintf(log_buffer + cur, sizeof(log_buffer) - cur, "\n");
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
    size_t cur = strlen(log_buffer);
    snprintf(log_buffer + cur, sizeof(log_buffer) - cur, "  ERROR: ");
    cur = strlen(log_buffer);
    vsnprintf(log_buffer + cur, sizeof(log_buffer) - cur, fmt, args);
    cur = strlen(log_buffer);
    snprintf(log_buffer + cur, sizeof(log_buffer) - cur, "\n");
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

struct config netdata_config = { .first_section = NULL,
                                 .last_section = NULL,
                                 .mutex = NETDATA_MUTEX_INITIALIZER,
                                 .index = { .avl_tree = { .root = NULL, .compar = inicfg_section_compare },
                                            .rwlock = AVL_LOCK_INITIALIZER } };

const char *http_headers[] = { "Host: 254.254.0.1",
                               "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_" // No ,
                               "0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.70 Safari/537.36",
                               "Connection: keep-alive",
                               "X-Forwarded-For: 1.254.1.251",
                               "Cookie: _ga=GA1.1.1227576758.1571113676; _gid=GA1.2.1222321739.1573628979",
                               "X-Requested-With: XMLHttpRequest",
                               "Accept-Encoding: gzip, deflate",
                               "Cache-Control: no-cache, no-store" };
#define MAX_HEADERS (sizeof(http_headers) / (sizeof(const char *)))

static void build_request(struct web_buffer *wb, const char *url, bool use_cr, size_t num_headers)
{
    buffer_reset(wb);
    buffer_strcat(wb, "GET ");
    buffer_strcat(wb, url);
    buffer_strcat(wb, " HTTP/1.1");
    if (use_cr)
        buffer_strcat(wb, "\r");
    buffer_strcat(wb, "\n");
    for (size_t i = 0; i < num_headers && i < MAX_HEADERS; i++) {
        buffer_strcat(wb, http_headers[i]);
        if (use_cr)
            buffer_strcat(wb, "\r");
        buffer_strcat(wb, "\n");
    }
    if (use_cr)
        buffer_strcat(wb, "\r");
    buffer_strcat(wb, "\n");
}

/* Note: this is not a CMocka group_test_setup/teardown pair. This is performed per-test.
*/
static struct web_client *setup_fresh_web_client()
{
    struct web_client *w = (struct web_client *)malloc(sizeof(struct web_client));
    memset(w, 0, sizeof(struct web_client));
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    w->response.data->date = 0; // Valgrind uninitialised value
    w->response.data->expires = 0; // Valgrind uninitialised value
    w->response.data->options = 0; // Valgrind uninitialised value
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

// ---------------------------------- Parameterized test-families -----------------------------------------------------
// There is no way to pass a parameter block into the setup fixture, we would have to patch CMocka and maintain it
// locally. (The void **current_state in _run_group_tests would be set from a parameter). This is unfortunate as a
// parametric unit-tester needs to be to pass parameters to the fixtures. We are faking this by calculating the
// space of tests in the launcher, passing an array of identical unit-tests to CMocka and then counting through the
// parameters in the shared state passed between tests. To initialise this counter structure we use this global to
// pass from the launcher (test-builder) to the setup-fixture.

void *shared_test_state = NULL;

// -------------------------------- Test family for /api/v1/info ------------------------------------------------------

struct test_def {
    size_t num_headers; // Index coordinate
    size_t prefix_len;  // Index coordinate
    char name[80];
    size_t full_len;
    struct web_client *instance; // Used within this single test
    bool completed, use_cr;
    struct test_def *next, *prev;
};

static void api_info(void **state)
{
    (void)state;
    struct test_def *def = (struct test_def *)shared_test_state;
    shared_test_state = def->next;

    if (def->prev != NULL && !def->prev->completed && strlen(log_buffer) > 0) {
        printf("Log of failing case %s:\n", def->prev->name);
        puts(log_buffer);
    }
    log_buffer[0] = 0;
    if (localhost != NULL)
        free(localhost);
    localhost = calloc(1,sizeof(RRDHOST));

    def->instance = setup_fresh_web_client();
    build_request(def->instance->response.data, "/api/v1/info", def->use_cr, def->num_headers);
    def->instance->response.data->len = def->prefix_len;

    char buffer_repr[1024];
    repr(buffer_repr, sizeof(buffer_repr), def->instance->response.data->buffer,def->prefix_len);
    netdata_log_info("Buffer contains: %s [first %zu]", buffer_repr,def->prefix_len);
    if (def->prefix_len == def->full_len) {
        expect_value(__wrap_web_client_api_request_v1, host, localhost);
        expect_value(__wrap_web_client_api_request_v1, w, def->instance);
        expect_string(__wrap_web_client_api_request_v1, url_repr, "info");
    }

    web_client_process_request(def->instance);

    if (def->prefix_len == def->full_len)
        assert_int_equal(def->instance->flags & WEB_CLIENT_FLAG_WAIT_RECEIVE, 0);
    else
        assert_int_equal(def->instance->flags & WEB_CLIENT_FLAG_WAIT_RECEIVE, WEB_CLIENT_FLAG_WAIT_RECEIVE);
    assert_int_equal(def->instance->mode, WEB_CLIENT_MODE_NORMAL);
    def->completed = true;
    log_buffer[0] = 0;
}

static int api_info_launcher()
{
    size_t num_tests = 0;
    struct web_client *template = setup_fresh_web_client();
    struct test_def *current, *head = NULL;
    struct test_def *prev = NULL;

    for (size_t i = 0; i < MAX_HEADERS; i++) {
        build_request(template->response.data, "/api/v1/info", true, i);
        for (size_t j = 0; j <= template->response.data->len; j++) {
            if (j == 0 && i > 0)
                continue; // All zero-length prefixes are identical, skip after first time
            current = malloc(sizeof(struct test_def));
            if (prev != NULL)
                prev->next = current;
            else
                head = current;
            current->prev = prev;
            prev = current;

            current->num_headers = i;
            current->prefix_len = j;
            current->full_len = template->response.data->len;
            current->instance = NULL;
            current->next = NULL;
            current->use_cr = true;
            current->completed = false;
            sprintf(
                current->name, "/api/v1/info@%zu,%zu/%zu+%d", current->num_headers, current->prefix_len,
                current->full_len,true);
            num_tests++;
        }
    }
    for (size_t i = 0; i < MAX_HEADERS; i++) {
        build_request(template->response.data, "/api/v1/info", false, i);
        for (size_t j = 0; j <= template->response.data->len; j++) {
            if (j == 0 && i > 0)
                continue; // All zero-length prefixes are identical, skip after first time
            current = malloc(sizeof(struct test_def));
            if (prev != NULL)
                prev->next = current;
            else
                head = current;
            current->prev = prev;
            prev = current;

            current->num_headers = i;
            current->prefix_len = j;
            current->full_len = template->response.data->len;
            current->instance = NULL;
            current->next = NULL;
            current->use_cr = false;
            current->completed = false;
            sprintf(
                current->name, "/api/v1/info@%zu,%zu/%zu+%d", current->num_headers, current->prefix_len,
                current->full_len,false);
            num_tests++;
        }
    }

    struct CMUnitTest *tests = calloc(num_tests, sizeof(struct CMUnitTest));
    current = head;
    for (size_t i = 0; i < num_tests; i++) {
        tests[i].name = current->name;
        tests[i].test_func = api_info;
        tests[i].setup_func = NULL;
        tests[i].teardown_func = NULL;
        tests[i].initial_state = NULL;
        current = current->next;
    }

    printf("Setup %zu tests in %p\n", num_tests, head);
    shared_test_state = head;
    int fails = _cmocka_run_group_tests("web_api", tests, num_tests, NULL, NULL);
    free(tests);
    destroy_web_client(template);
    current = head;
    while (current != NULL) {
        struct test_def *c = current;
        current = current->next;
        if (c->instance != NULL) // Clean up resources from tests that failed
            destroy_web_client(c->instance);
        free(c);
    }
    if (localhost!=NULL)
        free(localhost);
    return fails;
}

/* Raw notes for the cases that we did not use in the unit testing suite.
   Leaving them here instead of deleting them in-case we expand the suite during the
   work on the URL parser.

   Any ' ' in the URI -> invalid response   (Description in 5.1 of RFC2616)
   Characters that can't be in paths #;?
   "GET /apb/../api/v1/info" HTTP/1.1\r\n"

        https://github.com/uriparser/uriparser/blob/uriparser-0.9.3/test/FourSuite.cpp
        Not clear why some of these are illegal -> reserved chars?

	ASSERT_TRUE(testBadUri("beepbeep\x07\x07", 8));
	ASSERT_TRUE(testBadUri("\n", 0));
	ASSERT_TRUE(testBadUri("::", 0)); // not OK, per Roy Fielding on the W3C uri list on 2004-04-01

	// the following test cases are from a Perl script by David A. Wheeler
	// at http://www.dwheeler.com/secure-programs/url.pl
	ASSERT_TRUE(testBadUri("http://www yahoo.com", 10));
	ASSERT_TRUE(testBadUri("http://www.yahoo.com/hello world/", 26));
	ASSERT_TRUE(testBadUri("http://www.yahoo.com/yelp.html#\"", 31));

	// the following test cases are from a Haskell program by Graham Klyne
	// at http://www.ninebynine.org/Software/HaskellUtils/Network/URITest.hs
	ASSERT_TRUE(testBadUri("[2010:836B:4179::836B:4179]", 0));
	ASSERT_TRUE(testBadUri(" ", 0));
	ASSERT_TRUE(testBadUri("%", 1));
	ASSERT_TRUE(testBadUri("A%Z", 2));
	ASSERT_TRUE(testBadUri("%ZZ", 1));
	ASSERT_TRUE(testBadUri("%AZ", 2));
	ASSERT_TRUE(testBadUri("A C", 1));
	ASSERT_TRUE(testBadUri("A\\'C", 1)); // r"A\'C"
	ASSERT_TRUE(testBadUri("A`C", 1));
	ASSERT_TRUE(testBadUri("A<C", 1));
	ASSERT_TRUE(testBadUri("A>C", 1));
	ASSERT_TRUE(testBadUri("A^C", 1));
	ASSERT_TRUE(testBadUri("A\\\\C", 1)); // r'A\\C'
	ASSERT_TRUE(testBadUri("A{C", 1));
	ASSERT_TRUE(testBadUri("A|C", 1));
	ASSERT_TRUE(testBadUri("A}C", 1));
	ASSERT_TRUE(testBadUri("A[C", 1));
	ASSERT_TRUE(testBadUri("A]C", 1));
	ASSERT_TRUE(testBadUri("A[**]C", 1));
	ASSERT_TRUE(testBadUri("http://[xyz]/", 8));
	ASSERT_TRUE(testBadUri("http://]/", 7));
	ASSERT_TRUE(testBadUri("http://example.org/[2010:836B:4179::836B:4179]", 19));
	ASSERT_TRUE(testBadUri("http://example.org/abc#[2010:836B:4179::836B:4179]", 23));
	ASSERT_TRUE(testBadUri("http://example.org/xxx/[qwerty]#a[b]", 23));

	// from a post to the W3C uri list on 2004-02-17
	// breaks at 22 instead of 17 because everything up to that point is a valid userinfo
	ASSERT_TRUE(testBadUri("http://w3c.org:80path1/path2", 22));

*/

int main(void)
{
    debug_flags = 0xffffffffffff;
    int fails = 0;
    fails += api_info_launcher();

    return fails;
}
