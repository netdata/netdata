// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include "ws_client.h"
#include "endian_compat.h"

int ws_client_process_rx_ws(ws_client *client);
int ws_client_parse_handshake_resp(ws_client *client);

#define WS_TEST(condition, msg) do {                                           \
        if (!(condition)) {                                                    \
            fprintf(stderr, "ws_client unittest FAILED: %s (%s:%d)\n",        \
                    (msg), __FUNCTION__, __LINE__);                            \
            errors++;                                                          \
        }                                                                      \
    } while(0)

static void ws_client_unittest_payload_fill(char *dst, size_t offset, size_t len)
{
    for (size_t i = 0; i < len; i++)
        dst[i] = (char)('A' + ((offset + i) % 26));
}

static int ws_client_unittest_payload_check(const char *src, size_t offset, size_t len)
{
    for (size_t i = 0; i < len; i++) {
        if (src[i] != (char)('A' + ((offset + i) % 26)))
            return 0;
    }

    return 1;
}

static size_t ws_client_unittest_binary_header(char *dst, size_t payload_len)
{
    dst[0] = (char)(0x80 | WS_OP_BINARY_FRAME);

    if (payload_len < 126) {
        dst[1] = (char)payload_len;
        return 2;
    }

    if (payload_len <= UINT16_MAX) {
        uint16_t len = htobe16((uint16_t)payload_len);
        dst[1] = 126;
        memcpy(&dst[2], &len, sizeof(len));
        return 2 + sizeof(len);
    }

    uint64_t len = htobe64((uint64_t)payload_len);
    dst[1] = 127;
    memcpy(&dst[2], &len, sizeof(len));
    return 2 + sizeof(len);
}

static size_t ws_client_unittest_control_header(char *dst, enum websocket_opcode opcode, size_t payload_len)
{
    dst[0] = (char)(0x80 | opcode);
    dst[1] = (char)payload_len;
    return 2;
}

static int ws_client_unittest_process_frame(ws_client *client, size_t payload_len)
{
    char header[10];
    size_t header_len = ws_client_unittest_binary_header(header, payload_len);
    size_t frame_len = header_len + payload_len;
    char *frame = mallocz(frame_len);

    memcpy(frame, header, header_len);
    ws_client_unittest_payload_fill(frame + header_len, 0, payload_len);

    if (rbuf_push(client->buf_read, frame, frame_len) != frame_len) {
        freez(frame);
        return WS_CLIENT_INTERNAL_ERROR;
    }

    freez(frame);

    int ret;
    do {
        ret = ws_client_process_rx_ws(client);
    } while (ret == 0);

    return ret;
}

static int ws_client_unittest_verify_mqtt_payload(ws_client *client, size_t payload_len)
{
    int errors = 0;
    size_t checked = 0;
    char out[4096];

    WS_TEST(rbuf_bytes_available(client->buf_to_mqtt) == payload_len, "decoded MQTT payload size");

    while (checked < payload_len) {
        size_t want = MIN(sizeof(out), payload_len - checked);
        WS_TEST(rbuf_pop(client->buf_to_mqtt, out, want) == want, "pop decoded MQTT payload chunk");
        WS_TEST(ws_client_unittest_payload_check(out, checked, want), "decoded MQTT payload byte order");
        checked += want;
    }

    WS_TEST(rbuf_bytes_available(client->buf_to_mqtt) == 0, "decoded MQTT payload drained");
    return errors;
}

static int ws_client_unittest_drain(rbuf_t buf, size_t bytes)
{
    char tmp[256];

    while (bytes) {
        size_t want = MIN(sizeof(tmp), bytes);
        if (rbuf_pop(buf, tmp, want) != want)
            return 0;
        bytes -= want;
    }

    return 1;
}

