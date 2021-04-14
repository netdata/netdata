

// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_RX_MSGS_H
#define ACLK_RX_MSGS_H

#include "../daemon/common.h"
#include "libnetdata/libnetdata.h"

int aclk_handle_cloud_message(char *payload);
void aclk_set_rx_handlers(int version);

#endif /* ACLK_RX_MSGS_H */
