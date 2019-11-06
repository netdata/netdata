// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../../libnetdata/libnetdata.h"
#include "../../../libnetdata/required_dummies.h"
#include "../../../database/rrd.h"
#include "../../../web/server/web_client.h"
#include <setjmp.h>
#include <cmocka.h>

RRDHOST *__wrap_rrdhost_find_by_hostname(const char *hostname, uint32_t hash)
{
    return NULL;
}

/* Note: we've got some intricate code inside the global statistics module, might be useful to pull it inside the
         test set instead of mocking it. */
void __wrap_finished_web_request_statistics(uint64_t dt, uint64_t bytes_received, uint64_t bytes_sent,
                                            uint64_t content_size, uint64_t compressed_content_size)
{
}

char *__wrap_config_get(struct config *root, const char *section, const char *name, const char *default_value)
{
    if (!strcmp(section, CONFIG_SECTION_WEB) && !strcmp(name, "web files owner"))
        return "netdata";
    return "UNKNOWN FIX ME";
}

int __wrap_web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url)
{
    printf("api requests: %s\n", url);
}

int __wrap_rrdpush_receiver_thread_spawn(RRDHOST *host, struct web_client *w, char *url)
{
    return 0;
}

RRDHOST *__wrap_rrdhost_find_by_guid(const char *guid, uint32_t hash)
{
    printf("FIXME: rrdset_find_guid\n");
    return NULL;
}

RRDSET *__wrap_rrdset_find_byname(RRDHOST *host, const char *name)
{
    printf("FIXME: rrdset_find_byname\n");
    return NULL;
}

RRDSET *__wrap_rrdset_find(RRDHOST *host, const char *id)
{
    printf("FIXME: rrdset_find\n");
    return NULL;
}

void __wrap_debug_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    printf("DEBUG: ");
    vprintf(fmt, args);
    printf("\n");
    va_end(args);
}

void __wrap_info_int(const char *file, const char *function, const unsigned long line, const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    printf("INFO: ");
    vprintf(fmt, args);
    printf("\n");
    va_end(args);
}

void __wrap_error_int(const char *prefix, const char *file, const char *function, const unsigned long line,
                      const char *fmt, ...)
{
    va_list args;
    va_start(args, fmt);
    printf("ERROR: ");
    vprintf(fmt, args);
    printf("\n");
    va_end(args);
}

WEB_SERVER_MODE web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;
char *netdata_configured_web_dir = "UNKNOWN FIXME";
RRDHOST *localhost = NULL;

struct config netdata_config = {.sections = NULL,
                                .mutex = NETDATA_MUTEX_INITIALIZER,
                                .index = {.avl_tree = {.root = NULL, .compar = appconfig_section_compare},
                                          .rwlock = AVL_LOCK_INITIALIZER}};

char req[] = "GET /api/v1/info HTTP/1.1\r\nHost: localhost:19999\r\nUser-Agent: python-requests/2.21.0\r\n"
             "Accept-Encoding: gzip, deflate\r\nAccept: */*\r\nConnection: keep-alive\r\n\r\n";

static void test_api(void **state)
{
    localhost = malloc(sizeof(RRDHOST));
    struct web_client *w = (struct web_client *)malloc(sizeof(struct web_client));
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    w->response.header = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    w->response.header_output = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    strcpy(w->origin, "*"); // Simulate web_client_create_on_fd()
    w->cookie1[0] = 0;      // Simulate web_client_create_on_fd()
    w->cookie2[0] = 0;      // Simulate web_client_create_on_fd()

    buffer_strcat(w->response.data, req);
    web_client_process_request(w);
    assert_int_equal(w->flags & WEB_CLIENT_FLAG_WAIT_RECEIVE, 0);

    free(w);
    free(localhost);
}

int main(void)
{
    const struct CMUnitTest tests[] = {cmocka_unit_test(test_api)};
    debug_flags = 0xffffffffffff;

    return cmocka_run_group_tests_name("web_api", tests, NULL, NULL);
}
