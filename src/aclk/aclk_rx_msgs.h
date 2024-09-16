// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_RX_MSGS_H
#define ACLK_RX_MSGS_H

#include "libnetdata/libnetdata.h"

int aclk_handle_cloud_cmd_message(char *payload);
void aclk_init_rx_msg_handlers(void);
void aclk_handle_new_cloud_msg(const char *message_type, const char *msg, size_t msg_len, const char *topic);

#endif /* ACLK_RX_MSGS_H */
