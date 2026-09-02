// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_client.h"

#define WEB_CLIENT_TEST(condition, ...)                                                                                \
    do {                                                                                                               \
        if (!(condition)) {                                                                                            \
            fprintf(stderr, "  FAILED: ");                                                                             \
            fprintf(stderr, __VA_ARGS__);                                                                              \
            fprintf(stderr, "\n");                                                                                     \
            errors++;                                                                                                  \
        }                                                                                                              \
    } while (0)

static char *web_client_test_target(size_t length)
{
    char *target = mallocz(length + 1);
    target[0] = '/';
    if (length > 1)
        memset(&target[1], 'a', length - 1);
    target[length] = '\0';
    return target;
}

static char *web_client_test_request(size_t target_length, size_t *request_length)
{
    static const char prefix[] = "GET ";
    static const char suffix[] = " HTTP/1.1\r\n\r\n";

    *request_length = sizeof(prefix) - 1 + target_length + sizeof(suffix) - 1;
    char *request = mallocz(*request_length + 1);
    char *p = request;

    memcpy(p, prefix, sizeof(prefix) - 1);
    p += sizeof(prefix) - 1;
    *p++ = '/';
    if (target_length > 1) {
        memset(p, 'a', target_length - 1);
        p += target_length - 1;
    }
    memcpy(p, suffix, sizeof(suffix));

    return request;
}

static char *
web_client_test_payload_request(const char *method, size_t payload_length, size_t *request_length, size_t *header_length)
{
    char prefix[128];
    int prefix_length = snprintf(prefix, sizeof(prefix), "%s /api/v1/info HTTP/1.1\r\nX-Pad: ", method);

    char suffix[128];
    int suffix_length =
        snprintf(suffix, sizeof(suffix), "\r\nContent-Length: %zu\r\nContent-Type: text/plain\r\n\r\n", payload_length);

    if (prefix_length < 0 || (size_t)prefix_length >= sizeof(prefix) || suffix_length < 0 ||
        (size_t)suffix_length >= sizeof(suffix) ||
        (size_t)prefix_length + (size_t)suffix_length + payload_length > NETDATA_WEB_REQUEST_MAX_SIZE)
        return NULL;

    *request_length = NETDATA_WEB_REQUEST_MAX_SIZE;
    size_t padding_length = *request_length - (size_t)prefix_length - (size_t)suffix_length - payload_length;
    *header_length = (size_t)prefix_length + padding_length + (size_t)suffix_length;

    char *request = mallocz(*request_length + 1);
    char *p = request;

    memcpy(p, prefix, (size_t)prefix_length);
    p += prefix_length;
    memset(p, 'a', padding_length);
    p += padding_length;
    memcpy(p, suffix, (size_t)suffix_length);
    p += suffix_length;
    memset(p, 'b', payload_length);

    return request;
}

