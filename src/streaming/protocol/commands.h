// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAMING_PROTCOL_COMMANDS_H
#define NETDATA_STREAMING_PROTCOL_COMMANDS_H

#include "../rrdpush.h"

void rrdpush_sender_get_node_and_claim_id_from_parent(struct sender_state *s);
void rrdpush_receiver_send_node_and_claim_id_to_child(RRDHOST *host);
void rrdpush_sender_clear_parent_claim_id(RRDHOST *host);

void rrdpush_sender_send_claimed_id(RRDHOST *host);

#endif //NETDATA_STREAMING_PROTCOL_COMMANDS_H
