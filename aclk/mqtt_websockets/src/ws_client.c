// Copyright (C) 2020 Timotej Šiškovič
// SPDX-License-Identifier: GPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free Software Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY;
// without even the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
// See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with this program.
// If not, see <https://www.gnu.org/licenses/>.

#include <fcntl.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>
#include <ctype.h>

#include <openssl/evp.h>

#include "ws_client.h"
#include "common_internal.h"

#ifdef MQTT_WEBSOCKETS_DEBUG
#include "../c-rbuf/src/ringbuffer_internal.h"
#endif

#define UNIT_LOG_PREFIX "ws_client: "
#define FATAL(fmt, ...) mws_fatal(client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define ERROR(fmt, ...) mws_error(client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define WARN(fmt, ...)  mws_warn (client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define INFO(fmt, ...)  mws_info (client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)
#define DEBUG(fmt, ...) mws_debug(client->log, UNIT_LOG_PREFIX fmt, ##__VA_ARGS__)

const char *websocket_upgrage_hdr = "GET /mqtt HTTP/1.1\x0D\x0A"
                              "Host: %s\x0D\x0A"
                              "Upgrade: websocket\x0D\x0A"
                              "Connection: Upgrade\x0D\x0A"
                              "Sec-WebSocket-Key: %s\x0D\x0A"
                              "Origin: http://example.com\x0D\x0A"
                              "Sec-WebSocket-Protocol: mqtt\x0D\x0A"
                              "Sec-WebSocket-Version: 13\x0D\x0A\x0D\x0A";

const char *mqtt_protoid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";

#define DEFAULT_RINGBUFFER_SIZE (1024*128)
#define ENTROPY_SOURCE "/dev/urandom"
ws_client *ws_client_new(size_t buf_size, char **host, mqtt_wss_log_ctx_t log)
{
    ws_client *client;

    if(!host)
        return NULL;

    client = mw_calloc(1, sizeof(ws_client));
    if (!client)
        return NULL;

    client->host = host;
    client->log = log;

    client->buf_read = rbuf_create(buf_size ? buf_size : DEFAULT_RINGBUFFER_SIZE);
    if (!client->buf_read)
        goto cleanup;

    client->buf_write = rbuf_create(buf_size ? buf_size : DEFAULT_RINGBUFFER_SIZE);
    if (!client->buf_write)
        goto cleanup_1;

    client->buf_to_mqtt = rbuf_create(buf_size ? buf_size : DEFAULT_RINGBUFFER_SIZE);
    if (!client->buf_to_mqtt)
        goto cleanup_2;

    client->entropy_fd = open(ENTROPY_SOURCE, O_RDONLY);
    if (client->entropy_fd < 1) {
        ERROR("Error opening entropy source \"" ENTROPY_SOURCE "\". Reason: \"%s\"", strerror(errno));
        goto cleanup_3;
    }

    return client;

cleanup_3:
    rbuf_free(client->buf_to_mqtt);
cleanup_2:
    rbuf_free(client->buf_write);
cleanup_1:
    rbuf_free(client->buf_read);
cleanup:
    mw_free(client);
    return NULL;
}

void ws_client_free_headers(ws_client *client)
{
    struct http_header *ptr = client->hs.headers;
    struct http_header *tmp;

    while (ptr) {
        tmp = ptr;
        ptr = ptr->next;
        mw_free(tmp);
    }

    client->hs.headers = NULL;
    client->hs.headers_tail = NULL;
    client->hs.hdr_count = 0;
}

void ws_client_destroy(ws_client *client)
{
    ws_client_free_headers(client);
    mw_free(client->hs.nonce_reply);
    mw_free(client->hs.http_reply_msg);
    close(client->entropy_fd);
    rbuf_free(client->buf_read);
    rbuf_free(client->buf_write);
    rbuf_free(client->buf_to_mqtt);
    mw_free(client);
}

