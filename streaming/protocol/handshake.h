#ifndef PROTOCOL_HANDSHAKE_H
#define PROTOCOL_HANDSHAKE_H

#include "daemon/common.h"

#ifdef __cplusplus
extern "C" {
#endif

bool sender_handshake_start(struct sender_state *ss);
bool receiver_handshake_start(struct receiver_state *rs);

#ifdef __cplusplus
};
#endif

#endif /* PROTOCOL_HANDSHAKE_H */
