// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef PROTOCOL_HANDSHAKE_H
#define PROTOCOL_HANDSHAKE_H

#include "daemon/common.h"

#ifdef __cplusplus
extern "C" {
#endif

#ifdef ENABLE_HANDSHAKE
bool sender_handshake_start(struct sender_state *ss);
bool receiver_handshake_start(struct receiver_state *rs);
#else
static inline bool sender_handshake_start(struct sender_state *ss) {
    UNUSED(ss);
    return false;
}

static inline bool receiver_handshake_start(struct receiver_state *rs) {
    UNUSED(rs);
    return true;
}
#endif

#ifdef __cplusplus
};
#endif

#endif /* PROTOCOL_HANDSHAKE_H */
