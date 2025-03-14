// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNALS_H
#define NETDATA_SIGNALS_H

#include "../common.h"

void signals_block_all_except_deadly(void);
void signals_block_all(void);

void signals_unblock_one(int signo);
void signals_unblock(int signals[], size_t count);
void signals_unblock_deadly(void);

#include "signal-code.h"

#endif //NETDATA_SIGNALS_H