// Sends payload_len bytes through ws_client_send() with the write ringbuffer's head
// positioned by prefill/drain, then verifies the queued frame byte for byte against the
// reference masking of RFC6455 5.3. prefill/drain place the head close to the end of the
// ringbuffer so the payload has to be written in two chunks.
static int ws_client_unittest_masked_frame(size_t buf_size, size_t payload_len, size_t prefill, size_t drain,
                                          size_t expect_written, const char *label)
{
    int errors = 0;
    char msg[256];
    ws_client client = {
        .state = WS_ESTABLISHED,
        .rx.parse_state = WS_FIRST_2BYTES,
        .buf_read = rbuf_create(64, 64),
        .buf_write = rbuf_create(buf_size, buf_size),
        .buf_to_mqtt = rbuf_create(64, 64),
    };

    if (prefill) {
        char *filler = mallocz(prefill);
        memset(filler, 'x', prefill);
        snprintfz(msg, sizeof(msg), "%s: prefill write buffer", label);
        WS_TEST(rbuf_push(client.buf_write, filler, prefill) == prefill, msg);
        freez(filler);

        snprintfz(msg, sizeof(msg), "%s: drain write buffer", label);
        WS_TEST(ws_client_unittest_drain(client.buf_write, drain), msg);
    }

    // exact-sized allocation on purpose: reading past the payload is then a heap
    // overflow that ASAN/valgrind can see
    char *payload = mallocz(payload_len);
    ws_client_unittest_payload_fill(payload, 0, payload_len);

    int written = ws_client_send(&client, WS_OP_BINARY_FRAME, payload, payload_len);
    snprintfz(msg, sizeof(msg), "%s: ws_client_send() writes exactly the payload", label);
    WS_TEST(written == (int)expect_written, msg);

    // 2 bytes of frame header + 4 bytes of mask key, plus the extended length field
    size_t hdr_len = 2 + 4;
    if (expect_written > 125)
        hdr_len += 2;
    if (expect_written > 65535)
        hdr_len += 6;

    snprintfz(msg, sizeof(msg), "%s: queued frame size", label);
    WS_TEST(rbuf_bytes_available(client.buf_write) == (prefill - drain) + hdr_len + expect_written, msg);

    snprintfz(msg, sizeof(msg), "%s: drain bytes queued before the frame", label);
    WS_TEST(ws_client_unittest_drain(client.buf_write, prefill - drain), msg);

    char hdr[2];
    snprintfz(msg, sizeof(msg), "%s: pop frame header", label);
    WS_TEST(rbuf_pop(client.buf_write, hdr, sizeof(hdr)) == sizeof(hdr), msg);

    snprintfz(msg, sizeof(msg), "%s: FIN and binary opcode", label);
    WS_TEST((unsigned char)hdr[0] == (0x80 | WS_OP_BINARY_FRAME), msg);

    snprintfz(msg, sizeof(msg), "%s: mask bit set", label);
    WS_TEST(((unsigned char)hdr[1] & 0x80) != 0, msg);

    size_t declared = (unsigned char)hdr[1] & 0x7f;
    if (declared == 126) {
        char len[2];
        snprintfz(msg, sizeof(msg), "%s: pop 16bit length", label);
        WS_TEST(rbuf_pop(client.buf_write, len, sizeof(len)) == sizeof(len), msg);
        declared = ((size_t)(unsigned char)len[0] << 8) | (size_t)(unsigned char)len[1];
    }

    snprintfz(msg, sizeof(msg), "%s: declared payload length", label);
    WS_TEST(declared == expect_written, msg);

    char mask[4];
    snprintfz(msg, sizeof(msg), "%s: pop mask key", label);
    WS_TEST(rbuf_pop(client.buf_write, mask, sizeof(mask)) == sizeof(mask), msg);

    char *masked = mallocz(expect_written);
    snprintfz(msg, sizeof(msg), "%s: pop masked payload", label);
    WS_TEST(rbuf_pop(client.buf_write, masked, expect_written) == expect_written, msg);

    size_t mismatches = 0;
    for (size_t i = 0; i < expect_written; i++) {
        if (masked[i] != (char)(payload[i] ^ mask[i & 3]))
            mismatches++;
    }

    snprintfz(msg, sizeof(msg), "%s: payload masked per RFC6455 5.3", label);
    WS_TEST(mismatches == 0, msg);

    snprintfz(msg, sizeof(msg), "%s: write buffer fully consumed", label);
    WS_TEST(rbuf_bytes_available(client.buf_write) == 0, msg);

    freez(masked);
    freez(payload);
    rbuf_free(client.buf_read);
    rbuf_free(client.buf_write);
    rbuf_free(client.buf_to_mqtt);
    return errors;
}

