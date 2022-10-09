// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNALS_H
#define NETDATA_SIGNALS_H 1

void signals_init(void);
void signals_block(void);
void signals_unblock(void);
void signals_restore_SIGCHLD(void);
void signals_reset(void);
void signals_handle(void) NORETURN;

#endif //NETDATA_SIGNALS_H
