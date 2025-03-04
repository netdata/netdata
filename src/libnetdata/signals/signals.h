// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNAL_HANDLER_H
#define NETDATA_SIGNAL_HANDLER_H

#include "../common.h"

void signals_block_all_except_deadly(void);
void signals_block_all(void);
void signals_unblock_one(int signo);

#endif //NETDATA_SIGNAL_HANDLER_H