static int ws_client_unittest_masking(void)
{
    int errors = 0;

    // shorter than the word-at-a-time step, so only the byte tail runs
    errors += ws_client_unittest_masked_frame(1024, 5, 0, 0, 5, "5 byte payload");

    // not a multiple of 8: word blocks plus a 4 byte tail, and the 16bit length header
    errors += ws_client_unittest_masked_frame(1024, 300, 0, 0, 300, "300 byte payload");

    // split across the ringbuffer wrap at payload offset 18, so the mask phase has to be
    // carried into the second chunk at a non-multiple of 4
    errors += ws_client_unittest_masked_frame(1024, 100, 1000, 990, 100, "wrapped payload");

    // free space (108) exceeds the payload (100) by less than the first chunk (18), so the
    // second chunk (90) is larger than what is left (82) while still being <= payload size.
    // A clamp against the payload size instead of the remainder over-copies here.
    errors += ws_client_unittest_masked_frame(1024, 100, 1000, 90, 100, "wrapped payload, nearly full buffer");

    return errors;
}

static int ws_client_unittest_create_defaults(void)
{
    int errors = 0;
    char *host = (char *)"localhost";
    ws_client *client = ws_client_new(1024, &host);

    WS_TEST(client != NULL, "ws_client_new succeeds");
    if (!client)
        return errors;

    WS_TEST(rbuf_get_capacity(client->buf_read) == 1024, "buf_read initial capacity");
    WS_TEST(rbuf_get_max_capacity(client->buf_read) == 1024, "buf_read remains fixed");
    WS_TEST(rbuf_get_capacity(client->buf_write) == 1024, "buf_write initial capacity");
    WS_TEST(rbuf_get_max_capacity(client->buf_write) == 1024, "buf_write remains fixed");
    WS_TEST(rbuf_get_capacity(client->buf_to_mqtt) == 1024, "buf_to_mqtt initial capacity");
    WS_TEST(rbuf_get_max_capacity(client->buf_to_mqtt) == 16 * 1024 * 1024, "buf_to_mqtt hard cap");

    ws_client_destroy(client);
    return errors;
}

static int ws_client_unittest_clamps_mqtt_input_initial_size(void)
{
    int errors = 0;
    char *host = (char *)"localhost";
    const size_t hard_cap = 16 * 1024 * 1024;
    const size_t oversized = hard_cap + 1;
    ws_client *client = ws_client_new(oversized, &host);

    WS_TEST(client != NULL, "ws_client_new succeeds with oversized buffer request");
    if (!client)
        return errors;

    WS_TEST(rbuf_get_capacity(client->buf_read) == oversized, "buf_read keeps requested fixed capacity");
    WS_TEST(rbuf_get_capacity(client->buf_write) == oversized, "buf_write keeps requested fixed capacity");
    WS_TEST(rbuf_get_capacity(client->buf_to_mqtt) == hard_cap, "buf_to_mqtt initial capacity is capped");
    WS_TEST(rbuf_get_max_capacity(client->buf_to_mqtt) == hard_cap, "buf_to_mqtt max capacity is capped");

    ws_client_destroy(client);
    return errors;
}

static int ws_client_unittest_observed_size_payload(void)
{
    int errors = 0;
    const size_t payload_len = 369065;
    ws_client client = {
        .state = WS_ESTABLISHED,
        .rx.parse_state = WS_FIRST_2BYTES,
        .buf_read = rbuf_create(payload_len + 10, payload_len + 10),
        .buf_write = rbuf_create(1024, 1024),
        .buf_to_mqtt = rbuf_create(128 * 1024, 16 * 1024 * 1024),
    };

    int ret = ws_client_unittest_process_frame(&client, payload_len);
    WS_TEST(ret == WS_CLIENT_PARSING_DONE, "observed-size WebSocket payload parses");
    WS_TEST(rbuf_get_capacity(client.buf_to_mqtt) > 128 * 1024, "observed-size payload grows MQTT buffer");
    errors += ws_client_unittest_verify_mqtt_payload(&client, payload_len);

    rbuf_free(client.buf_read);
    rbuf_free(client.buf_write);
    rbuf_free(client.buf_to_mqtt);
    return errors;
}

