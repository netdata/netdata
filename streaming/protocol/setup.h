// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROTOCOL_SETUP_H
#define NETDATA_PROTOCOL_SETUP_H

#include "streaming/rrdpush.h"

bool protocol_setup_on_receiver(struct receiver_state *rpt);
bool protocol_setup_on_sender(struct sender_state *s, int timeout);

#endif /* NETDATA_PROTOCOL_SETUP_H */