void ws_client_reset(ws_client *client)
{
    ws_client_free_headers(client);
    mw_free(client->hs.nonce_reply);
    client->hs.nonce_reply = NULL;
    mw_free(client->hs.http_reply_msg);
    client->hs.http_reply_msg = NULL;
    rbuf_flush(client->buf_read);
    rbuf_flush(client->buf_write);
    rbuf_flush(client->buf_to_mqtt);
    client->state = WS_RAW;
    client->hs.hdr_state = WS_HDR_HTTP;
    client->rx.parse_state = WS_FIRST_2BYTES;
}

#define MAX_HTTP_HDR_COUNT 128
int ws_client_add_http_header(ws_client *client, struct http_header *hdr)
{
    if (client->hs.hdr_count > MAX_HTTP_HDR_COUNT) {
        ERROR("Too many HTTP response header fields");
        return -1;
    }

    if (client->hs.headers)
        client->hs.headers_tail->next = hdr;
    else
        client->hs.headers = hdr;

    client->hs.headers_tail = hdr;
    client->hs.hdr_count++;

    return 0;
}

int ws_client_want_write(ws_client *client)
{
    return rbuf_bytes_available(client->buf_write);
}

#define RAND_SRC "/dev/urandom"
static int ws_client_get_nonce(ws_client *client, char *dest, unsigned int size)
{
    // we do not need crypto secure random here
    // it's just used for protocol negotiation
    int rd;
    int f = open(RAND_SRC, O_RDONLY);
    if (f < 0) {
        ERROR("Error opening \"%s\". Err: \"%s\"", RAND_SRC, strerror(errno));
        return -2;
    }

    if ((rd = read(f, dest, size)) > 0) {
        close(f);
        return rd;
    }
    close(f);
    return -1;
}

#define WEBSOCKET_NONCE_SIZE 16
#define TEMP_BUF_SIZE 4096
int ws_client_start_handshake(ws_client *client)
{
    char nonce[WEBSOCKET_NONCE_SIZE];
    char nonce_b64[256];
    char second[TEMP_BUF_SIZE];
    unsigned int md_len;
    unsigned char *digest;
    EVP_MD_CTX *md_ctx;
    const EVP_MD *md;

    if(!*client->host) {
        ERROR("Hostname has not been set. We should not be able to come here!");
        return 1;
    }

    ws_client_get_nonce(client, nonce, WEBSOCKET_NONCE_SIZE);
    EVP_EncodeBlock((unsigned char *)nonce_b64, (const unsigned char *)nonce, WEBSOCKET_NONCE_SIZE);
    snprintf(second, TEMP_BUF_SIZE, websocket_upgrage_hdr,
        *client->host,
        nonce_b64);
    if(rbuf_bytes_free(client->buf_write) < strlen(second)) {
        ERROR("Write buffer capacity too low.");
        return 1;
    }

    rbuf_push(client->buf_write, second, strlen(second));
    client->state = WS_HANDSHAKE;

    //Calculating expected Sec-WebSocket-Accept reply
    snprintf(second, TEMP_BUF_SIZE, "%s%s", nonce_b64, mqtt_protoid);

#if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
    md_ctx = EVP_MD_CTX_create();
#else
    md_ctx = EVP_MD_CTX_new();
#endif
    if (md_ctx == NULL) {
        ERROR("Cant create EVP_MD Context");
        return 1;
    }

    md = EVP_get_digestbyname("sha1");
    if (!md) {
        ERROR("Unknown message digest");
        return 1;
    }

    if ((digest = (unsigned char *)OPENSSL_malloc(EVP_MD_size(EVP_sha256()))) == NULL) {
        ERROR("Cant alloc digest");
        return 1;
    }

    EVP_DigestInit_ex(md_ctx, md, NULL);
    EVP_DigestUpdate(md_ctx, second, strlen(second));
    EVP_DigestFinal_ex(md_ctx, digest, &md_len);

    EVP_EncodeBlock((unsigned char *)nonce_b64, digest, md_len);

    mw_free(client->hs.nonce_reply);
    client->hs.nonce_reply = mw_strdup(nonce_b64);

    OPENSSL_free(digest);

#if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
    EVP_MD_CTX_destroy(md_ctx);
#else
    EVP_MD_CTX_free(md_ctx);
#endif

    return 0;
}