static int ws_client_unittest_cap_hit(void)
{
    int errors = 0;
    const size_t payload_len = 4097;
    const size_t recovery_payload_len = 32;
    ws_client client = {
        .state = WS_ESTABLISHED,
        .rx.parse_state = WS_FIRST_2BYTES,
        .buf_read = rbuf_create(payload_len + 10, payload_len + 10),
        .buf_write = rbuf_create(1024, 1024),
        .buf_to_mqtt = rbuf_create(1024, 4096),
    };

    int ret = ws_client_unittest_process_frame(&client, payload_len);
    WS_TEST(ret == WS_CLIENT_BUFFER_FULL, "over-cap WebSocket payload reports buffer full");
    WS_TEST(rbuf_get_capacity(client.buf_to_mqtt) == 4096, "over-cap MQTT buffer stops at cap");
    WS_TEST(rbuf_bytes_available(client.buf_to_mqtt) == 4096, "over-cap MQTT buffer keeps accepted bytes");

    ws_client_reset(&client);
    client.state = WS_ESTABLISHED;

    WS_TEST(rbuf_get_capacity(client.buf_to_mqtt) == 4096, "reset keeps grown MQTT buffer capacity");

    ret = ws_client_unittest_process_frame(&client, recovery_payload_len);
    WS_TEST(ret == WS_CLIENT_PARSING_DONE, "post-cap reset parses next WebSocket payload");
    errors += ws_client_unittest_verify_mqtt_payload(&client, recovery_payload_len);

    rbuf_free(client.buf_read);
    rbuf_free(client.buf_write);
    rbuf_free(client.buf_to_mqtt);
    return errors;
}

static int ws_client_unittest_reset_frees_partial_close_reason(void)
{
    int errors = 0;
    const size_t reason_len = 3;
    const size_t close_payload_len = sizeof(uint16_t) + reason_len;
    char frame[2 + sizeof(uint16_t)];
    size_t header_len = ws_client_unittest_control_header(frame, WS_OP_CONNECTION_CLOSE, close_payload_len);
    uint16_t close_code = htobe16(1000);
    memcpy(&frame[header_len], &close_code, sizeof(close_code));

    ws_client client = {
        .state = WS_ESTABLISHED,
        .rx.parse_state = WS_FIRST_2BYTES,
        .buf_read = rbuf_create(64, 64),
        .buf_write = rbuf_create(1024, 1024),
        .buf_to_mqtt = rbuf_create(1024, 1024),
    };

    WS_TEST(rbuf_push(client.buf_read, frame, sizeof(frame)) == sizeof(frame), "push partial close frame");

    int ret;
    do {
        ret = ws_client_process_rx_ws(&client);
    } while (ret == 0);

    WS_TEST(ret == WS_CLIENT_NEED_MORE_BYTES, "partial close reason needs more bytes");
    WS_TEST(client.rx.specific_data.op_close.reason != NULL, "partial close reason allocated");
#ifdef NETDATA_TRACE_ALLOCATIONS
    WS_TEST(mallocz_usable_size(client.rx.specific_data.op_close.reason) == reason_len + 1,
            "partial close reason buffer fits reason plus terminator");
#endif

    ws_client_reset(&client);

    WS_TEST(client.rx.specific_data.op_close.reason == NULL, "reset frees partial close reason");
    WS_TEST(client.rx.payload_processed == 0, "reset clears close payload progress");

    rbuf_free(client.buf_read);
    rbuf_free(client.buf_write);
    rbuf_free(client.buf_to_mqtt);
    return errors;
}

static int ws_client_unittest_ping_payload_cleanup(void)
{
    int errors = 0;
    const char payload[] = "abc";
    char frame[2 + sizeof(payload) - 1];
    size_t header_len = ws_client_unittest_control_header(frame, WS_OP_PING, sizeof(payload) - 1);
    memcpy(&frame[header_len], payload, sizeof(payload) - 1);

    ws_client client = {
        .state = WS_ESTABLISHED,
        .rx.parse_state = WS_FIRST_2BYTES,
        .buf_read = rbuf_create(64, 64),
        .buf_write = rbuf_create(1024, 1024),
        .buf_to_mqtt = rbuf_create(1024, 1024),
    };

    WS_TEST(rbuf_push(client.buf_read, frame, sizeof(frame)) == sizeof(frame), "push ping frame");

    int ret;
    do {
        ret = ws_client_process_rx_ws(&client);
    } while (ret == 0);

    WS_TEST(ret == WS_CLIENT_PARSING_DONE, "ping frame parses");
    WS_TEST(client.rx.specific_data.ping_msg == NULL, "ping payload released after PONG generation");
    WS_TEST(rbuf_bytes_available(client.buf_write) > 0, "PONG queued");

    ws_client_reset(&client);

    rbuf_free(client.buf_read);
    rbuf_free(client.buf_write);
    rbuf_free(client.buf_to_mqtt);
    return errors;
}

