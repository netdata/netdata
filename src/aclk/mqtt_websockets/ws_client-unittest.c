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

int ws_client_unittest(void)
{
    int errors = 0;

    fprintf(stderr, "\nrunning ws_client unittest\n");

    errors += ws_client_unittest_create_defaults();
    errors += ws_client_unittest_observed_size_payload();
    errors += ws_client_unittest_cap_hit();

    if (errors)
        fprintf(stderr, "ws_client unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "ws_client unittest: OK\n");

    return errors;
}