#define BUF_READ_MEMCMP_CONST(const, err)                                                                              \
    if (rbuf_memcmp_n(client->buf_read, const, strlen(const))) {                                                       \
        ERROR(err);                                                                                                    \
        rbuf_flush(client->buf_read);                                                                                  \
        return WS_CLIENT_PROTOCOL_ERROR;                                                                               \
    }

#define BUF_READ_CHECK_AT_LEAST(x)                                                                                     \
    if (rbuf_bytes_available(client->buf_read) < x)                                                                    \
        return WS_CLIENT_NEED_MORE_BYTES;

#define MAX_HTTP_LINE_LENGTH 1024*4
#define HTTP_SC_LENGTH 4 // "XXX " http status code as C string
#define WS_CLIENT_HTTP_HDR "HTTP/1.1 "
#define WS_CONN_ACCEPT "sec-websocket-accept"
#define HTTP_HDR_SEPARATOR ": "
#define WS_NONCE_STRLEN_B64 28
#define WS_HTTP_NEWLINE "\r\n"
#define HTTP_HEADER_NAME_MAX_LEN 256
#if HTTP_HEADER_NAME_MAX_LEN > MAX_HTTP_LINE_LENGTH
#error "Buffer too small"
#endif
#if WS_NONCE_STRLEN_B64 > MAX_HTTP_LINE_LENGTH
#error "Buffer too small"
#endif

#define HTTP_HDR_LINE_CHECK_LIMIT(x) if ((x) >= MAX_HTTP_LINE_LENGTH) \
{ \
    ERROR("HTTP line received is too long. Maximum is %d", MAX_HTTP_LINE_LENGTH); \
    return WS_CLIENT_PROTOCOL_ERROR; \
}

