// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include "ws_client.h"
#include "endian_compat.h"

int ws_client_process_rx_ws(ws_client *client);

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

    if (errors)
        fprintf(stderr, "ws_client unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "ws_client unittest: OK\n");

    return errors;
}
