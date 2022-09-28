// SPDX-License-Identifier: GPL-3.0-or-later

#include "handshake.h"
#include "message.h"

#include "proto/command.pb.h"

enum class handshake_version : uint8_t {
    unknown = 0,
    v1 = 1,
};

typedef struct {
    enum handshake_version version;
} handshake_info_t;

static std::string derseBuffer;

bool send_handshake_info(connection_handle_t *conn, handshake_info_t *info) {
    protocol::HandshakeInfo pb_info;
    switch (info->version) {
        case handshake_version::v1:
            pb_info.set_version(protocol::VersionValue::V1);
            break;
        default:
            pb_info.set_version(protocol::VersionValue::Unknown);
            break;
    }

    derseBuffer.clear();
    pb_info.SerializeToString(&derseBuffer);

    binary_message_t msg;
    msg.buf = const_cast<char *>(derseBuffer.data());
    msg.len = derseBuffer.size();
    return binary_message_send(conn, &msg);
}

bool recv_handshake_info(connection_handle_t *conn, handshake_info_t *info) {
    binary_message_t msg;
    bool ok = binary_message_recv(conn, &msg);
    if (!ok)
        return false;

    protocol::HandshakeInfo pb_info;
    pb_info.ParseFromArray(msg.buf, msg.len);

    switch (pb_info.version()) {
        case protocol::VersionValue::V1:
            info->version = handshake_version::v1;
            break;
        default:
            info->version = handshake_version::unknown;
            break;
    }
    freez(msg.buf);
    return true;
}

bool sender_handshake_start(struct sender_state *ss) {
    bool ok = true;

    connection_handle_t conn;
    conn.host = ss->host;
    conn.ssl = &ss->host->ssl;
    conn.sockfd = ss->host->rrdpush_sender_socket;
    conn.flags = 0;
    conn.timeout = 60;

    handshake_info_t local_info;
    local_info.version = handshake_version::v1;

    ok = send_handshake_info(&conn, &local_info);
    if (!ok)
        return false;

    handshake_info_t remote_info;
    ok = recv_handshake_info(&conn, &remote_info);
    if (!ok)
        return false;

    return ok;
}

bool receiver_handshake_start(struct receiver_state *rs) {
    bool ok = true;

    connection_handle_t conn;
    conn.host = rs->host;
    conn.ssl = &rs->ssl;
    conn.sockfd = rs->fd;
    conn.flags = 0;
    conn.timeout = 60;

    handshake_info_t remote_info;
    ok = recv_handshake_info(&conn, &remote_info);
    if (!ok)
        return false;

    handshake_info_t local_info;
    local_info.version = handshake_version::v1;

    ok = send_handshake_info(&conn, &local_info);
    if (!ok)
        return false;

    return true;
}