int ws_client_parse_handshake_resp(ws_client *client)
{
    char buf[HTTP_SC_LENGTH];
    int idx_crlf, idx_sep;
    char *ptr;
    size_t bytes;
    switch (client->hs.hdr_state) {
        case WS_HDR_HTTP:
            BUF_READ_CHECK_AT_LEAST(strlen(WS_CLIENT_HTTP_HDR))
            BUF_READ_MEMCMP_CONST(WS_CLIENT_HTTP_HDR, "Expected \"HTTP1.1\" header");
            rbuf_bump_tail(client->buf_read, strlen(WS_CLIENT_HTTP_HDR));
            client->hs.hdr_state = WS_HDR_RC;
            break;
        case WS_HDR_RC:
            BUF_READ_CHECK_AT_LEAST(HTTP_SC_LENGTH); // "XXX " http return code
            rbuf_pop(client->buf_read, buf, HTTP_SC_LENGTH);
            if (buf[HTTP_SC_LENGTH - 1] != 0x20) {
                ERROR("HTTP status code received is not terminated by space (0x20)");
                return WS_CLIENT_PROTOCOL_ERROR;
            }
            buf[HTTP_SC_LENGTH - 1] = 0;
            client->hs.http_code = atoi(buf);
            if (client->hs.http_code < 100 || client->hs.http_code >= 600) {
                ERROR("HTTP status code received not in valid range 100-600");
                return WS_CLIENT_PROTOCOL_ERROR;
            }
            client->hs.hdr_state = WS_HDR_ENDLINE;
            break;
        case WS_HDR_ENDLINE:
            ptr = rbuf_find_bytes(client->buf_read, WS_HTTP_NEWLINE, strlen(WS_HTTP_NEWLINE), &idx_crlf);
            if (!ptr) {
                bytes = rbuf_bytes_available(client->buf_read);
                HTTP_HDR_LINE_CHECK_LIMIT(bytes);
                return WS_CLIENT_NEED_MORE_BYTES;
            }
            HTTP_HDR_LINE_CHECK_LIMIT(idx_crlf);

            client->hs.http_reply_msg = mw_malloc(idx_crlf+1);
            rbuf_pop(client->buf_read, client->hs.http_reply_msg, idx_crlf);
            client->hs.http_reply_msg[idx_crlf] = 0;
            rbuf_bump_tail(client->buf_read, strlen(WS_HTTP_NEWLINE));
            client->hs.hdr_state = WS_HDR_PARSE_HEADERS;
            break;
        case WS_HDR_PARSE_HEADERS:
            ptr = rbuf_find_bytes(client->buf_read, WS_HTTP_NEWLINE, strlen(WS_HTTP_NEWLINE), &idx_crlf);
            if (!ptr) {
                bytes = rbuf_bytes_available(client->buf_read);
                HTTP_HDR_LINE_CHECK_LIMIT(bytes);
                return WS_CLIENT_NEED_MORE_BYTES;
            }
            HTTP_HDR_LINE_CHECK_LIMIT(idx_crlf);

            if (!idx_crlf) { // empty line, header end
                rbuf_bump_tail(client->buf_read, strlen(WS_HTTP_NEWLINE));
                client->hs.hdr_state = WS_HDR_PARSE_DONE;
                return 0;
            }

            ptr = rbuf_find_bytes(client->buf_read, HTTP_HDR_SEPARATOR, strlen(HTTP_HDR_SEPARATOR), &idx_sep);
            if (!ptr || idx_sep > idx_crlf) {
                ERROR("Expected HTTP hdr field key/value separator \": \" before endline in non empty HTTP header line");
                return WS_CLIENT_PROTOCOL_ERROR;
            }
            if (idx_crlf == idx_sep + (int)strlen(HTTP_HDR_SEPARATOR)) {
                ERROR("HTTP Header value cannot be empty");
                return WS_CLIENT_PROTOCOL_ERROR;
            }

            if (idx_sep > HTTP_HEADER_NAME_MAX_LEN) {
                ERROR("HTTP header too long (%d)", idx_sep);
                return WS_CLIENT_PROTOCOL_ERROR;
            }

            struct http_header *hdr = mw_calloc(1, sizeof(struct http_header) + idx_crlf); //idx_crlf includes ": " that will be used as 2 \0 bytes
            hdr->key = ((char*)hdr) + sizeof(struct http_header);
            hdr->value = hdr->key + idx_sep + 1;

            bytes = rbuf_pop(client->buf_read, hdr->key, idx_sep);
            rbuf_bump_tail(client->buf_read, strlen(HTTP_HDR_SEPARATOR));

            bytes = rbuf_pop(client->buf_read, hdr->value, idx_crlf - idx_sep - strlen(HTTP_HDR_SEPARATOR));
            rbuf_bump_tail(client->buf_read, strlen(WS_HTTP_NEWLINE));

            for (int i = 0; hdr->key[i]; i++)
                hdr->key[i] = tolower(hdr->key[i]);

//            DEBUG("HTTP header \"%s\" received. Value \"%s\"", hdr->key, hdr->value);

            if (ws_client_add_http_header(client, hdr))
                return WS_CLIENT_PROTOCOL_ERROR;

            if (!strcmp(hdr->key, WS_CONN_ACCEPT)) {
                if (strcmp(client->hs.nonce_reply, hdr->value)) {
                    ERROR("Received NONCE \"%s\" does not match expected nonce of \"%s\"", hdr->value, client->hs.nonce_reply);
                    return WS_CLIENT_PROTOCOL_ERROR;
                }
                client->hs.nonce_matched = 1;
            }

            break;
        case WS_HDR_PARSE_DONE:
            if (!client->hs.nonce_matched) {
                ERROR("Missing " WS_CONN_ACCEPT " header");
                return WS_CLIENT_PROTOCOL_ERROR;
            }
            if (client->hs.http_code != 101) {
                ERROR("HTTP return code not 101. Received %d with msg \"%s\".", client->hs.http_code, client->hs.http_reply_msg);
                return WS_CLIENT_PROTOCOL_ERROR;
            }

            client->state = WS_ESTABLISHED;
            client->hs.hdr_state = WS_HDR_ALL_DONE;
            INFO("Websocket Connection Accepted By Server");
            return WS_CLIENT_PARSING_DONE;
        case WS_HDR_ALL_DONE:
            FATAL("This is error we should never come here!");
            return WS_CLIENT_PROTOCOL_ERROR;
    }
    return 0;
}