#define WS_UT_NONCE "{NONCE}"

// Copies `tmpl` to `dst`, replacing every WS_UT_NONCE with `nonce`. Returns the length, or -1 if it does not fit.
static int ws_client_unittest_fill(char *dst, size_t dst_size, const char *tmpl, const char *nonce)
{
    size_t len = 0, token_len = strlen(WS_UT_NONCE), nonce_len = strlen(nonce);
    while (*tmpl) {
        const char *add = tmpl;
        size_t add_len = 1;
        if (!strncmp(tmpl, WS_UT_NONCE, token_len)) {
            add = nonce;
            add_len = nonce_len;
            tmpl += token_len;
        }
        else
            tmpl++;

        if (len + add_len >= dst_size)
            return -1;
        memcpy(dst + len, add, add_len);
        len += add_len;
    }
    dst[len] = '\0';
    return (int)len;
}

// Feeds one canned upgrade reply to a client that has sent its upgrade request.
// `reply_tmpl` may hold WS_UT_NONCE, replaced by the accept value the client expects.
static int ws_client_unittest_handshake_reply(ws_client *client, const char *reply_tmpl)
{
    char reply[1024];
    int len = ws_client_unittest_fill(reply, sizeof(reply), reply_tmpl, client->hs.nonce_reply);
    if (len < 0 || rbuf_push(client->buf_read, reply, len) != (size_t)len)
        return WS_CLIENT_INTERNAL_ERROR;

    int ret;
    do {
        ret = ws_client_parse_handshake_resp(client);
    } while (ret == 0);

    return ret;
}

// `expected_err` is the reason ws_client_check_handshake_reply() gives, NULL when the reply is accepted;
// like `reply_tmpl`, it may hold WS_UT_NONCE for the expected accept value.
static int ws_client_unittest_handshake_case(const char *name, const char *reply_tmpl, const char *expected_err)
{
    int errors = 0;
    char *host = (char *)"localhost";
    ws_client *client = ws_client_new(4096, &host);

    WS_TEST(client != NULL, name);
    if (!client)
        return errors;

    WS_TEST(ws_client_start_handshake(client) == 0, name);

    fprintf(stderr, "ws_client handshake case: %s\n", name);
    int ret = ws_client_unittest_handshake_reply(client, reply_tmpl);
    WS_TEST(ret == (expected_err ? WS_CLIENT_PROTOCOL_ERROR : WS_CLIENT_PARSING_DONE), name);
    WS_TEST(client->state == (expected_err ? WS_HANDSHAKE : WS_ESTABLISHED), name);

    char err[512] = "", expected[512] = "";
    int rc = ws_client_check_handshake_reply(client, err, sizeof(err));
    if (expected_err) {
        WS_TEST(ws_client_unittest_fill(expected, sizeof(expected), expected_err, client->hs.nonce_reply) >= 0, name);
        WS_TEST(rc == -1, name);
        WS_TEST(!strcmp(err, expected), name);
        if (strcmp(err, expected))
            fprintf(stderr, "    got:      %s\n    expected: %s\n", err, expected);
    }
    else
        WS_TEST(rc == 0, name);

    ws_client_destroy(client);
    return errors;
}

#define WS_UT_101 "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
#define WS_UT_404 "HTTP/1.1 404 Not Found\r\nServer: nginx/1.25.3\r\nContent-Length: 0\r\n"
#define WS_UT_404_ERR "HTTP return code not 101. Received 404 with msg \"Not Found\" (server: \"nginx/1.25.3\")."
#define WS_UT_WRONG "AAAAAAAAAAAAAAAAAAAAAAAAAAA="
#define WS_UT_WRONG_ACCEPT "Sec-WebSocket-Accept: " WS_UT_WRONG "\r\n"
#define WS_UT_WRONG_ERR "Received NONCE \"" WS_UT_WRONG "\" does not match expected nonce of \"" WS_UT_NONCE "\""
#define WS_UT_MISSING_ERR "Missing sec-websocket-accept header"

