#include "handshake.h"
#include "message.h"

#include "proto/command.pb.h"
#include <google/protobuf/io/zero_copy_stream_impl.h>

enum class handshake_version : uint8_t {
    unknown = 0,
    v1 = 1,
};

typedef struct {
    enum handshake_version version;
    bool enable_replication;
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
    pb_info.set_enablereplication(info->enable_replication);

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
    info->enable_replication = pb_info.enablereplication();
    freez(msg.buf);
    return true;
}

bool recv_replication_gaps(connection_handle_t *conn) {
    binary_message_t msg;
    bool ok = binary_message_recv(conn, &msg);
    if (!ok)
        return false;

    if (msg.len) {
        replication_set_sender_gaps(conn->host, msg.buf, msg.len);
        freez(msg.buf);
    }

    return true;
}

bool send_replication_gaps(connection_handle_t *conn) {
    binary_message_t msg;

    replication_get_receiver_gaps(conn->host, &msg.buf, &msg.len);
    bool ok = binary_message_send(conn, &msg);
    freez(msg.buf);
    return ok;
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
    local_info.enable_replication = true;

    ok = send_handshake_info(&conn, &local_info);
    if (!ok)
        return false;

    handshake_info_t remote_info;
    ok = recv_handshake_info(&conn, &remote_info);
    if (!ok)
        return false;

    bool enable_replication = local_info.enable_replication &&
                              remote_info.enable_replication;
    if (enable_replication)
        return recv_replication_gaps(&conn);

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
    local_info.enable_replication = true;

    ok = send_handshake_info(&conn, &local_info);
    if (!ok)
        return false;

    bool enable_replication = local_info.enable_replication &&
                              remote_info.enable_replication;
    if (enable_replication)
        return send_replication_gaps(&conn);

    return true;
}