#define BYTE_MSB          0x80
#define WS_FINAL_FRAG     BYTE_MSB
#define WS_PAYLOAD_MASKED BYTE_MSB

static inline size_t get_ws_hdr_size(size_t payload_size)
{
    size_t hdr_len = 2 + 4 /*mask*/;
    if(payload_size > 125)
        hdr_len += 2;
    if(payload_size > 65535)
        hdr_len += 6;
    return hdr_len;
}

#define MAX_POSSIBLE_HDR_LEN 14
int ws_client_send(ws_client *client, enum websocket_opcode frame_type, const char *data, size_t size)
{
    // TODO maybe? implement fragmenting, it is not necessary though
    // as both tested MQTT brokers have no reuirement of one MQTT envelope
    // be equal to one WebSockets envelope. Therefore there is no need to send
    // one big MQTT message as single fragmented WebSocket envelope
    char hdr[MAX_POSSIBLE_HDR_LEN];
    char *ptr = hdr;
    char *mask;
    int size_written = 0;
    size_t j = 0;

    size_t w_buff_free = rbuf_bytes_free(client->buf_write);
    size_t hdr_len = get_ws_hdr_size(size);

    if (w_buff_free < hdr_len * 2) {
#ifdef DEBUG_ULTRA_VERBOSE
        DEBUG("Write buffer full. Can't write requested %d size.", size);
#endif
        return 0;
    }

    if (w_buff_free < (hdr_len + size)) {
#ifdef DEBUG_ULTRA_VERBOSE
        DEBUG("Can't write whole MQTT packet of %d bytes into the buffer. Will do partial send of %d.", size, w_buff_free - hdr_len);
#endif
        size = w_buff_free - hdr_len;
        hdr_len = get_ws_hdr_size(size);
        // the actual needed header size might decrease if we cut number of bytes
        // if decrease of size crosses 65535 or 125 boundary
        // but I can live with that at least for now
        // worst case is we have 6 more bytes we could have written
        // no bigus dealus
    }

    *ptr++ = frame_type | WS_FINAL_FRAG;

    //generate length
    *ptr = WS_PAYLOAD_MASKED;
    if (size > 65535) {
        *ptr++ |= 0x7f;
        uint64_t be = htobe64(size);
        memcpy(ptr, (void *)&be, sizeof(be));
        ptr += sizeof(be);
    } else if (size > 125) {
        *ptr++ |= 0x7e;
        uint16_t be = htobe16(size);
        memcpy(ptr, (void *)&be, sizeof(be));
        ptr += sizeof(be);
    } else
        *ptr++ |= size;
    
    mask = ptr;
    if (read(client->entropy_fd, mask, sizeof(uint32_t)) < (ssize_t)sizeof(uint32_t)) {
        ERROR("Unable to get mask from \"" ENTROPY_SOURCE "\"");
        return -2;
    }

    rbuf_push(client->buf_write, hdr, hdr_len);

    if (!size)
        return 0;

    // copy and mask data in the write ringbuffer
    while (size - size_written) {
        size_t writable_bytes;
        char *w_ptr = rbuf_get_linear_insert_range(client->buf_write, &writable_bytes);
        if(!writable_bytes)
            break;

        writable_bytes = (writable_bytes > size) ? (size - size_written) : writable_bytes;

        memcpy(w_ptr, &data[size_written], writable_bytes);
        rbuf_bump_head(client->buf_write, writable_bytes);

        for (size_t i = 0; i < writable_bytes; i++, j++)
            w_ptr[i] ^= mask[j % 4];
        size_written += writable_bytes;
    }
    return size_written;
}