static int ws_client_unittest_handshake(void)
{
    int errors = 0;

    // a non-101 reply fails whatever it says about the accept header, and the reason names the status
    errors += ws_client_unittest_handshake_case("404 without accept", WS_UT_404 "\r\n", WS_UT_404_ERR);
    errors += ws_client_unittest_handshake_case("404 with wrong accept", WS_UT_404 WS_UT_WRONG_ACCEPT "\r\n", WS_UT_404_ERR);
    errors += ws_client_unittest_handshake_case("404 with matching accept", WS_UT_404 "Sec-WebSocket-Accept: " WS_UT_NONCE "\r\n\r\n", WS_UT_404_ERR);
    errors += ws_client_unittest_handshake_case("502 with empty reason, no server", "HTTP/1.1 502 \r\n\r\n",
                                                "HTTP return code not 101. Received 502 with msg \"\".");
    errors += ws_client_unittest_handshake_case("403 with control bytes", "HTTP/1.1 403 Forbid\x01" "den\r\nServer: evil\x1b[31mred\r\n\r\n",
                                                "HTTP return code not 101. Received 403 with msg \"Forbid?den\" (server: \"evil?[31mred\").");
    errors += ws_client_unittest_handshake_case("403 with C1 and non-ASCII bytes", "HTTP/1.1 403 Forbid\x9b" "den\r\nServer: caf\xc3\xa9 \xc2\x9b" "2J\r\n\r\n",
                                                "HTTP return code not 101. Received 403 with msg \"Forbid?den\" (server: \"caf?? ??2J\").");
    // header values may be empty and have optional whitespace around them
    errors += ws_client_unittest_handshake_case("404 with empty server value", "HTTP/1.1 404 Not Found\r\nServer: \r\n\r\n",
                                                "HTTP return code not 101. Received 404 with msg \"Not Found\" (server: \"\").");
    errors += ws_client_unittest_handshake_case("404 with bare server field", "HTTP/1.1 404 Not Found\r\nServer:\r\n\r\n",
                                                "HTTP return code not 101. Received 404 with msg \"Not Found\" (server: \"\").");
    errors += ws_client_unittest_handshake_case("404 with unpadded and padded fields", "HTTP/1.1 404 Not Found\r\nX-Id:1\r\nServer: \t envoy \t\r\n\r\n",
                                                "HTTP return code not 101. Received 404 with msg \"Not Found\" (server: \"envoy\").");
    {
        // peer strings are cut to 127 bytes in the log
        char reply[700], expected[400], longval[301];
        memset(longval, 'x', 300);
        longval[300] = '\0';
        snprintf(reply, sizeof(reply), "HTTP/1.1 400 %s\r\nServer: %s\r\n\r\n", longval, longval);
        snprintf(expected, sizeof(expected), "HTTP return code not 101. Received 400 with msg \"%.127s\" (server: \"%.127s\").", longval, longval);
        errors += ws_client_unittest_handshake_case("400 with long reason and server", reply, expected);
    }

    // a 101 reply needs exactly the expected accept value, in every accept header it carries
    errors += ws_client_unittest_handshake_case("101 without accept", WS_UT_101 "\r\n", WS_UT_MISSING_ERR);
    errors += ws_client_unittest_handshake_case("101 with wrong accept", WS_UT_101 WS_UT_WRONG_ACCEPT "\r\n", WS_UT_WRONG_ERR);
    errors += ws_client_unittest_handshake_case("101 with control bytes in wrong accept", WS_UT_101 "Sec-WebSocket-Accept: \x07" "bell\r\n\r\n",
                                                "Received NONCE \"?bell\" does not match expected nonce of \"" WS_UT_NONCE "\"");
    errors += ws_client_unittest_handshake_case("101 with matching then wrong accept", WS_UT_101 "Sec-WebSocket-Accept: " WS_UT_NONCE "\r\n" WS_UT_WRONG_ACCEPT "\r\n", WS_UT_WRONG_ERR);
    errors += ws_client_unittest_handshake_case("101 with wrong then matching accept", WS_UT_101 WS_UT_WRONG_ACCEPT "Sec-WebSocket-Accept: " WS_UT_NONCE "\r\n\r\n", WS_UT_WRONG_ERR);
    errors += ws_client_unittest_handshake_case("101 with matching accept", WS_UT_101 "sec-websocket-accept: " WS_UT_NONCE "\r\n\r\n", NULL);
    errors += ws_client_unittest_handshake_case("101 with unpadded matching accept", WS_UT_101 "Sec-WebSocket-Accept:" WS_UT_NONCE "\r\n\r\n", NULL);
    errors += ws_client_unittest_handshake_case("101 with padded matching accept", WS_UT_101 "Sec-WebSocket-Accept: \t " WS_UT_NONCE " \t\r\n\r\n", NULL);
    errors += ws_client_unittest_handshake_case("101 with empty accept", WS_UT_101 "Sec-WebSocket-Accept: \r\n\r\n",
                                                "Received NONCE \"\" does not match expected nonce of \"" WS_UT_NONCE "\"");
    // a proxy may answer with an HTTP/1.0 error page; its status is still reported, but 1.0 cannot upgrade
    errors += ws_client_unittest_handshake_case("HTTP/1.0 503 from a proxy", "HTTP/1.0 503 Service Unavailable\r\nServer: haproxy\r\n\r\n",
                                                "HTTP return code not 101. Received 503 with msg \"Service Unavailable\" (server: \"haproxy\").");
    errors += ws_client_unittest_handshake_case("HTTP/1.0 101 with matching accept", "HTTP/1.0 101 Switching Protocols\r\nSec-WebSocket-Accept: " WS_UT_NONCE "\r\n\r\n",
                                                "Received HTTP/1.0 101 reply; a WebSocket upgrade requires HTTP/1.1");

    // a reply that is not HTTP/1.x fails while the status line is parsed
    {
        char *junk_host = (char *)"localhost";
        ws_client *junk = ws_client_new(4096, &junk_host);
        WS_TEST(junk != NULL, "junk status line: client");
        if (junk) {
            WS_TEST(ws_client_start_handshake(junk) == 0, "junk status line: handshake starts");
            fprintf(stderr, "ws_client handshake case: junk status line\n");
            WS_TEST(ws_client_unittest_handshake_reply(junk, "SSH-2.0-OpenSSH_9.6\r\n\r\n") == WS_CLIENT_PROTOCOL_ERROR,
                    "junk status line: rejected");
            WS_TEST(junk->state == WS_HANDSHAKE, "junk status line: not established");
            ws_client_destroy(junk);
        }
    }

    // a reconnect must not inherit the previous connection's accepted handshake
    char *host = (char *)"localhost";
    ws_client *client = ws_client_new(4096, &host);
    WS_TEST(client != NULL, "reconnect: client");
    if (!client)
        return errors;

    WS_TEST(ws_client_start_handshake(client) == 0, "reconnect: first handshake starts");
    fprintf(stderr, "ws_client handshake case: reconnect, first 101 with matching accept\n");
    WS_TEST(ws_client_unittest_handshake_reply(client, WS_UT_101 "Sec-WebSocket-Accept: " WS_UT_NONCE "\r\n\r\n") == WS_CLIENT_PARSING_DONE,
            "reconnect: first handshake accepted");

    ws_client_reset(client);
    WS_TEST(ws_client_start_handshake(client) == 0, "reconnect: second handshake starts");
    fprintf(stderr, "ws_client handshake case: reconnect, second 101 without accept\n");
    WS_TEST(ws_client_unittest_handshake_reply(client, WS_UT_101 "\r\n") == WS_CLIENT_PROTOCOL_ERROR,
            "reconnect: 101 without accept after reset is rejected");
    WS_TEST(client->state == WS_HANDSHAKE, "reconnect: not established");

    ws_client_destroy(client);
    return errors;
}

int ws_client_unittest(void)
{
    int errors = 0;

    fprintf(stderr, "\nrunning ws_client unittest\n");

    errors += ws_client_unittest_masking();
    errors += ws_client_unittest_create_defaults();
    errors += ws_client_unittest_clamps_mqtt_input_initial_size();
    errors += ws_client_unittest_observed_size_payload();
    errors += ws_client_unittest_cap_hit();
    errors += ws_client_unittest_reset_frees_partial_close_reason();
    errors += ws_client_unittest_ping_payload_cleanup();
    errors += ws_client_unittest_handshake();

    if (errors)
        fprintf(stderr, "ws_client unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "ws_client unittest: OK\n");

    return errors;
}
