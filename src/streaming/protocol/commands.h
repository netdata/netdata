// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAMING_PROTCOL_COMMANDS_H
#define NETDATA_STREAMING_PROTCOL_COMMANDS_H

#include "../rrdpush.h"

void streaming_sender_command_node_id_parser(struct sender_state *s);
void rrdpush_send_child_node_id(RRDHOST *host);

#endif //NETDATA_STREAMING_PROTCOL_COMMANDS_H