static int check_opcode(ws_client *client,enum websocket_opcode oc)
{
    switch(oc) {
        case WS_OP_BINARY_FRAME:
        case WS_OP_CONNECTION_CLOSE:
        case WS_OP_PING:
            return 0;
        case WS_OP_CONTINUATION_FRAME:
            FATAL("WS_OP_CONTINUATION_FRAME NOT IMPLEMENTED YET!!!!");
            return 0;
        case WS_OP_TEXT_FRAME:
            FATAL("WS_OP_TEXT_FRAME NOT IMPLEMENTED YET!!!!");
            return 0;
        case WS_OP_PONG:
            FATAL("WS_OP_PONG NOT IMPLEMENTED YET!!!!");
            return 0;
        default:
            return WS_CLIENT_PROTOCOL_ERROR;
    }
}

static inline void ws_client_rx_post_hdr_state(ws_client *client)
{
    switch(client->rx.opcode) {
        case WS_OP_BINARY_FRAME:
            client->rx.parse_state = WS_PAYLOAD_DATA;
            return;
        case WS_OP_CONNECTION_CLOSE:
            client->rx.parse_state = WS_PAYLOAD_CONNECTION_CLOSE;
            return;
        case WS_OP_PING:
            client->rx.parse_state = WS_PAYLOAD_PING_REQ_PAYLOAD;
            return;
        default:
            client->rx.parse_state = WS_PAYLOAD_SKIP_UNKNOWN_PAYLOAD;
            return;
    }
}