int web_client_request_size_unittest(void)
{
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    int errors = 0;
    size_t memory_accounting = 0;
    struct web_client *w = web_client_create(&memory_accounting);

    WEB_CLIENT_TEST(!w->url_decode_buffer, "new client has a URL decode buffer");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == 0, "new client URL decode capacity is %zu, expected 0", w->url_decode_buffer_size);

    const char small_target[] = "/api/v1/info?value=a%20b";
    WEB_CLIENT_TEST(web_client_decode_path_and_query_string(w, small_target), "small target was rejected");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == NETDATA_WEB_REQUEST_URL_DECODE_INITIAL_SIZE,
        "small target capacity is %zu, expected %zu",
        w->url_decode_buffer_size,
        (size_t)NETDATA_WEB_REQUEST_URL_DECODE_INITIAL_SIZE);
    WEB_CLIENT_TEST(
        strcmp(buffer_tostring(w->url_path_decoded), "/api/v1/info") == 0, "small target path decoded incorrectly");
    WEB_CLIENT_TEST(
        strcmp(buffer_tostring(w->url_query_string_decoded), "?value=a b") == 0,
        "small target query decoded incorrectly");

    char *retained = w->url_decode_buffer;
    web_client_reuse_from_cache(w);
    WEB_CLIENT_TEST(w->url_decode_buffer == retained, "cached 4 KiB buffer was not retained");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == NETDATA_WEB_REQUEST_URL_DECODE_INITIAL_SIZE,
        "cached 4 KiB capacity was not retained");

    char *target = web_client_test_target(NETDATA_WEB_REQUEST_URL_DECODE_CACHE_MAX_SIZE - 1);
    WEB_CLIENT_TEST(web_client_decode_path_and_query_string(w, target), "target fitting 64 KiB was rejected");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == NETDATA_WEB_REQUEST_URL_DECODE_CACHE_MAX_SIZE,
        "target fitting 64 KiB grew to %zu",
        w->url_decode_buffer_size);
    retained = w->url_decode_buffer;
    web_client_trim_url_decode_buffer_for_cache(w);
    WEB_CLIENT_TEST(w->url_decode_buffer == retained, "exactly 64 KiB was not retained for cache reuse");

    freez(target);
    target = web_client_test_target(NETDATA_WEB_REQUEST_URL_DECODE_CACHE_MAX_SIZE);
    WEB_CLIENT_TEST(web_client_decode_path_and_query_string(w, target), "target requiring 128 KiB was rejected");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == 2 * NETDATA_WEB_REQUEST_URL_DECODE_CACHE_MAX_SIZE,
        "target requiring 128 KiB grew to %zu",
        w->url_decode_buffer_size);
    web_client_trim_url_decode_buffer_for_cache(w);
    WEB_CLIENT_TEST(!w->url_decode_buffer, "buffer above 64 KiB was retained");
    WEB_CLIENT_TEST(w->url_decode_buffer_size == 0, "freed buffer retained capacity %zu", w->url_decode_buffer_size);

    web_client_reuse_from_cache(w);
    WEB_CLIENT_TEST(!w->url_decode_buffer, "NULL cached buffer was not preserved");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == 0, "NULL cached buffer restored capacity %zu", w->url_decode_buffer_size);

    freez(target);
    target = web_client_test_target(NETDATA_WEB_REQUEST_MAX_SIZE - 1);
    WEB_CLIENT_TEST(web_client_decode_path_and_query_string(w, target), "largest fitting target was rejected");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == NETDATA_WEB_REQUEST_MAX_SIZE,
        "largest fitting target capacity is %zu",
        w->url_decode_buffer_size);

    char *maximum_buffer = w->url_decode_buffer;
    freez(target);
    target = web_client_test_target(NETDATA_WEB_REQUEST_MAX_SIZE);
    WEB_CLIENT_TEST(!web_client_decode_path_and_query_string(w, target), "oversized target was accepted");
    WEB_CLIENT_TEST(w->url_decode_buffer == maximum_buffer, "oversized target replaced the 1 MiB buffer");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == NETDATA_WEB_REQUEST_MAX_SIZE,
        "oversized target changed capacity to %zu",
        w->url_decode_buffer_size);

    web_client_trim_url_decode_buffer_for_cache(w);
    WEB_CLIENT_TEST(!w->url_decode_buffer, "1 MiB buffer was retained for cache reuse");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == 0, "trimmed 1 MiB buffer retained capacity %zu", w->url_decode_buffer_size);
    WEB_CLIENT_TEST(
        !web_client_decode_path_and_query_string(w, target), "oversized target with NULL scratch buffer was accepted");
    WEB_CLIENT_TEST(!w->url_decode_buffer, "oversized target allocated scratch before failing");

    freez(target);
    target = NULL;
    web_client_reuse_from_cache(w);

    static const char request_prefix[] = "GET ";
    static const char request_suffix[] = " HTTP/1.1\r\n\r\n";
    size_t exact_target_length =
        NETDATA_WEB_REQUEST_MAX_SIZE - (sizeof(request_prefix) - 1) - (sizeof(request_suffix) - 1);
    size_t request_length;
    char *request = web_client_test_request(exact_target_length, &request_length);
    WEB_CLIENT_TEST(
        request_length == NETDATA_WEB_REQUEST_MAX_SIZE,
        "boundary request size is %zu, expected %zu",
        request_length,
        (size_t)NETDATA_WEB_REQUEST_MAX_SIZE);
    buffer_flush(w->response.data);
    buffer_memcat(w->response.data, request, request_length);
    WEB_CLIENT_TEST(http_request_validate(w) == HTTP_VALIDATION_OK, "exactly 1 MiB whole request did not validate");

    freez(request);
    web_client_reuse_from_cache(w);

    request = web_client_test_request(exact_target_length, &request_length);
    for (size_t offset = 0; offset < request_length;) {
        size_t chunk_size = MIN((size_t)8192, request_length - offset);
        buffer_memcat(w->response.data, &request[offset], chunk_size);
        offset += chunk_size;

        HTTP_VALIDATION validation = http_request_validate(w);
        if (offset < request_length)
            WEB_CLIENT_TEST(validation == HTTP_VALIDATION_INCOMPLETE, "fragmented request failed at byte %zu", offset);
        else
            WEB_CLIENT_TEST(validation == HTTP_VALIDATION_OK, "fragmented 1 MiB request did not validate");
    }

    freez(request);
    web_client_reuse_from_cache(w);

    static const char complete_payload_request[] =
        "POST /api/v1/info HTTP/1.1\r\nContent-Length: 4\r\nContent-Type: text/plain\r\n\r\ntest";
    buffer_memcat(w->response.data, complete_payload_request, sizeof(complete_payload_request) - 1);
    WEB_CLIENT_TEST(http_request_validate(w) == HTTP_VALIDATION_OK, "complete POST request did not validate");
    WEB_CLIENT_TEST(
        w->payload && buffer_strlen(w->payload) == 4 && !memcmp(buffer_tostring(w->payload), "test", 4),
        "complete POST payload changed");
    WEB_CLIENT_TEST(w->header_parse_expected_size == 0, "complete POST framing state was not reset");
    web_client_reuse_from_cache(w);

    static const char split_put_header[] =
        "PUT /api/v1/info HTTP/1.1\r\nContent-Length: 1048496\r\nContent-Type: text/plain\r\n\r\n";
    const size_t split_put_payload_length = NETDATA_WEB_REQUEST_MAX_SIZE - (sizeof(split_put_header) - 1);
    WEB_CLIENT_TEST(split_put_payload_length == 1048496, "split PUT test header length changed");

    request = mallocz(NETDATA_WEB_REQUEST_MAX_SIZE + 1);
    memcpy(request, split_put_header, sizeof(split_put_header) - 1);
    memset(&request[sizeof(split_put_header) - 1], 'b', split_put_payload_length);

    for (size_t offset = 0; offset < NETDATA_WEB_REQUEST_MAX_SIZE;) {
        size_t chunk_size = offset ? MIN((size_t)1024, NETDATA_WEB_REQUEST_MAX_SIZE - offset) : 4;
        buffer_memcat(w->response.data, &request[offset], chunk_size);
        offset += chunk_size;

        HTTP_VALIDATION validation = http_request_validate(w);
        if (offset < NETDATA_WEB_REQUEST_MAX_SIZE)
            WEB_CLIENT_TEST(
                validation == HTTP_VALIDATION_INCOMPLETE, "split-after-method PUT failed at byte %zu", offset);
        else
            WEB_CLIENT_TEST(validation == HTTP_VALIDATION_OK, "split-after-method 1 MiB PUT did not validate");
    }

    WEB_CLIENT_TEST(
        w->payload && buffer_strlen(w->payload) == split_put_payload_length,
        "split-after-method PUT payload size is %zu",
        w->payload ? buffer_strlen(w->payload) : 0);
    WEB_CLIENT_TEST(w->header_parse_expected_size == 0, "split-after-method PUT framing state was not reset");
    freez(request);
    web_client_reuse_from_cache(w);

    static const char *payload_methods[] = {"POST", "PUT"};
    for (size_t method = 0; method < sizeof(payload_methods) / sizeof(payload_methods[0]); method++) {
        size_t header_length;
        request = web_client_test_payload_request(payload_methods[method], 4096, &request_length, &header_length);
        WEB_CLIENT_TEST(request, "%s test request allocation failed", payload_methods[method]);
        if (!request)
            continue;

        bool framing_state_seen = false;
        for (size_t offset = 0; offset < request_length;) {
            size_t chunk_size = MIN((size_t)64, request_length - offset);
            buffer_memcat(w->response.data, &request[offset], chunk_size);
            offset += chunk_size;

            HTTP_VALIDATION validation = http_request_validate(w);
            if (offset < request_length) {
                WEB_CLIENT_TEST(
                    validation == HTTP_VALIDATION_INCOMPLETE,
                    "fragmented %s request failed at byte %zu",
                    payload_methods[method],
                    offset);

                if (offset >= header_length) {
                    framing_state_seen = true;
                    WEB_CLIENT_TEST(
                        w->header_parse_expected_size == request_length,
                        "%s expected request size is %zu, expected %zu",
                        payload_methods[method],
                        w->header_parse_expected_size,
                        request_length);
                }
            } else
                WEB_CLIENT_TEST(
                    validation == HTTP_VALIDATION_OK,
                    "fragmented 1 MiB %s request did not validate",
                    payload_methods[method]);
        }

        WEB_CLIENT_TEST(
            framing_state_seen, "%s framing state was not retained while receiving its body", payload_methods[method]);
        WEB_CLIENT_TEST(
            w->payload && buffer_strlen(w->payload) == 4096,
            "%s payload size is %zu",
            payload_methods[method],
            w->payload ? buffer_strlen(w->payload) : 0);
        if (w->payload)
            WEB_CLIENT_TEST(
                !memcmp(buffer_tostring(w->payload), &request[request_length - 4096], 4096),
                "%s payload content changed",
                payload_methods[method]);
        WEB_CLIENT_TEST(
            w->header_parse_expected_size == 0,
            "%s framing state was not reset after completion",
            payload_methods[method]);

        freez(request);
        web_client_reuse_from_cache(w);
    }

    static const char missing_content_length[] = "POST /api/v1/info HTTP/1.1\r\nX-Pad: a\r\n\r\n";
    buffer_memcat(w->response.data, missing_content_length, sizeof(missing_content_length) - 1);
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE,
        "POST without Content-Length did not remain incomplete");
    size_t invalid_framing_state = w->header_parse_expected_size;
    WEB_CLIENT_TEST(invalid_framing_state != 0, "missing Content-Length framing state was not remembered");

    char missing_content_length_body[64];
    memset(missing_content_length_body, 'b', sizeof(missing_content_length_body));
    while (buffer_strlen(w->response.data) < NETDATA_WEB_REQUEST_MAX_SIZE) {
        size_t chunk_size =
            MIN(sizeof(missing_content_length_body), NETDATA_WEB_REQUEST_MAX_SIZE - buffer_strlen(w->response.data));
        buffer_memcat(w->response.data, missing_content_length_body, chunk_size);
        WEB_CLIENT_TEST(
            http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE,
            "POST without Content-Length changed validation while growing");
        WEB_CLIENT_TEST(
            w->header_parse_expected_size == invalid_framing_state,
            "missing Content-Length framing state changed while receiving body bytes");
    }
    WEB_CLIENT_TEST(
        w->header_parse_tries == 0,
        "progressive POST without Content-Length consumed %zu retries",
        w->header_parse_tries);
    for (size_t attempt = 0; attempt < HTTP_REQ_MAX_HEADER_FETCH_TRIES; attempt++) {
        WEB_CLIENT_TEST(
            http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE,
            "POST without Content-Length exhausted no-progress retries early");
        WEB_CLIENT_TEST(
            w->header_parse_expected_size == invalid_framing_state,
            "missing Content-Length framing state changed during no-progress validation");
    }
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_TOO_MANY_READ_RETRIES,
        "POST without Content-Length did not exhaust its no-progress retry budget");
    WEB_CLIENT_TEST(
        w->header_parse_expected_size == 0,
        "missing Content-Length framing state was not reset after retry exhaustion");

    web_client_reuse_from_cache(w);
    WEB_CLIENT_TEST(w->header_parse_expected_size == 0, "cached client retained POST framing state");

    buffer_strcat(w->response.data, "GET /api/v1/info");
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE, "initial partial request was not incomplete");
    for (size_t attempt = 0; attempt < HTTP_REQ_MAX_HEADER_FETCH_TRIES; attempt++)
        WEB_CLIENT_TEST(http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE, "no-progress request failed early");
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_TOO_MANY_READ_RETRIES,
        "no-progress request did not exhaust its retry budget");

    web_client_reuse_from_cache(w);
    request = web_client_test_request(NETDATA_WEB_REQUEST_MAX_SIZE, &request_length);
    buffer_flush(w->response.data);
    buffer_memcat(w->response.data, request, request_length);
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_URI_TOO_LONG,
        "oversized request target did not return URI-too-long validation");
    WEB_CLIENT_TEST(
        w->url_decode_buffer_size == NETDATA_WEB_REQUEST_MAX_SIZE,
        "oversized validator target grew scratch to %zu",
        w->url_decode_buffer_size);
    WEB_CLIENT_TEST(HTTP_RESP_URI_TOO_LONG == 414, "URI-too-long HTTP status is not 414");

    freez(request);
    web_client_free(w);
    WEB_CLIENT_TEST(memory_accounting == 0, "web client leaked %zu accounted bytes", memory_accounting);

    if (errors)
        fprintf(stderr, "%s() failed with %d error(s)\n", __FUNCTION__, errors);
    else
        fprintf(stderr, "%s() passed\n", __FUNCTION__);

    return errors;
}
