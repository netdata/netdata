// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef WS_CLIENT_H
#define WS_CLIENT_H

#define WS_CLIENT_NEED_MORE_BYTES            0x10
#define WS_CLIENT_PARSING_DONE               0x11
#define WS_CLIENT_CONNECTION_CLOSED          0x12
#define WS_CLIENT_CONNECTION_REMOTE_CLOSED   0x13
#define WS_CLIENT_PROTOCOL_ERROR            -0x10
#define WS_CLIENT_BUFFER_FULL               -0x11
#define WS_CLIENT_INTERNAL_ERROR            -0x12

enum websocket_client_conn_state {
    WS_RAW = 0,
    WS_HANDSHAKE,
    WS_ESTABLISHED,
    WS_ERROR,        // connection has to be restarted if this is reached
    WS_CONN_CLOSED_GRACEFUL,
    WS_CONN_CLOSED_GRACEFUL_BY_REMOTE,
};

enum websocket_client_hdr_parse_state {
    WS_HDR_HTTP = 0,        // need to check HTTP/1.1
    WS_HDR_RC,              // need to read HTTP code
    WS_HDR_ENDLINE,         // need to read rest of the first line
    WS_HDR_PARSE_HEADERS,   // rest of the header until CRLF CRLF
    WS_HDR_PARSE_DONE,
    WS_HDR_ALL_DONE
};

enum websocket_client_rx_ws_parse_state {
    WS_FIRST_2BYTES = 0,
    WS_PAYLOAD_EXTENDED_16,
    WS_PAYLOAD_EXTENDED_64,
    WS_PAYLOAD_DATA, // BINARY payload to be passed to MQTT
    WS_PAYLOAD_CONNECTION_CLOSE,
    WS_PAYLOAD_CONNECTION_CLOSE_EC,
    WS_PAYLOAD_CONNECTION_CLOSE_MSG,
    WS_PAYLOAD_SKIP_UNKNOWN_PAYLOAD,
    WS_PAYLOAD_PING_REQ_PAYLOAD, // PING payload to be sent back as PONG
    WS_PACKET_DONE
};

enum websocket_opcode {
    WS_OP_CONTINUATION_FRAME = 0x0,
    WS_OP_TEXT_FRAME         = 0x1,
    WS_OP_BINARY_FRAME       = 0x2,
    WS_OP_CONNECTION_CLOSE   = 0x8,
    WS_OP_PING               = 0x9,
    WS_OP_PONG               = 0xA
};

struct ws_op_close_payload {
    uint16_t ec;
    char *reason;
};

struct http_header {
    char *key;
    char *value;
    struct http_header *next;
};

typedef struct websocket_client {
    enum websocket_client_conn_state state;

    struct ws_handshake {
        enum websocket_client_hdr_parse_state hdr_state;
        char *nonce_reply;
        int nonce_matched;
        int http_code;
        char *http_reply_msg;
        struct http_header *headers;
        struct http_header *headers_tail;
        int hdr_count;
    } hs;

    struct ws_rx {
        enum websocket_client_rx_ws_parse_state parse_state;
        enum websocket_opcode opcode;
        bool remote_closed;
        uint64_t payload_length;
        uint64_t payload_processed;
        union {
            struct ws_op_close_payload op_close;
            char *ping_msg;
        } specific_data;
    } rx;

    rbuf_t buf_read;    // from SSL
    rbuf_t buf_write;   // to SSL and then to socket
    // TODO if ringbuffer gets multiple tail support
    // we can work without buf_to_mqtt and thus reduce
    // memory usage and remove one more memcpy buf_read->buf_to_mqtt
    rbuf_t buf_to_mqtt; // RAW data for MQTT lib

    // careful host is borrowed, don't free
    char **host;
} ws_client;

ws_client *ws_client_new(size_t buf_size, char **host);
void ws_client_destroy(ws_client *client);
void ws_client_reset(ws_client *client);

int ws_client_start_handshake(ws_client *client);

int ws_client_want_write(const ws_client *client);

int ws_client_process(ws_client *client);

int ws_client_send(const ws_client *client, enum websocket_opcode frame_type, const char *data, size_t size);

#endif /* WS_CLIENT_H */