#define LONGEST_POSSIBLE_HDR_PART 8
int ws_client_process_rx_ws(ws_client *client)
{
    char buf[LONGEST_POSSIBLE_HDR_PART];
    size_t size;
    switch (client->rx.parse_state) {
        case WS_FIRST_2BYTES:
            BUF_READ_CHECK_AT_LEAST(2);
            rbuf_pop(client->buf_read, buf, 2);
            client->rx.opcode = buf[0] & (char)~BYTE_MSB;

            if (!(buf[0] & (char)~WS_FINAL_FRAG)) {
                ERROR("Not supporting fragmented messages yet!");
                return WS_CLIENT_PROTOCOL_ERROR;
            }

            if (check_opcode(client, client->rx.opcode) == WS_CLIENT_PROTOCOL_ERROR)
                return WS_CLIENT_PROTOCOL_ERROR;

            if (buf[1] & (char)WS_PAYLOAD_MASKED) {
                ERROR("Mask is not allowed in Server->Client Websocket direction.");
                return WS_CLIENT_PROTOCOL_ERROR;
            }

            switch (buf[1]) {
                case 127:
                    client->rx.parse_state = WS_PAYLOAD_EXTENDED_64;
                    break;
                case 126:
                    client->rx.parse_state = WS_PAYLOAD_EXTENDED_16;
                    break;
                default:
                    client->rx.payload_length = buf[1];
                    ws_client_rx_post_hdr_state(client);
            }
            break;
        case WS_PAYLOAD_EXTENDED_16:
            BUF_READ_CHECK_AT_LEAST(2);
            rbuf_pop(client->buf_read, buf, 2);
            client->rx.payload_length = be16toh(*((uint16_t *)buf));
            ws_client_rx_post_hdr_state(client);
            break;
        case WS_PAYLOAD_EXTENDED_64:
            BUF_READ_CHECK_AT_LEAST(LONGEST_POSSIBLE_HDR_PART);
            rbuf_pop(client->buf_read, buf, LONGEST_POSSIBLE_HDR_PART);
            client->rx.payload_length = be64toh(*((uint64_t *)buf));
            ws_client_rx_post_hdr_state(client);
            break;
        case WS_PAYLOAD_DATA:
            // TODO not pretty?
            while (client->rx.payload_processed < client->rx.payload_length) {
                size_t remaining = client->rx.payload_length - client->rx.payload_processed;
                if (!rbuf_bytes_available(client->buf_read))
                    return WS_CLIENT_NEED_MORE_BYTES;
                char *insert = rbuf_get_linear_insert_range(client->buf_to_mqtt, &size);
                if (!insert) {
#ifdef DEBUG_ULTRA_VERBOSE
                    DEBUG("BUFFER TOO FULL. Avail %d req %d", (int)size, (int)remaining);
#endif
                    return WS_CLIENT_BUFFER_FULL;
                }
                size = (size > remaining) ? remaining : size;
                size = rbuf_pop(client->buf_read, insert, size);
                rbuf_bump_head(client->buf_to_mqtt, size);
                client->rx.payload_processed += size;
            }
            client->rx.parse_state = WS_PACKET_DONE;
            break;
        case WS_PAYLOAD_CONNECTION_CLOSE:
            // for WS_OP_CONNECTION_CLOSE allowed is
            // a) empty payload
            // b) 2byte reason code
            // c) 2byte reason code followed by message
            if (client->rx.payload_length == 1) {
                ERROR("WebScoket CONNECTION_CLOSE can't have payload of size 1");
                return WS_CLIENT_PROTOCOL_ERROR;
            }
            if (!client->rx.payload_length) {
                INFO("WebSocket server closed the connection without giving reason.");
                client->rx.parse_state = WS_PACKET_DONE;
                break;
            }
            client->rx.parse_state = WS_PAYLOAD_CONNECTION_CLOSE_EC;
            break;
        case WS_PAYLOAD_CONNECTION_CLOSE_EC:
            BUF_READ_CHECK_AT_LEAST(sizeof(uint16_t));

            rbuf_pop(client->buf_read, buf, sizeof(uint16_t));
            client->rx.specific_data.op_close.ec = be16toh(*((uint16_t *)buf));
            client->rx.payload_processed += sizeof(uint16_t);

            if(client->rx.payload_processed == client->rx.payload_length) {
                INFO("WebSocket server closed the connection with EC=%d. Without message.",
                    client->rx.specific_data.op_close.ec);
                client->rx.parse_state = WS_PACKET_DONE;
                break;
            }
            client->rx.parse_state = WS_PAYLOAD_CONNECTION_CLOSE_MSG;
            break;
        case WS_PAYLOAD_CONNECTION_CLOSE_MSG:
            if (!client->rx.specific_data.op_close.reason)
                client->rx.specific_data.op_close.reason = mw_malloc(client->rx.payload_length + 1);

            while (client->rx.payload_processed < client->rx.payload_length) {
                if (!rbuf_bytes_available(client->buf_read))
                    return WS_CLIENT_NEED_MORE_BYTES;
                client->rx.payload_processed += rbuf_pop(client->buf_read,
                                                         &client->rx.specific_data.op_close.reason[client->rx.payload_processed - sizeof(uint16_t)],
                                                         client->rx.payload_length - client->rx.payload_processed);
            }
            client->rx.specific_data.op_close.reason[client->rx.payload_length] = 0;
            INFO("WebSocket server closed the connection with EC=%d and reason \"%s\"",
                client->rx.specific_data.op_close.ec,
                client->rx.specific_data.op_close.reason);
            mw_free(client->rx.specific_data.op_close.reason);
            client->rx.specific_data.op_close.reason = NULL;
            client->rx.parse_state = WS_PACKET_DONE;
            break;
        case WS_PAYLOAD_SKIP_UNKNOWN_PAYLOAD:
            BUF_READ_CHECK_AT_LEAST(client->rx.payload_length);
            WARN("Skipping Websocket Packet of unsupported/unknown type");
            if (client->rx.payload_length)
                rbuf_bump_tail(client->buf_read, client->rx.payload_length);
            client->rx.parse_state = WS_PACKET_DONE;
            return WS_CLIENT_PARSING_DONE;
        case WS_PAYLOAD_PING_REQ_PAYLOAD:
            if (client->rx.payload_length > rbuf_get_capacity(client->buf_read) / 2) {
                ERROR("Ping arrived with payload which is too big!");
                return WS_CLIENT_INTERNAL_ERROR;
            }
            BUF_READ_CHECK_AT_LEAST(client->rx.payload_length);
            client->rx.specific_data.ping_msg = mw_malloc(client->rx.payload_length);
            rbuf_pop(client->buf_read, client->rx.specific_data.ping_msg, client->rx.payload_length);
            // TODO schedule this instead of sending right away
            // then attempt to send as soon as buffer space clears up
            size = ws_client_send(client, WS_OP_PONG, client->rx.specific_data.ping_msg, client->rx.payload_length);
            if (size != client->rx.payload_length) {
                ERROR("Unable to send the PONG as one packet back. Closing connection.");
                return WS_CLIENT_PROTOCOL_ERROR;
            }
            client->rx.parse_state = WS_PACKET_DONE;
            return WS_CLIENT_PARSING_DONE;
        case WS_PACKET_DONE:
            client->rx.parse_state = WS_FIRST_2BYTES;
            client->rx.payload_processed = 0;
            if (client->rx.opcode == WS_OP_CONNECTION_CLOSE)
                return WS_CLIENT_CONNECTION_CLOSED;
            return WS_CLIENT_PARSING_DONE;
        default:
            FATAL("Unknown parse state");
            return WS_CLIENT_INTERNAL_ERROR;
    }
    return 0;
}

