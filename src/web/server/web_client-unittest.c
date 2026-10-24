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

    // A four-byte GET first receive used to trigger an early incomplete return
    // on the next receive. GET splits at 5 to 7 bytes and DELETE/STREAM splits
    // at 7 bytes instead made completeness searches start before the buffer.
    // Every accepted method must survive a two-receive split at those sizes. The
    // smallest first receive per method is its token length: shorter first receives
    // are rejected as an unsupported method before the cursor is ever used.
    // SPLIT_CASE carries each request's length from sizeof at compile time, so the
    // loop never has to measure a string at runtime.
#define SPLIT_CASE(request, smallest_first) {(request), sizeof(request) - 1, (smallest_first)}
    static const struct {
        const char *request;
        size_t request_length;
        size_t smallest_first;
    } split_methods[] = {
        SPLIT_CASE("GET /api/v1/info HTTP/1.1\r\nHost: x\r\n\r\n", 4),
        SPLIT_CASE("PUT /api/v1/info HTTP/1.1\r\nContent-Length: 4\r\n\r\ntest", 4),
        SPLIT_CASE("POST /api/v1/info HTTP/1.1\r\nContent-Length: 4\r\n\r\ntest", 5),
        SPLIT_CASE("DELETE /api/v1/info HTTP/1.1\r\nHost: x\r\n\r\n", 7),
        SPLIT_CASE("STREAM key=abc HTTP/1.1\r\nHost: x\r\n\r\n", 7),
        SPLIT_CASE("OPTIONS /api/v1/info HTTP/1.1\r\nHost: x\r\n\r\n", 8),
    };
#undef SPLIT_CASE
    for (size_t m = 0; m < sizeof(split_methods) / sizeof(split_methods[0]); m++) {
        const char *split_request = split_methods[m].request;
        size_t split_request_length = split_methods[m].request_length;

        for (size_t first = split_methods[m].smallest_first; first <= 8; first++) {
            web_client_reuse_from_cache(w);

            buffer_memcat(w->response.data, split_request, first);
            WEB_CLIENT_TEST(
                http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE,
                "'%.*s' alone was not incomplete",
                (int)first,
                split_request);

            buffer_memcat(w->response.data, &split_request[first], split_request_length - first);
            WEB_CLIENT_TEST(
                http_request_validate(w) == HTTP_VALIDATION_OK,
                "'%.*s' split after %zu byte(s) did not validate once complete",
                (int)first,
                split_request,
                first);
        }
    }
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
            // 1024-byte fragments: a 1 MiB request costs ~1024 parse attempts,
            // within HTTP_REQ_MAX_HEADER_FETCH_TRIES. Smaller fragments are
            // rejected by the budget on purpose (see the exhaustion tests below).
            size_t chunk_size = MIN((size_t)1024, request_length - offset);
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

    char missing_content_length_body[1024];
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
    // Every parse attempt consumes budget now, including the ones that saw new
    // bytes, so a growing request spends part of it - but delivering 1 MiB in
    // 1024-byte fragments must stay well inside the allowance. Pin the exact
    // count: a loose bound would still pass if the growth loop consumed the
    // whole budget, which would silently make the exhaustion loop below vacuous.
    // 40 header bytes + 1023 full 1024-byte fragments + one 984-byte remainder,
    // plus the initial validation above = 1025 attempts.
    WEB_CLIENT_TEST(
        w->header_parse_tries == 1025,
        "progressive POST without Content-Length consumed %zu of %zu attempts, expected 1025",
        w->header_parse_tries,
        (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES);

    // WEB_CLIENT_TEST only counts errors and keeps going, so the pin above may
    // have failed. Clamp instead of subtracting blindly: an unsigned underflow
    // here would spin this loop ~SIZE_MAX times and hang the run rather than
    // failing it.
    size_t remaining_attempts = (w->header_parse_tries <= (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES)
                                    ? (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES - w->header_parse_tries
                                    : 0;
    for (size_t attempt = 0; attempt < remaining_attempts; attempt++) {
        WEB_CLIENT_TEST(
            http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE,
            "POST without Content-Length exhausted its attempt budget early");
        WEB_CLIENT_TEST(
            w->header_parse_expected_size == invalid_framing_state,
            "missing Content-Length framing state changed during no-progress validation");
    }
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_TOO_MANY_READ_RETRIES,
        "POST without Content-Length did not exhaust its attempt budget");
    WEB_CLIENT_TEST(
        w->header_parse_expected_size == 0,
        "missing Content-Length framing state was not reset after retry exhaustion");

    web_client_reuse_from_cache(w);
    WEB_CLIENT_TEST(w->header_parse_expected_size == 0, "cached client retained POST framing state");

    buffer_strcat(w->response.data, "GET /api/v1/info");
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE, "initial partial request was not incomplete");
    // The initial validation above already consumed attempt #1, so only
    // HTTP_REQ_MAX_HEADER_FETCH_TRIES - 1 further attempts remain.
    for (size_t attempt = 0; attempt < (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES - 1; attempt++)
        WEB_CLIENT_TEST(http_request_validate(w) == HTTP_VALIDATION_INCOMPLETE, "no-progress request failed early");
    WEB_CLIENT_TEST(
        http_request_validate(w) == HTTP_VALIDATION_TOO_MANY_READ_RETRIES,
        "no-progress request did not exhaust its retry budget");

    // The production-reachable case: a client that keeps delivering bytes. The
    // socket server only validates after web_client_receive() returned bytes>0,
    // so every real validation of an incomplete request has seen new data. This
    // is the case that could never exhaust the budget while the counter only
    // incremented on no-progress attempts.
    web_client_reuse_from_cache(w);
    buffer_strcat(w->response.data, "GET /api/v1/info");
    HTTP_VALIDATION dribble = http_request_validate(w);
    size_t dribble_attempts = 1;
    while (dribble == HTTP_VALIDATION_INCOMPLETE && dribble_attempts < (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES + 10) {
        buffer_memcat(w->response.data, "a", 1);
        dribble = http_request_validate(w);
        dribble_attempts++;
    }
    WEB_CLIENT_TEST(
        dribble == HTTP_VALIDATION_TOO_MANY_READ_RETRIES,
        "request growing by one byte per attempt was not disconnected (validation %d after %zu attempts)",
        (int)dribble,
        dribble_attempts);
    WEB_CLIENT_TEST(
        dribble_attempts == (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES + 1,
        "request growing by one byte per attempt was disconnected after %zu attempts, expected %zu",
        dribble_attempts,
        (size_t)HTTP_REQ_MAX_HEADER_FETCH_TRIES + 1);
    WEB_CLIENT_TEST(
        buffer_strlen(w->response.data) < NETDATA_WEB_REQUEST_MAX_SIZE,
        "growing request hit the %zu byte size cap instead of the attempt budget",
        (size_t)NETDATA_WEB_REQUEST_MAX_SIZE);

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