int ws_client_process(ws_client *client)
{
    int ret;
    switch(client->state) {
        case WS_RAW:
            if (ws_client_start_handshake(client))
                return WS_CLIENT_INTERNAL_ERROR;
            return WS_CLIENT_NEED_MORE_BYTES;
        case WS_HANDSHAKE:
            do {
                ret = ws_client_parse_handshake_resp(client);
                if (ret == WS_CLIENT_PROTOCOL_ERROR)
                    client->state = WS_ERROR;
                if (ret == WS_CLIENT_PARSING_DONE && client->state == WS_ESTABLISHED)
                    ret = WS_CLIENT_NEED_MORE_BYTES;
            } while (!ret);
            break;
        case WS_ESTABLISHED:
            do {
                ret = ws_client_process_rx_ws(client);
                switch(ret) {
                    case WS_CLIENT_PROTOCOL_ERROR:
                        client->state = WS_ERROR;
                        break;
                    case WS_CLIENT_CONNECTION_CLOSED:
                        client->state = WS_CONN_CLOSED_GRACEFUL;
                        break;
                }
                // if ret == 0 we can continue parsing
                // if ret == WS_CLIENT_PARSING_DONE we processed
                //    one websocket packet and attempt processing
                //    next one if data available in the buffer
            } while (!ret || ret == WS_CLIENT_PARSING_DONE);
            break;
        case WS_ERROR:
            ERROR("ws_client is in error state. Restart the connection!");
            return WS_CLIENT_PROTOCOL_ERROR;
        case WS_CONN_CLOSED_GRACEFUL:
            ERROR("Connection has been gracefully closed. Calling this is useless (and probably bug) until you reconnect again.");
            return WS_CLIENT_CONNECTION_CLOSED;
        default:
            FATAL("Unknown connection state! Probably memory corruption.");
            return WS_CLIENT_INTERNAL_ERROR;
    }
    return ret;
}
